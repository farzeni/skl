package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/errs"
)

// run runs args against a table of fake commands, the real ones included.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	c := &cli{st: Streams{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}, getenv: func(string) string { return "" }}
	// The fake import stands in for the real one: it reports what it was given.
	c.cmds = slices.DeleteFunc(c.commands(), func(cmd *Command) bool { return cmd.Name == "import" })
	c.cmds = append(c.cmds,
		&Command{
			Name: "import", Args: "<name>…", Summary: "Import", Flags: []Flag{{Name: "group", Value: "group"}}, MinArgs: 1, MaxArgs: -1,
			Run: func(_ context.Context, in Input) error {
				fmt.Fprintf(c.st.Stdout, "import %q group=%q\n", in.Args, in.Flags["group"])
				return nil
			},
		},
		&Command{Name: "fail", MaxArgs: 1, Run: func(_ context.Context, in Input) error {
			switch in.Args[0] {
			case "hinted":
				return fmt.Errorf("init: %w", errs.Hint(errors.New("no vault on this machine"), "skl init <repo>"))
			case "usage":
				return errs.Usagef("skl learn x", "x is someone else's skill")
			case "canceled":
				return fmt.Errorf("swap: %w", context.Canceled)
			case "held":
				// A save made its generation; Ctrl-C came before the next line.
				c.ui.StepHeld("generation", "13")
				return context.Canceled
			default:
				return errors.New("plain")
			}
		}},
	)
	code = c.run(context.Background(), args)
	return code, out.String(), errOut.String()
}

// commands.md "Exit codes".
func TestExitCodes(t *testing.T) {
	tests := []struct {
		args   []string
		code   int
		stderr string
	}{
		{[]string{"import", "a", "b", "--group", "w"}, 0, ""},
		{[]string{"fail", "hinted"}, 1, "✗ init: no vault on this machine · skl init <repo>\n"},
		{[]string{"fail", "plain"}, 1, "✗ plain\n"},
		{[]string{"fail", "usage"}, 2, "✗ x is someone else's skill · skl learn x\n"},
		{[]string{"fail", "canceled"}, 130, ""},
		// The generation a command made before Ctrl-C still shows.
		{[]string{"fail", "held"}, 130, "◇ generation 13\n"},
		// "2: A usage error: an unknown command or flag, a missing argument"
		{[]string{"stauts"}, 2, "✗ unknown command stauts · skl status\n"},
		{[]string{"imprt"}, 2, "✗ unknown command imprt · skl import\n"},
		{[]string{"stauts", "-h"}, 2, "✗ unknown command stauts · skl status\n"},
		{[]string{"help", "imprt"}, 2, "✗ unknown command imprt · skl help import\n"},
		{[]string{"import"}, 2, "✗ skl import needs <name>… · skl help import\n"},
		{[]string{"help", "a", "b"}, 2, "✗ unexpected argument b · skl help help\n"},
		{[]string{"import", "x", "--group"}, 2, "✗ --group needs a group · skl help import\n"},
		{[]string{"import", "x", "--frob"}, 2, "✗ unknown flag --frob · skl help import\n"},
		{[]string{"--frob"}, 2, "✗ unknown flag --frob · skl help\n"},
		{[]string{"help", "--group=w"}, 2, "✗ skl help doesn't take --group · skl help help\n"},
		{[]string{"import", "x", "--version"}, 2, "✗ skl import doesn't take --version · skl help import\n"},
		{[]string{"--group=w"}, 2, "✗ unknown flag --group · skl help\n"},
		// Two flags the command doesn't take: the first on the line is named.
		{[]string{"help", "--group=w", "--version"}, 2, "✗ skl help doesn't take --group · skl help help\n"},
		{[]string{"help", "--version", "--group=w"}, 2, "✗ skl help doesn't take --version · skl help help\n"},
		{[]string{"help", "--version", "--group=w", "--version"}, 2, "✗ skl help doesn't take --version · skl help help\n"},
		// An empty or spaced command is named, quoted.
		{[]string{""}, 2, "✗ unknown command \"\" · skl help\n"},
		{[]string{"st atus"}, 2, "✗ unknown command \"st atus\" · skl status\n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			code, _, stderr := run(t, tt.args...)
			if code != tt.code || stderr != tt.stderr {
				t.Errorf("skl %q = %d %q, want %d %q", tt.args, code, stderr, tt.code, tt.stderr)
			}
		})
	}
}

// commands.md "Flags": "-h, which always means --help" and "Show the
// command's help, and change nothing."
func TestHelpChangesNothing(t *testing.T) {
	for _, args := range [][]string{
		{"import", "a", "-h"},
		{"-h", "import"},
		{"import", "--help", "--frob"}, // help wins over a flag mistake
		{"help", "import"},
	} {
		code, stdout, stderr := run(t, args...)
		if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "Import\n\nUsage\n  skl import <name>…\n") {
			t.Errorf("skl %q = %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	// After --, -h is an argument.
	code, stdout, _ := run(t, "import", "--", "-h")
	if code != 0 || stdout != "import [\"-h\"] group=\"\"\n" {
		t.Errorf("skl import -- -h = %d %q", code, stdout)
	}
}

func TestVersionLine(t *testing.T) {
	defer func(v, c string) { version, commit = v, c }(version, commit)
	version, commit = "v0.1.0", "b0a0fb2c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a"
	if got := versionLine(); got != "skl 0.1.0 (b0a0fb2)" {
		t.Errorf("versionLine() = %q", got)
	}
	version, commit = "v0.0.0-20261025120000-b2618b8abcde+dirty", "b2618b8"
	if got := versionLine(); got != "skl dev (b2618b8)" {
		t.Errorf("versionLine() = %q, want a pseudo-version shown as dev", got)
	}
}
