package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/farzeni/skl/internal/errs"
)

// golden compares got with testdata/<name>.golden, and rewrites the file
// when UPDATE=1 is set. Escapes are shown as ␛ so the file reads in a diff.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = strings.ReplaceAll(got, "\x1b", "␛")
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s: got\n%s\nwant\n%s", path, got, want)
	}
}

type streams struct{ stdout, stderr bytes.Buffer }

func testUI(profile colorprofile.Profile, width int, o Options) (*UI, *streams) {
	s := &streams{}
	o.Stdin, o.Stdout, o.Stderr = strings.NewReader(""), &s.stdout, &s.stderr
	return newUI(o, profile, width, false), s
}

// every prints each mark once, with the docs' examples, and lines long
// enough to wrap at 60 columns.
func every(u *UI) {
	u.Plan("copy into %s", "someone/pdf")
	u.Step("saved", "pr-review → review/pr-review")
	u.StepMore("SKILL.md, evals/evals.json")
	u.Step("committed", "sets/backend.toml")
	u.Step("pushed", "")
	u.Change(Loaded, "pdf", "")
	u.Change(Replaced, "pr-review", "someone/pr-review replaces review/pr-review")
	u.Change(Unloaded, "writing/changelog", "")
	u.Warn("2 changes not pushed", "skl sync")
	u.Warn("pdf   hidden in opencode by ~/.agents/skills/pdf", "skl drop someone/pdf")
	u.Error("someone/agent-skills/pdf is someone else's skill", "skl learn someone/agent-skills/pdf")
	u.Error("can't reach github.com (Could not resolve host)", "")
	u.Done("generation 14 · skl rollback to undo")
}

// commands.md "Output": the marks → ◇ + ~ − ! ✗ ✓; "A command that doesn't
// fit on the line moves to the next one, after ·"; "The marks themselves
// stay the same" without color.
func TestMarks(t *testing.T) {
	for _, tt := range []struct {
		name    string
		profile colorprofile.Profile
		width   int
	}{
		{"marks-80", colorprofile.NoTTY, 80},
		{"marks-60", colorprofile.NoTTY, 60},
		{"marks-color", colorprofile.ANSI, 80},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, s := testUI(tt.profile, tt.width, Options{})
			every(u)
			if s.stdout.Len() != 0 {
				t.Errorf("marks went to stdout: %q", s.stdout.String())
			}
			golden(t, tt.name, s.stderr.String())
		})
	}
}

// commands.md "Color": "The marks themselves stay the same."
func TestColorKeepsMarks(t *testing.T) {
	plain, sp := testUI(colorprofile.NoTTY, 80, Options{})
	color, sc := testUI(colorprofile.TrueColor, 80, Options{})
	every(plain)
	every(color)
	if strings.Contains(sp.stderr.String(), "\x1b") {
		t.Errorf("NoTTY wrote escapes: %q", sp.stderr.String())
	}
	if !strings.Contains(sc.stderr.String(), "\x1b[") {
		t.Fatalf("color wrote no escapes")
	}
	if got := ansi.Strip(sc.stderr.String()); got != sp.stderr.String() {
		t.Errorf("color without escapes = %q, want %q", got, sp.stderr.String())
	}
}

// commands.md "Where output goes": "What you asked for goes to standard
// output … Everything else, the marks of a running command and its
// questions, goes to standard error."
func TestReportIsStdout(t *testing.T) {
	u, s := testUI(colorprofile.NoTTY, 80, Options{})
	fmt.Fprint(u.Report(), "report\n")
	u.Done("nothing to change")
	if s.stdout.String() != "report\n" || s.stderr.String() != "✓ nothing to change\n" {
		t.Errorf("stdout %q, stderr %q", s.stdout.String(), s.stderr.String())
	}
}

// commands.md "Color": "skl colors its marks only when the output is a
// terminal. Setting NO_COLOR to a non-empty value or TERM=dumb turns color off."
// A buffer is never a terminal; the terminal cases run in a pty, in the
// card's proofs.
func TestPickProfile(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, getenv := range []func(string) string{
		env("TERM", "xterm-256color"),
		env("TERM", "xterm-256color", "CLICOLOR_FORCE", "1"),
	} {
		if p := pickProfile(&bytes.Buffer{}, getenv); p != colorprofile.NoTTY {
			t.Errorf("not a terminal: profile %v, want NoTTY", p)
		}
	}
}

