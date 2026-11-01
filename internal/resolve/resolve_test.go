package resolve

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/selector"
)

// index is an in-memory Index: skills are "group/folder" or
// "group/folder=frontmatter-name". A set's lines are kept as written.
type index struct {
	skills []string
	sets   map[string][]string
	err    map[string]error // what Set returns instead of lines
}

func (x index) Groups() []string {
	var groups []string
	for _, s := range x.skills {
		g, _, _ := strings.Cut(s, "/")
		if len(groups) == 0 || groups[len(groups)-1] != g {
			groups = append(groups, g)
		}
	}
	return groups
}

func (x index) Skills(group string) []Skill {
	var skills []Skill
	for _, s := range x.skills {
		g, rest, _ := strings.Cut(s, "/")
		if g != group || rest == "" {
			continue
		}
		folder, name, _ := strings.Cut(rest, "=")
		skills = append(skills, Skill{Group: g, Folder: folder, Name: name})
	}
	return skills
}

func (x index) Set(name string) ([]string, error) {
	if err := x.err[name]; err != nil {
		return nil, err
	}
	lines, ok := x.sets[name]
	if !ok {
		return nil, ErrNoSet
	}
	return lines, nil
}

var vault = index{
	skills: []string{
		"data/sql-explain",
		"data/erd=entity-diagram",
		"review/pr-review",
		"review/test-gaps",
		"review/style=house-style",
		"someone/pdf",
		"someone/pr-review",
		"someone/lint=house-style",
		"writing/changelog",
		"empty/",
	},
	sets: map[string][]string{
		"base":     {"writing/changelog"},
		"backend":  {"@base", "review", "-review/test-gaps", "someone/pdf"},
		"diamond":  {"@left", "@right"},
		"left":     {"@bottom", "data/sql-explain"},
		"right":    {"@bottom"},
		"bottom":   {"writing/changelog", "review/old-name"},
		"drops":    {"-review/pr-review"},
		"clashing": {"review", "someone/pr-review"},
		"stale":    {"review/old-name", "@gone", "pdf", "nope", "-nope"},
		"outer":    {"@stale"},
		"self":     {"writing/changelog", "@self"},
		"cycle-a":  {"@cycle-b"},
		"cycle-b":  {"@cycle-c"},
		"cycle-c":  {"-@cycle-a"},
		"hasref":   {"someone/agent-skills/pdf"},
		"selfdrop": {"review", "-@selfdrop"},
	},
}

