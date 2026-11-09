package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/gitx"
)

// HandEdits lists the files changed in the vault's work tree and not committed:
// edits made in ~/.skl/vault by hand. The vault's .gitignore applies.
func (v *Vault) HandEdits(ctx context.Context) ([]string, error) {
	out, err := v.git.Run(ctx, v.dir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	var files []string
	for entry := range strings.SplitSeq(out, "\x00") {
		if len(entry) > 3 {
			files = append(files, entry[3:]) // "XY <path>"
		}
	}
	return files, nil
}

// ErrMerging is a vault with an unfinished git merge: a sync or a save-back
// conflict that waits for skl sync --continue or --abort.
var ErrMerging = errors.New("a sync is in progress")

// Merging reports whether a git merge is in progress in the vault.
func (v *Vault) Merging(ctx context.Context) (bool, error) {
	_, err := v.git.Run(ctx, v.dir, "rev-parse", "--quiet", "--verify", "MERGE_HEAD")
	if err == nil {
		return true, nil
	}
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return false, nil
	}
	return false, err
}

// Objects are a folder's git objects, which WriteObjects puts in the vault's store.
type Objects struct {
	Files []string // paths of files on disk, written as blobs as they are
	Links []string // link targets, written as blobs
	Trees []string // tree bodies in mktree -z form, each folder after its subfolders
}

// WriteObjects writes a folder's objects into the vault's object store and returns
// their names, in the order given: files, links, then trees.
func (v *Vault) WriteObjects(ctx context.Context, o Objects) ([]string, error) {
	var names []string
	if len(o.Files) > 0 {
		if slices.ContainsFunc(o.Files, func(p string) bool { return strings.ContainsAny(p, "\n\r") }) {
			return nil, errors.New("vault: a file name holds a line break")
		}
		out, err := v.git.RunInput(ctx, v.dir, strings.Join(o.Files, "\n")+"\n",
			"hash-object", "-w", "--no-filters", "--stdin-paths")
		if err != nil {
			return nil, err
		}
		names = append(names, strings.Fields(out)...)
	}
	for _, target := range o.Links {
		out, err := v.git.RunInput(ctx, v.dir, target, "hash-object", "-w", "--stdin")
		if err != nil {
			return nil, err
		}
		names = append(names, strings.TrimSpace(out))
	}
	for _, body := range o.Trees {
		out, err := v.git.RunInput(ctx, v.dir, body, "mktree", "-z")
		if err != nil {
			return nil, err
		}
		names = append(names, strings.TrimSpace(out))
	}
	return names, nil
}

// File is one file of a tree, for comparing two versions of a skill.
type File struct{ Mode, Object string }

// Files lists the files under path at commit, by path inside it. A path that
// isn't there holds none.
func (v *Vault) Files(ctx context.Context, commit, path string) (map[string]File, error) {
	out, err := v.git.Run(ctx, v.dir, "ls-tree", "-r", "-z", "--full-tree", commit, "--", path+"/")
	if err != nil {
		return nil, err
	}
	files := map[string]File{}
	for _, e := range treeEntries(out) {
		files[strings.TrimPrefix(e.name, path+"/")] = File{Mode: e.mode, Object: e.object}
	}
	return files, nil
}

// Save is a loaded copy's folder to merge into the vault.
type Save struct {
	Path    string // the skill's folder in the vault, group/folder
	Base    string // the commit the copy was made from
	Tree    string // the copy's tree, already written by WriteObjects
	Message string // the commit's message
	Copy    string // the copy, as the synthetic commit names it: "<target> <folder>"
}

// ConflictError is a save-back whose merge left conflicts in these files, from
// the vault's root. The merge stays in progress in the vault.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string { return "conflict in " + strings.Join(e.Paths, ", ") }

// SaveSkill merges a copy into the vault and commits the result. It returns the
// vault's HEAD afterward, the same as before if the vault already held the copy's
// changes.
//
// The merge needs no --merge-base, which only git 2.40 has: a synthetic commit
// whose parent is the copy's commit, with the skill's folder replaced by the copy,
// makes that commit the natural merge base of HEAD and the copy. On a conflict
// the same merge is left in progress in the work tree, and the error is a
// *ConflictError. The work tree must be clean.
func (v *Vault) SaveSkill(ctx context.Context, s Save) (string, error) {
	head, err := v.Head(ctx)
	if err != nil {
		return "", err
	}
	merged, synthetic, report, err := v.mergeCopy(ctx, head, s)
	if err != nil {
		return "", err
	}
	if merged == "" {
		return "", v.leaveConflict(ctx, synthetic, s.Message, report)
	}
	headTree, err := v.rev(ctx, head+"^{tree}")
	if err != nil {
		return "", err
	}
	if merged == headTree {
		return head, nil
	}
	if _, err := v.git.Run(ctx, v.dir, "read-tree", "-m", "-u", head, merged); err != nil {
		return "", err
	}
	if _, err := v.Commit(ctx, s.Message); err != nil {
		// The commit didn't happen, so the work tree goes back to HEAD.
		_, _ = v.git.Run(context.WithoutCancel(ctx), v.dir, "read-tree", "--reset", "-u", head)
		return "", err
	}
	return v.Head(ctx)
}

// SaveOnto is what SaveSkill would commit if onto were HEAD: a commit whose parent
// is onto, or onto itself if the copy changes nothing there. It writes only git
// objects, never a ref, the index or the work tree: it is how sync --dry-run sees
// what step 1 would save. A conflict is a *ConflictError.
func (v *Vault) SaveOnto(ctx context.Context, onto string, s Save) (string, error) {
	merged, _, report, err := v.mergeCopy(ctx, onto, s)
	if err != nil {
		return "", err
	}
	if merged == "" {
		return "", &ConflictError{Paths: conflictPaths(report)}
	}
	return v.commitOnto(ctx, onto, merged, s.Message)
}

