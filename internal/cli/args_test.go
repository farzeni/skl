package cli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/farzeni/skl/internal/errs"
)

var testFlags = []Flag{
	{Name: "group", Value: "group"},
	{Name: "dry-run"},
	{Name: "yes"},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		args  []string
		flags map[string]string
		help  bool
	}{
		// commands.md "Flags": "Every other word is an argument, so
		// -review/test-gaps is a selector, not a flag."
		{"single dash is an argument", []string{"use", "-review/x"}, []string{"use", "-review/x"}, nil, false},
		{"lone dash is an argument", []string{"use", "-"}, []string{"use", "-"}, nil, false},
		// "as --group writing or --group=writing"
		{"value after =", []string{"import", "--group=w", "x"}, []string{"import", "x"}, map[string]string{"group": "w"}, false},
		{"value as next word", []string{"import", "--group", "w", "x"}, []string{"import", "x"}, map[string]string{"group": "w"}, false},
		{"value may start with one dash", []string{"import", "--group", "-w"}, []string{"import"}, map[string]string{"group": "-w"}, false},
		{"empty value after =", []string{"import", "--group="}, []string{"import"}, map[string]string{"group": ""}, false},
		// "can go anywhere on the line"
		{"flags before the command", []string{"--dry-run", "use", "@base"}, []string{"use", "@base"}, map[string]string{"dry-run": ""}, false},
		{"flags between arguments", []string{"use", "a", "--yes", "b"}, []string{"use", "a", "b"}, map[string]string{"yes": ""}, false},
		{"same switch twice", []string{"use", "--yes", "--yes"}, []string{"use"}, map[string]string{"yes": ""}, false},
		// "-- ends the flags: every word after it is an argument."
		{"-- ends flags", []string{"use", "--", "--yes", "-h", "--"}, []string{"use", "--yes", "-h", "--"}, nil, false},
		// "-h, which always means --help"
		{"-h anywhere", []string{"use", "a", "-h"}, []string{"use", "a"}, nil, true},
		{"-h first", []string{"-h", "use"}, []string{"use"}, nil, true},
		{"--help", []string{"use", "--help"}, []string{"use"}, nil, true},
		{"nothing", nil, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parse(tt.words, testFlags)
			if err != nil {
				t.Fatalf("parse(%q): %v", tt.words, err)
			}
			if tt.flags == nil {
				tt.flags = map[string]string{}
			}
			if !reflect.DeepEqual(in.Args, tt.args) || !reflect.DeepEqual(in.Flags, tt.flags) || in.Help != tt.help {
				t.Errorf("parse(%q) = %q %v help=%v, want %q %v help=%v",
					tt.words, in.Args, in.Flags, in.Help, tt.args, tt.flags, tt.help)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  string
		args  []string // still collected after the mistake
	}{
		{"unknown flag", []string{"use", "--frob", "a"}, "unknown flag --frob", []string{"use", "a"}},
		{"three dashes", []string{"---yes"}, "unknown flag ---yes", nil},
		{"value flag at the end", []string{"import", "x", "--group"}, "--group needs a group", []string{"import", "x"}},
		{"value flag before a flag", []string{"import", "--group", "--yes"}, "--group needs a group", []string{"import"}},
		{"value flag before --", []string{"import", "--group", "--", "x"}, "--group needs a group", []string{"import", "x"}},
		{"value flag before -h", []string{"import", "--group", "-h"}, "--group needs a group", []string{"import"}},
		{"switch with a value", []string{"use", "--yes=no"}, "--yes takes no value", []string{"use"}},
		{"help with a value", []string{"--help=use"}, "--help takes no value", nil},
		{"value flag twice", []string{"import", "--group", "a", "--group=b"}, "--group given twice", []string{"import"}},
		{"first mistake wins", []string{"--frob", "--group"}, "unknown flag --frob", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parse(tt.words, testFlags)
			var u *errs.Usage
			if !errors.As(err, &u) {
				t.Fatalf("parse(%q) error = %v, want a usage error", tt.words, err)
			}
			if err.Error() != tt.want {
				t.Errorf("parse(%q) error = %q, want %q", tt.words, err, tt.want)
			}
			if !reflect.DeepEqual(in.Args, tt.args) {
				t.Errorf("parse(%q) args = %q, want %q", tt.words, in.Args, tt.args)
			}
		})
	}
}

func TestSuggest(t *testing.T) {
	cmds := []string{"init", "import", "use", "add", "drop", "save", "status", "history", "sync", "help"}
	tests := []struct{ word, want string }{
		{"stauts", "status"}, // commands.md "Help": ✗ unknown command stauts · skl status
		{"statsu", "status"},
		{"hlep", "help"},
		{"histroy", "history"},
		{"ue", "use"},
		{"xyz", ""},
		{"a", ""},
		{"rollback", ""},
	}
	for _, tt := range tests {
		if got := suggest(tt.word, cmds); got != tt.want {
			t.Errorf("suggest(%q) = %q, want %q", tt.word, got, tt.want)
		}
	}
}

func TestErrorLine(t *testing.T) {
	tests := []struct {
		msg, hint string
		width     int
		want      string
	}{
		{"unknown command stauts", "skl status", 80, "✗ unknown command stauts · skl status\n"},
		{"boom", "", 80, "✗ boom\n"},
		// commands.md "Output": "A command that doesn't fit on the line moves
		// to the next one, after ·."
		{
			"another skl is running (pid 4182)", "skl sync, once it's done", 40,
			"✗ another skl is running (pid 4182)\n  · skl sync, once it's done\n",
		},
		{"abc", "skl x", 13, "✗ abc · skl x\n"},
		{"abc", "skl x", 12, "✗ abc\n  · skl x\n"},
	}
	for _, tt := range tests {
		if got := errorLine(tt.msg, tt.hint, tt.width); got != tt.want {
			t.Errorf("errorLine(%q, %q, %d) = %q, want %q", tt.msg, tt.hint, tt.width, got, tt.want)
		}
	}
}
