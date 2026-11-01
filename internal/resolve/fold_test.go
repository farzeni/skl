package resolve

import (
	"errors"
	"reflect"
	"testing"
)

// Card #92, docs/guide/skill-sets.md "Names and clashes": names that differ
// only by case or Unicode form clash. The report shows names as written.
func TestResolveFoldedClash(t *testing.T) {
	tests := []struct {
		name, words string
		skills      []string
		replaced    []string
		vault       []string
	}{
		{"case", "low/PDF2 Up/pdf2", []string{"Up/pdf2"}, []string{"Up/pdf2 replaces low/PDF2"}, []string{"low/PDF2", "Up/pdf2"}},
		{"form", "a/café b/café", []string{"b/café"}, []string{"b/café replaces a/café"}, []string{"a/café", "b/café"}},
		{"NFD folder, NFC frontmatter name", "a/café b/x", []string{"b/x"}, []string{"b/x replaces a/café"}, []string{"a/café=café", "b/x=CAFÉ"}},
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
	idx := index{skills: []string{"Résumé/Café", "g/PDF", "g/pdf", "g/a=Nom"}}
	for words, want := range map[string]string{
		"résumé/café": "Résumé/Café", // NFD finds NFC
		"g/PDF":          "g/PDF",
		"g/pdf":          "g/pdf", // exact wins over the first fold
		"G/nom":          "g/a",
	} {
		res, err := Resolve(parse(t, words), idx)
		if err != nil || len(res.Skills) != 1 || res.Skills[0].String() != want {
			t.Errorf("Resolve(%q) = %+v, %v, want %s", words, res.Skills, err, want)
		}
	}
	// And NFC finds an NFD folder.
	res, err := Resolve(parse(t, "g/café"), index{skills: []string{"g/café"}})
	if err != nil || len(res.Skills) != 1 {
		t.Errorf("NFC selector, NFD folder: %+v, %v", res, err)
	}
}

// Two groups that differ only by case or form stop, even on an exact match.
func TestResolveAmbiguousGroup(t *testing.T) {
	idx := index{skills: []string{"Review/a", "review/b", "é/c", "é/d", "other/z"}}
	for _, tt := range []struct {
		words  string
		groups []string
	}{
		{"review", []string{"Review", "review"}},
		{"Review/a", []string{"Review", "review"}},
		{"-REVIEW", []string{"Review", "review"}},
		{"é", []string{"é", "é"}},
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
