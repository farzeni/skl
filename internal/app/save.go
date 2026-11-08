package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Save is skl save <name>: the selectors of the active skill set become
// sets/<name>.toml (docs/guide/skill-sets.md "Named skill sets"). Overwriting a
// set shows how its lines change and asks first. The question comes before the
// frame, so without a terminal it stops before anything is saved back or written.
func (a *App) Save(ctx context.Context, name string, dryRun bool) error {
	if !selector.ValidName(name) {
		return errs.Usagef("skl help save", "%s is not a valid skill set name: %s", name, nameRule)
	}
	if _, err := a.userRoot(); err != nil {
		return err
	}
	command := strings.TrimPrefix(ui.CommandLine([]string{"save", name}), "skl ")
	v, err := a.openVault()
	if err != nil {
		return err
	}
	st, err := a.store.Load(ctx)
	if err != nil {
		return a.stateError(err)
	}
	asked, err := planSave(ctx, v, st, name)
	if err != nil {
		return err
	}
	if asked.changed {
		a.reportSave(asked)
	}
	if dryRun {
		if !asked.changed {
			a.ui.Done("nothing to change")
		}
		return nil
	}
	if asked.changed && asked.exists {
		answer, err := a.ui.Ask(ctx, ui.Question{Prompt: "Overwrite @" + asked.name + "?", Answers: []string{"yes", "no"}})
		if err != nil {
			return err
		}
		if answer != 0 {
			a.ui.Done("nothing to change")
			return nil
		}
	}
	return a.Changing(ctx, command, func(ctx context.Context, c *Change) (bool, error) {
		p, err := planSave(ctx, c.Vault, c.State, name)
		if err != nil {
			return false, err
		}
		if !p.same(asked) {
			return false, errs.Hint(fmt.Errorf("%s changed while skl was asking", vault.SetFile(p.name)), "skl "+command)
		}
		if !p.changed {
			return false, nil
		}
		return true, a.writeSet(ctx, c, p, "skl "+command)
	})
}

// nameRule is the Grammar's name, for the usage error.
const nameRule = "a name has letters, digits, - and _, and starts with a letter or a digit"

// savePlan is what skl save would write.
type savePlan struct {
	name    string   // the set's name as on disk, or as typed for a new one
	exists  bool     // sets/<name>.toml is there
	old     []string // its lines, nil if it can't be read
	lines   []string // the lines to write
	changed bool
}

func (p *savePlan) same(o *savePlan) bool {
	return p.name == o.name && p.exists == o.exists && slices.Equal(p.old, o.old) && slices.Equal(p.lines, o.lines)
}

// planSave works out the file. It only reads.
func planSave(ctx context.Context, v *vault.Vault, st *state.State, typed string) (*savePlan, error) {
	idx, err := v.Index(ctx)
	if err != nil {
		return nil, err
	}
	p := &savePlan{name: typed}
	found, err := idx.FindSet(typed)
	switch {
	case err == nil:
		p.name, p.exists = found, true
	case errors.Is(err, resolve.ErrNoSet):
	default:
		if ae, ok := errors.AsType[*vault.AmbiguousSetError](err); ok {
			return nil, errs.Hint(err, ui.Clean(ui.CommandLine([]string{"save", ae.Sets[0]})))
		}
		return nil, err
	}
	var oldErr error
	if p.exists {
		p.old, oldErr = idx.Set(p.name)
	}
	p.lines, err = selfExpanded(st.Scopes[state.User].Selectors, p, idx, oldErr)
	if err != nil {
		return nil, err
	}
	p.changed = !p.exists || oldErr != nil || !slices.Equal(p.lines, p.old)
	return p, nil
}

// selfExpanded is the active skill set with each line that names the set being
// saved replaced by that set's lines, so the file keeps its old lines plus the
// changes. A line it takes in that comes again later is left out, so saving again
// changes nothing; a later duplicate selects the same and wins the same clashes.
// A self-reference among the lines it takes in is left out too: it would be a
// cycle. A removal of the set has no lines to write in its place.
func selfExpanded(active []string, p *savePlan, idx *vault.Index, oldErr error) ([]string, error) {
	self := func(line string) (selector.Selector, bool) {
		sel, err := selector.Parse(line)
		if err != nil || sel.Kind != selector.Set {
			return sel, false
		}
		if p.exists {
			found, err := idx.FindSet(sel.Name)
			return sel, err == nil && found == p.name
		}
		return sel, selector.Fold(sel.Name) == selector.Fold(p.name)
	}
	lines := make([]string, 0, len(active))
	for i, line := range active {
		sel, isSelf := self(line)
		if !isSelf {
			lines = append(lines, line)
			continue
		}
		if sel.Remove {
			return nil, errs.Hint(
				fmt.Errorf("the active skill set removes @%s, so it can't be saved as %s", sel.Name, p.name),
				ui.Clean(ui.CommandLine([]string{"drop", line})))
		}
		if oldErr != nil {
			if se, ok := errors.AsType[*vault.SetError](oldErr); ok {
				return nil, errs.Hint(oldErr, "edit "+se.File)
			}
			return nil, oldErr
		}
		later := active[i+1:]
		for _, l := range p.old {
			if _, again := self(l); again || slices.ContainsFunc(later, func(o string) bool { return sameText(o, l) }) {
				continue
			}
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// sameText reports whether two lines are one selector, in any case or form.
func sameText(a, b string) bool {
	if sel, err := selector.Parse(b); err == nil {
		return sameLine(a, sel)
	}
	return a == b
}

// reportSave prints the plan: a new file, or how an old one's lines change.
func (a *App) reportSave(p *savePlan) {
	file := vault.SetFile(p.name)
	if !p.exists {
		a.ui.Plan("save @%s as %s", p.name, file)
		return
	}
	a.ui.Plan("overwrite %s", file)
	for _, d := range lineDiff(p.old, p.lines) {
		kind := ui.Loaded
		if d.removed {
			kind = ui.Unloaded
		}
		a.ui.Change(kind, d.line, "")
	}
}

// writeSet writes the file and commits it. The commit can be stopped from the
// terminal; then, or if the frame can't make the generation, the file goes back.
func (a *App) writeSet(ctx context.Context, c *Change, p *savePlan, message string) error {
	ctx = context.WithoutCancel(ctx)
	before, err := c.Vault.Head(ctx)
	if err != nil {
		return err
	}
	restore, err := c.Vault.WriteSet(p.name, p.lines)
	if err != nil {
		return err
	}
	file := vault.SetFile(p.name)
	if _, err := c.Vault.Commit(ctx, message, file); err != nil {
		return errors.Join(err, restore())
	}
	c.Undo = func() {
		c.Vault.Uncommit(ctx, before, file)
		_ = restore()
	}
	return nil
}

// diffLine is one line of how a set's lines change.
type diffLine struct {
	line    string
	removed bool
}

// lineDiff is the lines removed from old and added in next, in file order, from
// their longest common subsequence: a line that moved is removed, then added.
func lineDiff(old, next []string) []diffLine {
	// lcs[i][j] is the length of the longest common subsequence of old[i:] and next[j:].
	lcs := make([][]int, len(old)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(next)+1)
	}
	for i := len(old) - 1; i >= 0; i-- {
		for j := len(next) - 1; j >= 0; j-- {
			if old[i] == next[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < len(old) || j < len(next) {
		switch {
		case i < len(old) && j < len(next) && old[i] == next[j]:
			i, j = i+1, j+1
		case j == len(next) || (i < len(old) && lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, diffLine{line: old[i], removed: true})
			i++
		default:
			out = append(out, diffLine{line: next[j]})
			j++
		}
	}
	return out
}
