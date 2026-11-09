package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/gitx"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/ref"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Sync is skl sync (docs/guide/sync.md "What sync does"): the frame saves and
// commits first (step 1), then sync pulls with a merge (2), pushes (3) and reloads
// what's loaded (4). If the network fails, it stops after step 1. --continue
// finishes a merge resolved by hand and goes on from there; --abort undoes it.
func (a *App) Sync(ctx context.Context, mode SyncMode, dryRun bool) error {
	switch {
	case dryRun:
		return a.syncDryRun(ctx)
	case mode == SyncAbort:
		return a.syncAbort(ctx)
	case mode == SyncContinue:
		return a.changing(ctx, "sync", a.finishMerge, a.syncBody)
	}
	return a.Changing(ctx, "sync", a.syncBody)
}

// syncBody is sync after the frame's step 1.
func (a *App) syncBody(ctx context.Context, c *Change) (bool, error) {
	root, err := a.homeRoot()
	if err != nil {
		return false, err
	}
	branch, err := a.fetch(ctx, c.Vault)
	if err != nil {
		return false, err
	}
	before, err := c.Vault.Head(ctx)
	if err != nil {
		return false, err
	}
	merged, err := a.pull(ctx, c.Vault, branch)
	if err != nil {
		return false, err
	}
	pulled := c.pulled + merged
	if pulled > 0 {
		a.ui.Step("pulled", commits(pulled))
	}
	// Step 4 is planned before the push, so a problem in what was pulled, such
	// as a broken skill set, stops before it goes anywhere.
	next, err := a.prepare(ctx, c.Vault, c.State, reloading(), nil, root)
	if err != nil {
		return false, hinted(err, "skl sync")
	}
	changed := pulled > 0 || next.changed
	// Unless sync makes a generation of its own, the save's ✓ line is the last
	// and its ◇ line goes, but only once nothing can fail: a failed push still
	// shows the generation the save made.
	pushed, err := a.push(ctx, c.Vault, branch, merged, before, !changed)
	if err != nil {
		return false, err
	}
	if !changed {
		a.ui.DropHeld()
	}
	if pushed {
		c.Unchanged = "up to date" // sync.md: a push-only sync makes no generation
	}
	a.reportReload(c.State, next)
	if next.changed {
		if err := a.apply(ctx, c, next); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// fetch is the start of step 2: the vault's branch, with origin's fetched.
func (a *App) fetch(ctx context.Context, v *vault.Vault) (string, error) {
	branch, err := v.Branch(ctx)
	if errors.Is(err, vault.ErrDetached) {
		return "", errs.Hint(fmt.Errorf("%s %w", a.shown(v.Dir()), err), "git -C "+a.shown(v.Dir())+" switch <branch>")
	}
	if err != nil {
		return "", err
	}
	origin, err := v.Origin(ctx)
	if err != nil {
		return "", err
	}
	if origin == "" {
		return "", errs.Hint(fmt.Errorf("%s has no remote", a.shown(v.Dir())),
			"git -C "+a.shown(v.Dir())+" remote add origin <repo>")
	}
	if err := v.Fetch(ctx); err != nil {
		return "", a.reachError(err, origin)
	}
	return branch, nil
}

// pull merges what origin has that the vault doesn't, and says how many commits
// that was. A merge, never a rebase: no commit is rewritten. A conflict asks how
// to resolve it (conflict.go).
func (a *App) pull(ctx context.Context, v *vault.Vault, branch string) (int, error) {
	theirs, incoming, err := v.Incoming(ctx, branch)
	if err != nil || len(incoming) == 0 {
		return 0, err
	}
	if err := v.Merge(ctx, theirs); err != nil {
		if ce, ok := errors.AsType[*vault.ConflictError](err); ok {
			return 0, a.resolve(ctx, v, ce, syncSides)
		}
		return 0, err
	}
	return len(incoming), nil
}

// push is step 3, when the vault has commits origin doesn't. If it fails, a merge
// step 2 made is taken back, so the vault is as step 1 left it. It reports
// whether it pushed. dropHeld drops the held ◇ line once the push has worked.
func (a *App) push(ctx context.Context, v *vault.Vault, branch string, pulled int, before string, dropHeld bool) (bool, error) {
	ahead, err := v.Ahead(ctx)
	if err != nil || ahead == 0 {
		return false, err
	}
	if err := v.Push(ctx, branch); err != nil {
		if pulled > 0 {
			v.Unmerge(context.WithoutCancel(ctx), before)
		}
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		origin, _ := v.Origin(context.WithoutCancel(ctx))
		return false, a.reachError(err, origin)
	}
	if dropHeld {
		a.ui.DropHeld()
	}
	a.ui.Step("pushed", "")
	return true, nil
}

// reloading is step 4's selection: the selectors loaded in the user scope, resolved
// again against the vault as it is now. The project scope reloads with --scope
// project's card.
func reloading() selection {
	return selection{
		verb: "sync",
		next: func(active []string, _ []selector.Selector, _ *vault.Index) ([]string, []note, error) {
			return active, nil, nil
		},
	}
}

// reportReload prints the changes nested under the steps above, as group/name,
// then the warnings.
func (a *App) reportReload(st *state.State, next *loadNext) {
	current := st.Scopes[state.User].Copies
	items := make([]ui.Item, 0, len(next.plan.Changes))
	for _, ch := range next.plan.Changes {
		items = append(items, ui.Item{Kind: uiKind(ch.Kind), Name: skillOf(ch, next.plan.Copies, current), Detail: a.detail(ch)})
	}
	a.ui.Nested(items)
	for _, n := range next.notes {
		a.ui.Warn(n.msg, ui.Clean(n.hint))
	}
}

// skillOf is the vault skill a change is about, group/name: the one loaded after,
// or for an unload the one loaded before. A change neither names keeps its folder.
func skillOf(ch load.Change, after, before []state.Copy) string {
	list := after
	if ch.Kind == load.Unload {
		list = before
	}
	if i := slices.IndexFunc(list, func(c state.Copy) bool { return c.Name == ch.Name }); i >= 0 {
		return list[i].Skill
	}
	return ch.Name
}

// reachError is a remote sync can't reach: "✗ can't reach github.com (Could not
// resolve host) · skl sync" (docs/reference/commands.md "Errors").
func (a *App) reachError(err error, origin string) error {
	if !errors.Is(err, gitx.ErrUnreachable) {
		return err
	}
	where := a.shown(origin)
	if r, perr := ref.ParseRepo(origin, a.refOptions("")); perr == nil && r.Host != "" {
		where = r.Host
	}
	reason := ""
	if ge, ok := errors.AsType[*gitx.Error](err); ok {
		reason = gitx.Message(ge.Reason())
	}
	if reason == "" {
		return errs.Hint(fmt.Errorf("can't reach %s", where), "skl sync")
	}
	return errs.Hint(fmt.Errorf("can't reach %s (%s)", where, reason), "skl sync")
}

func commits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}

// syncDryRun prints what sync would commit, save, pull, push and reload. It
// fetches, which changes only origin's branches in the vault, and nothing else:
// no lock, no merge, no push.
func (a *App) syncDryRun(ctx context.Context) error {
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
	root, err := a.homeRoot()
	if err != nil {
		return err
	}
	head, err := v.Head(ctx)
	if err != nil {
		return err
	}
	// Step 1 as it would go: the hand edits on HEAD, then the copies saved onto
	// that, as git objects only.
	edits, err := v.HandEdits(ctx)
	if err != nil {
		return err
	}
	pending, err := v.Pending(ctx)
	if err != nil {
		return err
	}
	pending, saves, err := load.Preview(ctx, load.SaveInput{
		Vault: v, HomeRoot: root, Targets: a.cfg.Targets, Current: st.Scopes[state.User].Copies,
	}, pending)
	if ce, ok := errors.AsType[*vault.ConflictError](err); ok {
		a.planCommit(edits, nil)
		a.ui.Warn(ce.Error(), "skl sync")
		return nil
	}
	if err != nil {
		return a.planError(err)
	}
	// Fetching changes only origin's branches in the vault, which every git fetch
	// may do, so the dry run takes no lock.
	branch, err := a.fetch(ctx, v)
	if err != nil {
		return err
	}
	theirs, incoming, err := v.Incoming(ctx, branch)
	if err != nil {
		return err
	}
	ahead, err := v.Ahead(ctx)
	if err != nil {
		return err
	}

	a.planCommit(edits, saves)
	// The plan reloads against the copies as step 1 would record them.
	recordSaved(st, saves)
	reload := reloading()
	for _, s := range saves {
		reload.saved = append(reload.saved, state.Copy{
			Name: s.Name, Target: s.Target, Skill: s.Skill, Commit: s.Commit, Tree: s.Tree,
		})
	}
	after := pending
	if len(incoming) > 0 {
		a.ui.Plan("%s", planLine("pull", commits(len(incoming))))
		tree, conflicts, err := v.MergedTree(ctx, pending, theirs)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			a.ui.Warn("conflict in "+strings.Join(conflicts, ", "), "skl sync, in a terminal")
			return nil
		}
		after = tree
	}
	if ahead > 0 || pending != head {
		a.ui.Plan("push")
	}
	next, err := a.planReload(ctx, v, st, reload, root, head, after)
	if err != nil {
		return hinted(err, "skl sync")
	}
	a.reportReload(st, next)
	if pending == head && len(incoming) == 0 && ahead == 0 && !next.changed {
		a.ui.Done("nothing to change")
	}
	return nil
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

// planReload plans step 4 against the vault as at, a commit or a tree, would leave
// it: the vault itself if that's HEAD, else its files written to a temporary
// folder, as rollback --dry-run does. Nothing in the vault changes.
func (a *App) planReload(ctx context.Context, v *vault.Vault, st *state.State, s selection, root, head, at string) (*loadNext, error) {
	if at == head {
		return a.prepare(ctx, v, st, s, nil, root)
	}
	tmp, err := os.MkdirTemp("", "skl-sync-")
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	dir := filepath.Join(tmp, "vault")
	if err := v.Snapshot(ctx, at, dir); err != nil {
		return nil, err
	}
	idx, err := vault.IndexAt(ctx, dir)
	if err != nil {
		return nil, err
	}
	return a.prepareAt(ctx, vaultView{dir: dir, head: at, idx: idx}, st, s, nil, root)
}
