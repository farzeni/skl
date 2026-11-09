package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// twoClones is a remote with one commit and two clones of it: this machine's
// vault and another machine's checkout.
func twoClones(t *testing.T) (mine *Vault, other string) {
	t.Helper()
	git, home := env(t)
	remote := bare(t, home, "remote.git")
	seed := filepath.Join(home, "seed")
	run(t, home, "init", "--quiet", seed)
	write(t, filepath.Join(seed, "a.txt"), "one\ntwo\nthree\n")
	run(t, seed, "add", "-A")
	run(t, seed, "commit", "--quiet", "-m", "seed")
	run(t, seed, "push", "--quiet", remote, "HEAD:refs/heads/main")
	dir := filepath.Join(home, "vault")
	run(t, home, "clone", "--quiet", remote, dir)
	other = filepath.Join(home, "other")
	run(t, home, "clone", "--quiet", remote, other)
	return &Vault{git: git, dir: dir}, other
}

func commitFile(t *testing.T, dir, file, text string) {
	t.Helper()
	write(t, filepath.Join(dir, file), text)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "--quiet", "-m", "edit "+file)
}

// sync.md: "skl fetches the vault and merges. It never rebases, so no commit is
// rewritten."
func TestFetchMergePush(t *testing.T) {
	ctx := context.Background()
	v, other := twoClones(t)
	commitFile(t, other, "b.txt", "theirs\n")
	run(t, other, "push", "--quiet", "origin", "main")
	theirHead := run(t, other, "rev-parse", "HEAD")
	commitFile(t, v.dir, "c.txt", "mine\n")
	myHead := run(t, v.dir, "rev-parse", "HEAD")

	branch, err := v.Branch(ctx)
	if err != nil || branch != "main" {
		t.Fatalf("Branch = %q, %v", branch, err)
	}
	if err := v.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	ref, incoming, err := v.Incoming(ctx, branch)
	if err != nil || ref != "refs/remotes/origin/main" || !slices.Equal(incoming, []string{theirHead}) {
		t.Fatalf("Incoming = %q, %v, %v", ref, incoming, err)
	}
	if err := v.Merge(ctx, ref); err != nil {
		t.Fatal(err)
	}
	// A merge commit with both heads as parents: neither side rewritten.
	if parents := run(t, v.dir, "log", "-1", "--format=%P"); parents != myHead+" "+theirHead {
		t.Errorf("merge parents = %q, want %q", parents, myHead+" "+theirHead)
	}
	if err := v.Push(ctx, branch); err != nil {
		t.Fatal(err)
	}
	// Pushing to origin moves origin/main too: nothing left to push.
	if ahead, err := v.Ahead(ctx); err != nil || ahead != 0 {
		t.Errorf("Ahead after push = %d, %v", ahead, err)
	}
	if got := run(t, other, "ls-remote", "origin", "refs/heads/main"); got[:40] != run(t, v.dir, "rev-parse", "HEAD") {
		t.Errorf("remote main = %s", got)
	}
	// Nothing more to take.
	if _, incoming, err := v.Incoming(ctx, branch); err != nil || len(incoming) != 0 {
		t.Errorf("Incoming after merge = %v, %v", incoming, err)
	}
}

// A conflict is undone: HEAD and the work tree are as before, and the files that
// clashed are named.
func TestMergeConflictUndone(t *testing.T) {
	ctx := context.Background()
	v, other := twoClones(t)
	commitFile(t, other, "a.txt", "one\ntheirs\nthree\n")
	run(t, other, "push", "--quiet", "origin", "main")
	commitFile(t, v.dir, "a.txt", "one\nmine\nthree\n")
	before := run(t, v.dir, "rev-parse", "HEAD")
	if err := v.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	tree, conflicts, err := v.MergedTree(ctx, "refs/remotes/origin/main")
	if err != nil || tree == "" || !slices.Equal(conflicts, []string{"a.txt"}) {
		t.Errorf("MergedTree = %q, %v, %v", tree, conflicts, err)
	}
	err = v.Merge(ctx, "refs/remotes/origin/main")
	ce, ok := errors.AsType[*ConflictError](err)
	if !ok || !slices.Equal(ce.Paths, []string{"a.txt"}) {
		t.Fatalf("Merge = %v", err)
	}
	if head := run(t, v.dir, "rev-parse", "HEAD"); head != before {
		t.Errorf("HEAD moved: %s", head)
	}
	if merging, err := v.Merging(ctx); err != nil || merging {
		t.Errorf("Merging = %v, %v", merging, err)
	}
	if st := run(t, v.dir, "status", "--porcelain"); st != "" {
		t.Errorf("work tree not clean:\n%s", st)
	}
}

