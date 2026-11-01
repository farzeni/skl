package selector

import (
	"testing"
	"unicode"
	"unicode/utf8"
)

// docs/guide/skill-sets.md "Names and clashes": "`low/PDF2` and `Up/pdf2` clash,
// and so do `\u00e9` written as one character and `e` plus an accent."
func TestFold(t *testing.T) {
	tests := []struct {
		a, b string
		same bool
	}{
		{"PDF2", "pdf2", true},
		{"Review", "review", true},
		{"\u00e9", "e\u0301", true},
		{"É", "e\u0301", true},
		{"straße", "STRASSE", true},
		// Folding can leave a pair NFC would join: only the last NFC finds these.
		{"ß\u0301", "S\u015a", true},
		// U+0345 reorders under NFC, then folds to a letter that blocks the
		// accent: only the first NFC finds these.
		{"\u03b1\u0345\u0301", "\u1fb4", true},
		// Folding has no locale: the Turkish letters stay their own.
		{"ı", "i", false},
		{"ı", "I", false},
		{"İ", "i", false},
		{"pdf", "pdf2", false},
	}
	for _, tt := range tests {
		if got := Fold(tt.a) == Fold(tt.b); got != tt.same {
			t.Errorf("Fold(%q) == Fold(%q) = %v, want %v", tt.a, tt.b, got, tt.same)
		}
	}
}

// Every rune and its other cases fold to one name. This caught x/text swapping
// the two cases of Cherokee.
func TestFoldCasePairs(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		for o := unicode.SimpleFold(r); o != r; o = unicode.SimpleFold(o) {
			if Fold(string(r)) != Fold(string(o)) {
				t.Errorf("Fold(%U) = %q, Fold(%U) = %q", r, Fold(string(r)), o, Fold(string(o)))
			}
		}
	}
}

func TestReservedGroupFolds(t *testing.T) {
	for _, name := range []string{"sets", "SETS", "ſets", "Lessons", "H"} {
		if !ReservedGroup(name) {
			t.Errorf("ReservedGroup(%q) = false", name)
		}
	}
	if ReservedGroup("sets-x") {
		t.Error(`ReservedGroup("sets-x") = true`)
	}
}
