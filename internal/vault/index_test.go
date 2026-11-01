package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
)

// tree writes files under a new temp folder, the vault's working tree. It needs
// no git: the index only reads folders.
func tree(t *testing.T, files map[string]string) *Vault {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Vault{dir: dir}
}

func index(t *testing.T, v *Vault) *Index {
	t.Helper()
	idx, err := v.Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func skills(idx *Index, group string) []string {
	var out []string
	for _, s := range idx.Skills(group) {
		out = append(out, s.String())
	}
	return out
}

func messages(idx *Index) []string {
	var out []string
	for _, w := range idx.Warnings() {
		out = append(out, w.Message())
	}
	return out
}

const plain = "---\ndescription: d\n---\n"

// files.md "Your vault": "Every other top-level folder is a group".
func TestIndexGroups(t *testing.T) {
	v := tree(t, map[string]string{
		"review/pr-review/SKILL.md":  plain,
		"data/sql/SKILL.md":          plain,
		"empty/notes.txt":            "a folder with no skill is still a group",
		"LICENSE":                    "a file is no group",
		".github/workflows/ci.yml":   "a hidden folder is no group",
		".hidden/pr-review/SKILL.md": plain,
		"sets/backend.toml":          "select = []\n",
		"lessons/review/1.md":        "x",
		"h/x/SKILL.md":               plain,
		"Sets/x/SKILL.md":            plain,
		"LESSONS/x/SKILL.md":         plain,
		"H/y/SKILL.md":               plain,
	})
	if err := os.Mkdir(filepath.Join(v.dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	idx := index(t, v)
	if got, want := idx.Groups(), []string{"data", "empty", "review"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Groups() = %q, want %q", got, want)
	}
	if got := idx.Skills("empty"); len(got) != 0 {
		t.Errorf("Skills(empty) = %v", got)
	}
	if got := idx.Skills("missing"); len(got) != 0 {
		t.Errorf("Skills(missing) = %v", got)
	}
	if got := idx.Skills("sets"); len(got) != 0 {
		t.Errorf("Skills(sets) = %v", got)
	}
	if got := idx.Warnings(); len(got) != 0 {
		t.Errorf("Warnings() = %v", got)
	}
}

// files.md "Your vault": "every folder in a group that holds a SKILL.md is a
// skill. A SKILL.md at any other depth is ignored."
func TestIndexSkills(t *testing.T) {
	v := tree(t, map[string]string{
		"SKILL.md":                     plain, // the vault's root
		"review/SKILL.md":              plain, // a group's own folder
		"review/zeta/SKILL.md":         plain,
		"review/alpha/SKILL.md":        plain,
		"review/alpha/nested/SKILL.md": plain,
		"review/no-skill/README.md":    "no SKILL.md",
		"review/lower/skill.md":        "another name",
		"review/a-file":                "a file is no skill",
		"review/deep/er/SKILL.md":      plain,
	})
	// A folder called SKILL.md holds no SKILL.md, and neither does a dangling link.
	if err := os.MkdirAll(filepath.Join(v.dir, "review/dir-named/SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(v.dir, "review/dangling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(v.dir, "review/dangling/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	got := skills(index(t, v), "review")
	if runtime.GOOS == "darwin" {
		// The default macOS disk is case-insensitive: skill.md is SKILL.md there.
		got = slices.DeleteFunc(got, func(s string) bool { return s == "review/lower" })
	}
	if want := []string{"review/alpha", "review/zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Skills(review) = %q, want %q", got, want)
	}
}

// skill-sets.md "Names and clashes": "A skill's names are its folder name and
// the name in its SKILL.md frontmatter." The design: "a SKILL.md without valid
// frontmatter is still a skill named by its folder, with a warning".
func TestIndexFrontmatter(t *testing.T) {
	tests := []struct {
		name, content string
		skillName     string
		description   string
		warning       string // part of the warning; none if empty
	}{
		{name: "name and description", content: "---\nname: pretty\ndescription: Does things\n---\nbody\n", skillName: "pretty", description: "Does things"},
		{name: "CRLF", content: "---\r\nname: pretty\r\ndescription: Does things\r\n---\r\nbody\r\n", skillName: "pretty", description: "Does things"},
		{name: "byte order mark", content: "\ufeff---\nname: pretty\n---\n", skillName: "pretty"},
		{name: "nothing after the closing line", content: "---\nname: pretty\n---", skillName: "pretty"},
		{name: "spaces after a marker", content: "--- \nname: pretty\n---  \n", skillName: "pretty"},
		{name: "other fields", content: "---\nname: pretty\ncontext: fork\nhooks: [a, b]\n---\n", skillName: "pretty"},
		{name: "quoted", content: "---\nname: \"pretty\"\n---\n", skillName: "pretty"},
		{name: "a number is the text it's written as", content: "---\nname: 007\n---\n", skillName: "007"},
		{name: "folded description", content: "---\ndescription: >\n  one\n  two\n---\n", description: "one two\n"},
		{name: "no name", content: plain, description: "d"},
		{name: "empty frontmatter", content: "---\n---\nbody\n"},
		{name: "null name", content: "---\nname:\n---\n"},
		{name: "a marker in the body is the body", content: "---\nname: pretty\n---\n---\nname: other\n---\n", skillName: "pretty"},
		{name: "a dashed line inside a value", content: "---\nname: pretty\ndescription: |\n  ---\n  more\n---\n", skillName: "pretty", description: "---\nmore\n"},

		{name: "no frontmatter", content: "# A skill\n\nname: not-this\n", warning: "no frontmatter"},
		{name: "empty file", content: "", warning: "no frontmatter"},
		{name: "a rule isn't frontmatter", content: "----\nname: x\n----\n", warning: "no frontmatter"},
		{name: "starts on a later line", content: "\n---\nname: x\n---\n", warning: "no frontmatter"},
		{name: "not closed", content: "---\nname: x\nbody\n", warning: "frontmatter isn't closed"},
		{name: "a longer rule doesn't close it", content: "---\nname: x\n----\n", warning: "frontmatter isn't closed"},
		{name: "broken YAML", content: "---\nname: x\ndescription: Use when: y\n---\n", warning: "invalid frontmatter"},
		{name: "not a mapping", content: "---\n- a\n- b\n---\n", warning: "invalid frontmatter"},
		{name: "a list for a name", content: "---\nname: [a, b]\n---\n", warning: "invalid frontmatter"},
		{name: "duplicate keys", content: "---\nname: a\nname: b\n---\n", warning: "invalid frontmatter"},
		{name: "not UTF-8", content: "---\nname: caf\xe9\n---\n", warning: "invalid frontmatter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := index(t, tree(t, map[string]string{"g/folder/SKILL.md": tt.content}))
			want := resolve.Skill{Group: "g", Folder: "folder", Name: tt.skillName}
			if got := idx.Skills("g"); !reflect.DeepEqual(got, []resolve.Skill{want}) {
				t.Fatalf("Skills(g) = %+v, want %+v", got, want)
			}
			if got := idx.Description(want); got != tt.description {
				t.Errorf("Description = %q, want %q", got, tt.description)
			}
			got := messages(idx)
			if tt.warning == "" {
				if len(got) != 0 {
					t.Errorf("warnings = %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.HasPrefix(got[0], "g/folder/SKILL.md   ") || !strings.Contains(got[0], tt.warning) || strings.Contains(got[0], "\n") {
				t.Errorf("warnings = %q, want one for g/folder/SKILL.md with %q", got, tt.warning)
			}
		})
	}
}

// A SKILL.md that can't be read is still a skill named by its folder.
func TestIndexUnreadableSkill(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	v := tree(t, map[string]string{"g/locked/SKILL.md": "---\nname: x\n---\n"})
	p := filepath.Join(v.dir, "g/locked/SKILL.md")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	idx := index(t, v)
	if got := idx.Skills("g"); !reflect.DeepEqual(got, []resolve.Skill{{Group: "g", Folder: "locked"}}) {
		t.Errorf("Skills(g) = %+v", got)
	}
	if got, want := messages(idx), []string{"g/locked/SKILL.md   can't read it: permission denied"}; !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

// A FIFO can't exist in git, but a read of one would hang skl forever.
func TestIndexSkillMdNotAFile(t *testing.T) {
	v := tree(t, map[string]string{"g/real/SKILL.md": plain, "sets/x": "", "g/pipe/keep": ""})
	if err := syscall.Mkfifo(filepath.Join(v.dir, "g/pipe/SKILL.md"), 0o644); err != nil {
		t.Skip(err)
	}
	if err := syscall.Mkfifo(filepath.Join(v.dir, "sets/pipe.toml"), 0o644); err != nil {
		t.Skip(err)
	}
	idx := index(t, v)
	if got, want := skills(idx, "g"), []string{"g/real"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Skills(g) = %q, want %q", got, want)
	}
	var setErr *SetError
	if _, err := idx.Set("pipe"); !errors.As(err, &setErr) || setErr.File != "sets/pipe.toml" {
		t.Errorf("Set(pipe) = %v, want an error naming the file", err)
	}
}

// A skill found by its frontmatter name through the real resolver: skill-sets.md
// and repos.md "A skill is found by its folder name or its frontmatter name".
func TestIndexResolves(t *testing.T) {
	idx := index(t, tree(t, map[string]string{
		"review/pr-review/SKILL.md": "---\nname: pretty\n---\n",
		"review/other/SKILL.md":     plain,
		"data/café/SKILL.md":        plain,
		"sets/base.toml":            "select = [\n  \"review\",\n  \"-review/other\",\n]\n",
		"a/x/SKILL.md":              plain,
		"b/y/SKILL.md":              "---\nname: x\n---\n",
	}))
	for words, want := range map[string][]string{
		"review/pretty":    {"review/pr-review"},
		"review/PRETTY":    {"review/pr-review"},
		"review/pr-review": {"review/pr-review"},
		"@base":            {"review/pr-review"},
		"data/café":       {"data/café"},
		"a/x b/y":          {"b/y"}, // "`a/x` and `b/y` with `name: x` clash"
		"data/nope":        nil,
		"@nope":            nil,
	} {
		var sels []selector.Selector
		for w := range strings.FieldsSeq(words) {
			sel, err := selector.Parse(w)
			if err != nil {
				t.Fatal(err)
			}
			sels = append(sels, sel)
		}
		res, err := resolve.Resolve(sels, idx)
		if err != nil {
			t.Errorf("Resolve(%q): %v", words, err)
			continue
		}
		var got []string
		for _, s := range res.Skills {
			got = append(got, s.String())
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Resolve(%q) = %q, want %q", words, got, want)
		}
	}
}

// Folder names that aren't valid UTF-8 can't be typed, printed or compared, so
// they're skipped with a warning. Linux keeps such a name; macOS refuses it.
func TestIndexInvalidUTF8(t *testing.T) {
	v := tree(t, map[string]string{"ok/a/SKILL.md": plain})
	for _, p := range []string{"b\xffd/s/SKILL.md", "ok/b\xffd/SKILL.md", "ok/b\xff-none/README.md", "sets/b\xffd.toml", "sets/b\xffd.origin.toml"} {
		full := filepath.Join(v.dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Skipf("this disk keeps no such name: %v", err)
		}
		if err := os.WriteFile(full, []byte(plain), 0o644); err != nil {
			t.Skipf("this disk keeps no such name: %v", err)
		}
	}
	idx := index(t, v)
	if got, want := idx.Groups(), []string{"ok"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Groups() = %q, want %q", got, want)
	}
	if got, want := skills(idx, "ok"), []string{"ok/a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Skills(ok) = %q, want %q", got, want)
	}
	if got := idx.Sets(); len(got) != 0 {
		t.Errorf("Sets() = %q", got)
	}
	want := []string{
		"b�d   skipped: the name isn't valid UTF-8",
		"ok/b�d   skipped: the name isn't valid UTF-8",
		"sets/b�d.toml   skipped: the name isn't valid UTF-8",
	}
	if got := messages(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

// files.md "sets/<name>.toml": "A named skill set holds selectors". "skl never
// reads a file ending in .origin.toml as a skill set."
func TestIndexSets(t *testing.T) {
	idx := index(t, tree(t, map[string]string{
		"sets/backend.toml":        "select = [\n  \"@base\",\n  \"review\",\n  \"-review/test-gaps\",\n  \"someone/pdf\",\n]\n",
		"sets/backend.origin.toml": "[source]\nurl = \"u\"\n",
		"sets/lone.origin.toml":    "select = [\"never\"]\n",
		"sets/empty.toml":          "select = []\n",
		"sets/extra.toml":          "select = [\"a\"]\nnote = \"another key is left alone\"\n",
		"sets/origin.toml":         "select = [\"o\"]\n",
		"sets/.toml":               "select = [\"no name\"]\n",
		"sets/README.md":           "not a set",
		"sets/sub/deep.toml":       "select = [\"not at this depth\"]\n",
		"sets/folder.toml/x":       "a folder isn't a set",
	}))
	if got, want := idx.Sets(), []string{"backend", "empty", "extra", "origin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Sets() = %q, want %q", got, want)
	}
	lines, err := idx.Set("backend")
	if want := []string{"@base", "review", "-review/test-gaps", "someone/pdf"}; err != nil || !reflect.DeepEqual(lines, want) {
		t.Errorf("Set(backend) = %q, %v, want %q", lines, err, want)
	}
	if lines, err := idx.Set("empty"); err != nil || len(lines) != 0 {
		t.Errorf("Set(empty) = %q, %v", lines, err)
	}
	for _, name := range []string{"missing", "", "lone", "lone.origin", "backend.origin", "README", "README.md", "sub", "sub/deep", "folder", "../sets/backend", "backend.toml"} {
		if lines, err := idx.Set(name); !errors.Is(err, resolve.ErrNoSet) {
			t.Errorf("Set(%q) = %q, %v, want ErrNoSet", name, lines, err)
		}
	}
}

// A vault without a sets folder has no sets.
func TestIndexNoSets(t *testing.T) {
	idx := index(t, tree(t, map[string]string{"g/s/SKILL.md": plain}))
	if got := idx.Sets(); len(got) != 0 {
		t.Errorf("Sets() = %q", got)
	}
	if _, err := idx.Set("x"); !errors.Is(err, resolve.ErrNoSet) {
		t.Errorf("Set(x) = %v", err)
	}
}

// The design: "A broken set file is an error naming the file."
func TestIndexBrokenSet(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":          "select = [\n",
		"string":          "select = \"review\"\n",
		"numbers":         "select = [1, 2]\n",
		"mixed":           "select = [\"a\", 2]\n",
		"table":           "[select]\na = 1\n",
		"no select":       "selct = [\"typo\"]\n",
		"an empty file":   "",
		"not UTF-8":       "select = [\"caf\xe9\"]\n",
		"null byte":       "select = [\"a\x00\"]\n",
		"select in table": "[x]\nselect = [\"a\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			idx := index(t, tree(t, map[string]string{"sets/bad.toml": content, "sets/good.toml": "select = [\"a\"]\n"}))
			if got, want := idx.Sets(), []string{"bad", "good"}; !reflect.DeepEqual(got, want) {
				t.Errorf("Sets() = %q, want %q", got, want)
			}
			lines, err := idx.Set("bad")
			if err == nil {
				t.Fatalf("Set(bad) = %q, want an error", lines)
			}
			if errors.Is(err, resolve.ErrNoSet) {
				t.Errorf("Set(bad) = %v: a broken set isn't a missing one", err)
			}
			var setErr *SetError
			if !errors.As(err, &setErr) || setErr.File != "sets/bad.toml" || !strings.HasPrefix(err.Error(), "sets/bad.toml: ") {
				t.Errorf("Set(bad) = %v, want the file's name", err)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Errorf("Set(bad) = %q: more than one line after ✗", err)
			}
			if _, err := idx.Set("good"); err != nil {
				t.Errorf("Set(good) = %v: one broken set stops only that set", err)
			}
		})
	}
}

// repos.md "Grammar": "résumé works whether é is one character or e plus an
// accent", in a set's name too, and skill-sets.md "Names that differ only by case
// or Unicode form are the same name".
func TestIndexSetNames(t *testing.T) {
	idx := index(t, tree(t, map[string]string{
		"sets/Backend.toml": "select = [\"a\"]\n",
		"sets/résumé.toml":  "select = [\"b\"]\n",
		"sets/café.toml":   "select = [\"c\"]\n",
	}))
	for name, want := range map[string]string{
		"Backend":  "a",
		"backend":  "a",
		"BACKEND":  "a",
		"résumé":   "b",
		"résumé": "b",
		"café":     "c",
		"café":    "c",
	} {
		lines, err := idx.Set(name)
		if err != nil || !reflect.DeepEqual(lines, []string{want}) {
			t.Errorf("Set(%q) = %q, %v, want %q", name, lines, err, want)
		}
	}
}

// As with two skills: an exact name is one set, and anything else that matches
// two stops and lists both.
func TestIndexSetClash(t *testing.T) {
	v := tree(t, map[string]string{"sets/Dup.toml": "select = [\"upper\"]\n"})
	if err := os.WriteFile(filepath.Join(v.dir, "sets/dup.toml"), []byte("select = [\"lower\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(v.dir, "sets")); len(entries) != 2 {
		t.Skip("this disk keeps one folder entry for both names")
	}
	idx := index(t, v)
	for name, want := range map[string]string{"Dup": "upper", "dup": "lower"} {
		if lines, err := idx.Set(name); err != nil || !reflect.DeepEqual(lines, []string{want}) {
			t.Errorf("Set(%q) = %q, %v, want %q", name, lines, err, want)
		}
	}
	_, err := idx.Set("DUP")
	var amb *AmbiguousSetError
	if !errors.As(err, &amb) || !reflect.DeepEqual(amb.Sets, []string{"Dup", "dup"}) {
		t.Fatalf("Set(DUP) = %v, want both", err)
	}
	if want := "@DUP matches 2 skill sets: @Dup, @dup"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
}

func TestIndexErrors(t *testing.T) {
	v := &Vault{dir: filepath.Join(t.TempDir(), "gone")}
	if _, err := v.Index(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a vault that's gone: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v = tree(t, map[string]string{"g/s/SKILL.md": plain})
	if _, err := v.Index(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}

	v = tree(t, map[string]string{"g/s/SKILL.md": plain, "sets": "a file where the folder goes"})
	if _, err := v.Index(context.Background()); err == nil || !strings.Contains(err.Error(), "sets") {
		t.Errorf("a file named sets: %v", err)
	}
}

// A change to one result doesn't reach the next.
func TestIndexSetReadsTheFile(t *testing.T) {
	v := tree(t, map[string]string{"sets/s.toml": "select = [\"a\"]\n"})
	idx := index(t, v)
	first, err := idx.Set("s")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = "changed"
	again, err := idx.Set("s")
	if err != nil || !reflect.DeepEqual(again, []string{"a"}) {
		t.Errorf("Set(s) = %q, %v", again, err)
	}
}
