package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newVault is a clone with one commit holding g/a/SKILL.md, five lines apart.
func newVault(t *testing.T) (*Vault, string) {
	t.Helper()
	git, dir := env(t)
	vdir := filepath.Join(dir, "vault")
	run(t, dir, "init", "--quiet", vdir)
	writeFile(t, filepath.Join(vdir, "g/a/SKILL.md"), "one\n\ntwo\n\nthree\n\nfour\n\nfive\n")
	writeFile(t, filepath.Join(vdir, "g/b/SKILL.md"), "b\n")
	run(t, vdir, "add", "-A")
	run(t, vdir, "commit", "--quiet", "-m", "first")
	v, err := Open(git, vdir)
	if err != nil {
		t.Fatal(err)
	}
	return v, run(t, vdir, "rev-parse", "HEAD")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyTree is the tree of g/a as a copy made from base holds it, with SKILL.md
// replaced: made on a side branch, which is then deleted, as a copy's tree has no
// commit of its own.
func copyTree(t *testing.T, v *Vault, base, skill string) string {
	t.Helper()
	head := run(t, v.dir, "rev-parse", "HEAD")
	run(t, v.dir, "checkout", "--quiet", "-b", "side", base)
	writeFile(t, filepath.Join(v.dir, "g/a/SKILL.md"), skill)
	run(t, v.dir, "commit", "--quiet", "-am", "side")
	tree := run(t, v.dir, "rev-parse", "HEAD:g/a")
	run(t, v.dir, "checkout", "--quiet", head)
	run(t, v.dir, "checkout", "--quiet", "-B", "main")
	run(t, v.dir, "branch", "--quiet", "-D", "side")
	return tree
}

func commitInVault(t *testing.T, v *Vault, file, content string) {
	t.Helper()
	writeFile(t, filepath.Join(v.dir, file), content)
	run(t, v.dir, "commit", "--quiet", "-am", "from elsewhere")
}

func read(t *testing.T, v *Vault, file string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(v.dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// architecture.md "Saving back": "merges each folder that changed into the vault
// with git merge-tree, using the recorded commit as the base".
func TestSaveSkill(t *testing.T) {
	ctx := context.Background()
	t.Run("only the copy changed", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "one\n\ntwo\n\nthree\n\nfour\n\nfive, here\n")
		head, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "skl: save a", Copy: "claude a"})
		if err != nil {
			t.Fatal(err)
		}
		if got := run(t, v.dir, "rev-parse", "HEAD"); head != got || head == base {
			t.Fatalf("head = %s, HEAD = %s, base = %s", head, got, base)
		}
		if got := run(t, v.dir, "rev-parse", "HEAD:g/a"); got != tree {
			t.Errorf("g/a = %s, want the copy's %s", got, tree)
		}
		// One commit on HEAD: the synthetic one never enters the history.
		if n := run(t, v.dir, "rev-list", "--count", "HEAD"); n != "2" {
			t.Errorf("%s commits", n)
		}
		if msg := run(t, v.dir, "log", "-1", "--format=%s"); msg != "skl: save a" {
			t.Errorf("message %q", msg)
		}
		if st := run(t, v.dir, "status", "--porcelain"); st != "" {
			t.Errorf("work tree: %s", st)
		}
	})
	t.Run("both changed apart", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "one\n\ntwo\n\nthree\n\nfour\n\nfive, here\n")
		commitInVault(t, v, "g/a/SKILL.md", "one, there\n\ntwo\n\nthree\n\nfour\n\nfive\n")
		if _, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"}); err != nil {
			t.Fatal(err)
		}
		if got := read(t, v, "g/a/SKILL.md"); got != "one, there\n\ntwo\n\nthree\n\nfour\n\nfive, here\n" {
			t.Errorf("merged:\n%s", got)
		}
	})
	t.Run("the vault already has it", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "same\n")
		commitInVault(t, v, "g/a/SKILL.md", "same\n")
		before := run(t, v.dir, "rev-parse", "HEAD")
		head, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"})
		if err != nil || head != before {
			t.Fatalf("head = %s, err = %v; want %s", head, err, before)
		}
	})
	t.Run("same lines", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "one, here\n\ntwo\n\nthree\n\nfour\n\nfive\n")
		commitInVault(t, v, "g/a/SKILL.md", "one, there\n\ntwo\n\nthree\n\nfour\n\nfive\n")
		before := run(t, v.dir, "rev-parse", "HEAD")
		_, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"})
		ce, ok := errors.AsType[*ConflictError](err)
		if !ok || !slices.Equal(ce.Paths, []string{"g/a/SKILL.md"}) {
			t.Fatalf("err = %v", err)
		}
		if got := run(t, v.dir, "rev-parse", "HEAD"); got != before {
			t.Error("a conflict committed something")
		}
		merging, err := v.Merging(ctx)
		if err != nil || !merging {
			t.Errorf("Merging = %v, %v", merging, err)
		}
		if !strings.Contains(read(t, v, "g/a/SKILL.md"), "<<<<<<<") {
			t.Error("no conflict markers")
		}
		// The merge names the copy, for skl sync --continue.
		if msg := run(t, v.dir, "log", "-1", "--format=%B", "MERGE_HEAD"); !strings.Contains(msg, "Skl-Copy: claude a") {
			t.Errorf("MERGE_HEAD message %q", msg)
		}
	})
	t.Run("deleted in the vault, changed in the copy", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "changed\n")
		run(t, v.dir, "rm", "--quiet", "-r", "g/a")
		run(t, v.dir, "commit", "--quiet", "-m", "gone")
		_, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"})
		if _, ok := errors.AsType[*ConflictError](err); !ok {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a group the vault no longer has", func(t *testing.T) {
		v, base := newVault(t)
		tree := copyTree(t, v, base, "one\n\ntwo\n\nthree\n\nfour\n\nfive, here\n")
		run(t, v.dir, "mv", "g/b", "b-moved")
		run(t, v.dir, "commit", "--quiet", "-m", "moved")
		if _, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"}); err != nil {
			t.Fatal(err)
		}
		if got := run(t, v.dir, "rev-parse", "HEAD:g/a"); got != tree {
			t.Errorf("g/a = %s", got)
		}
	})
	t.Run("a base the vault doesn't have", func(t *testing.T) {
		v, _ := newVault(t)
		_, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: strings.Repeat("1", 40), Tree: strings.Repeat("2", 40), Message: "m"})
		if err == nil || !strings.Contains(err.Error(), "the commit 1111111 was copied from") {
			t.Fatalf("err = %v", err)
		}
	})
}

