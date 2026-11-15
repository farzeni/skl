package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Rollback is skl rollback [N] (docs/guide/rollback.md "Go back"): the vault's
// skills and files that changed since generation N are put back in revert
// commits, and N's active skill set is loaded again. It's a generation of its
// own, so another rollback undoes it. It never rewrites history.
//
// The target is picked before the frame saves edits first, so a save this
// command makes is kept, as rollback.md says of any save, unless the rollback
// reverts the same skill. Only the user scope is restored: no command changes a
// project scope yet.
func (a *App) Rollback(ctx context.Context, args []string, dryRun bool) error {
	if _, err := a.openVault(); err != nil {
		return err
	}
	target, last, err := a.rollbackTarget(ctx, args)
	if err != nil {
		return err
	}
	r := &rollback{to: target, from: last}
	if dryRun {
		return a.rollbackDryRun(ctx, r)
	}
	return a.Changing(ctx, "rollback "+strconv.Itoa(target.N), func(ctx context.Context, c *Change) (bool, error) {
		return a.rollBack(ctx, c, r)
	})
}

// rollback is one run: back to generation to, undoing everything after it up to
// from, the newest generation when the command started.
type rollback struct {
	to, from state.Generation
}

// rollbackTarget reads the generation to go back to and the newest one. With no
// N it's the one before the newest.
func (a *App) rollbackTarget(ctx context.Context, args []string) (target, last state.Generation, err error) {
	newest, err := a.store.LastGeneration(ctx)
	if err != nil {
		return target, last, a.generationError(err)
	}
	n := newest - 1
	if len(args) > 0 {
		if n, err = strconv.Atoi(args[0]); err != nil || n < 0 || args[0] != strconv.Itoa(n) {
			return target, last, errs.Usagef("skl history", "%s isn't a generation number", ui.Clean(args[0]))
		}
	} else if n < 1 {
		return target, last, errs.Hint(errors.New("nothing to go back to before generation 1"), "skl history")
	}
	if target, err = a.store.Generation(ctx, n); err != nil {
		if _, ok := errors.AsType[*state.NoGenerationError](err); ok {
			return target, last, errs.Hint(err, "skl history")
		}
		return target, last, hinted(a.generationError(err), "skl history")
	}
	if last, err = a.store.Generation(ctx, newest); err != nil {
		return target, last, hinted(a.generationError(err), "skl history")
	}
	return target, last, nil
}

// selection is loading the target's active skill set, as skl use would.
func (r *rollback) selection() selection {
	selectors := r.to.Scopes[state.User].Selectors
	return selection{
		verb:  "rollback",
		label: "back to generation",
		next: func([]string, []selector.Selector, *vault.Index) ([]string, []note, error) {
			return selectors, nil, nil
		},
	}
}

// reverted is one revert commit.
type reverted struct{ unit, commit string }

// rollBack is the command's body: the revert commits, then the load. If either
// fails, or the frame can't record the generation, the vault goes back to where
// it was, with its history as it was.
func (a *App) rollBack(ctx context.Context, c *Change, r *rollback) (bool, error) {
	v := c.Vault
	before, err := v.Head(ctx)
	if err != nil {
		return false, err
	}
	undo := func() { v.ResetTo(context.WithoutCancel(ctx), before) }
	pulled, err := a.pulled(ctx, v, r)
	if err != nil {
		return false, err
	}
	units, err := v.Units(ctx, r.to.VaultCommit, r.from.VaultCommit)
	if err != nil {
		return false, err
	}
	var done []reverted
	for _, u := range units {
		msg := fmt.Sprintf("skl: revert %s to generation %d", u, r.to.N)
		commit, err := v.Revert(ctx, r.to.VaultCommit, u, msg)
		if err != nil {
			undo()
			return false, err
		}
		if commit != "" {
			done = append(done, reverted{unit: u, commit: commit})
		}
	}
	root, err := a.homeRoot()
	if err != nil {
		undo()
		return false, err
	}
	s := r.selection()
	next, err := a.prepare(ctx, v, c.State, s, nil, root)
	if err != nil {
		undo()
		return false, err
	}
	if !next.changed && len(done) == 0 {
		a.reportRollback(r, next, nil, nil)
		return false, nil
	}
	a.reportRollback(r, next, done, nil)
	if pulled > 0 && len(done) > 0 {
		a.ui.Warn("reverts "+commits(pulled)+" pulled by skl sync", "skl rollback to undo")
	}
	if next.changed {
		if err := a.apply(ctx, c, next); err != nil {
			undo()
			return false, err
		}
	}
	undoLoad := c.Undo
	c.Undo = func() {
		if undoLoad != nil {
			undoLoad()
		}
		undo()
	}
	return true, nil
}