func TestPickWidth(t *testing.T) {
	getenv := func(v string) func(string) string { return func(string) string { return v } }
	for _, tt := range []struct {
		columns string
		want    int
	}{{"60", 60}, {"", 80}, {"x", 80}, {"0", 80}} {
		if got := pickWidth(&bytes.Buffer{}, getenv(tt.columns)); got != tt.want {
			t.Errorf("COLUMNS=%q: width %d, want %d", tt.columns, got, tt.want)
		}
	}
}

var addIt = Question{Prompt: "Add it?", Answers: []string{"yes", "no"}}

// commands.md "Questions": "--yes takes the first answer to every question
// without showing it."
func TestAskYes(t *testing.T) {
	u, s := testUI(colorprofile.NoTTY, 80, Options{Yes: true, Command: "skl learn x"})
	i, err := u.Ask(context.Background(), addIt)
	if i != 0 || err != nil || s.stderr.Len() != 0 || s.stdout.Len() != 0 {
		t.Errorf("Ask = %d, %v; stderr %q, stdout %q", i, err, s.stderr.String(), s.stdout.String())
	}
}

// commands.md "Questions": "Without a terminal and without --yes, a
// question stops the command before it changes anything", as
// "✗ Add it? needs an answer · skl learn someone/agent-skills/pdf --yes".
func TestAskNoTerminal(t *testing.T) {
	u, s := testUI(colorprofile.NoTTY, 80, Options{Command: CommandLine([]string{"learn", "someone/agent-skills/pdf"})})
	_, err := u.Ask(context.Background(), addIt)
	var h *errs.Hinted
	if !errors.Is(err, ErrNeedsAnswer) || !errors.As(err, &h) {
		t.Fatalf("Ask err = %v, want ErrNeedsAnswer with a hint", err)
	}
	if s.stderr.Len() != 0 {
		t.Errorf("Ask printed %q; the ✗ line is cli's", s.stderr.String())
	}
	u.Error(err.Error(), h.Hint)
	golden(t, "needs-an-answer", s.stderr.String())
}

