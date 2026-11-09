package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// A conflict, from sync's merge or from a save-back, follows docs/guide/sync.md
// "When both machines changed the same file": skl shows what each side did and
// asks; by hand leaves the merge for skl sync --continue, abort undoes it, and
// without a terminal it's undone too (docs/reference/commands.md "Questions").
// The engine's "with Claude" answer comes first once the engine exists (v0.3).

// SyncMode is what skl sync is asked to do.
type SyncMode int

// The modes of skl sync.
const (
	SyncRun      SyncMode = iota // skl sync
	SyncContinue                 // skl sync --continue
	SyncAbort                    // skl sync --abort
)

// sides names the two sides of a merge in the conflict lines, in the order they
// print.
type sides struct {
	ours, theirs string
	theirsFirst  bool
}

var (
	// syncSides are sync's merge: HEAD is this machine, origin the other one.
	syncSides = sides{ours: "this machine", theirs: "other machine"}
	// saveSides are a save-back: HEAD is the vault, the synthetic commit the loaded copy.
	saveSides = sides{ours: "vault", theirs: "loaded", theirsFirst: true}
)

// resolve shows a conflict and asks how to resolve it. The merge is in progress;
// it stays so only if the user picks by hand.
func (a *App) resolve(ctx context.Context, v *vault.Vault, ce *vault.ConflictError, s sides) error {
	head, err := v.Head(ctx)
	if err != nil {
		return err // the merge stays in progress, for skl sync --abort
	}
	undo :=func() { v.Unmerge(context.WithoutCancel(ctx), head) }
	a.showConflict(ctx, v, ce.Paths, s)
	answer, err := a.ui.Ask(ctx, ui.Question{Prompt: "Resolve", Answers: []string{"by hand", "abort"}, Terminal: true})
	switch {
	case errors.Is(err, ui.ErrNoTerminal):
		undo()
		return errs.Hint(fmt.Errorf("%w needs you", ce), "skl sync, in a terminal")
	case err != nil: // Ctrl-C takes the last answer, abort
		undo()
		return err
	case answer == 0:
		return errs.Hint(ce, "skl sync --continue")
	}
	undo()
	return errs.Hint(fmt.Errorf("%w: merge undone", ce), "skl sync")
}

// showConflict is "! conflict   sets/backend.toml", then what each side did.
func (a *App) showConflict(ctx context.Context, v *vault.Vault, paths []string, s sides) {
	type side struct {
		label string
		did   []string
	}
	for _, path := range paths {
		a.ui.Warn("conflict   "+path, "")
		three, err := v.SidesOf(ctx, path)
		if err != nil {
			continue // the conflict itself is what matters; the details are a help
		}
		list := []side{{s.ours, sideDid(path, three.Base, three.Ours)}, {s.theirs, sideDid(path, three.Base, three.Theirs)}}
		if s.theirsFirst {
			list[0], list[1] = list[1], list[0]
		}
		width := max(len(s.ours), len(s.theirs)) + 2
		for _, sd := range list {
			for i, did := range sd.did {
				label := ""
				if i == 0 {
					label = sd.label
				}
				a.ui.Under(fmt.Sprintf("%-*s%s", width, label, did))
			}
		}
	}
}

// sideDid is what one side did to a file since the merge base: a skill set's
// selectors it added, dropped or replaced ("use"), else added, deleted or changed.
func sideDid(path string, base, side vault.Side) []string {
	switch {
	case side.Absent:
		return []string{"deleted"}
	case base.Absent:
		return []string{"added"}
	}
	if isSetFile(path) {
		before, err1 := vault.ParseSet(base.Data)
		after, err2 := vault.ParseSet(side.Data)
		if err1 == nil && err2 == nil {
			if did := setChanges(before, after); len(did) > 0 {
				return did
			}
		}
	}
	return []string{"changed"}
}

func isSetFile(path string) bool {
	name, ok := strings.CutPrefix(path, "sets/")
	return ok && !strings.Contains(name, "/") && strings.HasSuffix(name, ".toml") &&
		!strings.HasSuffix(name, ".origin.toml")
}