// Unmerge takes back a merge that landed, for a push that failed.
func TestUnmergeLanded(t *testing.T) {
	ctx := context.Background()
	v, other := twoClones(t)
	commitFile(t, other, "b.txt", "theirs\n")
	run(t, other, "push", "--quiet", "origin", "main")
	before := run(t, v.dir, "rev-parse", "HEAD")
	if err := v.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.Merge(ctx, "refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	v.Unmerge(ctx, before)
	if head := run(t, v.dir, "rev-parse", "HEAD"); head != before {
		t.Errorf("HEAD = %s, want %s", head, before)
	}
	if _, err := os.Stat(filepath.Join(v.dir, "b.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("b.txt still there: %v", err)
	}
}

// A new vault never pushed has nothing on origin: nothing to take.
func TestIncomingNoRemoteBranch(t *testing.T) {
	ctx := context.Background()
	v, _ := twoClones(t)
	ref, incoming, err := v.Incoming(ctx, "other")
	if err != nil || ref != "" || incoming != nil {
		t.Errorf("Incoming = %q, %v, %v", ref, incoming, err)
	}
}

func TestBranchDetached(t *testing.T) {
	ctx := context.Background()
	v, _ := twoClones(t)
	run(t, v.dir, "checkout", "--quiet", "--detach")
	if _, err := v.Branch(ctx); !errors.Is(err, ErrDetached) {
		t.Errorf("Branch = %v", err)
	}
}

// sync --dry-run reads the vault a merge would leave from a Snapshot: files,
// modes and links, and the Head it was given.
func TestSnapshot(t *testing.T) {
	ctx := context.Background()
	v, other := twoClones(t)
	if err := os.MkdirAll(filepath.Join(other, "g", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(other, "g", "s", "SKILL.md"), "skill\n")
	write(t, filepath.Join(other, "g", "s", "run.sh"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(other, "g", "s", "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(other, "g", "s", "link")); err != nil {
		t.Fatal(err)
	}
	run(t, other, "add", "-A")
	run(t, other, "commit", "--quiet", "-m", "skill")
	run(t, other, "push", "--quiet", "origin", "main")
	if err := v.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	tree, conflicts, err := v.MergedTree(ctx, "refs/remotes/origin/main")
	if err != nil || len(conflicts) > 0 {
		t.Fatalf("MergedTree = %v, %v", conflicts, err)
	}
	head, err := v.Resolve(ctx, "refs/remotes/origin/main")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "snap")
	snap, err := v.Snapshot(ctx, tree, head, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := snap.Head(ctx); err != nil || got != head {
		t.Errorf("Head = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "g", "s", "SKILL.md")); err != nil || string(got) != "skill\n" {
		t.Errorf("SKILL.md = %q, %v", got, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "g", "s", "run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("run.sh mode = %v, %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "g", "s", "link")); err != nil || target != "SKILL.md" {
		t.Errorf("link = %q, %v", target, err)
	}
	idx, err := snap.Index(ctx)
	if err != nil || len(idx.Skills("g")) != 1 {
		t.Errorf("Index: %v", err)
	}
	// The vault itself didn't change.
	if _, err := os.Stat(filepath.Join(v.dir, "g")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("vault has g: %v", err)
	}
}
