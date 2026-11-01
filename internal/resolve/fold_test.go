package resolve

import (
	"errors"
	"reflect"
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
		{"NFD folder, NFC frontmatter name", "a/cafe\u0301 b/x", []string{"b/x"}, []string{"b/x replaces a/cafe\u0301"}, []string{"a/cafe\u0301=caf\u00e9", "b/x=CAFÉ"}},
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

// "Group names clash the same way: `Review` and `review`." Card #92, item 2: a
// selector that matches two such groups stops and lists both, even if one is exact.
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