// sync.md "What gets committed": hand edits in ~/.skl/vault become one commit.
func TestHandEditsAndCommit(t *testing.T) {
	ctx := context.Background()
	v, _ := newVault(t)
	writeFile(t, filepath.Join(v.dir, ".gitignore"), "*.log\n")
	run(t, v.dir, "add", ".gitignore")
	run(t, v.dir, "commit", "--quiet", "-m", "ignore")
	if files, err := v.HandEdits(ctx); err != nil || len(files) != 0 {
		t.Fatalf("clean: %v, %v", files, err)
	}
	if ok, err := v.Commit(ctx, "nothing"); ok || err != nil {
		t.Fatalf("Commit on a clean vault = %v, %v", ok, err)
	}
	writeFile(t, filepath.Join(v.dir, "g/a/SKILL.md"), "edited\n")
	writeFile(t, filepath.Join(v.dir, "sets/x.toml"), "select = []\n")
	writeFile(t, filepath.Join(v.dir, "g/a/run.log"), "ignored\n")
	if err := os.Remove(filepath.Join(v.dir, "g/b/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	files, err := v.HandEdits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	if want := []string{"g/a/SKILL.md", "g/b/SKILL.md", "sets/x.toml"}; !slices.Equal(files, want) {
		t.Errorf("HandEdits = %v, want %v", files, want)
	}
	if ok, err := v.Commit(ctx, "skl: commit hand edits"); !ok || err != nil {
		t.Fatalf("Commit = %v, %v", ok, err)
	}
	if st := run(t, v.dir, "status", "--porcelain"); st != "" {
		t.Errorf("left: %s", st)
	}
	if got := run(t, v.dir, "show", "--name-only", "--format=", "HEAD"); got != "g/a/SKILL.md\ng/b/SKILL.md\nsets/x.toml" {
		t.Errorf("committed:\n%s", got)
	}
}

// A save whose commit fails puts the work tree back.
func TestSaveSkillCommitFails(t *testing.T) {
	ctx := context.Background()
	v, base := newVault(t)
	tree := copyTree(t, v, base, "changed\n")
	writeFile(t, filepath.Join(v.dir, ".git/hooks/pre-commit"), "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(filepath.Join(v.dir, ".git/hooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := v.SaveSkill(ctx, Save{Path: "g/a", Base: base, Tree: tree, Message: "m", Copy: "claude a"}); err == nil {
		t.Fatal("the hook should stop it")
	}
	if st := run(t, v.dir, "status", "--porcelain"); st != "" {
		t.Errorf("work tree: %q", st)
	}
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != base {
		t.Error("HEAD moved")
	}
}

func TestWriteObjectsAndFiles(t *testing.T) {
	ctx := context.Background()
	v, base := newVault(t)
	file := filepath.Join(t.TempDir(), "f")
	writeFile(t, file, "hello\n")
	names, err := v.WriteObjects(ctx, Objects{
		Files: []string{file},
		Links: []string{"../elsewhere"},
		Trees: []string{"100644 blob ce013625030ba8dba906f756967f9e9ca394464a\tf\x00"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "ce013625030ba8dba906f756967f9e9ca394464a" { // "hello\n"
		t.Fatalf("names = %v", names)
	}
	if got := run(t, v.dir, "cat-file", "-p", names[1]); got != "../elsewhere" {
		t.Errorf("link blob holds %q", got)
	}
	if kind := run(t, v.dir, "cat-file", "-t", names[2]); kind != "tree" {
		t.Errorf("tree object is a %s", kind)
	}
	files, err := v.Files(ctx, base, "g/a")
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := files["SKILL.md"]; len(files) != 1 || !ok || f.Mode != "100644" {
		t.Errorf("Files = %+v", files)
	}
	if files, err := v.Files(ctx, base, "g/none"); err != nil || len(files) != 0 {
		t.Errorf("missing path: %v, %v", files, err)
	}
}
