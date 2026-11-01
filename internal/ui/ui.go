// Package ui is the only package that writes to the terminal. It prints the
// marks of docs/reference/commands.md "Output" on standard error, reports on
// standard output, and asks the questions of "Questions". It prints plain
// lines and never takes over the screen; Bubble Tea runs only while a
// question waits for an answer on a terminal.
package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// Options are what New needs from the process.
type Options struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	// Yes takes the first answer of every question without showing it.
	Yes bool
	// Command is the command line as typed, such as "skl learn x". A
	// question that can't be asked suggests it again with --yes.
	Command string
	// DryRun sends the plan, the → and + ~ − lines, to standard output: with
	// --dry-run it is what was asked for.
	DryRun bool
}

// UI prints one command's output.
type UI struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer // colorprofile.Writer: escapes are cut to what the terminal shows
	plan        io.Writer // where → and + ~ − go: stderr, or stdout for a dry run
	tty         io.Writer // the raw stderr, for Bubble Tea, which needs its fd and its cursor moves
	profile     colorprofile.Profile
	width       int
	interactive bool // stdin and stderr are terminals
	dumb        bool // TERM=dumb: questions are plain lines, no Bubble Tea
	yes         bool
	command     string
}

// New returns the UI for the given streams and environment.
func New(o Options) *UI {
	u := newUI(o, pickProfile(o.Stderr, o.Getenv), pickWidth(o.Stderr, o.Getenv),
		isTerminal(o.Stdin) && isTerminal(o.Stderr))
	u.dumb = o.Getenv("TERM") == "dumb"
	if o.DryRun {
		u.plan = &colorprofile.Writer{Forward: o.Stdout, Profile: pickProfile(o.Stdout, o.Getenv)}
	}
	return u
}

func newUI(o Options, profile colorprofile.Profile, width int, interactive bool) *UI {
	stderr := &colorprofile.Writer{Forward: o.Stderr, Profile: profile}
	return &UI{
		stdin:       o.Stdin,
		stdout:      o.Stdout,
		stderr:      stderr,
		plan:        stderr,
		tty:         o.Stderr,
		profile:     profile,
		width:       width,
		interactive: interactive,
		yes:         o.Yes,
		command:     o.Command,
	}
}

// pickProfile is commands.md "Color": color only when standard error is a
// terminal, and never with NO_COLOR (any value) or TERM=dumb. NoTTY writes
// no escapes at all; colorprofile's own NO_COLOR handling still writes
// resets, so skl decides that part itself.
func pickProfile(w io.Writer, getenv func(string) string) colorprofile.Profile {
	if !isTerminal(w) || getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return colorprofile.NoTTY
	}
	var environ []string
	for _, k := range []string{"TERM", "COLORTERM", "CLICOLOR", "CLICOLOR_FORCE", "TMUX"} {
		if v := getenv(k); v != "" {
			environ = append(environ, k+"="+v)
		}
	}
	return colorprofile.Detect(w, environ)
}

// pickWidth is the terminal's width, else COLUMNS, else 80.
func pickWidth(w io.Writer, getenv func(string) string) int {
	if f, ok := w.(fder); ok {
		if n, _, err := term.GetSize(int(f.Fd())); err == nil && n > 0 {
			return n
		}
	}
	if n, err := strconv.Atoi(getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return 80
}

type fder interface{ Fd() uintptr }

func isTerminal(f any) bool {
	fd, ok := f.(fder)
	return ok && term.IsTerminal(int(fd.Fd()))
}

// Kind is what happened to a loaded skill.
type Kind int

// The kinds of Change, with their marks.
const (
	Loaded   Kind = iota // +
	Replaced             // ~ replaced or updated
	Unloaded             // −
)

// The marks, each with its color. Only the mark is colored; the text stays
// plain, so the line reads the same without color.
var (
	markPlan     = mark("→", lipgloss.NewStyle().Foreground(lipgloss.Cyan))
	markStep     = mark("◇", lipgloss.NewStyle().Faint(true))
	markLoaded   = mark("+", lipgloss.NewStyle().Foreground(lipgloss.Green))
	markReplaced = mark("~", lipgloss.NewStyle().Foreground(lipgloss.Yellow))
	markUnloaded = mark("−", lipgloss.NewStyle().Foreground(lipgloss.Red))
	markAsk      = mark("?", lipgloss.NewStyle().Foreground(lipgloss.Magenta))
	markWarn     = mark("!", lipgloss.NewStyle().Foreground(lipgloss.Yellow).Bold(true))
	markError    = mark("✗", lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true))
	markDone     = mark("✓", lipgloss.NewStyle().Foreground(lipgloss.Green))
	selected     = lipgloss.NewStyle().Bold(true)
	changeMarks  = [...]string{Loaded: markLoaded, Replaced: markReplaced, Unloaded: markUnloaded}
)