func parse(t *testing.T, words string) []selector.Selector {
	t.Helper()
	var sels []selector.Selector
	for w := range strings.FieldsSeq(words) {
		s, err := selector.Parse(w)
		if err != nil {
			t.Fatalf("Parse(%q): %v", w, err)
		}
		sels = append(sels, s)
	}
	return sels
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string // the doc sentence, or the rule it follows from
		words    string
		skills   []string
		replaced []string
		warnings []string
	}{
		// docs/reference/repos.md "Grammar"
		{
			name:   "One part is a group",
			words:  "review",
			skills: []string{"review/pr-review", "review/test-gaps", "review/style"},
		},
		{
			name:   "two parts are a skill in a group",
			words:  "review/test-gaps",
			skills: []string{"review/test-gaps"},
		},
		{
			name:   "@name is a named skill set in sets/",
			words:  "@base",
			skills: []string{"writing/changelog"},
		},
		{
			name:   "A skill is found by its folder name",
			words:  "data/erd",
			skills: []string{"data/erd"},
		},
		{
			name:   "A skill is found by its frontmatter name",
			words:  "data/entity-diagram",
			skills: []string{"data/erd"},
		},
		{
			name:   "- removes a skill from what the earlier selectors added: -review/test-gaps",
			words:  "review -review/test-gaps",
			skills: []string{"review/pr-review", "review/style"},
		},
		{
			name:   "- removes a skill found by its frontmatter name",
			words:  "data -data/entity-diagram",
			skills: []string{"data/sql-explain"},
		},
		{
			name:   "- removes a group: -review",
			words:  "review writing/changelog -review",
			skills: []string{"writing/changelog"},
		},
		{
			name:   "- removes a set: -@base",
			words:  "writing review/pr-review -@base",
			skills: []string{"review/pr-review"},
		},
		{
			name:   "- removes only from the earlier selectors: a later one adds it back",
			words:  "-review/test-gaps review",
			skills: []string{"review/pr-review", "review/test-gaps", "review/style"},
		},
		{
			name:  "- on a skill nothing added changes nothing and warns about nothing",
			words: "-review/test-gaps",
		},
		{
			name:   "-@set removes what the set adds, after its own removals",
			words:  "review -@backend",
			skills: []string{"review/test-gaps"},
		},
		{
			name:     "A word that names a skill but no group: the warning names the skill",
			words:    "pdf",
			warnings: []string{"pdf   matches no group → someone/pdf"},
		},
		{
			name:     "A word that names a skill by its frontmatter name: the hint uses its folder",
			words:    "entity-diagram",
			warnings: []string{"entity-diagram   matches no group → data/erd"},
		},
		{
			name:     "A word that names a skill in two groups: the hint picks the first",
			words:    "pr-review",
			warnings: []string{"pr-review   matches no group → review/pr-review"},
		},
		{
			name:     "A removal that names a skill but no group",
			words:    "-pdf",
			warnings: []string{"-pdf   matches no group → someone/pdf"},
		},

		// docs/guide/skill-sets.md "Selectors"
		{
			name:   "Selectors are applied left to right",
			words:  "writing data/erd review/pr-review",
			skills: []string{"writing/changelog", "data/erd", "review/pr-review"},
		},
		{
			name:   "The same skill twice keeps its place and is no clash",
			words:  "review/pr-review writing review",
			skills: []string{"review/pr-review", "writing/changelog", "review/test-gaps", "review/style"},
		},

		// docs/guide/skill-sets.md "Names and clashes"
		{
			name:     "The later selector wins, and skl says which skill replaced which",
			words:    "review someone/pr-review",
			skills:   []string{"review/test-gaps", "review/style", "someone/pr-review"},
			replaced: []string{"pr-review   someone/pr-review replaces review/pr-review"},
		},
		{
			name:     "Two skills clash when they have the same folder name",
			words:    "someone/pr-review review/pr-review",
			skills:   []string{"review/pr-review"},
			replaced: []string{"pr-review   review/pr-review replaces someone/pr-review"},
		},
		{
			name:     "Two skills clash when they have the same frontmatter name",
			words:    "review/style someone/lint",
			skills:   []string{"someone/lint"},
			replaced: []string{"lint   someone/lint replaces review/style"},
		},
		{
			name:   "Skills whose names don't overlap don't clash",
			words:  "review/pr-review data/sql-explain data/erd",
			skills: []string{"review/pr-review", "data/sql-explain", "data/erd"},
		},
		{
			name:     "A clash through a skill set is reported too",
			words:    "someone/pr-review @backend",
			skills:   []string{"writing/changelog", "review/pr-review", "review/style", "someone/pdf"},
			replaced: []string{"pr-review   review/pr-review replaces someone/pr-review"},
		},
		{
			name:   "A replaced skill that comes back is reported against the one it replaces now",
			words:  "review/pr-review someone/pr-review review/pr-review",
			skills: []string{"review/pr-review"},
			replaced: []string{
				"pr-review   review/pr-review replaces someone/pr-review",
			},
		},
		{
			name:   "A removal comes before clashes: the skill that would have lost stays",
			words:  "review/pr-review someone/pr-review -someone/pr-review",
			skills: []string{"review/pr-review"},
		},
		{
			name:   "A removal comes before clashes: @set -skill",
			words:  "@clashing -someone/pr-review",
			skills: []string{"review/pr-review", "review/test-gaps", "review/style"},
		},
		{
			name:   "-@set removes what the set selects, the skill that lost a clash in it too",
			words:  "review/pr-review writing -@clashing",
			skills: []string{"writing/changelog"},
		},
		{
			name:   "No replacement is reported for a replaced skill that was removed",
			words:  "review/pr-review someone/pr-review -review/pr-review",
			skills: []string{"someone/pr-review"},
		},

		// docs/guide/skill-sets.md "Named skill sets": a set can include other sets.
		{
			name:   "A skill set can include other skill sets",
			words:  "@backend",
			skills: []string{"writing/changelog", "review/pr-review", "review/style", "someone/pdf"},
		},
		{
			name:     "Two sets including the same one is no cycle",
			words:    "@diamond",
			skills:   []string{"writing/changelog", "data/sql-explain"},
			warnings: []string{"review/old-name   matches no skill (in @bottom)"},
		},
		{
			name:   "A set's lines are selectors, applied in its place",
			words:  "review/pr-review @drops",
			skills: nil,
		},

		// docs/guide/skills.md "Rename or delete a skill": "A selector that no
		// longer matches anything, in a command or in a skill set, is a warning,
		// not an error."
		{
			name:     "A skill that matches nothing, in a command",
			words:    "review/old-name",
			warnings: []string{"review/old-name   matches no skill"},
		},
		{
			name:     "A skill in a group that doesn't exist",
			words:    "nope/x",
			warnings: []string{"nope/x   matches no skill"},
		},
		{
			name:     "A group that matches nothing",
			words:    "nope",
			warnings: []string{"nope   matches no group"},
		},
		{
			name:     "A group with no skills",
			words:    "empty",
			warnings: []string{"empty   matches no skill"},
		},
		{
			name:     "A skill set that matches nothing",
			words:    "@nope",
			warnings: []string{"@nope   matches no skill set"},
		},
		{
			name:  "A removal that matches nothing",
			words: "-nope/x -nope -@nope",
			warnings: []string{
				"-nope/x   matches no skill",
				"-nope   matches no group",
				"-@nope   matches no skill set",
			},
		},
		{
			name:  "A selector that matches nothing in a skill set: ! review/old-name   matches no skill (in @backend)",
			words: "@stale",
			warnings: []string{
				"review/old-name   matches no skill (in @stale)",
				"@gone   matches no skill set (in @stale)",
				"pdf   matches no group (in @stale) → someone/pdf",
				"nope   matches no group (in @stale)",
				"-nope   matches no group (in @stale)",
			},
		},
		{
			name:  "A warning names the set that holds the line, however deep",
			words: "@outer",
			warnings: []string{
				"review/old-name   matches no skill (in @stale)",
				"@gone   matches no skill set (in @stale)",
				"pdf   matches no group (in @stale) → someone/pdf",
				"nope   matches no group (in @stale)",
				"-nope   matches no group (in @stale)",
			},
		},
		{
			name:  "The warnings of a removed set still show",
			words: "-@bottom",
			warnings: []string{
				"review/old-name   matches no skill (in @bottom)",
			},
		},
		{
			name:     "The same warning shows once",
			words:    "nope nope",
			warnings: []string{"nope   matches no group"},
		},
		{
			name:   "A warning never stops the rest",
			words:  "nope writing",
			skills: []string{"writing/changelog"},
			warnings: []string{
				"nope   matches no group",
			},
		},
		{name: "No selectors, no skills", words: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Resolve(parse(t, tt.words), vault)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tt.words, err)
			}
			var skills, replaced, warnings []string
			for _, s := range res.Skills {
				skills = append(skills, s.String())
			}
			for _, r := range res.Replaced {
				replaced = append(replaced, r.New.Folder+"   "+r.Detail())
			}
			for _, w := range res.Warnings {
				text := w.Message()
				if w.Suggest != "" {
					text += " → " + w.Suggest
				}
				warnings = append(warnings, text)
			}
			if !reflect.DeepEqual(skills, tt.skills) {
				t.Errorf("Resolve(%q) skills:\n got %q\nwant %q", tt.words, skills, tt.skills)
			}
			if !reflect.DeepEqual(replaced, tt.replaced) {
				t.Errorf("Resolve(%q) replaced:\n got %q\nwant %q", tt.words, replaced, tt.replaced)
			}
			if !reflect.DeepEqual(warnings, tt.warnings) {
				t.Errorf("Resolve(%q) warnings:\n got %q\nwant %q", tt.words, warnings, tt.warnings)
			}
		})
	}
}

