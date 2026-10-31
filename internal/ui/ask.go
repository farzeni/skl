package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/farzeni/skl/internal/errs"
)

// ErrNeedsAnswer is a question asked without a terminal and without --yes.
var ErrNeedsAnswer = errors.New("needs an answer")

// Question is one question and its answers. The first answer is the
// default: the one --yes and Enter take.
type Question struct {
	Prompt  string // "Add it?"
	Answers []string
}

// Ask returns the index of the answer picked (commands.md "Questions").
//   - With --yes, it returns 0 and prints nothing.
//   - Without a terminal on stdin and stderr, it returns
//     "✗ <prompt> needs an answer · <command> --yes", before anything changes.
//   - On a terminal, ← → or Tab move and Enter picks; the line then collapses
//     to "? <prompt>   › <answer>". Ctrl-C, or a cancelled ctx, picks the
//     last answer and returns context.Canceled with it.
func (u *UI) Ask(ctx context.Context, q Question) (int, error) {
	if len(q.Answers) == 0 {
		return 0, fmt.Errorf("question %q has no answers", q.Prompt)
	}
	if u.yes {
		return 0, nil
	}
	if !u.interactive {
		return 0, errs.Hint(fmt.Errorf("%s %w", q.Prompt, ErrNeedsAnswer), u.command+" --yes")
	}
	m := newQuestion(q, u.width)
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(u.stdin),
		// The raw stderr: the profile writer would strip the cursor moves
		// under NO_COLOR, and hide the fd Bubble Tea reads the size from.
		// Bubble Tea applies the profile to the view's colors itself.
		tea.WithOutput(u.tty),
		tea.WithColorProfile(u.profile),
		tea.WithoutSignalHandler(), // cli owns signals; in raw mode Ctrl-C is a key
	)
	final, err := p.Run()
	last := len(q.Answers) - 1
	switch {
	case errors.Is(err, tea.ErrProgramKilled) || ctx.Err() != nil:
		// The program stopped before its final view: print the collapsed line.
		m.cur, m.done = last, true
		fmt.Fprint(u.stderr, m.View().Content)
		return last, context.Canceled
	case err != nil:
		return 0, fmt.Errorf("ask %q: %w", q.Prompt, err)
	}
	m = final.(question)
	if m.canceled {
		return last, context.Canceled
	}
	return m.cur, nil
}

// question is the Bubble Tea model of one inline question.
type question struct {
	Question
	width    int
	cur      int
	done     bool
	canceled bool
}

func newQuestion(q Question, width int) question {
	return question{Question: q, width: width}
}

func (m question) Init() tea.Cmd { return nil }

func (m question) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "shift+tab":
			m.cur = max(m.cur-1, 0)
		case "right", "tab":
			m.cur = min(m.cur+1, len(m.Answers)-1)
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c":
			m.cur, m.done, m.canceled = len(m.Answers)-1, true, true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View is "? <prompt>   › yes   no", the answers on the next line when they
// don't fit; once answered, "? <prompt>   › <answer>".
func (m question) View() tea.View {
	head := markAsk + " " + m.Prompt
	if m.done {
		return tea.NewView(head + "   › " + m.Answers[m.cur] + "\n")
	}
	answers := make([]string, len(m.Answers))
	for i, a := range m.Answers {
		if i == m.cur {
			a = "› " + selected.Render(a)
		}
		answers[i] = a
	}
	if one := head + "   " + strings.Join(answers, "   "); lipgloss.Width(one) <= m.width {
		return tea.NewView(one)
	}
	// Answers that don't fit after the prompt go below it, indented, as
	// many on a line as fit.
	var b strings.Builder
	b.WriteString(head)
	line := ""
	for _, a := range answers {
		if line != "" && lipgloss.Width(line+"   "+a) > m.width {
			b.WriteString("\n" + line)
			line = ""
		}
		if line == "" {
			line = "  " + a
		} else {
			line += "   " + a
		}
	}
	b.WriteString("\n" + line)
	return tea.NewView(b.String())
}
