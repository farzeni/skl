package load

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/farzeni/skl/internal/config"
)

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// docs/guide/skills.md "Add a skill you made": skl status lists the skills skl
// didn't create; a skill's workspace and hidden folders aren't skills.
func TestForeigners(t *testing.T) {
	home := t.TempDir()
	claude := SkillsDir(home, config.Claude)
	writeSkill(t, filepath.Join(claude, "b-skill"), "b-named")
	writeSkill(t, filepath.Join(claude, "a-skill"), "")
	writeSkill(t, filepath.Join(claude, ".hidden"), "hidden")
	if err := os.MkdirAll(filepath.Join(claude, "a-skill-workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(claude, "loaded"), "loaded")
	if err := writeManifest(filepath.Join(claude, "loaded", manifestFile), Manifest{Source: "g/loaded"}); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(SkillsDir(home, config.Agents), "c-skill"), "")

	got, err := Foreigners(home, []config.Target{config.Claude, config.Agents})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range got {
		names = append(names, string(f.Target)+":"+f.Folder+":"+f.Name)
	}
	want := []string{"claude:a-skill:", "claude:b-skill:b-named", "agents:c-skill:"}
	if len(names) != len(want) {
		t.Fatalf("got %q, want %q", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %q, want %q", names, want)
		}
	}
}

// Two skills share a name when a folder or frontmatter name of one is a folder or
// frontmatter name of the other, in any case.
func TestForeignClashes(t *testing.T) {
	for _, tt := range []struct {
		a, b Foreign
		want bool
	}{
		{Foreign{Folder: "x", Name: "dup"}, Foreign{Folder: "y", Name: "DUP"}, true},
		{Foreign{Folder: "x", Name: "y"}, Foreign{Folder: "Y"}, true},
		{Foreign{Folder: "x"}, Foreign{Folder: "y", Name: "x"}, true},
		{Foreign{Folder: "pdf"}, Foreign{Folder: "PDF"}, true},
		{Foreign{Folder: "x"}, Foreign{Folder: "y"}, false},
		{Foreign{Folder: "x", Name: "a"}, Foreign{Folder: "y", Name: "b"}, false},
	} {
		if got := tt.a.Clashes(tt.b); got != tt.want || tt.b.Clashes(tt.a) != tt.want {
			t.Errorf("%+v and %+v: clashes %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// docs/design/architecture.md "Import": "If the folder is a link, as npx skills may
// leave it, skl replaces the link with a real copy and says so, so .skl-copy.toml is
// never written into another tool's folder."
func TestAdoptReplacesALink(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	theirs := filepath.Join(home, ".agents", "skills", "pdf")
	writeSkill(t, theirs, "pdf")
	claude := SkillsDir(home, config.Claude)
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(claude, "pdf")
	if err := os.Symlink(theirs, link); err != nil {
		t.Fatal(err)
	}
	found, err := Foreigners(home, []config.Target{config.Claude})
	if err != nil || len(found) != 1 || !found[0].Link {
		t.Fatalf("found %+v, %v", found, err)
	}
	staging := filepath.Join(home, ".skl", "staging")
	m := Manifest{Source: "you/pdf", Commit: "c0ffee", Tree: "t"}
	ad, err := Adopt(ctx, found[0], staging, m)
	if err != nil {
		t.Fatal(err)
	}
	if !ad.Replaced() || ad.Path() != link {
		t.Errorf("Replaced %v, Path %q", ad.Replaced(), ad.Path())
	}
	info, err := os.Lstat(link)
	if err != nil || !info.IsDir() {
		t.Fatalf("%s is not a real folder: %v %v", link, info, err)
	}
	if _, err := os.Stat(filepath.Join(link, manifestFile)); err != nil {
		t.Error("no .skl-copy.toml in the copy")
	}
	if _, err := os.Stat(filepath.Join(theirs, manifestFile)); err == nil {
		t.Error(".skl-copy.toml written into the folder the link pointed at")
	}

	// Undo puts the link back, untouched.
	ad.Undo()
	info, err = os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link isn't back: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(theirs, manifestFile)); err == nil {
		t.Error("their folder holds a .skl-copy.toml")
	}
}

// A folder that's adopted keeps its files; Close lets the replaced link go.
func TestAdoptAFolderAndClose(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	dir := filepath.Join(SkillsDir(home, config.Claude), "notes")
	writeSkill(t, dir, "notes")
	found, err := Foreigners(home, []config.Target{config.Claude})
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, %v", found, err)
	}
	staging := filepath.Join(home, ".skl", "staging")
	tree, err := SkillTree(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	ad, err := Adopt(ctx, found[0], staging, Manifest{Source: "you/notes", Commit: "c", Tree: tree})
	if err != nil {
		t.Fatal(err)
	}
	ad.Close()
	if ad.Replaced() {
		t.Error("a folder isn't a link")
	}
	// It's now skl's copy, current with the tree it records.
	got, err := Tree(ctx, dir)
	if err != nil || got != tree {
		t.Errorf("tree %q, recorded %q, %v", got, tree, err)
	}
	after, err := Foreigners(home, []config.Target{config.Claude})
	if err != nil || len(after) != 0 {
		t.Errorf("still someone else's: %+v %v", after, err)
	}
}