// mergeCopy merges a copy into the commit onto with git merge-tree, and returns the
// merged tree. The merge needs no --merge-base, which only git 2.40 has: a
// synthetic commit whose parent is the copy's commit, with the skill's folder
// replaced by the copy, makes that commit the natural merge base of onto and the
// copy. On a conflict merged is "" and report is merge-tree's output, which names
// the files that clash.
func (v *Vault) mergeCopy(ctx context.Context, onto string, s Save) (merged, synthetic, report string, err error) {
	baseTree, err := v.rev(ctx, s.Base+"^{tree}")
	if err != nil {
		return "", "", "", fmt.Errorf("vault: the commit %s was copied from: %w", short(s.Base), err)
	}
	tree, err := v.replaceAt(ctx, baseTree, strings.Split(s.Path, "/"), s.Tree)
	if err != nil {
		return "", "", "", err
	}
	if tree == "" { // the vault held nothing else
		if tree, err = v.git.Run(ctx, v.dir, "mktree", "-z"); err != nil {
			return "", "", "", err
		}
		tree = strings.TrimSpace(tree)
	}
	msg := "skl: " + s.Path + " as loaded\n\nSkl-Copy: " + s.Copy + "\n"
	synthetic, err = v.git.Run(ctx, v.dir, "commit-tree", tree, "-p", s.Base, "-m", msg)
	if err != nil {
		return "", "", "", err
	}
	synthetic = strings.TrimSpace(synthetic)
	out, err := v.git.Run(ctx, v.dir, "merge-tree", "--write-tree", "-z", "--name-only", "--no-messages", onto, synthetic)
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return "", synthetic, ge.Stdout, nil // 1 is merge-tree's "conflicts"
	}
	if err != nil {
		return "", "", "", err
	}
	merged, _, _ = strings.Cut(out, "\x00")
	return merged, synthetic, out, nil
}

// commitOnto is a commit of tree whose parent is onto, or onto itself when tree is
// onto's tree. It changes no ref.
func (v *Vault) commitOnto(ctx context.Context, onto, tree, message string) (string, error) {
	ontoTree, err := v.rev(ctx, onto+"^{tree}")
	if err != nil {
		return "", err
	}
	if tree == ontoTree {
		return onto, nil
	}
	out, err := v.git.Run(ctx, v.dir, "commit-tree", tree, "-p", onto, "-m", message)
	return strings.TrimSpace(out), err
}

// leaveConflict runs the merge that merge-tree found conflicts in, so it stays in
// progress for skl sync --continue or --abort.
func (v *Vault) leaveConflict(ctx context.Context, synthetic, message, report string) error {
	paths := conflictPaths(report)
	_, err := v.git.Run(ctx, v.dir, "merge", "--no-ff", "--no-commit", "--quiet", "-m", message, synthetic)
	if ge, ok := errors.AsType[*gitx.Error](err); err != nil && (!ok || ge.Code != 1 || ctx.Err() != nil) {
		return err
	}
	return &ConflictError{Paths: paths}
}

// conflictPaths reads the files that clash from merge-tree -z --name-only output.
func conflictPaths(report string) []string {
	_, tail, _ := strings.Cut(report, "\x00") // the merged tree comes first
	var paths []string
	for p := range strings.SplitSeq(tail, "\x00") {
		if p == "" {
			break // the end of the conflicted files
		}
		paths = append(paths, p)
	}
	return paths
}

// replaceAt is tree with the entry at parts set to leaf, a tree's name, or removed
// if leaf is "". A folder left empty is removed too.
func (v *Vault) replaceAt(ctx context.Context, tree string, parts []string, leaf string) (string, error) {
	var entries []treeEntry
	if tree != "" {
		out, err := v.git.Run(ctx, v.dir, "ls-tree", "-z", tree)
		if err != nil {
			return "", err
		}
		entries = treeEntries(out)
	}
	name := parts[0]
	i := slices.IndexFunc(entries, func(e treeEntry) bool { return e.name == name })
	child := leaf
	if len(parts) > 1 {
		sub := ""
		if i >= 0 && entries[i].kind == "tree" {
			sub = entries[i].object
		}
		var err error
		if child, err = v.replaceAt(ctx, sub, parts[1:], leaf); err != nil {
			return "", err
		}
	}
	if i >= 0 {
		entries = slices.Delete(entries, i, i+1)
	}
	if child != "" {
		entries = append(entries, treeEntry{mode: "040000", kind: "tree", object: child, name: name})
	}
	var body strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&body, "%s %s %s\t%s\x00", e.mode, e.kind, e.object, e.name)
	}
	if body.Len() == 0 {
		return "", nil
	}
	out, err := v.git.RunInput(ctx, v.dir, body.String(), "mktree", "-z")
	return strings.TrimSpace(out), err
}

func (v *Vault) rev(ctx context.Context, name string) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "rev-parse", "--verify", "--quiet", name)
	return strings.TrimSpace(out), err
}

type treeEntry struct{ mode, kind, object, name string }

// treeEntries parses ls-tree -z output: "<mode> <type> <object>\t<name>\x00".
func treeEntries(out string) []treeEntry {
	var entries []treeEntry
	for line := range strings.SplitSeq(out, "\x00") {
		meta, name, ok := strings.Cut(line, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 {
			continue
		}
		entries = append(entries, treeEntry{mode: f[0], kind: f[1], object: f[2], name: name})
	}
	return entries
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
