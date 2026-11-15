package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

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
		return a.resolve(ctx, c.Vault, ce, saveSides)
	}
	if saveErr != nil && !errors.Is(saveErr, context.Canceled) {
		return a.shownPaths(saveErr)
	}
	return saveErr
}

// pendingSave is saveFirst as a --dry-run sees it: what it would commit and save,
// and the vault commit that would leave, written as git objects only.
type pendingSave struct {
	edits []string     // hand edits it would commit
	saves []load.Saved // copies it would save
	head  string       // the vault's HEAD now
	at    string       // the vault as the save would leave it: head if it does nothing
}

// previewSave works out what saveFirst would do, in the same order: the hand
// edits, then the copies saved onto them. It changes no ref, file or copy. A
// conflict the save would meet is a *vault.ConflictError, with the edits still
// in the returned pendingSave.
func (a *App) previewSave(ctx context.Context, v *vault.Vault, st *state.State, root string) (*pendingSave, error) {
	head, err := v.Head(ctx)
	if err != nil {
		return nil, err
	}
	edits, err := v.HandEdits(ctx)
	if err != nil {
		return nil, err
	}
	p := &pendingSave{edits: edits, head: head, at: head}
	if len(edits) > 0 {
		if p.at, err = v.Pending(ctx); err != nil {
			return nil, err
		}
	}
	at, saves, err := load.Preview(ctx, load.SaveInput{
		Vault: v, HomeRoot: root, Targets: a.cfg.Targets, Current: st.Scopes[state.User].Copies,
	}, p.at)
	if err != nil {
		return p, err
	}
	p.at, p.saves = at, saves
	return p, nil
}

// any reports whether the real run would save or commit something first.
func (p *pendingSave) any() bool { return len(p.edits) > 0 || len(p.saves) > 0 }

// recorded makes st, and the plan s makes, see the copies as the save would
// record them.
func (p *pendingSave) recorded(st *state.State, s *selection) {
	recordSaved(st, p.saves)
	for _, sv := range p.saves {
		s.saved = append(s.saved, state.Copy{
			Name: sv.Name, Target: sv.Target, Skill: sv.Skill, Commit: sv.Commit, Tree: sv.Tree,
		})
	}
}

// dryRunSave is the start of command's --dry-run plan: the → lines for what the
// real run would commit and save first (docs/reference/commands.md "Dry run").
// stop is a conflict that save would meet: it's warned about, and the command's
// own plan can't follow, since the real run stops there to resolve it.
func (a *App) dryRunSave(ctx context.Context, v *vault.Vault, st *state.State, root, command string) (p *pendingSave, stop bool, err error) {
	p, err = a.previewSave(ctx, v, st, root)
	if ce, ok := errors.AsType[*vault.ConflictError](err); ok {
		a.planCommit(p.edits, nil)
		a.ui.Warn(ce.Error(), "skl "+command)
		return p, true, nil
	}
	if err != nil {
		return nil, false, a.planError(err)
	}
	a.planCommit(p.edits, p.saves)
	return p, false, nil
}

// recordSave writes the saved copies' records into state.json and makes the save's
// generation.
func (a *App) recordSave(ctx context.Context, c *Change, saved []load.Saved, command string) error {
	if _, ok := c.State.Scopes[state.User]; ok && len(saved) > 0 {
		recordSaved(c.State, saved)
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

// recordSaved points the user scope's copy records at the commits and trees they
// were saved as, in st only.
func recordSaved(st *state.State, saved []load.Saved) {
	sc, ok := st.Scopes[state.User]
	if !ok {
		return
	}
	sc.Copies = slices.Clone(sc.Copies)
	for i, cp := range sc.Copies {
		for _, s := range saved {
			if cp.Target == s.Target && cp.Name == s.Name && cp.Skill == s.Skill {
				sc.Copies[i].Commit, sc.Copies[i].Tree = s.Commit, s.Tree
			}
		}
	}
	st.Scopes[state.User] = sc
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

// planCommit is the plan's step 1: the hand edits it would commit, then the copies
// it would save.
func (a *App) planCommit(edits []string, saves []load.Saved) {
	if len(edits) > 0 {
		a.ui.Plan("%s", planLine("commit", strings.Join(edits, ", ")))
	}
	for _, s := range savedLines(saves) {
		a.ui.Plan("%s", planLine("save", s.Name+" → "+s.Skill))
	}
}

// planLine is a → line laid out like a ◇ step: the label padded so details line up.
func planLine(label, detail string) string {
	return fmt.Sprintf("%-10s %s", label, detail)
}
