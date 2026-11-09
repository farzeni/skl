package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
)

// History is skl history (docs/guide/rollback.md "See what changed"). Like status
// it only reads: it takes no lock and never goes online.
func (a *App) History(ctx context.Context, asJSON bool) error {
	if _, err := a.openVault(); err != nil {
		return err
	}
	gens, err := a.store.Generations(ctx)
	if err != nil {
		return hinted(a.generationError(err), "skl history")
	}
	if asJSON {
		return hinted(a.ui.ReportJSON(historyOf(gens)), "skl history")
	}
	a.printHistory(gens)
	return hinted(a.ui.ReportErr(), "skl history")
}

// generationError is a generation file that can't be read, with ~ for HOME.
func (a *App) generationError(err error) error {
	if !errors.Is(err, state.ErrCorrupt) {
		return err
	}
	text := strings.ReplaceAll(err.Error(), a.paths.Generations, a.shown(a.paths.Generations))
	return errs.Hint(&shownError{Err: err, Text: text}, "move it aside, then run skl history")
}

// printHistory is the list: the number, right-aligned, the age, then the command.
func (a *App) printHistory(gens []state.Generation) {
	if len(gens) == 0 {
		a.ui.ReportLine("no generations yet", "")
		return
	}
	now := a.now()
	ages := make([]string, len(gens))
	numbers, column := 0, len("59m ago") // the age column doesn't move until days
	for i, g := range gens {
		ages[i] = age(now, g.Time)
		numbers = max(numbers, len(strconv.Itoa(g.N)))
		column = max(column, len(ages[i]))
	}
	for i, g := range gens {
		a.ui.ReportLine(fmt.Sprintf("%*d  %s  %s", numbers, g.N, ui.Pad(ages[i], column), g.Command), "")
	}
}

// age is how long ago t was, as rollback.md writes it: "now", then minutes, hours
// and days. A time ahead of the clock is "now".
func age(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", d/time.Minute)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", d/time.Hour)
	}
	return fmt.Sprintf("%dd ago", d/(24*time.Hour))
}

// The JSON of docs/reference/commands.md "JSON".
type (
	historyJSON struct {
		Generations []generationJSON `json:"generations"`
	}
	generationJSON struct {
		Generation int    `json:"generation"`
		Time       string `json:"time"`
		Command    string `json:"command"`
		Commit     string `json:"commit"`
	}
)

func historyOf(gens []state.Generation) historyJSON {
	out := historyJSON{Generations: make([]generationJSON, 0, len(gens))}
	for _, g := range gens {
		out.Generations = append(out.Generations, generationJSON{
			Generation: g.N,
			Time:       g.Time.UTC().Format(time.RFC3339),
			Command:    g.Command,
			Commit:     g.VaultCommit,
		})
	}
	return out
}
