package vault

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// history is a vault with two commits: the first holds review/pr-review,
// writing/changelog and sets/base.toml; the second changes pr-review, removes
// changelog, adds data/sql and a set, and leaves base.toml alone.
func history(t *testing.T) (v *Vault, first, second string) {
	t.Helper()
	git, home := env(t)
	dir := filepath.Join(home, "vault")
	run(t, home, "init", "--quiet", dir)
	files := map[string]string{
		"review/pr-review/SKILL.md":         "one\n",
		"review/pr-review/scripts/check.sh": "#!/bin/sh\n",
		"writing/changelog/SKILL.md":        "log\n",
		"sets/base.toml":                    "select = []\n",
		"LICENSE":                           "MIT\n",
	}
	for f, text := range files {
		mkfile(t, filepath.Join(dir, f), text)
	}
	if err := os.Chmod(filepath.Join(dir, "review/pr-review/scripts/check.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "--quiet", "-m", "first")
	first = run(t, dir, "rev-parse", "HEAD")
	mkfile(t, filepath.Join(dir, "review/pr-review/SKILL.md"), "two\n")
	run(t, dir, "rm", "-r", "-q", "writing")
	mkfile(t, filepath.Join(dir, "data/sql/SKILL.md"), "sql\n")
	mkfile(t, filepath.Join(dir, "sets/extra.toml"), "select = [\"data\"]\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "--quiet", "-m", "second")
	second = run(t, dir, "rev-parse", "HEAD")
	return &Vault{git: git, dir: dir}, first, second
}

func mkfile(t *testing.T, file, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, file, text)
}

// docs/guide/rollback.md "Go back": a rollback reverts each skill that changed.
// A skill's folder is one unit; a file outside a skill is one of its own.
func TestUnits(t *testing.T) {
	v, first, second := history(t)
	units, err := v.Units(context.Background(), first, second)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"data/sql", "review/pr-review", "sets/extra.toml", "writing/changelog"}
	if !slices.Equal(units, want) {
		t.Errorf("Units = %v, want %v", units, want)
	}
}

// "If skill content changed in the meantime ... skl also adds a revert commit to
// your vault. It never rewrites git history."
func TestRevert(t *testing.T) {
	v, first, second := history(t)
	ctx := context.Background()
	units, _ := v.Units(ctx, first, second)
	would, err := v.Reverts(ctx, run(t, v.dir, "rev-parse", "HEAD"), first, units)
	if err != nil || !slices.Equal(would, units) {
		t.Fatalf("Reverts = %v, %v", would, err)
	}
	for _, u := range units {
		commit, err := v.Revert(ctx, first, u, "revert "+u)
		if err != nil || commit == "" {
			t.Fatalf("Revert(%s) = %q, %v", u, commit, err)
		}
		if got := run(t, v.dir, "log", "-1", "--format=%s"); got != "revert "+u {
			t.Errorf("last commit: %q", got)
		}
	}
	// The tree is the first commit's again, on top of the history.
	if got, want := run(t, v.dir, "rev-parse", "HEAD^{tree}"), run(t, v.dir, "rev-parse", first+"^{tree}"); got != want {
		t.Errorf("tree %s, want %s", got, want)
	}
	if got := run(t, v.dir, "rev-list", "--count", "HEAD"); got != "6" {
		t.Errorf("%s commits, want 6", got)
	}
	run(t, v.dir, "merge-base", "--is-ancestor", second, "HEAD")
	if got := run(t, v.dir, "status", "--porcelain"); got != "" {
		t.Errorf("status: %q", got)
	}
	// Nothing left to revert: no commit.
	if commit, err := v.Revert(ctx, first, "review/pr-review", "again"); err != nil || commit != "" {
		t.Errorf("second Revert = %q, %v", commit, err)
	}
	if would, _ := v.Reverts(ctx, run(t, v.dir, "rev-parse", "HEAD"), first, units); len(would) != 0 {
		t.Errorf("Reverts after = %v", would)
	}
}

// A revert that fails leaves the vault as it was.
func TestRevertFailsAndUndoes(t *testing.T) {
	v, first, _ := history(t)
	head := run(t, v.dir, "rev-parse", "HEAD")
	hook := filepath.Join(v.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Revert(context.Background(), first, "writing/changelog", "refused"); err == nil {
		t.Fatal("Revert succeeded")
	}
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved to %s", got)
	}
	if got := run(t, v.dir, "status", "--porcelain"); got != "" {
		t.Errorf("status: %q", got)
	}
}

// ResetTo takes back skl's own commits, work tree included.
func TestResetTo(t *testing.T) {
	v, first, _ := history(t)
	ctx := context.Background()
	head := run(t, v.dir, "rev-parse", "HEAD")
	if _, err := v.Revert(ctx, first, "review/pr-review", "revert"); err != nil {
		t.Fatal(err)
	}
	v.ResetTo(ctx, head)
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %s, want %s", got, head)
	}
	if got := run(t, v.dir, "status", "--porcelain"); got != "" {
		t.Errorf("status: %q", got)
	}
}

// --dry-run: Preview writes the vault as the reverts would leave it, and changes
// nothing in the vault.
func TestPreview(t *testing.T) {
	v, first, second := history(t)
	ctx := context.Background()
	units := []string{"review/pr-review", "data/sql", "writing/changelog"} // not sets/extra.toml
	dir := filepath.Join(t.TempDir(), "preview")
	if err := v.Preview(ctx, second, first, units, dir); err != nil {
		t.Fatal(err)
	}
	read := func(f string) string {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return "<" + err.Error() + ">"
		}
		return string(data)
	}
	for f, want := range map[string]string{
		"review/pr-review/SKILL.md":  "one\n",
		"writing/changelog/SKILL.md": "log\n",
		"sets/extra.toml":            "select = [\"data\"]\n",
		"LICENSE":                    "MIT\n",
	} {
		if got := read(f); got != want {
			t.Errorf("%s = %q, want %q", f, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "data")); !os.IsNotExist(err) {
		t.Errorf("data/ is there: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "review/pr-review/scripts/check.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("check.sh lost its mode: %v %v", info, err)
	}
	idx, err := IndexAt(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.Groups(); !slices.Equal(got, []string{"review", "writing"}) {
		t.Errorf("groups = %v", got)
	}
	if got := run(t, v.dir, "rev-parse", "HEAD"); got != second {
		t.Errorf("HEAD moved")
	}
	if got := run(t, v.dir, "status", "--porcelain"); got != "" {
		t.Errorf("status: %q", got)
	}
}

// Nothing in an archive lands outside the folder.
func TestUntarStaysInside(t *testing.T) {
	for _, name := range []string{"../evil", "/abs/evil", "a/../../evil"} {
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte("x"))
		_ = tw.Close()
		dir := filepath.Join(t.TempDir(), "out")
		err := untar(context.Background(), &b, dir)
		if err == nil || !strings.Contains(err.Error(), "git archive holds") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
