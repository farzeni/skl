package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// selection is one run of use, add or drop: they differ only in how the words
// change the active skill set.
type selection struct {
	verb  string   // the command
	fixed []string // words between the command and the selectors, such as init's repo
	label string   // what the → line calls it
	words []string
	// next returns the selectors that become active, and any warnings of its own.
	next func(active []string, sels []selector.Selector, idx *vault.Index) ([]string, []note, error)
	// saved are copies a save not yet made would record: a dry run plans as if it
	// had (load.Input.Saved).
	saved []state.Copy
	// release are copies to give back in place instead of unloading them
	// (load.Input.Release).
	release []state.Copy
}

// note is a warning, built for the command being run.
type note struct{ msg, hint string }

// command is the line as typed, without flags and without "skl".
func (s selection) command() string {
	return strings.TrimPrefix(ui.CommandLine(slices.Concat([]string{s.verb}, s.fixed, s.words)), "skl ")
}

// loading is what the three commands share: docs/design/architecture.md "Loading
// pipeline", resolve → plan → apply, and with --dry-run the plan is the end.
func (a *App) loading(ctx context.Context, s selection, dryRun bool) error {
	sels, err := s.parseWords()
	if err != nil {
		return err
	}
	root, err := a.userRoot()
	if err != nil {
		return err
	}
	if dryRun {
		return a.loadDryRun(ctx, s, sels, root)
	}
	return a.Changing(ctx, s.command(), func(ctx context.Context, c *Change) (bool, error) {
		next, err := a.prepare(ctx, c.Vault, c.State, s, sels, root)
		if err != nil {
			return false, err
		}
		a.report(s, next)
		if !next.changed {
			return false, nil
		}
		return true, a.apply(ctx, c, next)
	})
}

// loadDryRun prints the plan: what the frame would save first, then the load, as
// planned on the vault and copies that save would leave. It takes no lock and
// writes nothing but git objects.
func (a *App) loadDryRun(ctx context.Context, s selection, sels []selector.Selector, root string) error {
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
	first, stop, err := a.dryRunSave(ctx, v, st, root)
	if err != nil || stop {
		return err
	}
	first.recorded(st, &s)
	next, err := a.planAt(ctx, v, st, s, sels, root, first.head, first.at)
	if err != nil {
		return hinted(err, "skl "+s.command())
	}
	a.report(s, next)
	if !next.changed && !first.any() {
		a.ui.Done("nothing to change")
	}
	return nil
}

// userRoot is what the user scope's skills folders are under. --scope project comes
// with its own card, so a config that asks for it stops rather than load elsewhere.
func (a *App) userRoot() (string, error) {
	if a.cfg.Scope != config.User {
		return "", errs.Hint(errors.New(`scope = "project" isn't supported yet`), "edit scope in "+a.shown(a.paths.Config))
	}
	return a.homeRoot()
}

// homeRoot is the home folder, which the user scope's skills folders are under.
func (a *App) homeRoot() (string, error) {
	home := a.getenv("HOME")
	if home == "" {
		return "", errs.Hint(errors.New("HOME is not set, so skl can't find your skills folders"), "set HOME")
	}
	return home, nil
}

// parseWords reads the command's words. A word that isn't a selector stops here,
// before anything looks at the vault: nothing has changed.
func (s selection) parseWords() ([]selector.Selector, error) {
	sels := make([]selector.Selector, len(s.words))
	for i, word := range s.words {
		sel, err := selector.Parse(word)
		if err != nil {
			if re, ok := errors.AsType[*selector.ReferenceError](err); ok {
				return nil, &errs.Usage{Err: err, Hint: ui.CommandLine([]string{"learn", re.Ref})}
			}
			return nil, errs.Usagef("skl help "+s.verb, "%w", err)
		}
		sels[i] = sel
	}
	return sels, nil
}

func parseLines(lines []string) ([]selector.Selector, error) {
	sels := make([]selector.Selector, len(lines))
	for i, line := range lines {
		sel, err := selector.Parse(line)
		if err != nil {
			return nil, err
		}
		sels[i] = sel
	}
	return sels, nil
}

