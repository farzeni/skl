package vault

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Units lists what changed between two commits, one entry per skill folder or
// file: group/skill for anything inside a skill's folder, the path itself for a
// file at the top or in a group (sets/backend.toml, LICENSE). A rollback reverts
// each of them on its own. They're sorted.
func (v *Vault) Units(ctx context.Context, from, to string) ([]string, error) {
	out, err := v.git.Run(ctx, v.dir, "diff", "--name-only", "-z", "--no-renames", from, to, "--")
	if err != nil {
		return nil, err
	}
	var units []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p == "" {
			continue
		}
		if u := unitOf(p); !slices.Contains(units, u) {
			units = append(units, u)
		}
	}
	slices.Sort(units)
	return units, nil
}

// unitOf is the skill folder a path is in, or the path for a file at the top or
// in a group.
func unitOf(p string) string {
	parts := strings.SplitN(p, "/", 3)
	if len(parts) < 3 {
		return p
	}
	return parts[0] + "/" + parts[1]
}

// Revert puts unit back as it was at commit, in a commit of its own on top of
// HEAD, and returns that commit. It reports "" if HEAD already holds unit as it
// was. History is never rewritten: the commit adds to it. The work tree must be
// clean; on an error it's as it was.
func (v *Vault) Revert(ctx context.Context, commit, unit, message string) (string, error) {
	was, err := v.object(ctx, commit, unit)
	if err != nil {
		return "", err
	}
	head, err := v.Head(ctx)
	if err != nil {
		return "", err
	}
	now, err := v.object(ctx, head, unit)
	if err != nil || now == was {
		return "", err
	}
	if _, err := v.git.Run(ctx, v.dir, "rm", "-r", "-q", "--ignore-unmatch", "--", unit); err != nil {
		v.ResetTo(context.WithoutCancel(ctx), head)
		return "", err
	}
	if was != "" {
		if _, err := v.git.Run(ctx, v.dir, "checkout", commit, "--", unit); err != nil {
			v.ResetTo(context.WithoutCancel(ctx), head)
			return "", err
		}
	}
	committed, err := v.Commit(ctx, message, unit)
	if err != nil || !committed {
		v.ResetTo(context.WithoutCancel(ctx), head)
		return "", err
	}
	return v.Head(ctx)
}

// Reverts lists the units that HEAD holds differently from commit: the ones Revert
// would make a commit for.
func (v *Vault) Reverts(ctx context.Context, commit string, units []string) ([]string, error) {
	head, err := v.Head(ctx)
	if err != nil {
		return nil, err
	}
	var differ []string
	for _, u := range units {
		was, err := v.object(ctx, commit, u)
		if err != nil {
			return nil, err
		}
		now, err := v.object(ctx, head, u)
		if err != nil {
			return nil, err
		}
		if now != was {
			differ = append(differ, u)
		}
	}
	return differ, nil
}

// object is the object unit names at commit, "" if it isn't there.
func (v *Vault) object(ctx context.Context, commit, unit string) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "ls-tree", "-z", "--full-tree", commit, "--", unit)
	if err != nil {
		return "", err
	}
	for _, e := range treeEntries(out) {
		if e.name == unit {
			return e.mode + " " + e.object, nil
		}
	}
	return "", nil
}

// ResetTo takes the vault back to before, a commit skl made the current one
// earlier in this command: HEAD and the work tree. It is for undoing skl's own
// commits before anything records them, and does its best.
func (v *Vault) ResetTo(ctx context.Context, before string) {
	if head, err := v.Head(ctx); err == nil && head != before {
		_, _ = v.git.Run(ctx, v.dir, "update-ref", "HEAD", before, head)
	}
	_, _ = v.git.Run(ctx, v.dir, "read-tree", "--reset", "-u", before)
}

// Preview writes into dir, which must not exist yet, the vault as reverting each
// of units to commit would leave it: HEAD's files, with those units as they were
// at commit. It changes nothing in the vault, so --dry-run can read it there.
func (v *Vault) Preview(ctx context.Context, commit string, units []string, dir string) error {
	head, err := v.Head(ctx)
	if err != nil {
		return err
	}
	if err := v.extract(ctx, head, dir); err != nil {
		return err
	}
	for _, u := range units {
		if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(u))); err != nil {
			return err
		}
		was, err := v.object(ctx, commit, u)
		if err != nil {
			return err
		}
		if was != "" {
			if err := v.extract(ctx, commit, dir, u); err != nil {
				return err
			}
		}
	}
	return nil
}

// extract writes the files of commit into dir, or only those under paths.
func (v *Vault) extract(ctx context.Context, commit, dir string, paths ...string) error {
	args := append([]string{"archive", "--format=tar", commit, "--"}, paths...)
	out, err := v.git.Run(ctx, v.dir, args...)
	if err != nil {
		return err
	}
	return untar(ctx, strings.NewReader(out), dir)
}

// untar writes a tar from git archive into dir. Names are checked: nothing lands
// outside dir.
func untar(ctx context.Context, r io.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("vault: reading git archive: %w", err)
		}
		name := path.Clean(h.Name)
		if !fs.ValidPath(name) {
			return fmt.Errorf("vault: git archive holds %q", h.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				err = os.Symlink(h.Linkname, target)
			}
		case tar.TypeReg:
			err = writeFrom(tr, target, fs.FileMode(h.Mode).Perm())
		}
		if err != nil {
			return err
		}
	}
}

func writeFrom(r io.Reader, file string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return errors.Join(err, f.Close())
}

// IndexAt reads the vault's layout in dir, a version of the vault written there by
// Preview.
func IndexAt(ctx context.Context, dir string) (*Index, error) {
	return (&Vault{dir: dir}).Index(ctx)
}
