package vault

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// committed is a vault with one commit holding a.txt and b.txt.
func committed(t *testing.T) *Vault {
	t.Helper()
	git, home := env(t)
	dir := filepath.Join(home, "vault")
	run(t, home, "init", "--quiet", dir)
	for _, f := range []string{"a.txt", "b.txt"} {
		write(t, filepath.Join(dir, f), "one\n")
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "--quiet", "-m", "first")
	return &Vault{git: git, dir: dir}
}

func write(t *testing.T, file, text string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitOnlyThePaths(t *testing.T) {
	v := committed(t)
	ctx := context.Background()
	write(t, filepath.Join(v.dir, "a.txt"), "two\n")
	write(t, filepath.Join(v.dir, "b.txt"), "two\n")
	ok, err := v.Commit(ctx, "skl save a", "a.txt")
	if err != nil || !ok {
		t.Fatalf("Commit = %v, %v", ok, err)
	}
	if got := run(t, v.dir, "log", "-1", "--format=%s", "--name-only"); got != "skl save a\n\na.txt" {
		t.Errorf("last commit: %q", got)
	}
	if got := run(t, v.dir, "status", "--porcelain"); got != "M b.txt" {
		t.Errorf("status: %q", got)
	}
	// Nothing left to commit in a.txt.
	if ok, err := v.Commit(ctx, "again", "a.txt"); err != nil || ok {
		t.Errorf("second Commit = %v, %v", ok, err)
	}
}

func TestCommitEverything(t *testing.T) {
	v := committed(t)
	write(t, filepath.Join(v.dir, "c.txt"), "new\n")
	// The vault's .gitignore applies: plain add -A, not --force.
	write(t, filepath.Join(v.dir, ".gitignore"), "ignored.txt\n")
	write(t, filepath.Join(v.dir, "ignored.txt"), "x\n")
	ok, err := v.Commit(context.Background(), "hand edits")
	if err != nil || !ok {
		t.Fatalf("Commit = %v, %v", ok, err)
	}
	if got := run(t, v.dir, "log", "-1", "--format=", "--name-only"); got != ".gitignore\nc.txt" {
		t.Errorf("committed: %q", got)
	}
}

func TestCommitFailsAndUndoes(t *testing.T) {
	v := committed(t)
	head := run(t, v.dir, "rev-parse", "HEAD")
	write(t, filepath.Join(v.dir, "a.txt"), "two\n")
	// A pre-commit hook that refuses.
	hook := filepath.Join(v.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Commit(context.Background(), "refused", "a.txt"); err == nil {
		t.Fatal("Commit succeeded")
	}
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved to %s", got)
	}
	if got := run(t, v.dir, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("still staged: %q", got)
	}
}

func TestUncommit(t *testing.T) {
	v := committed(t)
	ctx := context.Background()
	before := run(t, v.dir, "rev-parse", "HEAD")
	write(t, filepath.Join(v.dir, "a.txt"), "two\n")
	if ok, err := v.Commit(ctx, "change", "a.txt"); err != nil || !ok {
		t.Fatalf("Commit = %v, %v", ok, err)
	}
	v.Uncommit(ctx, before, "a.txt")
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != before {
		t.Errorf("HEAD is %s, want %s", got, before)
	}
	// The working tree is kept: the caller puts its file back.
	if got := run(t, v.dir, "status", "--porcelain"); got != "M a.txt" {
		t.Errorf("status: %q", got)
	}
}
