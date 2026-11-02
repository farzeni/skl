package resolve

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// docs/guide/skill-sets.md "Names and clashes": "Names that differ only by case
// or Unicode form are the same name". The report shows names as written (card #92).
func TestResolveFoldedClash(t *testing.T) {
	tests := []struct {
		name, words string
		skills      []string
		replaced    []string
		vault       []string
	}{
		{"case", "low/PDF2 Up/pdf2", []string{"Up/pdf2"}, []string{"Up/pdf2 replaces low/PDF2"}, []string{"low/PDF2", "Up/pdf2"}},
		{"form", "a/caf\u00e9 b/cafe\u0301", []string{"b/cafe\u0301"}, []string{"b/cafe\u0301 replaces a/caf\u00e9"}, []string{"a/caf\u00e9", "b/cafe\u0301"}},
		{"NFD folder, NFC frontmatter name", "a/cafe\u0301 b/x", []string{"b/x"}, []string{"b/x replaces a/cafe\u0301"}, []string{"a/cafe\u0301=CAFÉ", "b/x=CAFÉ"}},
		{"frontmatter names differ by case", "a/y b/x", []string{"b/x"}, []string{"b/x replaces a/y"}, []string{"a/y=CAFÉ", "b/x=caf\u00e9"}},
		{"a folder and a frontmatter name differ by case", "a/caf\u00e9 b/x", []string{"b/x"}, []string{"b/x replaces a/caf\u00e9"}, []string{"a/caf\u00e9", "b/x=CAFÉ"}},
		{"a frontmatter name and a folder differ by case", "b/x a/caf\u00e9", []string{"a/caf\u00e9"}, []string{"a/caf\u00e9 replaces b/x"}, []string{"a/caf\u00e9", "b/x=CAFÉ"}},
		{"two of one group, each named", "g/PDF g/pdf", []string{"g/pdf"}, []string{"g/pdf replaces g/PDF"}, []string{"g/PDF", "g/pdf"}},
		{"a removal settles a group", "g -g/PDF", []string{"g/pdf"}, nil, []string{"g/PDF", "g/pdf"}},
		{"ß and SS", "a/straße b/STRASSE", []string{"b/STRASSE"}, []string{"b/STRASSE replaces a/straße"}, []string{"a/straße", "b/STRASSE"}},
		{"dotless i is its own letter", "a/ı b/I", []string{"a/ı", "b/I"}, nil, []string{"a/ı", "b/I"}},
		{"a removal comes before the clash", "low/PDF2 Up/pdf2 -up/PDF2", []string{"low/PDF2"}, nil, []string{"low/PDF2", "Up/pdf2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Resolve(parse(t, tt.words), index{skills: tt.vault})
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
			if !reflect.DeepEqual(skills, tt.skills) || !reflect.DeepEqual(replaced, tt.replaced) {
				t.Errorf("skills %q replaced %q, want %q %q", skills, replaced, tt.skills, tt.replaced)
			}
		})
	}
}

func TestResolveFoldedLookup(t *testing.T) {
	idx := index{skills: []string{"R\u00e9sum\u00e9/Caf\u00e9", "g/PDF", "g/pdf", "g/a=Nom"}}
	for words, want := range map[string]string{
		"re\u0301sume\u0301/cafe\u0301": "R\u00e9sum\u00e9/Caf\u00e9", // NFD finds NFC
		"g/PDF":                         "g/PDF",
		"g/pdf":                         "g/pdf", // exact wins over the first fold
		"G/nom":                         "g/a",
	} {
		res, err := Resolve(parse(t, words), idx)
		if err != nil || len(res.Skills) != 1 || res.Skills[0].String() != want {
			t.Errorf("Resolve(%q) = %+v, %v, want %s", words, res.Skills, err, want)
		}
	}
	// And NFC finds an NFD folder.
	res, err := Resolve(parse(t, "g/caf\u00e9"), index{skills: []string{"g/cafe\u0301"}})
	if err != nil || len(res.Skills) != 1 {
		t.Errorf("NFC selector, NFD folder: %+v, %v", res, err)
	}
}

