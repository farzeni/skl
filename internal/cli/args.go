package cli

import (
	"strings"

	"github.com/farzeni/skl/internal/errs"
)

// Flag is a flag a command accepts. A flag with a Value placeholder takes a
// value (--group writing or --group=writing); one without is a switch.
type Flag struct {
	Name  string // without the leading "--"
	Value string // placeholder shown in help, such as "group"; "" for a switch
	Usage string
}

// Input is a parsed command line.
type Input struct {
	Args  []string          // every word that isn't a flag, the command first
	Flags map[string]string // flag name → value; "" for a switch
	Help  bool              // --help or -h appeared before "--"
}

// Has reports whether flag name was given.
func (in Input) Has(name string) bool {
	_, ok := in.Flags[name]
	return ok
}

// parse splits words into arguments and flags, as docs/reference/commands.md
// "Flags" describes: flags start with "--" and can go anywhere, as --x v or
// --x=v; every other word is an argument, so "-review/x" is one; "--" ends
// the flags; "-h" always means --help, except after "--".
//
// known lists every flag of every command, so a value flag takes its value
// before the command is known. Whether the command accepts a flag is
// checked later. parse reads every word even after a mistake, so the
// caller knows the command, and returns the first mistake.
func parse(words []string, known []Flag) (in Input, err error) {
	in.Flags = map[string]string{}
	fail := func(e error) {
		if err == nil {
			err = e
		}
	}
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "--":
			in.Args = append(in.Args, words[i+1:]...)
			return in, err
		case w == "-h":
			in.Help = true
		case strings.HasPrefix(w, "--"):
			name, value, hasValue := strings.Cut(w[2:], "=")
			if name == "help" {
				if hasValue {
					fail(errs.Usagef("", "--help takes no value"))
				}
				in.Help = true
				continue
			}
			f, ok := find(known, name)
			if !ok {
				fail(errs.Usagef("", "unknown flag --%s", name))
				continue
			}
			switch {
			case f.Value == "" && hasValue:
				fail(errs.Usagef("", "--%s takes no value", name))
				continue
			case f.Value != "" && !hasValue:
				if i+1 == len(words) || strings.HasPrefix(words[i+1], "--") || words[i+1] == "-h" {
					fail(errs.Usagef("", "--%s needs a %s", name, f.Value))
					continue
				}
				i++
				value = words[i]
			}
			if prev, dup := in.Flags[name]; dup && prev != value {
				fail(errs.Usagef("", "--%s given twice", name))
			}
			in.Flags[name] = value
		default:
			in.Args = append(in.Args, w)
		}
	}
	return in, err
}

func find(flags []Flag, name string) (Flag, bool) {
	for _, f := range flags {
		if f.Name == name {
			return f, true
		}
	}
	return Flag{}, false
}
