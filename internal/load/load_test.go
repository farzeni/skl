package load

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

// writeTree is what git says the folder's tree is.
func writeTree(t *testing.T, dir string) string {
	t.Helper()
	gitOut(t, dir, "init", "--quiet")
	gitOut(t, dir, "-c", "core.autocrlf=false", "add", "--all")
	return gitOut(t, dir, "write-tree")
}

// docs/reference/files.md "Loaded skills": "git tree hash of the folder". The card:
// "must equal git write-tree for the same files, modes and bytes".
func TestTreeEqualsGitWriteTree(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, dir string)
	}{
		{"one file", func(t *testing.T, dir string) { write(t, filepath.Join(dir, "SKILL.md"), "hi\n", 0o644) }},
		{"empty file", func(t *testing.T, dir string) { write(t, filepath.Join(dir, "empty"), "", 0o644) }},
		{"executable bit", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n", 0o755)
			write(t, filepath.Join(dir, "plain.sh"), "#!/bin/sh\n", 0o644)
		}},
		{"group and other bits don't count", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "a"), "a", 0o664)
			write(t, filepath.Join(dir, "b"), "b", 0o654)
		}},
		{"CRLF", func(t *testing.T, dir string) { write(t, filepath.Join(dir, "crlf.md"), "one\r\ntwo\r\n", 0o644) }},
		{"nested folders", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "a/b/c/deep.md"), "deep", 0o644)
			write(t, filepath.Join(dir, "a/top.md"), "top", 0o644)
		}},
		{"a folder sorts as if it ended in a slash", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "a/x"), "x", 0o644)
			write(t, filepath.Join(dir, "a.md"), "dot", 0o644)
			write(t, filepath.Join(dir, "a-b"), "dash", 0o644)
			write(t, filepath.Join(dir, "a0"), "zero", 0o644)
		}},
		{"empty folders aren't kept", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "f"), "f", 0o644)
			if err := os.MkdirAll(filepath.Join(dir, "none/deeper"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"a link", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "f"), "f", 0o644)
			if err := os.Symlink("../outside/nowhere", filepath.Join(dir, "link")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a non-ASCII name", func(t *testing.T, dir string) { write(t, filepath.Join(dir, "résumé.md"), "é", 0o644) }},
		{"bytes that aren't text", func(t *testing.T, dir string) { write(t, filepath.Join(dir, "bin"), "\x00\xff\x00", 0o644) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.build(t, dir)
			got, err := Tree(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if want := writeTree(t, dir); got != want {
				t.Errorf("Tree = %s, git write-tree = %s", got, want)
			}
		})
	}
}

// "`.skl-copy.toml` itself is never saved back", so it never counts as a change.
func TestTreeLeavesOutTheManifestAndGit(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "SKILL.md"), "hi", 0o644)
	before, err := Tree(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, manifestFile), "source = 'x'\n", 0o644)
	write(t, filepath.Join(dir, "sub/.git/HEAD"), "ref", 0o644)
	after, err := Tree(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("tree changed: %s, then %s", before, after)
	}
	// Only the top one is skl's: one further down is the skill's content.
	write(t, filepath.Join(dir, "sub", manifestFile), "x", 0o644)
	if deeper, _ := Tree(context.Background(), dir); deeper == before {
		t.Error("a nested .skl-copy.toml should count")
	}
}

