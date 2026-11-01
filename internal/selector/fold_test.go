package selector

import "testing"

// Card #92, docs/guide/skill-sets.md "Names and clashes".
func TestFold(t *testing.T) {
	tests := []struct {
		a, b string
		same bool
	}{
		{"PDF2", "pdf2", true},
		{"Review", "review", true},
		{"é", "é", true},
		{"É", "é", true},
		{"straße", "STRASSE", true},
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
