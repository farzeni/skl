package load

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/state"
)

func (f *fixture) inspect(t *testing.T, current []state.Copy, skills ...resolve.Skill) *Inspection {
	t.Helper()
	got, err := Inspect(t.Context(), Input{
		Result: result(t, skills...), Current: current, Targets: []config.Target{config.Claude}, HomeRoot: f.home,
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// getting-started.md: "changed means your edits to pr-review aren't in the vault
// yet", and undoing the edit undoes it.
func TestInspectChanged(t *testing.T) {
	f := newFixture(t)
	skill := f.a
	current := f.load(t, skill)
	file := filepath.Join(f.skills(), "a", "SKILL.md")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	got := f.inspect(t, current, skill)
	want := []Loaded{{Name: "a", Skill: "g/a"}}
	if !slices.Equal(got.Loaded, want) {
		t.Fatalf("Loaded = %+v, want %+v", got.Loaded, want)
	}

	write(t, file, "edited\n", 0o644)
	want[0].Changed = true
	if got := f.inspect(t, current, skill); !slices.Equal(got.Loaded, want) {
		t.Errorf("edited: Loaded = %+v", got.Loaded)
	}

	write(t, file, string(before), 0o644)
	want[0].Changed = false
	if got := f.inspect(t, current, skill); !slices.Equal(got.Loaded, want) {
		t.Errorf("edit undone: Loaded = %+v", got.Loaded)
	}
}

func TestInspectGoneOnlyIfStillWanted(t *testing.T) {
	f := newFixture(t)
	skill := f.a
	current := f.load(t, skill)
	if err := os.RemoveAll(filepath.Join(f.skills(), "a")); err != nil {
		t.Fatal(err)
	}
	got := f.inspect(t, current, skill)
	if len(got.Loaded) != 0 || len(got.Gone) != 1 || got.Gone[0].Name != "a" {
		t.Errorf("wanted: %+v", got)
	}
	if got := f.inspect(t, current); len(got.Gone) != 0 {
		t.Errorf("no longer wanted: Gone = %+v", got.Gone)
	}
}

// skill-sets.md: "skl status keeps listing it as skipped until the name is free",
// once: a skipped skill's folder isn't repeated under "not in your vault".
func TestInspectForeign(t *testing.T) {
	f := newFixture(t)
	skills := f.skills()
	write(t, filepath.Join(skills, "a", "SKILL.md"), "---\nname: x\n---\n", 0o644)
	write(t, filepath.Join(skills, "mine", "SKILL.md"), "---\nname: mine\n---\n", 0o644)
	write(t, filepath.Join(skills, "mine-workspace", "notes.md"), "x", 0o644)
	write(t, filepath.Join(skills, ".hidden", "SKILL.md"), "x", 0o644)
	write(t, filepath.Join(skills, "README.md"), "x", 0o644)
	target := t.TempDir()
	write(t, filepath.Join(target, "SKILL.md"), "---\nname: linked\n---\n", 0o644)
	if err := os.Symlink(target, filepath.Join(skills, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(skills, "nowhere"), filepath.Join(skills, "dangling")); err != nil {
		t.Fatal(err)
	}

	got := f.inspect(t, nil, f.a)
	if !slices.Equal(got.Foreign, []string{"linked", "mine"}) {
		t.Errorf("Foreign = %q", got.Foreign)
	}
	if len(got.Skipped) != 1 || filepath.Base(got.Skipped[0].Path) != "a" {
		t.Errorf("Skipped = %+v", got.Skipped)
	}
	if !slices.Equal(got.Dirs, []string{skills}) {
		t.Errorf("Dirs = %q", got.Dirs)
	}
}

// What state.json lists comes first, in its order; a copy it doesn't list follows.
func TestInspectOrder(t *testing.T) {
	f := newFixture(t)
	a, b := f.a, f.b
	current := f.load(t, a, b)
	slices.Reverse(current)
	write(t, filepath.Join(f.skills(), "zzz", manifestFile), "source = \"old/zzz\"\n", 0o644)
	got := f.inspect(t, current, b, a)
	var names []string
	for _, l := range got.Loaded {
		names = append(names, l.Name)
	}
	if want := []string{current[0].Name, current[1].Name, "zzz"}; !slices.Equal(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
}