// A copy holds what git would check out, so its hash is the vault folder's.
func TestCopyHashesTheSame(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "SKILL.md"), "hi\r\n", 0o644)
	write(t, filepath.Join(src, "bin/run"), "#!/bin/sh\n", 0o750)
	write(t, filepath.Join(src, originFile), "x", 0o644)
	if err := os.MkdirAll(filepath.Join(src, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyFolder(ctx, src, dst, 0, []string{manifestFile, originFile}); err != nil {
		t.Fatal(err)
	}
	want, err := treeOf(ctx, src, manifestFile, originFile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Tree(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("copy %s, source %s", got, want)
	}
	if _, err := os.Lstat(filepath.Join(dst, originFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s was copied", originFile)
	}
	info, err := os.Lstat(filepath.Join(dst, "link"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link: %v %v", info, err)
	}
}

// fixture is a vault with two skills, loaded into one target.
type fixture struct {
	vault, home, staging string
	targets              []config.Target
	a, b                 resolve.Skill
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		vault:   filepath.Join(root, "vault"),
		home:    filepath.Join(root, "home"),
		staging: filepath.Join(root, "staging"),
		targets: []config.Target{config.Claude},
		a:       resolve.Skill{Group: "g", Folder: "a"},
		b:       resolve.Skill{Group: "g", Folder: "b"},
	}
	write(t, filepath.Join(f.vault, "g/a/SKILL.md"), "A\n", 0o644)
	write(t, filepath.Join(f.vault, "g/b/SKILL.md"), "B\n", 0o644)
	return f
}

// skillList is an index that holds just these skills.
type skillList []resolve.Skill

func (l skillList) Groups() []string {
	var groups []string
	for _, s := range l {
		if !slices.Contains(groups, s.Group) {
			groups = append(groups, s.Group)
		}
	}
	return groups
}

func (l skillList) Skills(group string) []resolve.Skill {
	var skills []resolve.Skill
	for _, s := range l {
		if s.Group == group {
			skills = append(skills, s)
		}
	}
	return skills
}

func (l skillList) Set(string) ([]string, error) { return nil, resolve.ErrNoSet }

// result resolves one selector per skill, in order.
func result(t *testing.T, skills ...resolve.Skill) resolve.Result {
	t.Helper()
	sels := make([]selector.Selector, len(skills))
	for i, s := range skills {
		sel, err := selector.Parse(s.String())
		if err != nil {
			t.Fatal(err)
		}
		sels[i] = sel
	}
	res, err := resolve.Resolve(sels, skillList(skills))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f *fixture) plan(t *testing.T, current []state.Copy, skills ...resolve.Skill) *Plan {
	t.Helper()
	p, err := Compute(context.Background(), Input{
		Vault: f.vault, Commit: "c0", Result: result(t, skills...),
		Current: current, Targets: f.targets, HomeRoot: f.home,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fixture) skills() string { return SkillsDir(f.home, config.Claude) }

func (f *fixture) loaded(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(f.skills())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// load applies skills from scratch and returns what's recorded.
func (f *fixture) load(t *testing.T, skills ...resolve.Skill) []state.Copy {
	t.Helper()
	p := f.plan(t, nil, skills...)
	s, err := NewLoader(f.staging).Apply(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return p.Copies
}

func TestApplyLoadsAndCleansStaging(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a, f.b)
	if got := f.loaded(t); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("loaded = %v", got)
	}
	data, err := os.ReadFile(filepath.Join(f.skills(), "a", manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`source = "g/a"`, `commit = "c0"`, `tree = "` + copies[0].Tree + `"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("manifest %q lacks %q", data, want)
		}
	}
	got, err := Tree(context.Background(), filepath.Join(f.skills(), "a"))
	if err != nil || got != copies[0].Tree {
		t.Errorf("tree on disk %s (%v), recorded %s", got, err, copies[0].Tree)
	}
	if _, err := os.Stat(filepath.Join(f.staging, "load")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staging left behind: %v", err)
	}
}

// Architecture "Loading pipeline": "If the plan is empty, nothing is applied".
func TestPlanOfWhatsThereIsEmpty(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a)
	p := f.plan(t, copies, f.a)
	if !p.Empty() || len(p.Changes) != 0 {
		t.Fatalf("plan = %+v", p)
	}
	if !slices.Equal(p.Copies, copies) {
		t.Errorf("copies = %+v, want %+v", p.Copies, copies)
	}
}

// The folders decide: with state.json gone, a copy that's there is still there.
func TestPlanReadsTheFoldersNotTheState(t *testing.T) {
	f := newFixture(t)
	f.load(t, f.a)
	if p := f.plan(t, nil, f.a); !p.Empty() {
		t.Errorf("plan = %+v", p)
	}
}

// Architecture "Loading pipeline": "If a step fails, the previous set stays in place."
func TestFailedSwapPutsEverythingBack(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a)
	before, err := os.ReadFile(filepath.Join(f.skills(), "a", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.vault, "g/a/SKILL.md"), "A2\n", 0o644)
	p := f.plan(t, copies, f.a, f.b)
	l := NewLoader(f.staging)
	boom := errors.New("boom")
	l.rename = func(from, to string) error {
		if filepath.Base(to) == "b" {
			return boom
		}
		return os.Rename(from, to)
	}
	if _, err := l.Apply(context.Background(), p); !errors.Is(err, boom) {
		t.Fatalf("Apply = %v", err)
	}
	if got := f.loaded(t); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("loaded = %v", got)
	}
	after, err := os.ReadFile(filepath.Join(f.skills(), "a", "SKILL.md"))
	if err != nil || string(after) != string(before) {
		t.Errorf("a = %q (%v), want %q", after, err, before)
	}
}

// And Undo does the same for a swap that finished, when no generation can be made.
func TestUndoAfterASwap(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a)
	s, err := NewLoader(f.staging).Apply(context.Background(), f.plan(t, copies, f.b))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.loaded(t); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("after the swap: %v", got)
	}
	s.Undo()
	s.Close()
	if got := f.loaded(t); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("after undo: %v", got)
	}
}

// commands.md "Exit codes": "Ctrl-C while skl is changing things lets it finish or
// undo the step it's in". Before the swap, nothing has changed.
func TestCtrlCWhileStagingChangesNothing(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewLoader(f.staging).Apply(ctx, f.plan(t, copies, f.b))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply = %v", err)
	}
	if got := f.loaded(t); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("loaded = %v", got)
	}
	if _, err := os.Stat(filepath.Join(f.staging, "load")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staging left behind: %v", err)
	}
}

// During the swap, Ctrl-C waits for its end.
func TestCtrlCDuringTheSwapFinishesIt(t *testing.T) {
	f := newFixture(t)
	copies := f.load(t, f.a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := NewLoader(f.staging)
	l.rename = func(from, to string) error {
		if filepath.Base(filepath.Dir(to)) == "old" { // the swap's first move
			cancel()
		}
		return os.Rename(from, to)
	}
	s, err := l.Apply(ctx, f.plan(t, copies, f.b))
	if err != nil {
		t.Fatalf("Apply = %v", err)
	}
	s.Close()
	if ctx.Err() == nil {
		t.Fatal("the test didn't cancel")
	}
	if got := f.loaded(t); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("loaded = %v", got)
	}
}

// A target can live on another disk than ~/.skl, where a rename can't reach. The
// swap still happens in one rename, from a hidden folder next to the skills.
func TestTargetOnAnotherDisk(t *testing.T) {
	f := newFixture(t)
	l := NewLoader(f.staging)
	l.rename = func(from, to string) error {
		if strings.HasPrefix(from, f.staging) != strings.HasPrefix(to, f.staging) {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
		}
		return os.Rename(from, to)
	}
	s, err := l.Apply(context.Background(), f.plan(t, nil, f.a))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.skills(), ".skl-staging")); err != nil {
		t.Errorf("no staging next to the skills: %v", err)
	}
	s.Close()
	if got := f.loaded(t); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("loaded = %v", got)
	}
}

// files.md "Loaded skills": "A folder without it ... is a skill skl didn't create."
func TestWhatIsSomeoneElses(t *testing.T) {
	f := newFixture(t)
	dir := f.skills()
	write(t, filepath.Join(dir, "a/SKILL.md"), "mine", 0o644)                   // no manifest
	write(t, filepath.Join(dir, "mine/SKILL.md"), "---\nname: B\n---\n", 0o644) // holds b's name, in another case
	p := f.plan(t, nil, f.a, f.b)
	if !p.Empty() {
		t.Fatalf("steps = %+v", p.steps)
	}
	var folders []string
	for _, s := range p.Skipped {
		folders = append(folders, filepath.Base(s.Path))
	}
	if !slices.Equal(folders, []string{"a", "mine"}) {
		t.Errorf("skipped = %+v", p.Skipped)
	}
	if len(p.Copies) != 0 {
		t.Errorf("copies = %+v", p.Copies)
	}
}

// skill-sets.md: "A skill loses a clash only to a later skill that stays loaded".
func TestLoserLoadsWhereTheWinnerIsSkipped(t *testing.T) {
	f := newFixture(t)
	f.targets = []config.Target{config.Claude, config.Agents}
	f.b.Name = "a"
	write(t, filepath.Join(SkillsDir(f.home, config.Agents), "b/SKILL.md"), "mine", 0o644)
	p := f.plan(t, nil, f.a, f.b)

	var copies []string
	for _, c := range p.Copies {
		copies = append(copies, string(c.Target)+":"+c.Skill)
	}
	if want := []string{"claude:g/b", "agents:g/a"}; !slices.Equal(copies, want) {
		t.Errorf("copies = %v, want %v", copies, want)
	}
	if len(p.Skipped) != 1 || p.Skipped[0].Skill != f.b || filepath.Base(p.Skipped[0].Path) != "b" {
		t.Errorf("skipped = %+v", p.Skipped)
	}
}

func TestChangeLines(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.vault, "o/a/SKILL.md"), "other A\n", 0o644)
	other := resolve.Skill{Group: "o", Folder: "a"}
	copies := f.load(t, f.a, f.b)

	// A skill from somewhere else takes the name.
	p := f.plan(t, copies, other)
	want := []Change{
		{Kind: Replace, Name: "a", Skill: "o/a", Detail: "o/a replaces g/a"},
		{Kind: Unload, Name: "b", Skill: "g/b"},
	}
	if !slices.Equal(p.Changes, want) {
		t.Errorf("changes = %+v, want %+v", p.Changes, want)
	}

	// The same skill, changed in the vault.
	write(t, filepath.Join(f.vault, "g/a/SKILL.md"), "A2\n", 0o644)
	p = f.plan(t, copies, f.a, f.b)
	if want := []Change{{Kind: Replace, Name: "a", Skill: "g/a", Detail: "updated"}}; !slices.Equal(p.Changes, want) {
		t.Errorf("changes = %+v, want %+v", p.Changes, want)
	}
}

// One line per name, however many targets.
func TestTwoTargets(t *testing.T) {
	f := newFixture(t)
	f.targets = []config.Target{config.Claude, config.Agents}
	copies := f.load(t, f.a)
	if len(copies) != 2 || copies[0].Target != config.Claude || copies[1].Target != config.Agents {
		t.Fatalf("copies = %+v", copies)
	}
	for _, target := range f.targets {
		if _, err := os.Stat(filepath.Join(SkillsDir(f.home, target), "a", manifestFile)); err != nil {
			t.Error(err)
		}
	}
	p := f.plan(t, nil, f.b)
	if want := []Change{{Kind: Load, Name: "b", Skill: "g/b"}}; !slices.Equal(p.Changes, want) {
		t.Errorf("changes = %+v", p.Changes)
	}
	p = f.plan(t, copies, f.b)
	want := []Change{{Kind: Load, Name: "b", Skill: "g/b"}, {Kind: Unload, Name: "a", Skill: "g/a"}}
	if !slices.Equal(p.Changes, want) {
		t.Errorf("changes = %+v, want %+v", p.Changes, want)
	}
}

// commands.md "Dry run": init --dry-run plans as if each folder it would import
// were already its copy. Only the named folder in its own target is read so;
// someone else's skill with the same name in the other target is still skipped.
func TestSavedReadsAFolderToImportAsItsCopy(t *testing.T) {
	f := newFixture(t)
	f.targets = []config.Target{config.Claude, config.Agents}
	write(t, filepath.Join(f.skills(), "a/SKILL.md"), "A\n", 0o644) // the folder the vault's g/a came from
	write(t, filepath.Join(SkillsDir(f.home, config.Agents), "b/SKILL.md"), "mine", 0o644)
	tree, err := SkillTree(context.Background(), filepath.Join(f.vault, "g/a"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Compute(context.Background(), Input{
		Vault: f.vault, Result: result(t, f.a, f.b), Targets: f.targets, HomeRoot: f.home,
		Saved: []state.Copy{{Name: "a", Target: config.Claude, Skill: "g/a", Tree: tree}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Kind: Load, Name: "b", Skill: "g/b"}, {Kind: Load, Name: "a", Skill: "g/a"}}
	if !slices.Equal(p.Changes, want) {
		t.Errorf("changes = %+v, want %+v", p.Changes, want)
	}
	if len(p.Skipped) != 1 || p.Skipped[0].Skill != f.b || p.Skipped[0].Path != filepath.Join(SkillsDir(f.home, config.Agents), "b") {
		t.Errorf("skipped = %+v", p.Skipped)
	}
}