// loadNext is a prepared run.
type loadNext struct {
	selectors []string
	plan      *load.Plan
	notes     []note
	changed   bool // something to apply, or the active skill set or its record differ
}

// prepare resolves the new active skill set and plans it. It only reads.
func (a *App) prepare(ctx context.Context, v *vault.Vault, st *state.State, s selection, sels []selector.Selector, root string) (*loadNext, error) {
	idx, err := v.Index(ctx)
	if err != nil {
		return nil, err
	}
	head, err := v.Head(ctx)
	if err != nil {
		return nil, err
	}
	return a.prepareAt(ctx, vaultView{dir: v.Dir(), head: head, idx: idx}, st, s, sels, root)
}

// vaultView is a version of the vault to load from: the vault itself, or a preview
// of it written elsewhere, for a --dry-run that has to see changes it won't make.
type vaultView struct {
	dir, head string
	idx       *vault.Index
}

// prepareAt is prepare against a view of the vault.
func (a *App) prepareAt(ctx context.Context, view vaultView, st *state.State, s selection, sels []selector.Selector, root string) (*loadNext, error) {
	idx := view.idx
	current := st.Scopes[state.User]
	selectors, notes, err := s.next(current.Selectors, sels, idx)
	if err != nil {
		return nil, s.resolveError(err, idx)
	}
	lines, err := parseLines(selectors)
	if err != nil {
		return nil, s.resolveError(err, idx)
	}
	res, err := resolve.Resolve(lines, idx)
	if err != nil {
		return nil, s.resolveError(err, idx)
	}
	plan, err := load.Compute(ctx, load.Input{
		Vault: view.dir, Commit: view.head, Result: res, Current: current.Copies, Targets: a.cfg.Targets, HomeRoot: root,
		Saved: s.saved, Release: s.release,
	})
	if err != nil {
		return nil, a.planError(err)
	}
	notes = slices.Concat(s.selectorNotes(res, idx), notes, a.indexNotes(idx, plan), a.skipNotes(plan))
	return &loadNext{
		selectors: selectors,
		plan:      plan,
		notes:     notes,
		changed:   !plan.Empty() || !slices.Equal(selectors, current.Selectors) || !sameCopies(plan.Copies, current.Copies),
	}, nil
}

// sameCopies compares two lists of copies in any order.
func sameCopies(a, b []state.Copy) bool {
	byName := func(x, y state.Copy) int {
		return strings.Compare(string(x.Target)+"/"+x.Name, string(y.Target)+"/"+y.Name)
	}
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, byName)
	slices.SortFunc(b, byName)
	return slices.Equal(a, b)
}

// report prints the plan: → and + ~ − when something changes, then the warnings.
func (a *App) report(s selection, next *loadNext) {
	if next.changed {
		a.ui.Plan("%s %s", s.label, strings.Join(s.words, " "))
		a.printChanges(next.plan.Changes)
	}
	for _, n := range next.notes {
		a.ui.Warn(n.msg, ui.Clean(n.hint)) // a hint can quote a name from the vault
	}
}

// printChanges prints each change once. Neighbors of a kind with nothing more to say
// share a line.
func (a *App) printChanges(changes []load.Change) {
	for i := 0; i < len(changes); {
		first := changes[i]
		if d := a.detail(first); d != "" {
			a.ui.Change(uiKind(first.Kind), first.Name, d)
			i++
			continue
		}
		var names []string
		for ; i < len(changes) && changes[i].Kind == first.Kind && a.detail(changes[i]) == ""; i++ {
			names = append(names, changes[i].Name)
		}
		a.ui.Names(uiKind(first.Kind), names)
	}
}

func (a *App) detail(c load.Change) string {
	if c.From != "" {
		return "from " + a.shown(c.From)
	}
	return c.Detail
}

