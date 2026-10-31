// Package cli turns a command line into a run of one command: it parses the
// words, finds the command, prints help and the version, and maps the
// command's error to its ✗ line and exit code (docs/reference/commands.md
// "Flags", "Help", "Exit codes").
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/ui"
)

// Exit codes, from docs/reference/commands.md "Exit codes".
const (
	exitOK          = 0
	exitError       = 1
	exitUsage       = 2
	exitInterrupted = 130
)

// Streams are the process's standard streams.
type Streams struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Command is one skl command. The command table holds only commands that
// are built, with only the flags they accept.
type Command struct {
	Name    string
	Args    string // argument synopsis for help, such as "[command]"
	Summary string // one line, for the command list
	Help    string // a paragraph for "skl help <command>"; Summary if empty
	Example string
	Flags   []Flag
	MinArgs int
	MaxArgs int // -1 for any number
	Run     func(ctx context.Context, in Input) error
}

// globalFlags are accepted without a command. --help and -h are handled by
// the parser itself and accepted everywhere.
var globalFlags = []Flag{
	{Name: "version", Usage: "Print skl's version and the commit it was built from"},
}

var helpFlag = Flag{Name: "help", Usage: "Show this help, and change nothing"}

// Run runs skl with the process's arguments and streams and returns the exit
// code. The first SIGINT or SIGTERM cancels the context, so the command
// finishes or undoes its current step; a second one stops at once.
func Run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop() // restore the default handlers: the next signal kills
	}()
	return Main(ctx, os.Args[1:], Streams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, os.Getenv)
}

// Main runs one command line and returns its exit code.
func Main(ctx context.Context, args []string, st Streams, getenv func(string) string) int {
	c := &cli{st: st, getenv: getenv}
	c.cmds = c.commands()
	return c.run(ctx, args)
}

type cli struct {
	st     Streams
	getenv func(string) string
	cmds   []*Command
	ui     *ui.UI
}

func (c *cli) run(ctx context.Context, args []string) int {
	in, perr := parse(args, c.knownFlags())
	c.ui = ui.New(ui.Options{
		Stdin: c.st.Stdin, Stdout: c.st.Stdout, Stderr: c.st.Stderr, Getenv: c.getenv,
		Yes: in.Has("yes"), Command: ui.CommandLine(args),
	})
	var cmd *Command
	if len(in.Args) > 0 {
		name := in.Args[0]
		in.Args = in.Args[1:]
		if cmd = c.lookup(name); cmd == nil {
			return c.fail(c.unknownCommand(name, "skl "))
		}
	}
	if in.Help {
		c.printHelp(cmd)
		return exitOK
	}
	if err := c.check(cmd, in, perr); err != nil {
		return c.fail(err)
	}
	switch {
	case cmd == nil && in.Has("version"):
		fmt.Fprintln(c.ui.Report(), versionLine())
		return exitOK
	case cmd == nil:
		c.printHelp(nil)
		return exitOK
	}
	return c.fail(cmd.Run(ctx, in))
}

// check returns the first usage mistake: one from the parser, a flag the
// command doesn't take, or the wrong number of arguments.
func (c *cli) check(cmd *Command, in Input, perr error) error {
	hint := "skl help"
	accepted := globalFlags
	if cmd != nil {
		hint = "skl help " + cmd.Name
		accepted = cmd.Flags
	}
	if perr != nil {
		var u *errs.Usage
		if errors.As(perr, &u) && u.Hint == "" {
			u.Hint = hint
		}
		return perr
	}
	for _, name := range in.order {
		if _, ok := find(accepted, name); !ok {
			if cmd != nil {
				return errs.Usagef(hint, "skl %s doesn't take --%s", cmd.Name, name)
			}
			return errs.Usagef(hint, "unknown flag --%s", name)
		}
	}
	if cmd == nil {
		return nil
	}
	switch n := len(in.Args); {
	case n < cmd.MinArgs:
		return errs.Usagef(hint, "skl %s needs %s", cmd.Name, cmd.Args)
	case cmd.MaxArgs >= 0 && n > cmd.MaxArgs:
		return errs.Usagef(hint, "unexpected argument %s", in.Args[cmd.MaxArgs])
	}
	return nil
}

// knownFlags is every flag any command or skl itself takes, so the parser
// knows which ones take a value before it knows the command.
func (c *cli) knownFlags() []Flag {
	flags := append([]Flag(nil), globalFlags...)
	for _, cmd := range c.cmds {
		flags = append(flags, cmd.Flags...)
	}
	return flags
}

func (c *cli) lookup(name string) *Command {
	for _, cmd := range c.cmds {
		if cmd.Name == name {
			return cmd
		}
	}
	return nil
}

// unknownCommand is the usage error for a mistyped command. The hint is
// prefix plus the closest command, or skl help.
func (c *cli) unknownCommand(name, prefix string) error {
	names := make([]string, len(c.cmds))
	for i, cmd := range c.cmds {
		names[i] = cmd.Name
	}
	hint := "skl help"
	if s := suggest(name, names); s != "" {
		hint = prefix + s
	}
	if name == "" || strings.ContainsFunc(name, unicode.IsSpace) {
		name = strconv.Quote(name)
	}
	return errs.Usagef(hint, "unknown command %s", name)
}

// fail prints err as its ✗ line and returns its exit code. Ctrl-C prints
// nothing: the command has already said what it finished or undid.
func (c *cli) fail(err error) int {
	if err == nil {
		return exitOK
	}
	if errors.Is(err, context.Canceled) {
		return exitInterrupted
	}
	code, hint := exitError, ""
	var u *errs.Usage
	var h *errs.Hinted
	switch {
	case errors.As(err, &u):
		code, hint = exitUsage, u.Hint
	case errors.As(err, &h):
		hint = h.Hint
	}
	c.ui.Error(err.Error(), hint)
	return code
}

// suggest returns the candidate closest to word, if it is close enough to
// be a typo: at most two edits, and fewer edits than half the word.
func suggest(word string, candidates []string) string {
	best, bestDist := "", 3
	for _, cand := range candidates {
		d := editDistance(word, cand)
		if d < bestDist && 2*d < utf8.RuneCountInString(cand) {
			best, bestDist = cand, d
		}
	}
	return best
}

// editDistance is the optimal string alignment distance: insertions,
// deletions, substitutions and swaps of two neighbors each cost one.
func editDistance(a, b string) int {
	s, t := []rune(a), []rune(b)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(s)][len(t)]
}

// indent prefixes every line of s with two spaces.
func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
