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

// selection is loading the target's active skill set, as skl use would. What an
// import adopted is given back rather than unloaded (h.release).
func (r *rollback) selection(h handovers) selection {
	selectors := r.to.Scopes[state.User].Selectors
	return selection{
		verb:    "rollback",
		label:   "back to generation",
		release: h.release,
		next: func([]string, []selector.Selector, *vault.Index) ([]string, []note, error) {
			return selectors, nil, nil
		},
	}
}

// undone reads the generations r goes back over, oldest first. One that's missing
// is skipped.
func (a *App) undone(ctx context.Context, r *rollback) ([]state.Generation, error) {
	var gens []state.Generation
	for n := r.to.N + 1; n <= r.from.N; n++ {
		g, err := a.store.Generation(ctx, n)
		if _, ok := errors.AsType[*state.NoGenerationError](err); ok {
			continue
		}
		if err != nil {
			return nil, hinted(a.generationError(err), "skl history")
		}
		gens = append(gens, g)
	}
	return gens, nil
}

// handovers are the folders a rollback hands back over (docs/guide/rollback.md "Go
// back"): release are copies an import adopted, to leave as plain folders, and
// adopt are plain folders a rollback gave back, to make copies again.
type handovers struct{ release, adopt []state.Copy }

// handoversOf works out the handovers of going back over gens. For each folder the
// oldest generation that handed it over decides, since the rollback restores what
// was there before it: one that adopted it found a plain folder, one that released
// it found a copy.
func handoversOf(gens []state.Generation) handovers {
	type entry struct {
		cp      state.Copy
		release bool
	}
	var order []string
	oldest := map[string]entry{}
	note := func(cp state.Copy, release bool) {
		key := string(cp.Target) + "/" + selector.Fold(cp.Name)
		if _, ok := oldest[key]; !ok {
			order = append(order, key)
		}
		oldest[key] = entry{cp: cp, release: release}
	}
	for _, g := range slices.Backward(gens) {
		for _, cp := range g.Adopted {
			note(cp, true)
		}
		for _, cp := range g.Released {
			note(cp, false)
		}
	}
	var h handovers
	for _, key := range order {
		if e := oldest[key]; e.release {
			h.release = append(h.release, e.cp)
		} else {
			h.adopt = append(h.adopt, e.cp)
		}
	}
	return h
}

// readoptable are the folders of h.adopt that can be made copies again: a plain
// folder is where the target generation had the copy, in a target still
// configured, and the vault at dir holds its skill. Each comes with the copy it
// becomes, at commit. Any other is left alone, and the load treats it as usual.
func (a *App) readoptable(ctx context.Context, h handovers, r *rollback, dir, commit, root string) ([]load.Foreign, []state.Copy, error) {
	if len(h.adopt) == 0 {
		return nil, nil, nil
	}
	foreign, err := load.Foreigners(root, a.cfg.Targets)
	if err != nil {
		return nil, nil, a.shownPaths(err)
	}
	var found []load.Foreign
	var copies []state.Copy
	for _, cp := range h.adopt {
		had := slices.ContainsFunc(r.to.Scopes[state.User].Copies, func(o state.Copy) bool {
			return o.Target == cp.Target && o.Name == cp.Name && o.Skill == cp.Skill
		})
		i := slices.IndexFunc(foreign, func(f load.Foreign) bool { return f.Target == cp.Target && f.Folder == cp.Name && !f.Link })
		if !had || i < 0 {
			continue
		}
		skill := filepath.Join(dir, filepath.FromSlash(cp.Skill))
		if _, err := os.Stat(filepath.Join(skill, "SKILL.md")); err != nil {
			continue
		}
		tree, err := load.SkillTree(ctx, skill)
		if err != nil {
			return nil, nil, err
		}
		found = append(found, foreign[i])
		copies = append(copies, state.Copy{Name: cp.Name, Target: cp.Target, Skill: cp.Skill, Commit: commit, Tree: tree})
	}
	return found, copies, nil
}

// readopt makes each folder the copy it was before a rollback gave it back.
func (a *App) readopt(ctx context.Context, found []load.Foreign, copies []state.Copy) (load.Adoptions, error) {
	var done load.Adoptions
	for i, f := range found {
		cp := copies[i]
		ad, err := load.Adopt(ctx, f, a.paths.Staging, load.Manifest{Source: cp.Skill, Commit: cp.Commit, Tree: cp.Tree})
		if err != nil {
			done.Undo()
			return nil, a.shownPaths(err)
		}
		done = append(done, ad)
	}
	return done, nil
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
	gens, err := a.undone(ctx, r)
	if err != nil {
		return false, err
	}
	pulled, err := a.pulled(ctx, v, r, gens)
	if err != nil {
		return false, err
	}
	h := handoversOf(gens)
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
	head, err := v.Head(ctx)
	if err != nil {
		undo()
		return false, err
	}
	found, copies, err := a.readoptable(ctx, h, r, v.Dir(), head, root)
	if err != nil {
		undo()
		return false, err
	}
	adopted, err := a.readopt(ctx, found, copies)
	if err != nil {
		undo()
		return false, err
	}
	undoVault := undo
	undo = func() { adopted.Undo(); undoVault() }
	s := r.selection(h)
	next, err := a.prepare(ctx, v, c.State, s, nil, root)
	if err != nil {
		undo()
		return false, err
	}
	if !next.changed && len(done) == 0 {
		a.reportRollback(r, next, nil, nil)
		undo()
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
	c.Adopted, c.Released = copies, next.plan.Released
	undoLoad, closeLoad := c.Undo, c.Cleanup
	c.Undo = func() {
		if undoLoad != nil {
			undoLoad()
		}
		undo()
	}
	c.Cleanup = func() {
		if closeLoad != nil {
			closeLoad()
		}
		adopted.Close()
	}
	return true, nil
}

// pulled counts the commits skl sync pulled in the generations r undoes, so the
// rollback can warn that it reverts another machine's work (rollback.md "Go
// back"): for each sync among them, what it brought in since the generation
// before it.
func (a *App) pulled(ctx context.Context, v *vault.Vault, r *rollback, gens []state.Generation) (int, error) {
	total := 0
	prev := r.to
	for _, g := range gens {
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
	gens, err := a.undone(ctx, r)
	if err != nil {
		return err
	}
	h := handoversOf(gens)
	// The folders a real run would adopt again are read as if it had.
	_, copies, err := a.readoptable(ctx, h, r, dir, head, root)
	if err != nil {
		return hinted(err, retry)
	}
	s := r.selection(h)
	s.saved = copies
	next, err := a.prepareAt(ctx, vaultView{dir: dir, head: head, idx: idx}, st, s, nil, root)
	if err != nil {
		return hinted(err, retry)
	}
	a.reportRollback(r, next, nil, would)
	if !next.changed && len(would) == 0 {
		a.ui.Done("nothing to change")
	}
	return nil
}