// One new skill can replace two: one by folder, the other by frontmatter name.
func TestResolveReplacesTwo(t *testing.T) {
	idx := index{skills: []string{"a/x", "b/y=n", "c/x=n"}}
	res, err := Resolve(parse(t, "a/x b/y c/x"), idx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Skills, []Skill{{Group: "c", Folder: "x", Name: "n"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("skills = %+v, want %+v", got, want)
	}
	var got []string
	for _, r := range res.Replaced {
		got = append(got, r.Detail())
	}
	if want := []string{"c/x replaces a/x", "c/x replaces b/y"}; !reflect.DeepEqual(got, want) {
		t.Errorf("replaced = %q, want %q", got, want)
	}
}

// Removals first, then clashes among what's left: a skill is unloaded only if
// a skill in the result clashes with it, and the report names that skill.
func TestResolveClashChains(t *testing.T) {
	idx := index{skills: []string{"a/x", "b/x", "c/x", "c/y=n", "e/x=n", "o/x=n", "w/y=n", "z/y"}}
	tests := []struct {
		words            string
		skills, replaced []string
	}{
		{"a/x b/x c/x", []string{"c/x"}, []string{"c/x replaces a/x", "c/x replaces b/x"}},
		{"a/x b/x c/x -b/x", []string{"c/x"}, []string{"c/x replaces a/x"}},
		{"a/x b/x c/x -c/x", []string{"b/x"}, []string{"b/x replaces a/x"}},
		{"a/x b/x a/x", []string{"a/x"}, []string{"a/x replaces b/x"}},
		// c/y and a/x don't clash; e/x clashes with both and loses to a/x.
		{"c/y e/x a/x", []string{"c/y", "a/x"}, []string{"a/x replaces e/x"}},
		{"c/y e/x", []string{"e/x"}, []string{"e/x replaces c/y"}},
		// w/y clashes with o/x by name and with z/y by folder.
		{"o/x w/y z/y", []string{"o/x", "z/y"}, []string{"z/y replaces w/y"}},
		{"o/x w/y z/y o/x", []string{"o/x", "z/y"}, []string{"o/x replaces w/y", "z/y replaces w/y"}},
	}
	for _, tt := range tests {
		res, err := Resolve(parse(t, tt.words), idx)
		if err != nil {
			t.Fatal(err)
		}
		var skills, replaced []string
		for _, s := range res.Skills {
			skills = append(skills, s.String())
		}
		for _, r := range res.Replaced {
			replaced = append(replaced, r.Detail())
		}
		if !reflect.DeepEqual(skills, tt.skills) {
			t.Errorf("Resolve(%q) skills = %q, want %q", tt.words, skills, tt.skills)
		}
		if !reflect.DeepEqual(replaced, tt.replaced) {
			t.Errorf("Resolve(%q) replaced = %q, want %q", tt.words, replaced, tt.replaced)
		}
	}
}

// A skill's names are its folder name and its frontmatter name. Two skills
// clash when those overlap, so a folder clashes with another skill's name.
func TestResolveNamesOverlap(t *testing.T) {
	idx := index{skills: []string{"a/x", "b/y=x", "c/z"}}
	tests := []struct {
		words  string
		skills string
		want   string
	}{
		{"a/x b/y", "b/y", "b/y replaces a/x"},
		{"b/y a/x", "a/x", "a/x replaces b/y"},
	}
	for _, tt := range tests {
		res, err := Resolve(parse(t, tt.words), idx)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skills) != 1 || res.Skills[0].String() != tt.skills {
			t.Errorf("Resolve(%q) skills = %+v, want %s", tt.words, res.Skills, tt.skills)
		}
		if len(res.Replaced) != 1 || res.Replaced[0].Detail() != tt.want {
			t.Errorf("Resolve(%q) replaced = %+v, want %s", tt.words, res.Replaced, tt.want)
		}
	}
	// Neither has a frontmatter name: an empty name overlaps nothing.
	res, err := Resolve(parse(t, "a/x c/z"), idx)
	if err != nil || len(res.Skills) != 2 || res.Replaced != nil {
		t.Errorf("Resolve(a/x c/z) = %+v, %v, want both, no clash", res, err)
	}
}

// resolve returns the data for a hint, never a command: app builds the hint
// for the command being run (the strings land with #64).
func TestWarningData(t *testing.T) {
	tests := []struct {
		word    string
		problem Problem
		remove  bool
		set     string
		suggest string
	}{
		{word: "pdf", problem: NoGroup, suggest: "someone/pdf"},
		{word: "-pdf", problem: NoGroup, remove: true, suggest: "someone/pdf"},
		{word: "nope", problem: NoGroup},
		{word: "nope/x", problem: NoSkill},
		{word: "@nope", problem: NoSet},
	}
	for _, tt := range tests {
		res, err := Resolve(parse(t, tt.word), vault)
		if err != nil || len(res.Warnings) != 1 {
			t.Fatalf("Resolve(%q) = %+v, %v", tt.word, res, err)
		}
		w := res.Warnings[0]
		if w.Selector.String() != tt.word || w.Selector.Remove != tt.remove || w.Problem != tt.problem || w.Set != tt.set || w.Suggest != tt.suggest {
			t.Errorf("Resolve(%q) warning = %+v", tt.word, w)
		}
	}
}

// A folder name wins over another skill's frontmatter name in the same group.
func TestResolveFolderBeforeFrontmatter(t *testing.T) {
	idx := index{skills: []string{"g/a=b", "g/b=c"}}
	res, err := Resolve(parse(t, "g/b"), idx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Folder != "b" {
		t.Errorf("skills = %+v, want folder b", res.Skills)
	}
}

// Card #58: "A set including itself (directly or through others) is an error
// naming the cycle."
func TestResolveCycle(t *testing.T) {
	tests := []struct {
		words string
		cycle []string
		msg   string
	}{
		{"@self", []string{"self", "self"}, "skill set @self includes itself: @self → @self"},
		{"@cycle-a", []string{"cycle-a", "cycle-b", "cycle-c", "cycle-a"}, "skill set @cycle-a includes itself: @cycle-a → @cycle-b → @cycle-c → @cycle-a"},
		{"-@selfdrop", []string{"selfdrop", "selfdrop"}, "skill set @selfdrop includes itself: @selfdrop → @selfdrop"},
		{"-@cycle-c", []string{"cycle-c", "cycle-a", "cycle-b", "cycle-c"}, "skill set @cycle-c includes itself: @cycle-c → @cycle-a → @cycle-b → @cycle-c"},
	}
	for _, tt := range tests {
		t.Run(tt.words, func(t *testing.T) {
			_, err := Resolve(parse(t, tt.words), vault)
			var c *CycleError
			if !errors.As(err, &c) {
				t.Fatalf("Resolve(%q) = %v, want a CycleError", tt.words, err)
			}
			if !reflect.DeepEqual(c.Cycle, tt.cycle) {
				t.Errorf("cycle = %q, want %q", c.Cycle, tt.cycle)
			}
			if err.Error() != tt.msg {
				t.Errorf("message = %q, want %q", err, tt.msg)
			}
		})
	}
}

// CLAUDE.md: "use/add/drop and set files reject a remote reference with a hint
// to skl learn it."; a line that can't be parsed stops too, naming its set.
func TestResolveBadSetLine(t *testing.T) {
	t.Run("reference", func(t *testing.T) {
		_, err := Resolve(parse(t, "@hasref"), vault)
		var ref *selector.ReferenceError
		if !errors.As(err, &ref) || ref.Ref != "someone/agent-skills/pdf" {
			t.Fatalf("err = %v, want the reference", err)
		}
		if want := "@hasref: someone/agent-skills/pdf is someone else's skill"; err.Error() != want {
			t.Errorf("message = %q, want %q", err, want)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		idx := index{sets: map[string][]string{"s": {"re view"}}}
		_, err := Resolve(parse(t, "@s"), idx)
		if !errors.Is(err, selector.ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if !strings.HasPrefix(err.Error(), "@s: ") {
			t.Errorf("message = %q, want it to name the set", err)
		}
	})
}

// An error reading a set stops; only a missing set is a warning.
func TestResolveSetError(t *testing.T) {
	boom := errors.New("broken toml")
	idx := index{err: map[string]error{"bad": boom}}
	for _, words := range []string{"@bad", "-@bad"} {
		if _, err := Resolve(parse(t, words), idx); !errors.Is(err, boom) {
			t.Errorf("Resolve(%q) = %v, want %v", words, err, boom)
		}
	}
}

// What must hold for any list: no two loaded skills clash, every replacement is
// real, and a selected skill only goes missing through a removal or a clash.
func TestResolveInvariants(t *testing.T) {
	idx := index{skills: []string{"a/x", "a/y=n", "b/x=m", "b/z=n", "c/m", "c/y", "d/n=x", "d/w"}}
	var words []string
	for _, s := range idx.skills {
		w, _, _ := strings.Cut(s, "=")
		words = append(words, w, "-"+w)
	}
	words = append(words, "a", "b", "c", "d", "-a", "-b", "-c", "-d")
	rnd := rand.New(rand.NewPCG(58, 85))
	for range 20000 {
		list := make([]string, 1+rnd.IntN(8))
		for i := range list {
			list[i] = words[rnd.IntN(len(words))]
		}
		line := strings.Join(list, " ")
		res, err := Resolve(parse(t, line), idx)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", line, err)
		}
		for i, s := range res.Skills {
			for _, o := range res.Skills[i+1:] {
				if s.clashes(o) {
					t.Fatalf("Resolve(%q): %s and %s clash", line, s, o)
				}
			}
		}
		for _, r := range res.Replaced {
			if slices.Contains(res.Skills, r.Old) || !slices.Contains(res.Skills, r.New) || !r.Old.clashes(r.New) {
				t.Fatalf("Resolve(%q): %s is not a real replacement in %v", line, r.Detail(), res.Skills)
			}
		}
		// Replay the list with removals only: what's still selected either
		// loaded or was replaced.
		var selected []Skill
		for _, sel := range parse(t, line) {
			for _, s := range idx.Skills(sel.Group) {
				if sel.Kind == selector.Skill && s.Folder != sel.Name && s.Name != sel.Name {
					continue
				}
				if sel.Remove {
					selected = slices.DeleteFunc(selected, func(o Skill) bool { return o == s })
				} else if !slices.Contains(selected, s) {
					selected = append(selected, s)
				}
			}
		}
		for _, s := range selected {
			lost := slices.ContainsFunc(res.Replaced, func(r Replacement) bool { return r.Old == s })
			if slices.Contains(res.Skills, s) == lost {
				t.Fatalf("Resolve(%q): %s loaded=%v replaced=%v", line, s, !lost, lost)
			}
		}
	}
}