func uiKind(k load.Kind) ui.Kind {
	switch k {
	case load.Replace:
		return ui.Replaced
	case load.Unload:
		return ui.Unloaded
	}
	return ui.Loaded
}

// apply carries the plan out and records it. After the swap, Ctrl-C waits: the
// change has happened, so what remains is writing it down. If that fails, or the
// frame can't make its generation, the previous set goes back.
func (a *App) apply(ctx context.Context, c *Change, next *loadNext) error {
	swap, err := load.NewLoader(a.paths.Staging).Apply(ctx, next.plan)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return a.shownPaths(err)
	}
	c.Cleanup = swap.Close
	ctx = context.WithoutCancel(ctx)
	before, had := c.State.Scopes[state.User]
	restore := func() {
		swap.Undo()
		if had {
			c.State.Scopes[state.User] = before
		} else {
			delete(c.State.Scopes, state.User)
		}
	}
	c.State.Scopes[state.User] = state.Scope{Selectors: next.selectors, Copies: next.plan.Copies}
	c.Scopes = []state.ScopeKey{state.User}
	if err := c.Lock.Save(ctx, c.State); err != nil {
		restore()
		return err
	}
	c.Undo = func() {
		restore()
		_ = c.Lock.Save(ctx, c.State)
	}
	return nil
}

// planError gives an error from reading the folders its ✗ line.
func (a *App) planError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return a.shownPaths(err)
}

// shownPaths is err with ~ for HOME.
func (a *App) shownPaths(err error) error {
	home := a.getenv("HOME")
	if home == "" {
		return err
	}
	return &shownError{Err: err, Text: strings.ReplaceAll(err.Error(), home+"/", "~/")}
}

// selectorNotes are the warnings for selectors that match nothing
// (docs/reference/repos.md "Grammar").
func (s selection) selectorNotes(res resolve.Result, idx *vault.Index) []note {
	notes := make([]note, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		notes = append(notes, note{msg: w.Message(), hint: s.warningHint(w, idx)})
	}
	return notes
}

// warningHint is what deals with a selector that matches nothing: the file of a set
// that holds it; the command as typed with the word replaced, if the word names a
// skill; else dropping it.
func (s selection) warningHint(w resolve.Warning, idx *vault.Index) string {
	if w.Set != "" {
		return "edit " + setFile(idx, w.Set)
	}
	word := w.Selector.String()
	if w.Suggest != "" && slices.Contains(s.words, word) && !ambiguousGroup(idx, w.Suggest) {
		replacement := w.Suggest
		if w.Selector.Remove {
			replacement = "-" + replacement
		}
		return s.replaced(word, replacement)
	}
	return ui.CommandLine([]string{"drop", word})
}

// replaced is the command as typed with one word replaced.
func (s selection) replaced(word, replacement string) string {
	words := slices.Clone(s.words)
	for i, w := range words {
		if w == word {
			words[i] = replacement
		}
	}
	return ui.CommandLine(slices.Concat([]string{s.verb}, s.fixed, words))
}

// ambiguousGroup reports whether a suggested skill's group would stop the command.
func ambiguousGroup(idx *vault.Index, skill string) bool {
	group, _, _ := strings.Cut(skill, "/")
	want := selector.Fold(group)
	n := 0
	for _, g := range idx.Groups() {
		if selector.Fold(g) == want {
			n++
		}
	}
	return n > 1
}

// setFile is the file of a set named in any case or form, as the docs' hints write
// it, from the vault's root.
func setFile(idx *vault.Index, name string) string {
	want := selector.Fold(name)
	for _, s := range idx.Sets() {
		if s == name || selector.Fold(s) == want {
			name = s
			break
		}
	}
	return path.Join("sets", name+".toml")
}

// indexNotes are the vault's own warnings that matter here: a name skl had to skip,
// and anything wrong in a skill being loaded. skl status lists the rest.
func (a *App) indexNotes(idx *vault.Index, plan *load.Plan) []note {
	var notes []note
	for _, w := range idx.Warnings() {
		switch {
		case w.BadName():
			notes = append(notes, note{msg: w.Message(), hint: "rename it in " + a.shown(a.paths.Vault)})
		case slices.ContainsFunc(plan.Copies, func(c state.Copy) bool { return strings.HasPrefix(w.Path, c.Skill+"/") }):
			notes = append(notes, note{msg: w.Message(), hint: "edit " + w.Path})
		}
	}
	return notes
}