// mark is a mark rendered once with its style. The stderr writer strips or
// downsamples the escapes for the terminal at hand.
func mark(s string, st lipgloss.Style) string { return st.Render(s) }

// Report is standard output, for what the user asked for: a report, a list,
// a URL, a --dry-run plan, --json, help.
func (u *UI) Report() io.Writer { return u.stdout }

// Plan prints "→ <what skl is about to do>".
func (u *UI) Plan(format string, a ...any) {
	fmt.Fprintln(u.plan, markPlan+" "+fmt.Sprintf(format, a...))
}

// Step prints "◇ <label> <detail>", the label padded to 10 columns so the
// details line up.
func (u *UI) Step(label, detail string) {
	if detail == "" {
		u.line(markStep + " " + label)
		return
	}
	u.line(stepLine(label, detail))
}

// StepHint is Step with what to do next, after "·": on the line if it fits,
// else on the next one.
func (u *UI) StepHint(label, detail, hint string) {
	mark, text, _ := strings.Cut(stepLine(label, detail), " ")
	u.line(u.hinted(mark, text, hint))
}

func stepLine(label, detail string) string {
	return fmt.Sprintf("%s %-10s %s", markStep, label, detail)
}

// Change prints "+ name", "~ name" or "− name", then the detail if any.
func (u *UI) Change(k Kind, name, detail string) {
	if detail != "" {
		name += "   " + detail
	}
	fmt.Fprintln(u.plan, changeMarks[k]+" "+name)
}

// Warn prints "! <msg> · <hint>". A warning never changes the exit code.
func (u *UI) Warn(msg, hint string) { u.line(u.hinted(markWarn, msg, hint)) }

// Error prints "✗ <msg> · <hint>". Only cli calls it, once, for the error a
// command returned.
func (u *UI) Error(msg, hint string) { u.line(u.hinted(markError, msg, hint)) }

// Done prints "✓ <msg>", the result, last.
func (u *UI) Done(msg string) { u.line(markDone + " " + msg) }

func (u *UI) line(s string) { fmt.Fprintln(u.stderr, s) }

// hinted is "<mark> <msg> · <hint>", with the hint on its own line after
// "·" when the whole doesn't fit in the width (commands.md "Output").
func (u *UI) hinted(mark, msg, hint string) string {
	line := mark + " " + msg
	if hint == "" {
		return line
	}
	if one := line + " · " + hint; lipgloss.Width(one) <= u.width {
		return one
	}
	return line + "\n  · " + hint
}

// shellQuote quotes the words of a command line that a shell would split
// or expand, so a hint can be pasted back as it is.
func shellQuote(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		// ~ expands and # starts a comment, only at the start of a word.
		if w != "" && w[0] != '~' && w[0] != '#' && !strings.ContainsFunc(w, unsafeRune) {
			q[i] = w
			continue
		}
		q[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}

func unsafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case strings.ContainsRune("-_./:@=+,%^#~", r), r > 0x7f: // non-ASCII letters, such as …
		return false
	}
	return true
}

// CommandLine is "skl" followed by args, each quoted when a shell would
// change it: the form a hint repeats.
func CommandLine(args []string) string {
	if len(args) == 0 {
		return "skl"
	}
	return "skl " + shellQuote(args)
}