// pulled counts the commits skl sync pulled in the generations r undoes, so the
// rollback can warn that it reverts another machine's work (rollback.md "Go
// back"): for each sync among them, what it brought in since the generation
// before it.
func (a *App) pulled(ctx context.Context, v *vault.Vault, r *rollback) (int, error) {
	total := 0
	prev := r.to
	for n := r.to.N + 1; n <= r.from.N; n++ {
		g, err := a.store.Generation(ctx, n)
		if _, ok := errors.AsType[*state.NoGenerationError](err); ok {
			continue
		}
		if err != nil {
			return 0, hinted(a.generationError(err), "skl history")
		}
		if g.Command == "sync" && prev.VaultCommit != "" && g.VaultCommit != "" {
			k, err := v.Pulled(ctx, prev.VaultCommit, g.VaultCommit)
			if err != nil {
				return 0, err
			}
			total += k
		}
		prev = g
	}
	return total, nil
}

// reportRollback prints the plan: "→ back to generation N", what's loaded and
// unloaded, then a ◇ line for each revert commit, or, for a dry run, a → line for
// each skill or file it would revert. The warnings come last.
func (a *App) reportRollback(r *rollback, next *loadNext, done []reverted, wouldRevert []string) {
	revertedUnits := slices.Clone(wouldRevert)
	for _, d := range done {
		revertedUnits = append(revertedUnits, d.unit)
	}
	if next.changed || len(revertedUnits) > 0 {
		a.ui.Plan("back to generation %d", r.to.N)
		if next.changed {
			a.ui.Nested(nestedChanges(next.plan.Changes, revertedUnits))
		}
	}
	for _, d := range done {
		a.ui.Step("reverted", fmt.Sprintf("%s (revert commit %s)", d.unit, shortCommit(d.commit)))
	}
	for _, u := range wouldRevert {
		a.ui.Plan("revert %s", u)
	}
	for _, n := range next.notes {
		a.ui.Warn(n.msg, ui.Clean(n.hint))
	}
}

// nestedChanges are the plan's lines by vault skill, for under a → line. A loaded
// skill that's only updated because it's reverted has no line: its revert line
// says so (rollback.md "Go back" shows the one, not both).
func nestedChanges(changes []load.Change, reverted []string) []ui.Item {
	items := make([]ui.Item, 0, len(changes))
	for _, c := range changes {
		if c.Kind == load.Replace && c.Detail == "updated" && slices.Contains(reverted, c.Skill) {
			continue
		}
		items = append(items, ui.Item{Kind: uiKind(c.Kind), Name: c.Skill, Detail: c.Detail})
	}
	return items
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// rollbackDryRun prints the plan. It takes no lock and changes nothing: the vault
// as the reverts would leave it is written to a temporary folder, and the plan is
// made from there.
func (a *App) rollbackDryRun(ctx context.Context, r *rollback) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	if err := a.notMerging(ctx, v); err != nil {
		return err
	}
	st, err := a.store.Load(ctx)
	if err != nil {
		return a.stateError(err)
	}
	retry := "skl rollback " + strconv.Itoa(r.to.N) + " --dry-run"
	units, err := v.Units(ctx, r.to.VaultCommit, r.from.VaultCommit)
	if err != nil {
		return hinted(err, retry)
	}
	tmp, err := os.MkdirTemp("", "skl-rollback-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	dir := filepath.Join(tmp, "vault")
	if err := v.Preview(ctx, r.to.VaultCommit, units, dir); err != nil {
		return hinted(err, retry)
	}
	idx, err := vault.IndexAt(ctx, dir)
	if err != nil {
		return hinted(err, retry)
	}
	head, err := v.Head(ctx)
	if err != nil {
		return err
	}
	would, err := v.Reverts(ctx, r.to.VaultCommit, units)
	if err != nil {
		return hinted(err, retry)
	}
	root, err := a.homeRoot()
	if err != nil {
		return err
	}
	next, err := a.prepareAt(ctx, vaultView{dir: dir, head: head, idx: idx}, st, r.selection(), nil, root)
	if err != nil {
		return hinted(err, retry)
	}
	a.reportRollback(r, next, nil, would)
	if !next.changed && len(would) == 0 {
		a.ui.Done("nothing to change")
	}
	return nil
}
