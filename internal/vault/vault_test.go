package vault

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/gitx"
)

// env gives a test a HOME with a git identity, and returns a Git.
func env(t *testing.T) (*gitx.Git, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig"))
	run(t, dir, "config", "--global", "user.name", "Ann Author")
	run(t, dir, "config", "--global", "user.email", "ann@example.com")
	run(t, dir, "config", "--global", "init.defaultBranch", "main")
	return gitx.New(nil), dir
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func bare(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	run(t, dir, "init", "--quiet", "--bare", p)
	return p
}

func TestOpen(t *testing.T) {
	git, dir := env(t)
	missing := filepath.Join(dir, "missing")
	if _, err := Open(git, missing); !errors.Is(err, ErrNoVault) {
		t.Errorf("missing: %v", err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(git, empty); !errors.Is(err, ErrNoVault) {
		t.Errorf("empty: %v", err)
	}
	full := filepath.Join(dir, "full")
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(git, full); !errors.Is(err, ErrNotClone) {
		t.Errorf("full: %v", err)
	}
	file := filepath.Join(full, "x")
	if _, err := Open(git, file); !errors.Is(err, ErrNotClone) {
		t.Errorf("a file: %v", err)
	}
	// A folder inside another repo isn't a vault because of that repo.
	run(t, dir, "init", "--quiet", "outer")
	inner := filepath.Join(dir, "outer", "vault")
	if _, err := Open(git, inner); !errors.Is(err, ErrNoVault) {
		t.Errorf("inside another repo: %v", err)
	}
	clone := filepath.Join(dir, "clone")
	run(t, dir, "init", "--quiet", clone)
	if v, err := Open(git, clone); err != nil || v.Dir() != clone {
		t.Errorf("a clone: %v, %v", v, err)
	}
}

// docs/guide/getting-started.md: "An empty repo becomes a new vault."
func TestCreate(t *testing.T) {
	git, dir := env(t)
	ctx := context.Background()
	url := bare(t, dir, "e.git")
	if empty, err := RemoteEmpty(ctx, git, url); err != nil || !empty {
		t.Fatalf("RemoteEmpty = %v, %v", empty, err)
	}
	v, err := Clone(ctx, git, url, filepath.Join(dir, "v"))
	if err != nil {
		t.Fatal(err)
	}
	if born, err := v.Born(ctx); err != nil || born {
		t.Fatalf("Born before the first commit = %v, %v", born, err)
	}
	now := time.Date(2031, 2, 3, 0, 0, 0, 0, time.UTC)
	if err := v.Create(ctx, "MIT", now); err != nil {
		t.Fatal(err)
	}
	if born, err := v.Born(ctx); err != nil || !born {
		t.Errorf("Born after = %v, %v", born, err)
	}
	if n, err := v.Ahead(ctx); err != nil || n != 1 {
		t.Errorf("Ahead = %d, %v, want 1", n, err)
	}
	text, err := os.ReadFile(filepath.Join(v.Dir(), "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "Copyright (c) 2031 Ann Author\n") || strings.Contains(string(text), "<") {
		t.Errorf("LICENSE:\n%s", text)
	}
	if got := run(t, v.Dir(), "log", "--format=%an <%ae> %s"); got != "Ann Author <ann@example.com> skl init: new vault" {
		t.Errorf("log = %q", got)
	}
	if empty, err := RemoteEmpty(ctx, git, url); err != nil || !empty {
		t.Errorf("the remote changed: %v, %v", empty, err)
	}

	// Ahead follows origin: pushed commits aren't changes.
	run(t, v.Dir(), "push", "--quiet", "origin", "HEAD")
	run(t, v.Dir(), "fetch", "--quiet", "origin")
	if n, err := v.Ahead(ctx); err != nil || n != 0 {
		t.Errorf("Ahead after a push = %d, %v, want 0", n, err)
	}
	run(t, v.Dir(), "commit", "--quiet", "--allow-empty", "-m", "a")
	run(t, v.Dir(), "commit", "--quiet", "--allow-empty", "-m", "b")
	if n, err := v.Ahead(ctx); err != nil || n != 2 {
		t.Errorf("Ahead = %d, %v, want 2", n, err)
	}
}

// docs/guide/learn.md "Licenses": "none" writes none.
func TestCreateNoLicense(t *testing.T) {
	git, dir := env(t)
	ctx := context.Background()
	v, err := Clone(ctx, git, bare(t, dir, "e.git"), filepath.Join(dir, "v"))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Create(ctx, NoLicense, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(v.Dir(), "LICENSE")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("LICENSE: %v", err)
	}
	if _, err := v.Head(ctx); err != nil {
		t.Errorf("no HEAD: %v", err)
	}
}

func TestLicenseText(t *testing.T) {
	for _, tt := range []struct{ id, want string }{
		{"MIT", "Copyright (c) 2030 Me\n"},
		{"Apache-2.0", "Version 2.0, January 2004"},
		{"CC0-1.0", "SPDX-License-Identifier: CC0-1.0\n"},
	} {
		if got := licenseText(tt.id, "Me", 2030); !strings.Contains(got, tt.want) {
			t.Errorf("%s:\n%s", tt.id, got)
		}
	}
}

func TestCheckIdentity(t *testing.T) {
	git, dir := env(t)
	ctx := context.Background()
	if err := CheckIdentity(ctx, git, ""); err != nil {
		t.Fatalf("with an identity: %v", err)
	}
	cfg := filepath.Join(dir, "none")
	run(t, dir, "config", "--file", cfg, "user.useConfigOnly", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	if err := CheckIdentity(ctx, git, ""); !errors.Is(err, ErrNoIdentity) {
		t.Errorf("without: %v", err)
	}
	t.Setenv("GIT_AUTHOR_NAME", "A")
	t.Setenv("GIT_AUTHOR_EMAIL", "a@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "A")
	t.Setenv("GIT_COMMITTER_EMAIL", "a@example.com")
	if err := CheckIdentity(ctx, git, ""); err != nil {
		t.Errorf("from the environment: %v", err)
	}
}

// git clone exits 0 for a repo whose HEAD names no branch, with nothing
// checked out.
func TestBornNoDefaultBranch(t *testing.T) {
	git, dir := env(t)
	ctx := context.Background()
	url := bare(t, dir, "o.git")
	run(t, dir, "clone", "--quiet", url, "w")
	work := filepath.Join(dir, "w")
	run(t, work, "commit", "--quiet", "--allow-empty", "-m", "x")
	run(t, work, "push", "--quiet", "origin", "HEAD:refs/heads/dev")
	if empty, err := RemoteEmpty(ctx, git, url); err != nil || empty {
		t.Errorf("RemoteEmpty = %v, %v", empty, err)
	}
	v, err := Clone(ctx, git, url, filepath.Join(dir, "v"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Born(ctx); !errors.Is(err, ErrNoDefaultBranch) {
		t.Errorf("Born: %v", err)
	}
}

func TestRemoteEmptyUnreachable(t *testing.T) {
	git, dir := env(t)
	if _, err := RemoteEmpty(context.Background(), git, filepath.Join(dir, "nope")); !errors.Is(err, gitx.ErrUnreachable) {
		t.Errorf("err = %v", err)
	}
}

func TestOrigin(t *testing.T) {
	git, dir := env(t)
	ctx := context.Background()
	clone := filepath.Join(dir, "c")
	run(t, dir, "init", "--quiet", clone)
	v, err := Open(git, clone)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := v.Origin(ctx); err != nil || u != "" {
		t.Errorf("no origin: %q, %v", u, err)
	}
	run(t, clone, "remote", "add", "origin", "https://x.example/you/skills")
	// As configured: url.insteadOf doesn't change what it is.
	run(t, dir, "config", "--global", "url.file:///nowhere/.insteadOf", "https://x.example/")
	if u, err := v.Origin(ctx); err != nil || u != "https://x.example/you/skills" {
		t.Errorf("origin: %q, %v", u, err)
	}
}

// docs/reference/files.md "Your vault": what counts as a group, a skill and a set.
func TestCount(t *testing.T) {
	git, dir := env(t)
	write := func(rel string) {
		p := filepath.Join(dir, "v", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		"review/pr-review/SKILL.md",
		"review/test-gaps/SKILL.md",
		"data/sql/SKILL.md",
		"docs/guide.md",     // no skill: not a group
		"deep/a/b/SKILL.md", // "A SKILL.md at any other depth is ignored."
		"loose/SKILL.md",    // same
		"sets/backend.toml",
		"sets/backend.origin.toml", // "never reads a file ending in .origin.toml as a skill set"
		"sets/notes.txt",
		"lessons/x/SKILL.md", // reserved
		"h/x/SKILL.md",       // reserved
		"sets/x/SKILL.md",    // reserved
		".github/x/SKILL.md", // hidden
		"README.md",
	} {
		write(f)
	}
	v := &Vault{git: git, dir: filepath.Join(dir, "v")}
	c, err := v.Count()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Counts{Groups: 2, Skills: 3, Sets: 1}); c != want {
		t.Errorf("Count = %+v, want %+v", c, want)
	}
}