func TestCommandLine(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "skl"},
		{[]string{"learn", "someone/agent-skills/pdf"}, "skl learn someone/agent-skills/pdf"},
		{[]string{"use", "-review/test-gaps", "@base", "--scope=project"}, "skl use -review/test-gaps @base --scope=project"},
		{[]string{"learn", "pdf", "add a section on tables"}, "skl learn pdf 'add a section on tables'"},
		{[]string{"learn", "pdf", "don't"}, `skl learn pdf 'don'\''t'`},
		{[]string{"learn", "~/code/x", ""}, "skl learn '~/code/x' ''"},
		{[]string{"open", "https://git.example.com/a/b#v1"}, "skl open https://git.example.com/a/b#v1"},
	} {
		if got := CommandLine(tt.args); got != tt.want {
			t.Errorf("CommandLine(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// commands.md "Output": "A command that doesn't fit on the line moves to
// the next one, after ·; one that fits stays on the line." Width counts
// columns, not bytes.
func TestHinted(t *testing.T) {
	for _, tt := range []struct {
		msg, hint string
		width     int
		want      string
	}{
		{"unknown command stauts", "skl status", 80, "✗ unknown command stauts · skl status\n"},
		{"boom", "", 80, "✗ boom\n"},
		{"another skl is running (pid 4182)", "skl sync, once it's done", 40, "✗ another skl is running (pid 4182)\n  · skl sync, once it's done\n"},
		{"abc", "skl x", 13, "✗ abc · skl x\n"},
		{"abc", "skl x", 12, "✗ abc\n  · skl x\n"},
		{"abc", "skl x\n    skl y", 200, "✗ abc\n  · skl x\n    skl y\n"},
	} {
		u, s := testUI(colorprofile.NoTTY, tt.width, Options{})
		u.Error(tt.msg, tt.hint)
		if got := s.stderr.String(); got != tt.want {
			t.Errorf("Error(%q, %q) at %d = %q, want %q", tt.msg, tt.hint, tt.width, got, tt.want)
		}
	}
}

// commands.md "Output": "A command that doesn't fit on the line moves to the next
// one, after ·; one that fits stays on the line."
func TestStepHint(t *testing.T) {
	for _, tt := range []struct {
		width int
		want  string
	}{
		{80, "◇ LICENSE    MIT, for the skills you write · edit LICENSE to change it\n"},
		{60, "◇ LICENSE    MIT, for the skills you write\n  · edit LICENSE to change it\n"},
	} {
		u, s := testUI(colorprofile.NoTTY, tt.width, Options{})
		u.StepHint("LICENSE", "MIT, for the skills you write", "edit LICENSE to change it")
		if got := s.stderr.String(); got != tt.want {
			t.Errorf("width %d: %q, want %q", tt.width, got, tt.want)
		}
	}
}

// commands.md "Where output goes": "What you asked for goes to standard output:
// ... a --dry-run plan". "Everything else, the marks of a running command and its
// questions, goes to standard error."
func TestDryRunPlanGoesToStdout(t *testing.T) {
	var stdout, stderr strings.Builder
	u := New(Options{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, DryRun: true,
		Getenv: func(string) string { return "" },
	})
	u.Plan("new vault %s", "you/skills")
	u.Change(Loaded, "pdf", "")
	u.Warn("careful", "skl sync")
	u.Done("nothing to change")
	if got, want := stdout.String(), "→ new vault you/skills\n+ pdf\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "! careful · skl sync\n✓ nothing to change\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// What comes from a vault can be someone else's, and reaches the terminal as text.
func TestClean(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"keeps   three spaces", "keeps   three spaces"},
		{"a\x1b[31mred\x1b[m", "ared"},
		{"title\x1b]0;owned\x07end", "titleend"},
		{"bell\x07", "bell"},
		{"two\nlines\r\tthree", "two lines  three"},
		{"sep\u2028line", "sep line"},
		{"rtl\u202eoverride", "rtloverride"},
		{"café", "café"},
		{"bad\xffutf8", "bad�utf8"},
	}
	for _, tt := range tests {
		if got := Clean(tt.in); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNamesShareALineUpToTheWidth(t *testing.T) {
	var out strings.Builder
	u := newUI(Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}, colorprofile.NoTTY, 20, false)
	u.Names(Loaded, []string{"aaaaa", "bbbbb", "ccccc"})
	u.Names(Loaded, nil)
	if want := "+ aaaaa   + bbbbb\n+ ccccc\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// rollback.md "Go back": "  − writing/changelog"; sync.md: "  + data/sql-explain
// − someone/pdf". Nested lines are indented and mix kinds up to the width.
func TestNestedChanges(t *testing.T) {
	var out strings.Builder
	u := newUI(Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}, colorprofile.NoTTY, 40, false)
	u.Nested([]Item{
		{Kind: Loaded, Name: "data/sql-explain"},
		{Kind: Unloaded, Name: "someone/pdf"},
		{Kind: Unloaded, Name: "writing/changelog"},
		{Kind: Replaced, Name: "review/pr-review", Detail: "updated"},
		{Kind: Loaded, Name: "x/y\x1b[2J"},
	})
	u.Nested(nil)
	want := "  + data/sql-explain   − someone/pdf\n" +
		"  − writing/changelog\n" +
		"  ~ review/pr-review   updated\n" +
		"  + x/y\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestEveryLineIsCleaned(t *testing.T) {
	var out strings.Builder
	u := newUI(Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}, colorprofile.NoTTY, 200, false)
	evil := "x\x1b[2Jy\nz"
	u.Plan("%s", evil)
	u.Step(evil, evil)
	u.Change(Loaded, evil, evil)
	u.Names(Loaded, []string{evil})
	u.Warn(evil, "x\x1b[2Jy")
	u.Error(evil, "x\x1b[2Jy")
	u.Done(evil)
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "[2J") {
		t.Errorf("an escape got through: %q", out.String())
	}
	if lines := strings.Count(out.String(), "\n"); lines > 9 {
		t.Errorf("%d lines: %q", lines, out.String())
	}
}

// commands.md "Where output goes": a report's lines, its marks included, go to
// standard output. "A command that doesn't fit on the line moves to the next one,
// after ·; one that fits stays on the line."
func TestReportLines(t *testing.T) {
	u, s := testUI(colorprofile.NoTTY, 30, Options{})
	u.ReportLine("vault   1 change", "skl sync")
	u.ReportLine(Pad("name", 8), "")
	u.ReportLine(Pad("name", 7), "skl sync")
	u.ReportLine(Pad("a long name that won't fit", 30), "skl sync")
	u.ReportWarn("lint   skipped", "skl import lint")
	want := "vault   1 change · skl sync\n" +
		"name\n" +
		"name    · skl sync\n" +
		"a long name that won't fit\n  · skl sync\n" +
		"! lint   skipped\n  · skl import lint\n"
	if s.stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", s.stdout.String(), want)
	}
	if s.stderr.Len() != 0 {
		t.Errorf("stderr %q", s.stderr.String())
	}
}

