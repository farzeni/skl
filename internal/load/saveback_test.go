package load

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/gitx"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/vault"
)

// saveFixture is a fixture whose vault is a git repo, with g/a and g/b loaded.
type saveFixture struct {
	*fixture
	v      *vault.Vault
	copies []state.Copy
}

func newSaveFixture(t *testing.T, targets ...config.Target) *saveFixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "Ann")
	t.Setenv("GIT_AUTHOR_EMAIL", "ann@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Ann")
	t.Setenv("GIT_COMMITTER_EMAIL", "ann@example.com")
	f := &saveFixture{fixture: newFixture(t)}
	if len(targets) > 0 {
		f.targets = targets
	}
	gitOut(t, f.vault, "init", "--quiet")
	gitOut(t, f.vault, "add", "-A")
	gitOut(t, f.vault, "commit", "--quiet", "-m", "first")
	v, err := vault.Open(gitx.New(nil), f.vault)
	if err != nil {
		t.Fatal(err)
	}
	f.v = v
	f.copies = f.loadAt(t, f.a, f.b)
	return f
}

func (f *saveFixture) head(t *testing.T) string { return gitOut(t, f.vault, "rev-parse", "HEAD") }

// loadAt loads skills from the vault's HEAD over what's there.
func (f *saveFixture) loadAt(t *testing.T, skills ...resolve.Skill) []state.Copy {
	t.Helper()
	p, err := Compute(context.Background(), Input{
		Vault: f.vault, Commit: f.head(t), Result: result(t, skills...),
		Current: f.copies, Targets: f.targets, HomeRoot: f.home,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewLoader(f.staging).Apply(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return p.Copies
}

func (f *saveFixture) save(t *testing.T) ([]Saved, error) {
	t.Helper()
	return SaveBack(context.Background(), SaveInput{Vault: f.v, HomeRoot: f.home, Targets: f.targets, Current: f.copies})
}

func (f *saveFixture) mustSave(t *testing.T) []Saved {
	t.Helper()
	saved, err := f.save(t)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

// skills.md "Edit as usual": "skl saves each change to your vault".
func TestSaveBackSavesWhatChanged(t *testing.T) {
	f := newSaveFixture(t)
	if saved := f.mustSave(t); len(saved) != 0 {
		t.Fatalf("nothing changed, saved %+v", saved)
	}
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A, better\n", 0o644)
	write(t, filepath.Join(f.skills(), "a/evals/evals.json"), "{}\n", 0o644)
	write(t, filepath.Join(f.skills(), "a/run.sh"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("SKILL.md", filepath.Join(f.skills(), "a/link")); err != nil {
		t.Fatal(err)
	}
	saved := f.mustSave(t)
	if len(saved) != 1 {
		t.Fatalf("saved %+v", saved)
	}
	s := saved[0]
	if s.Name != "a" || s.Skill != "g/a" || s.Target != config.Claude || s.Commit != f.head(t) {
		t.Errorf("saved %+v", s)
	}
	if want := []string{"SKILL.md", "evals/evals.json", "link", "run.sh"}; !slices.Equal(s.Files, want) {
		t.Errorf("files = %v, want %v", s.Files, want)
	}
	// The vault holds exactly the copy, modes and link included.
	tree, err := Tree(context.Background(), filepath.Join(f.skills(), "a"))
	if err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, f.vault, "rev-parse", "HEAD:g/a"); got != tree || s.Tree != tree {
		t.Errorf("vault g/a = %s, copy = %s, recorded %s", got, tree, s.Tree)
	}
	// The copy's record moves on, so it's unchanged again.
	m := readManifestFile(t, filepath.Join(f.skills(), "a"))
	if m.Commit != s.Commit || m.Tree != tree || m.Source != "g/a" {
		t.Errorf("manifest = %+v", m)
	}
	if again := f.mustSave(t); len(again) != 0 {
		t.Errorf("saved twice: %+v", again)
	}
}

// "git merge-tree sees added, deleted and renamed files".
func TestSaveBackDeletedFile(t *testing.T) {
	f := newSaveFixture(t)
	write(t, filepath.Join(f.vault, "g/a/extra.md"), "x\n", 0o644)
	gitOut(t, f.vault, "add", "-A")
	gitOut(t, f.vault, "commit", "--quiet", "-m", "extra")
	f.copies = f.loadAt(t, f.a, f.b)
	if err := os.Remove(filepath.Join(f.skills(), "a/extra.md")); err != nil {
		t.Fatal(err)
	}
	saved := f.mustSave(t)
	if len(saved) != 1 || !slices.Equal(saved[0].Files, []string{"extra.md"}) {
		t.Fatalf("saved %+v", saved)
	}
	if _, err := os.Stat(filepath.Join(f.vault, "g/a/extra.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("extra.md still in the vault: %v", err)
	}
}

// Comment on #66: "a loaded folder deleted by hand is not a deletion".
func TestSaveBackNeverDeletesASkill(t *testing.T) {
	f := newSaveFixture(t)
	if err := os.RemoveAll(filepath.Join(f.skills(), "a")); err != nil {
		t.Fatal(err)
	}
	// A folder emptied of every file but its record says no more.
	if err := os.Remove(filepath.Join(f.skills(), "b/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	before := f.head(t)
	if saved := f.mustSave(t); len(saved) != 0 || f.head(t) != before {
		t.Fatalf("saved %+v", saved)
	}
}

// skills.md: "Folders next to it ... stay out of your vault"; ".skl-copy.toml never
// enters the vault".
func TestSaveBackReadsOnlyTheCopy(t *testing.T) {
	f := newSaveFixture(t)
	write(t, filepath.Join(f.skills(), "a-workspace/run.json"), "{}\n", 0o644)
	write(t, filepath.Join(f.skills(), "a", manifestFile), "# a note\n", 0o644)
	if saved := f.mustSave(t); len(saved) != 0 {
		t.Fatalf("saved %+v", saved)
	}
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A2\n", 0o644)
	f.mustSave(t)
	if out := gitOut(t, f.vault, "ls-tree", "-r", "--name-only", "HEAD"); out != "g/a/SKILL.md\ng/b/SKILL.md" {
		t.Errorf("vault holds:\n%s", out)
	}
}

// A copy whose record skl can't read is saved from what state.json says.
func TestSaveBackUnreadableManifest(t *testing.T) {
	f := newSaveFixture(t)
	write(t, filepath.Join(f.skills(), "a", manifestFile), "not = [toml", 0o644)
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A2\n", 0o644)
	saved := f.mustSave(t)
	if len(saved) != 1 || saved[0].Skill != "g/a" {
		t.Fatalf("saved %+v", saved)
	}
	if m := readManifestFile(t, filepath.Join(f.skills(), "a")); m.Source != "g/a" || m.Commit != f.head(t) {
		t.Errorf("manifest = %+v", m)
	}
	// With no record anywhere, nothing says where it came from: it isn't saved.
	f.copies = nil
	write(t, filepath.Join(f.skills(), "a", manifestFile), "not = [toml", 0o644)
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A3\n", 0o644)
	if saved := f.mustSave(t); len(saved) != 0 {
		t.Errorf("saved %+v", saved)
	}
}

// A record can't send a save outside the vault's skill folders.
func TestSaveBackRefusesOddSources(t *testing.T) {
	for _, source := range []string{"../g/a", "g", "g/a/b", ".git/a", "g/.."} {
		if vaultSkill(source) {
			t.Errorf("%q counts as a vault skill", source)
		}
	}
	if !vaultSkill("review/pr-review") {
		t.Error("review/pr-review should count")
	}
}

// architecture.md: "With two targets, each copy is saved back on its own ... the
// second one merges into the vault the first one already changed."
func TestSaveBackTwoTargets(t *testing.T) {
	f := newSaveFixture(t, config.Claude, config.Agents)
	write(t, filepath.Join(f.vault, "g/a/SKILL.md"), "1\n\n2\n\n3\n\n4\n\n5\n", 0o644)
	gitOut(t, f.vault, "commit", "--quiet", "-am", "long")
	f.copies = f.loadAt(t, f.a, f.b)
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "1 claude\n\n2\n\n3\n\n4\n\n5\n", 0o644)
	agents := SkillsDir(f.home, config.Agents)
	write(t, filepath.Join(agents, "a/SKILL.md"), "1\n\n2\n\n3\n\n4\n\n5 agents\n", 0o644)
	saved := f.mustSave(t)
	if len(saved) != 2 || saved[0].Target != config.Claude || saved[1].Target != config.Agents {
		t.Fatalf("saved %+v", saved)
	}
	if got, err := os.ReadFile(filepath.Join(f.vault, "g/a/SKILL.md")); err != nil || string(got) != "1 claude\n\n2\n\n3\n\n4\n\n5 agents\n" {
		t.Errorf("vault: %q, %v", got, err)
	}
	if n := gitOut(t, f.vault, "rev-list", "--count", "HEAD"); n != "4" {
		t.Errorf("%s commits, want one per copy", n)
	}
}

// skills.md "When the vault changed too": same lines stop with the merge left in
// the vault; what was saved before stays saved.
func TestSaveBackConflict(t *testing.T) {
	f := newSaveFixture(t)
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A here\n", 0o644)
	write(t, filepath.Join(f.skills(), "b/SKILL.md"), "B here\n", 0o644)
	write(t, filepath.Join(f.vault, "g/b/SKILL.md"), "B there\n", 0o644)
	gitOut(t, f.vault, "commit", "--quiet", "-am", "elsewhere")
	saved, err := f.save(t)
	ce, ok := errors.AsType[*vault.ConflictError](err)
	if !ok || !slices.Equal(ce.Paths, []string{"g/b/SKILL.md"}) {
		t.Fatalf("err = %v", err)
	}
	if len(saved) != 1 || saved[0].Name != "a" {
		t.Errorf("saved %+v", saved)
	}
	// b's record stays as it was, for skl sync --continue to settle.
	if m := readManifestFile(t, filepath.Join(f.skills(), "b")); m.Commit == f.head(t) {
		t.Error("b's record moved on")
	}
}

func readManifestFile(t *testing.T, folder string) Manifest {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(folder))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == filepath.Base(folder) {
			m, _ := readManifest(e, folder)
			return m
		}
	}
	t.Fatalf("no folder %s", folder)
	return Manifest{}
}
