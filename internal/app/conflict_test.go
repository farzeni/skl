package app

import (
	"slices"
	"testing"

	"github.com/farzeni/skl/internal/vault"
)

// docs/guide/sync.md "When both machines changed the same file": skl shows what
// each side did, such as "drop someone/pdf", "add data/sql-explain", "use
// someone/pdf-tools".
func TestSetChanges(t *testing.T) {
	base := []string{"review/pr-review", "someone/pdf"}
	tests := []struct {
		name  string
		after []string
		want  []string
	}{
		{"the docs' this machine", []string{"review/pr-review"}, []string{"drop someone/pdf"}},
		{"the docs' other machine", []string{"review/pr-review", "data/sql-explain", "someone/pdf-tools"},
			[]string{"add data/sql-explain", "use someone/pdf-tools"}},
		{"added at the top", []string{"writing", "review/pr-review", "someone/pdf"}, []string{"add writing"}},
		{"two taken out for one", []string{"writing"}, []string{"drop review/pr-review", "use writing"}},
		{"reordered", []string{"someone/pdf", "review/pr-review"},
			[]string{"drop review/pr-review", "add review/pr-review"}},
		{"unchanged", base, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setChanges(base, tt.after); !slices.Equal(got, tt.want) {
				t.Errorf("setChanges = %q, want %q", got, tt.want)
			}
		})
	}
}

// A file that isn't a skill set, or one that doesn't parse, says added, deleted
// or changed.
func TestSideDid(t *testing.T) {
	set := func(lines string) vault.Side { return vault.Side{Data: []byte("select = [" + lines + "]\n")} }
	tests := []struct {
		name       string
		path       string
		base, side vault.Side
		want       []string
	}{
		{"deleted", "review/pr-review/SKILL.md", vault.Side{Data: []byte("x")}, vault.Side{Absent: true}, []string{"deleted"}},
		{"added", "review/pr-review/SKILL.md", vault.Side{Absent: true}, vault.Side{Data: []byte("x")}, []string{"added"}},
		{"changed", "review/pr-review/SKILL.md", vault.Side{Data: []byte("x")}, vault.Side{Data: []byte("y")}, []string{"changed"}},
		{"set", "sets/backend.toml", set(`"a"`), set(`"a", "b"`), []string{"add b"}},
		{"set with markers", "sets/backend.toml", set(`"a"`), vault.Side{Data: []byte("<<<<<<<\n")}, []string{"changed"}},
		{"set, same lines", "sets/backend.toml", set(`"a"`), set(`"a" `), []string{"changed"}},
		{"origin file", "sets/backend.origin.toml", set(`"a"`), set(`"b"`), []string{"changed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sideDid(tt.path, tt.base, tt.side); !slices.Equal(got, tt.want) {
				t.Errorf("sideDid = %q, want %q", got, tt.want)
			}
		})
	}
}
