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
