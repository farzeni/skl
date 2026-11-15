package app

import (
	"slices"
	"testing"
)

func TestLineDiff(t *testing.T) {
	for _, tc := range []struct {
		name      string
		old, next []string
		want      []diffLine
	}{
		{"same", []string{"a", "b"}, []string{"a", "b"}, nil},
		{"added", []string{"a"}, []string{"a", "b"}, []diffLine{{line: "b"}}},
		{"removed", []string{"a", "b", "c"}, []string{"a", "c"}, []diffLine{{line: "b", removed: true}}},
		{"moved", []string{"a", "b"}, []string{"b", "a"}, []diffLine{{line: "a", removed: true}, {line: "a"}}},
		{"replaced", []string{"a", "x", "c"}, []string{"a", "y", "c"}, []diffLine{{line: "x", removed: true}, {line: "y"}}},
		{"emptied", []string{"a"}, nil, []diffLine{{line: "a", removed: true}}},
		{"from nothing", nil, []string{"a"}, []diffLine{{line: "a"}}},
	} {
		if got := lineDiff(tc.old, tc.next); !slices.Equal(got, tc.want) {
			t.Errorf("%s: lineDiff(%q, %q) = %v, want %v", tc.name, tc.old, tc.next, got, tc.want)
		}
	}
}

// docs/reference/commands.md "Questions": "One question puts no first: skl save
// overwriting a named skill set with one that would remove every line."
func TestSavePlanEmpties(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    savePlan
		want bool
	}{
		{"every line goes", savePlan{exists: true, old: []string{"@base", "review"}}, true},
		{"one line stays", savePlan{exists: true, old: []string{"@base", "review"}, lines: []string{"review"}}, false},
		{"every line replaced", savePlan{exists: true, old: []string{"@base"}, lines: []string{"data"}}, false},
		{"over an empty set", savePlan{exists: true, old: []string{}}, false},
		{"a new set", savePlan{}, false},
		{"an unreadable set", savePlan{exists: true}, false},
	} {
		if got := tc.p.empties(); got != tc.want {
			t.Errorf("%s: empties() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