// "Group names clash the same way: `Review` and `review` make a selector
// ambiguous, so skl stops and lists both, as it does for a name in two groups."
// Even if one is an exact match (card #92, item 2).
func TestResolveAmbiguousGroup(t *testing.T) {
	idx := index{skills: []string{"Review/a", "review/b", "e\u0301/c", "\u00e9/d", "other/z"}}
	for _, tt := range []struct {
		words  string
		groups []string
	}{
		{"review", []string{"Review", "review"}},
		{"Review/a", []string{"Review", "review"}},
		{"-REVIEW", []string{"Review", "review"}},
		{"\u00e9", []string{"e\u0301", "\u00e9"}},
	} {
		_, err := Resolve(parse(t, tt.words), idx)
		var amb *AmbiguousGroupError
		if !errors.As(err, &amb) || !reflect.DeepEqual(amb.Groups, tt.groups) {
			t.Errorf("Resolve(%q) = %v, want both of %q", tt.words, err, tt.groups)
		}
	}
	if _, err := Resolve(parse(t, "other"), idx); err != nil {
		t.Errorf("an unrelated group: %v", err)
	}
}

// Card #94, item 3: skill-sets.md's rule for groups, for two skills in one group.
// An exact folder name always names one skill, so each one can still be selected.
func TestResolveAmbiguousSkill(t *testing.T) {
	idx := index{
		skills: []string{"g/PDF", "g/pdf", "g/caf\u00e9", "g/cafe\u0301", "g/x", "g/y=X", "n/a=Nom", "n/b=nom", "n/c=same", "n/d=same"},
		sets:   map[string][]string{"s": {"g/Pdf"}},
	}
	tests := []struct {
		words string
		typed string
		found []Skill
		msg   string
	}{
		{"g/Pdf", "g/Pdf", []Skill{{Group: "g", Folder: "PDF"}, {Group: "g", Folder: "pdf"}}, "g/Pdf matches 2 skills: g/PDF, g/pdf"},
		{"-G/pDF", "G/pDF", []Skill{{Group: "g", Folder: "PDF"}, {Group: "g", Folder: "pdf"}}, "G/pDF matches 2 skills: g/PDF, g/pdf"},
		{"g/CAF\u00c9", "g/CAF\u00c9", []Skill{{Group: "g", Folder: "caf\u00e9"}, {Group: "g", Folder: "cafe\u0301"}}, "g/CAF\u00c9 matches 2 skills: g/caf\u00e9, g/cafe\u0301"},
		{"n/NOM", "n/NOM", []Skill{{Group: "n", Folder: "a", Name: "Nom"}, {Group: "n", Folder: "b", Name: "nom"}}, "n/NOM matches 2 skills: n/a, n/b"},
		{"n/nom", "n/nom", []Skill{{Group: "n", Folder: "a", Name: "Nom"}, {Group: "n", Folder: "b", Name: "nom"}}, "n/nom matches 2 skills: n/a, n/b"},
		{"n/same", "n/same", []Skill{{Group: "n", Folder: "c", Name: "same"}, {Group: "n", Folder: "d", Name: "same"}}, "n/same matches 2 skills: n/c, n/d"},
		{"@s", "g/Pdf", []Skill{{Group: "g", Folder: "PDF"}, {Group: "g", Folder: "pdf"}}, "@s: g/Pdf matches 2 skills: g/PDF, g/pdf"},
		{"-@s", "g/Pdf", []Skill{{Group: "g", Folder: "PDF"}, {Group: "g", Folder: "pdf"}}, "@s: g/Pdf matches 2 skills: g/PDF, g/pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.words, func(t *testing.T) {
			_, err := Resolve(parse(t, tt.words), idx)
			var amb *AmbiguousSkillError
			if !errors.As(err, &amb) {
				t.Fatalf("Resolve(%q) = %v, want an AmbiguousSkillError", tt.words, err)
			}
			if amb.Name != tt.typed || !reflect.DeepEqual(amb.Skills, tt.found) {
				t.Errorf("Resolve(%q) = %+v, want %q and %+v", tt.words, amb, tt.typed, tt.found)
			}
			if err.Error() != tt.msg {
				t.Errorf("message = %q, want %q", err, tt.msg)
			}
		})
	}

	// One match is no ambiguity.
	for words, want := range map[string]string{
		"g/PDF":        "g/PDF",
		"g/pdf":        "g/pdf",
		"-g/pdf g/pdf": "g/pdf",
		"g/caf\u00e9":  "g/caf\u00e9",
		"g/cafe\u0301": "g/cafe\u0301",
		"n/c":          "n/c",
		// A folder in any case comes before another skill's frontmatter name.
		"g/X": "g/x",
	} {
		res, err := Resolve(parse(t, words), idx)
		if err != nil || len(res.Skills) != 1 || res.Skills[0].String() != want {
			t.Errorf("Resolve(%q) = %+v, %v, want %s", words, res.Skills, err, want)
		}
	}
}

