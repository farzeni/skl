package vault

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/farzeni/skl/internal/gitx"
)

// What skl sync does with the vault's remote (docs/guide/sync.md "What sync does"):
// fetch, merge (never rebase), push. Only sync calls these; every other command
// commits and never pushes.

// ErrDetached is a vault with no branch checked out: sync wouldn't know which
// branch to merge into or push.
var ErrDetached = errors.New("has no branch checked out")

// Branch is the branch checked out.
func (v *Vault) Branch(ctx context.Context) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return "", ErrDetached
	}
	return strings.TrimSpace(out), err
}

// Fetch updates origin's branches. It changes nothing checked out.
func (v *Vault) Fetch(ctx context.Context) error {
	_, err := v.git.Run(ctx, v.dir, "fetch", "--quiet", "--no-tags", "origin")
	return err
}

// Incoming is origin's copy of branch, as last fetched, and the commits on it
// that HEAD doesn't have, newest first. ref is "" when origin has no such branch
// yet, such as a new vault never pushed.
func (v *Vault) Incoming(ctx context.Context, branch string) (ref string, commits []string, err error) {
	ref = "refs/remotes/origin/" + branch
	if _, err := v.git.Run(ctx, v.dir, "rev-parse", "--quiet", "--verify", ref+"^{commit}"); err != nil {
		if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
			return "", nil, nil
		}
		return "", nil, err
	}
	out, err := v.git.Run(ctx, v.dir, "rev-list", ref, "--not", "HEAD")
	if err != nil {
		return "", nil, err
	}
	return ref, strings.Fields(out), nil
}

// Merge merges ref into the branch checked out: a fast-forward when HEAD has
// nothing of its own, else a merge commit. It never rebases. On a conflict it
// undoes the merge and returns a *ConflictError with the files that clashed. On
// any other error, Ctrl-C included, it undoes what it did.
func (v *Vault) Merge(ctx context.Context, ref string) (err error) {
	before, err := v.Head(ctx)
	if err != nil {
		return err
	}
	_, err = v.git.Run(ctx, v.dir, "merge", "--quiet", "--ff", "--no-edit", "--no-stat", ref)
	if err == nil {
		return nil
	}
	undo := context.WithoutCancel(ctx)
	var paths []string
	if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
		paths, _ = v.conflicted(undo)
	}
	v.Unmerge(undo, before)
	if len(paths) > 0 {
		return &ConflictError{Paths: paths}
	}
	return err
}

// conflicted lists the files a merge left with conflicts.
func (v *Vault) conflicted(ctx context.Context) ([]string, error) {
	out, err := v.git.Run(ctx, v.dir, "diff", "--name-only", "-z", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// Unmerge takes back a merge: one in progress is aborted, one that landed is
// undone, so HEAD is before again with the work tree to match. It does its best:
// the error that brought the caller here is the one to report.
func (v *Vault) Unmerge(ctx context.Context, before string) {
	if merging, err := v.Merging(ctx); err == nil && merging {
		_, _ = v.git.Run(ctx, v.dir, "merge", "--abort")
	}
	if head, err := v.Head(ctx); err == nil && head != before {
		// --keep moves HEAD and updates only the files the two commits differ in;
		// after a merge the work tree is clean, so nothing of the user's is lost.
		_, _ = v.git.Run(ctx, v.dir, "reset", "--quiet", "--keep", before)
	}
}

// Push pushes the branch checked out to the same branch on origin.
func (v *Vault) Push(ctx context.Context, branch string) error {
	_, err := v.git.Run(ctx, v.dir, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch)
	return err
}

// MergedTree is the tree a merge of ref into the commit from would check out, and
// the files that would clash. It writes git objects but changes no ref, the index or the
// work tree: it is how sync --dry-run sees what it would reload.
func (v *Vault) MergedTree(ctx context.Context, from, ref string) (tree string, conflicts []string, err error) {
	out, err := v.git.Run(ctx, v.dir, "merge-tree", "--write-tree", "--name-only", "-z", "--no-messages", from, ref)
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		// 1 is merge-tree's "conflicts": the tree, then the files that clash.
		fields := strings.Split(strings.TrimSuffix(ge.Stdout, "\x00"), "\x00")
		for _, f := range fields[1:] {
			if f != "" {
				conflicts = append(conflicts, f)
			}
		}
		return fields[0], conflicts, nil
	}
	if err != nil {
		return "", nil, err
	}
	tree, _, _ = strings.Cut(out, "\x00")
	return strings.TrimSpace(tree), nil, nil
}

// Snapshot writes tree's files into dir, which must not exist, and returns a
// Vault that reads them: an Index of what a merge would leave, for
// sync --dry-run. It has no git of its own: only its files, its Dir and its Head,
// which is head, can be read.
func (v *Vault) Snapshot(ctx context.Context, tree, head, dir string) (*Vault, error) {
	out, err := v.git.Run(ctx, v.dir, "archive", "--format=tar", tree)
	if err != nil {
		return nil, err
	}
	if err := untar(strings.NewReader(out), dir); err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	return &Vault{git: v.git, dir: dir, head: head}, nil
}

// untar writes a tar from git archive into dir: folders, files with their
// executable bit, and links, each kept inside dir.
func untar(r io.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }() // read-only use of the folder handle
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, 0o755)
		case tar.TypeSymlink:
			err = root.Symlink(h.Linkname, name)
		case tar.TypeReg:
			err = writeRootFile(root, name, tr, os.FileMode(h.Mode)&0o755|0o644)
		}
		if err != nil {
			return err
		}
	}
}

func writeRootFile(root *os.Root, name string, r io.Reader, mode os.FileMode) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return errors.Join(err, f.Close())
}

// Resolve is the commit ref names.
func (v *Vault) Resolve(ctx context.Context, ref string) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "rev-parse", "--verify", ref+"^{commit}")
	return strings.TrimSpace(out), err
}

// Pending is the commit the hand edits in the work tree would make on HEAD, as
// skl commits them (git add -A, so the vault's .gitignore applies), or HEAD if
// there are none. It builds the tree in an index of its own and writes only git
// objects: what's staged, the work tree and every ref stay as they are. It is how
// sync --dry-run sees what step 1 would commit.
func (v *Vault) Pending(ctx context.Context) (string, error) {
	head, err := v.Head(ctx)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "skl-index-")
	if err != nil {
		return "", fmt.Errorf("vault: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index := filepath.Join(tmp, "index")
	for _, args := range [][]string{{"read-tree", head}, {"add", "-A"}} {
		if _, err := v.git.RunWithIndex(ctx, v.dir, index, args...); err != nil {
			return "", err
		}
	}
	tree, err := v.git.RunWithIndex(ctx, v.dir, index, "write-tree")
	if err != nil {
		return "", err
	}
	return v.commitOnto(ctx, head, strings.TrimSpace(tree), "skl: commit hand edits")
}