// setChanges is how a skill set's lines changed, in the order they are in the
// file: "add x" for a new line, "drop x" for one taken out, "use x" for one put in
// the place of another. In a run of changed lines, the new lines at its end
// replace the old ones.
func setChanges(before, after []string) []string {
	var out []string
	var removed, added []string
	flush := func() {
		paired := min(len(removed), len(added))
		for _, r := range removed[:len(removed)-paired] {
			out = append(out, "drop "+r)
		}
		for i, ad := range added {
			if i < len(added)-paired {
				out = append(out, "add "+ad)
			} else {
				out = append(out, "use "+ad)
			}
		}
		removed, added = nil, nil
	}
	for _, op := range diffLines(before, after) {
		switch op.kind {
		case '-':
			removed = append(removed, op.line)
		case '+':
			added = append(added, op.line)
		default:
			flush()
		}
	}
	flush()
	return out
}

type lineOp struct {
	kind byte // ' ', '-' or '+'
	line string
}

// diffLines is a line diff of a and b by longest common subsequence: skill sets are
// short, so the plain table is enough.
func diffLines(a, b []string) []lineOp {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []lineOp
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, lineOp{' ', a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, lineOp{'-', a[i]})
			i++
		default:
			ops = append(ops, lineOp{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, lineOp{'-', a[i]})
	}
	for ; j < len(b); j++ {
		ops = append(ops, lineOp{'+', b[j]})
	}
	return ops
}

// syncAbort is skl sync --abort: the merge in progress is undone. Commits made
// before it stay, nothing is pushed and the loaded skills don't change.
func (a *App) syncAbort(ctx context.Context) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	err = a.locked(ctx, "sync --abort", func(ctx context.Context, c *Change) (bool, error) {
		c.Vault = v
		merging, err := v.Merging(ctx)
		if err != nil || !merging {
			return false, err
		}
		head, err := v.Head(ctx)
		if err != nil {
			return false, err
		}
		v.Unmerge(context.WithoutCancel(ctx), head)
		if merging, err := v.Merging(context.WithoutCancel(ctx)); err != nil || merging {
			return false, errors.Join(errors.New("the merge couldn't be undone"), err)
		}
		c.Unchanged = "merge undone"
		return false, nil
	})
	return hinted(err, "skl sync --abort")
}

// finishMerge is the start of skl sync --continue, in place of the frame's merge
// guard: the merge the user resolved by hand is committed, and sync goes on from
// there. A save-back's merge is that copy's save, a generation of its own; sync's
// merge counts as pulled.
func (a *App) finishMerge(ctx context.Context, c *Change) error {
	v := c.Vault
	merging, err := v.Merging(ctx)
	if err != nil {
		return err
	}
	if !merging {
		return errs.Hint(errors.New("no sync to continue"), "skl sync")
	}
	left, err := v.Markers(ctx)
	if err != nil {
		return err
	}
	if len(left) > 0 {
		return errs.Hint(fmt.Errorf("%s still has conflict markers", strings.Join(left, ", ")), "skl sync --continue")
	}
	open, err := v.Open(ctx)
	if err != nil {
		return err
	}
	if err := v.Continue(ctx); err != nil {
		return err
	}
	// The merge is committed: from here on, Ctrl-C waits for what records it.
	ctx = context.WithoutCancel(ctx)
	if open.Copy == "" {
		c.pulled = open.Incoming
		return nil
	}
	head, err := v.Head(ctx)
	if err != nil {
		return err
	}
	root, err := a.homeRoot()
	if err != nil {
		return err
	}
	saved, err := load.Resume(ctx, load.SaveInput{
		Vault: v, HomeRoot: root, Targets: a.cfg.Targets, Current: c.State.Scopes[state.User].Copies,
	}, open.Copy, head, open.Theirs)
	if err != nil {
		return a.shownPaths(err)
	}
	var did []string
	for _, s := range savedLines(saved) {
		a.ui.Step("saved", s.Name+" → "+s.Skill)
		a.ui.StepMore(strings.Join(s.Files, ", "))
		did = append(did, "saved "+s.Name+" → "+s.Skill)
	}
	if len(did) == 0 {
		did = append(did, "resolved a conflict")
	}
	return a.recordSave(ctx, c, saved, strings.Join(did, ", "))
}