// repos.md "Grammar": "the hint repeats your command with only that word replaced",
// so what it offers has to work: names outside the grammar can't be typed.
func TestResolveSuggestCanBeTyped(t *testing.T) {
	tests := []struct {
		word  string
		vault []string
		want  string
		skill Skill
	}{
		{"one", []string{"a.b/one"}, "", Skill{}},
		{"one", []string{"a.b/one", "z/one"}, "z/one", Skill{Group: "z", Folder: "one"}},
		{"nice", []string{"weird/c.d=nice"}, "weird/nice", Skill{Group: "weird", Folder: "c.d", Name: "nice"}},
		{"nice", []string{"weird/c.d=nice", "weird/e.f=nice"}, "", Skill{}},
		{"nice", []string{"weird/nice=x"}, "weird/nice", Skill{Group: "weird", Folder: "nice", Name: "x"}},
	}
	for _, tt := range tests {
		idx := index{skills: tt.vault}
		res, err := Resolve(parse(t, tt.word), idx)
		if err != nil || len(res.Warnings) != 1 {
			t.Fatalf("Resolve(%q) in %v = %+v, %v", tt.word, tt.vault, res, err)
		}
		if w := res.Warnings[0]; w.Suggest != tt.want || w.Skill != tt.skill {
			t.Errorf("Resolve(%q) in %v suggests %q %+v, want %q", tt.word, tt.vault, w.Suggest, w.Skill, tt.want)
		}
		if tt.want == "" {
			continue
		}
		got, err := Resolve(parse(t, tt.want), idx)
		if err != nil || len(got.Skills) != 1 || got.Skills[0] != tt.skill {
			t.Errorf("following %s: %+v, %v", tt.want, got, err)
		}
	}
}

// A one-word name that isn't a group only warns, so two matches don't stop it.
// The suggestion is a folder name as written, which an exact match can follow.
func TestResolveSuggestAmbiguous(t *testing.T) {
	idx := index{skills: []string{"g/PDF", "g/pdf", "n/a=Nom", "n/b=nom"}}
	for word, want := range map[string]string{"Pdf": "g/PDF", "nom": "n/a"} {
		res, err := Resolve(parse(t, word), idx)
		if err != nil || len(res.Warnings) != 1 || res.Warnings[0].Suggest != want {
			t.Errorf("Resolve(%q) = %+v, %v, want the suggestion %s", word, res, err, want)
			continue
		}
		if _, err := Resolve(parse(t, want), idx); err != nil {
			t.Errorf("following the suggestion %s: %v", want, err)
		}
	}
}

// Card #94, item 2. Worst cases for clashes: nothing clashes, so settling compares
// every pair. "groups" is a whole vault by group; "skills" names each skill, and
// "folded" does so in another case, so that each lookup folds its whole group.
func BenchmarkResolve(b *testing.B) {
	const skills, groups = 1000, 20
	idx := index{}
	var byGroup, bySkill, byFolded []string
	for i := range groups {
		byGroup = append(byGroup, fmt.Sprintf("g%02d", i))
	}
	for i := range skills {
		s := fmt.Sprintf("g%02d/skill-name-%d", i*groups/skills, i)
		idx.skills = append(idx.skills, s)
		bySkill = append(bySkill, s)
		byFolded = append(byFolded, strings.ToUpper(s))
	}
	for _, bench := range []struct {
		name  string
		words []string
	}{{"groups", byGroup}, {"skills", bySkill}, {"folded", byFolded}} {
		b.Run(bench.name, func(b *testing.B) {
			sels := parse(b, strings.Join(bench.words, " "))
			for b.Loop() {
				res, err := Resolve(sels, idx)
				if err != nil || len(res.Skills) != skills {
					b.Fatalf("Resolve: %d skills, %v", len(res.Skills), err)
				}
			}
		})
	}
}

