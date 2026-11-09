package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/vault"
)

// saveFirst is the frame's first step: hand edits in the vault are committed, then
// the loaded copies that changed are saved back, and both together are a
// generation of their own, before the command's (docs/guide/skills.md "Edit as
// usual", "Edit the vault directly"). Hand edits go first: a copy merges into a
// clean work tree. It prints a ◇ line for each and "◇ generation N".
func (a *App) saveFirst(ctx context.Context, c *Change) error {
	var did []string // what the generation is, for skl history
	edits, err := c.Vault.HandEdits(ctx)
	if err != nil {
		return err
	}
	if len(edits) > 0 {
		committed, err := c.Vault.Commit(ctx, "skl: commit hand edits")
		if err != nil {
			return err
		}
		if committed {
			list := strings.Join(edits, ", ")
			a.ui.Step("committed", list)
			did = append(did, "committed "+list)
		}
	}
	root, err := a.homeRoot()
	if err != nil {
		return err
	}
	saved, saveErr := load.SaveBack(ctx, load.SaveInput{
		Vault: c.Vault, HomeRoot: root, Targets: a.cfg.Targets, Current: c.State.Scopes[state.User].Copies,
	})
	for _, s := range savedLines(saved) {
		a.ui.Step("saved", s.Name+" → "+s.Skill)
		a.ui.StepMore(strings.Join(s.Files, ", "))
		did = append(did, "saved "+s.Name+" → "+s.Skill)
	}
	if len(did) > 0 {
		// The edits are in the vault now, whatever comes next: Ctrl-C waits for
		// the generation that records them.
		if err := a.recordSave(context.WithoutCancel(ctx), c, saved, strings.Join(did, ", ")); err != nil {
			return errors.Join(saveErr, err)
		}
	}
	if ce, ok := errors.AsType[*vault.ConflictError](saveErr); ok {
		return errs.Hint(ce, "skl sync --continue")
	}
	if saveErr != nil && !errors.Is(saveErr, context.Canceled) {
		return a.shownPaths(saveErr)
	}
	return saveErr
}

// recordSave writes the saved copies' records into state.json and makes the save's
// generation.
func (a *App) recordSave(ctx context.Context, c *Change, saved []load.Saved, command string) error {
	if sc, ok := c.State.Scopes[state.User]; ok && len(saved) > 0 {
		sc.Copies = slices.Clone(sc.Copies)
		for i, cp := range sc.Copies {
			for _, s := range saved {
				if cp.Target == s.Target && cp.Name == s.Name && cp.Skill == s.Skill {
					sc.Copies[i].Commit, sc.Copies[i].Tree = s.Commit, s.Tree
				}
			}
		}
		c.State.Scopes[state.User] = sc
		if err := c.Lock.Save(ctx, c.State); err != nil {
			return err
		}
	}
	g, err := a.addGeneration(ctx, command, c)
	if err != nil {
		return err
	}
	c.saved = g.N
	a.ui.StepHeld("generation", strconv.Itoa(g.N))
	return nil
}

// savedLines is one line per skill: with two targets, a skill saved from both has
// the files of both.
func savedLines(saved []load.Saved) []load.Saved {
	var lines []load.Saved
	for _, s := range saved {
		i := slices.IndexFunc(lines, func(l load.Saved) bool { return l.Name == s.Name && l.Skill == s.Skill })
		if i < 0 {
			s.Files = slices.Clone(s.Files)
			lines = append(lines, s)
			continue
		}
		for _, f := range s.Files {
			if !slices.Contains(lines[i].Files, f) {
				lines[i].Files = append(lines[i].Files, f)
			}
		}
		slices.Sort(lines[i].Files)
	}
	return lines
}
