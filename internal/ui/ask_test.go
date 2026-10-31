package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	enter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	right    = tea.KeyPressMsg{Code: tea.KeyRight}
	left     = tea.KeyPressMsg{Code: tea.KeyLeft}
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	ctrlC    = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// press sends keys to a question and returns the model and whether the
// last key quit the program.
func press(q question, keys ...tea.Msg) (question, bool) {
	var cmd tea.Cmd
	for _, k := range keys {
		var m tea.Model
		m, cmd = q.Update(k)
		q = m.(question)
	}
	if cmd == nil {
		return q, false
	}
	_, quit := cmd().(tea.QuitMsg)
	return q, quit
}

var landIt = Question{Prompt: "Land it", Answers: []string{"replace", "change it", "save as review/pr-review-sec", "discard"}}

// commands.md "Questions": "A question shows its answers; the first one is
// the default, taken with Enter." ← → and Tab move; Ctrl-C takes the last
// answer.
func TestQuestionKeys(t *testing.T) {
	for _, tt := range []struct {
		name     string
		keys     []tea.Msg
		cur      int
		canceled bool
	}{
		{"enter takes the first", []tea.Msg{enter}, 0, false},
		{"right enter", []tea.Msg{right, enter}, 1, false},
		{"tab tab enter", []tea.Msg{tab, tab, enter}, 2, false},
		{"stops at the last", []tea.Msg{right, right, right, right, right, enter}, 3, false},
		{"left stops at the first", []tea.Msg{left, enter}, 0, false},
		{"right left", []tea.Msg{right, right, left, enter}, 1, false},
		{"shift+tab", []tea.Msg{tab, tab, shiftTab, enter}, 1, false},
		// "Ctrl-C at a question takes its last answer."
		{"ctrl+c takes the last", []tea.Msg{ctrlC}, 3, true},
		{"ctrl+c after moving", []tea.Msg{right, ctrlC}, 3, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, quit := press(newQuestion(landIt, 80), tt.keys...)
			if !quit || !m.done || m.cur != tt.cur || m.canceled != tt.canceled {
				t.Errorf("quit %v done %v cur %d canceled %v, want quit, done, cur %d canceled %v",
					quit, m.done, m.cur, m.canceled, tt.cur, tt.canceled)
			}
		})
	}
}

func TestQuestionIgnoresOtherKeys(t *testing.T) {
	m, quit := press(newQuestion(addIt, 80), tea.KeyPressMsg{Code: 'x', Text: "x"}, tea.KeyPressMsg{Code: tea.KeyUp})
	if quit || m.done || m.cur != 0 {
		t.Errorf("quit %v done %v cur %d", quit, m.done, m.cur)
	}
}

// The views, before and after an answer, at 80 and 40 columns: "? Add it?
// › yes   no" (commands.md "Output"), the answers on their own line when
// they don't fit, and the line collapsed to the answer.
func TestQuestionView(t *testing.T) {
	var b strings.Builder
	view := func(title string, m question) {
		b.WriteString("== " + title + "\n" + ansi.Strip(m.View().Content) + "\n")
	}
	view("Add it? at 80", newQuestion(addIt, 80))
	m, _ := press(newQuestion(addIt, 80), right)
	view("Add it? after →", m)
	m, _ = press(newQuestion(addIt, 80), right, enter)
	view("Add it? answered no", m)
	view("Land it at 80", newQuestion(landIt, 80))
	view("Land it at 40", newQuestion(landIt, 40))
	m, _ = press(newQuestion(landIt, 80), tea.WindowSizeMsg{Width: 40})
	view("Land it resized to 40", m)
	m, _ = press(newQuestion(landIt, 40), ctrlC)
	view("Land it after Ctrl-C", m)
	golden(t, "question", b.String())
}

// commands.md "Output": "With TERM=dumb, a question is a plain line, `? Add it?
// yes / no:`: type an answer, or press Enter for the first one." Read
// from stdin. "the first one is the default, taken with Enter"; an answer
// that isn't one of them asks again.
func TestAskDumb(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		want        int
		prompts     int
	}{
		{"enter takes the first", "\n", 0, 1},
		{"an answer", "no\n", 1, 1},
		{"any case, spaces", "  No \n", 1, 1},
		{"unknown asks again", "maybe\nyes\n", 0, 2},
		{"a long answer", "maybe\n\nx\nsave as review/pr-review-sec\n", 0, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stderr strings.Builder
			u := newUI(Options{Stdin: strings.NewReader(tt.input), Stdout: io.Discard, Stderr: &stderr},
				colorprofile.NoTTY, 80, true)
			u.dumb = true
			q := addIt
			if tt.name == "a long answer" {
				q = landIt
			}
			i, err := u.Ask(context.Background(), q)
			if err != nil || i != tt.want {
				t.Errorf("Ask = %d, %v; want %d", i, err, tt.want)
			}
			if n := strings.Count(stderr.String(), "? "+q.Prompt); n != tt.prompts {
				t.Errorf("asked %d times, want %d: %q", n, tt.prompts, stderr.String())
			}
		})
	}
	var stderr strings.Builder
	u := newUI(Options{Stdin: strings.NewReader("maybe\n"), Stdout: io.Discard, Stderr: &stderr}, colorprofile.NoTTY, 80, true)
	u.dumb = true
	if _, err := u.Ask(context.Background(), addIt); !errors.Is(err, io.EOF) {
		t.Errorf("EOF: err %v", err)
	}
	golden(t, "question-dumb", stderr.String())
}

// "Ctrl-C at a question takes its last answer": in cooked mode Ctrl-C is
// SIGINT, which cancels ctx.
func TestAskDumbCanceled(t *testing.T) {
	var stderr strings.Builder
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	u := newUI(Options{Stdin: pr, Stdout: io.Discard, Stderr: &stderr}, colorprofile.NoTTY, 80, true)
	u.dumb = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	i, err := u.Ask(ctx, landIt)
	if i != 3 || !errors.Is(err, context.Canceled) {
		t.Errorf("Ask = %d, %v; want 3, context.Canceled", i, err)
	}
}