// A report's mark has color on a terminal, and the text never does.
func TestReportWarnColor(t *testing.T) {
	u, s := testUI(colorprofile.TrueColor, 80, Options{})
	u.ReportWarn("a\x1b[31mb", "skl sync")
	if !strings.Contains(s.stdout.String(), "\x1b[") {
		t.Errorf("no color: %q", s.stdout.String())
	}
	if got := ansi.Strip(s.stdout.String()); got != "! ab · skl sync\n" {
		t.Errorf("got %q", got)
	}
}

func TestPadCountsColumns(t *testing.T) {
	if got := Pad("日本", 6); got != "日本  " {
		t.Errorf("Pad = %q", got)
	}
	if got := Pad("long", 2); got != "long" {
		t.Errorf("Pad = %q", got)
	}
	if got := Width("a\x1b[31mb"); got != 2 {
		t.Errorf("Width = %d", got)
	}
}

// A name from the vault can't drive a terminal through JSON, and a script still
// reads the real name back.
func TestReportJSONEscapesControls(t *testing.T) {
	u, s := testUI(colorprofile.NoTTY, 80, Options{})
	name := "a\x7f\u009b\u202e\U000E0001\x1b<é日"
	if err := u.ReportJSON(map[string]string{"name": name}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"name\": \"a\\u007f\\u009b\\u202e\\udb40\\udc01\\u001b<é日\"\n}\n"
	if s.stdout.String() != want {
		t.Errorf("got %q, want %q", s.stdout.String(), want)
	}
	var back map[string]string
	if err := json.Unmarshal(s.stdout.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if back["name"] != name {
		t.Errorf("read back %q, want %q", back["name"], name)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// commands.md "skl status > status.txt saves the report": when it can't, that's an
// error, and the first failure is the one reported.
func TestReportErr(t *testing.T) {
	u := newUI(Options{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: io.Discard}, colorprofile.NoTTY, 80, false)
	if u.ReportErr() != nil {
		t.Fatal("error before any write")
	}
	u.ReportLine("a", "")
	u.ReportWarn("b", "c")
	if err := u.ReportErr(); err == nil || err.Error() != "disk full" {
		t.Errorf("ReportErr = %v", err)
	}
}

func TestStepHeld(t *testing.T) {
	var out bytes.Buffer
	u := newUI(Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}, colorprofile.NoTTY, 80, false)
	u.StepHeld("generation", "3")
	if out.Len() != 0 {
		t.Fatalf("held line printed at once: %q", out.String())
	}
	u.Plan("add data")
	u.Done("generation 4")
	if want := "◇ generation 3\n→ add data\n✓ generation 4\n"; out.String() != want {
		t.Errorf("before another line:\n%s\nwant\n%s", out.String(), want)
	}

	out.Reset()
	u.StepHeld("generation", "3")
	u.DropHeld()
	u.Done("generation 3")
	if want := "✓ generation 3\n"; out.String() != want {
		t.Errorf("dropped:\n%s\nwant\n%s", out.String(), want)
	}
}