// PR #102, the user's decision: a group selector that covers two skills that clash
// stops the command and lists both, whichever name they share.
func TestResolveOneGroupClash(t *testing.T) {
	tests := []struct {
		name, words string
		vault       []string
		a, b        string
	}{
		{"folders by case", "g", []string{"g/PDF", "g/pdf"}, "g/PDF", "g/pdf"},
		{"a frontmatter name", "g", []string{"g/a=x", "g/b=x"}, "g/a", "g/b"},
		{"a folder and a frontmatter name", "g", []string{"g/a=x", "g/x"}, "g/a", "g/x"},
		{"through a set", "@s", []string{"g/PDF", "g/pdf"}, "g/PDF", "g/pdf"},
		{"selected twice", "g g", []string{"g/PDF", "g/pdf"}, "g/PDF", "g/pdf"},
		{"a group then one of its skills", "g g/PDF", []string{"g/PDF", "g/pdf"}, "g/PDF", "g/pdf"},
		{"one of its skills then a group", "g/PDF g", []string{"g/PDF", "g/pdf"}, "g/PDF", "g/pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := index{skills: tt.vault, sets: map[string][]string{"s": {"g"}}}
			_, err := Resolve(parse(t, tt.words), idx)
			var ce *ClashError
			if !errors.As(err, &ce) || ce.A.String() != tt.a || ce.B.String() != tt.b {
				t.Fatalf("Resolve(%q) = %v, want %s and %s", tt.words, err, tt.a, tt.b)
			}
			if want := tt.a + " and " + tt.b + " have the same name"; err.Error() != want {
				t.Errorf("message = %q, want %q", err, want)
			}
		})
	}
	// Removing the whole group needs no choice, and two groups keep "the later wins".
	for _, words := range []string{"g -g", "-g", "other g/PDF"} {
		idx := index{skills: []string{"g/PDF", "g/pdf", "other/pdf"}}
		if _, err := Resolve(parse(t, words), idx); err != nil {
			t.Errorf("Resolve(%q) = %v", words, err)
		}
	}
}

// skill-sets.md: "A skill loses a clash only to a later skill that stays loaded".
func TestSkippingGivesTheClashBack(t *testing.T) {
	a := Skill{Group: "g", Folder: "x"}
	b := Skill{Group: "k", Folder: "y", Name: "x"}
	c := Skill{Group: "i", Folder: "z", Name: "y"}
	res, err := Resolve(parse(t, "g/x k/y i/z"), index{skills: []string{"g/x", "k/y=x", "i/z=y"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Skills, []Skill{a, c}) || !slices.Equal(res.Selected(), []Skill{a, b, c}) {
		t.Fatalf("skills = %v, selected = %v", res.Skills, res.Selected())
	}
	tests := []struct {
		skip         []Skill
		skills, gone []Skill
		replaced     []Replacement
	}{
		{nil, []Skill{a, c}, nil, []Replacement{{b, c}}},
		// c gone: b stays, and a loses to b instead.
		{[]Skill{c}, []Skill{b}, []Skill{c}, []Replacement{{a, b}}},
		// b lost to c anyway, so it isn't reported.
		{[]Skill{b}, []Skill{a, c}, nil, []Replacement{{b, c}}},
		// Each one gone in turn gives way to the one before it.
		{[]Skill{b, c}, []Skill{a}, []Skill{b, c}, nil},
		{[]Skill{a, b, c}, nil, []Skill{a, b, c}, nil},
	}
	for _, tt := range tests {
		got, gone := res.Skipping(func(s Skill) bool { return slices.Contains(tt.skip, s) })
		if !slices.Equal(got.Skills, tt.skills) || !slices.Equal(gone, tt.gone) || !slices.Equal(got.Replaced, tt.replaced) {
			t.Errorf("skip %v: skills %v, skipped %v, replaced %v", tt.skip, got.Skills, gone, got.Replaced)
		}
	}
}