// skipNotes are the skills a plan dropped because someone else's skill has the
// name. Your vault has the name, so skl import would refuse that folder: the hint
// frees the name instead (docs/guide/skill-sets.md "Skills skl didn't create").
func (a *App) skipNotes(p *load.Plan) []note {
	notes := make([]note, 0, len(p.Skipped))
	for _, sk := range p.Skipped {
		shown := a.shown(sk.Path)
		n := note{
			msg:  fmt.Sprintf("%s   skipped: %s is not in your vault", sk.Skill.Folder, shown),
			hint: "move " + shown + " aside, then skl sync",
		}
		if !slices.Contains(notes, n) { // two skills with one folder name say the same
			notes = append(notes, n)
		}
	}
	return notes
}

// resolveError gives the errors of resolving selectors their ✗ line's hint. Words
// that can't work were caught already, so these come from the vault, a set's file
// or an earlier command.
func (s selection) resolveError(err error, idx *vault.Index) error {
	if he, ok := errors.AsType[*errs.Hinted](s.hintFor(err, idx)); ok {
		he.Hint = ui.Clean(he.Hint) // it can quote a name from the vault
		return he
	}
	return err
}

func (s selection) hintFor(err error, idx *vault.Index) error {
	// What's wrong here is two folders, wherever the line that found them is.
	if ce, ok := errors.AsType[*resolve.ClashError](err); ok {
		return errs.Hint(ce, "rename one in your vault")
	}
	if se, ok := errors.AsType[*vault.SetError](err); ok {
		return errs.Hint(err, "edit "+se.File)
	}
	if sl, ok := errors.AsType[*resolve.SetLineError](err); ok {
		return errs.Hint(err, "edit "+setFile(idx, sl.Set))
	}
	if ce, ok := errors.AsType[*resolve.CycleError](err); ok {
		return errs.Hint(err, "edit "+setFile(idx, ce.Cycle[len(ce.Cycle)-2]))
	}
	if _, ok := errors.AsType[*resolve.AmbiguousGroupError](err); ok {
		return errs.Hint(err, "rename one of them in your vault")
	}
	if ae, ok := errors.AsType[*resolve.AmbiguousSkillError](err); ok {
		return errs.Hint(err, s.withExact(ae.Name, ae.Skills[0].String()))
	}
	if ae, ok := errors.AsType[*vault.AmbiguousSetError](err); ok {
		return errs.Hint(err, s.withExact("@"+ae.Name, "@"+ae.Sets[0]))
	}
	return err
}

// withExact is the command with an ambiguous word replaced by one exact name, or
// dropping the word if an earlier command left it in the active skill set.
func (s selection) withExact(word, exact string) string {
	switch {
	case slices.Contains(s.words, word):
		return s.replaced(word, exact)
	case slices.Contains(s.words, "-"+word):
		return s.replaced("-"+word, "-"+exact)
	}
	return ui.CommandLine([]string{"drop", word})
}

// planAt is prepare against the vault as at, a commit or a tree, would leave it:
// the vault itself if that's HEAD, else its files written to a temporary folder,
// as rollback --dry-run does. Nothing in the vault changes. It's how a --dry-run
// plans on what the real run's earlier steps would commit.
func (a *App) planAt(ctx context.Context, v *vault.Vault, st *state.State, s selection, sels []selector.Selector, root, head, at string) (*loadNext, error) {
	if at == head {
		return a.prepare(ctx, v, st, s, sels, root)
	}
	tmp, err := os.MkdirTemp("", "skl-plan-")
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
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
	return a.prepareAt(ctx, vaultView{dir: dir, head: at, idx: idx}, st, s, sels, root)
}
