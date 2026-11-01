package app

import (
	"context"
	"errors"
	"fmt"
	"path"
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
	verb  string // the command
	label string // what the → line calls it
	words []string
	// next returns the selectors that become active, and any warnings of its own.
	next func(active []string, sels []selector.Selector, idx *vault.Index) ([]string, []note, error)
}

// note is a warning, built for the command being run.
type note struct{ msg, hint string }

// command is the line as typed, without flags and without "skl".
func (s selection) command() string {
	return strings.TrimPrefix(ui.CommandLine(append([]string{s.verb}, s.words...)), "skl ")
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

// loadDryRun prints the plan. It takes no lock and writes nothing.
func (a *App) loadDryRun(ctx context.Context, s selection, sels []selector.Selector, root string) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	st, err := a.store.Load(ctx)
	if err != nil {
		return a.stateError(err)
	}
	next, err := a.prepare(ctx, v, st, s, sels, root)
	if err != nil {
		return hinted(err, "skl "+s.command())
	}
	a.report(s, next)
	if !next.changed {
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
	head, err := v.Head(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := load.Compute(ctx, load.Input{
		Vault: v.Dir(), Commit: head, Result: res, Current: current.Copies, Targets: a.cfg.Targets, HomeRoot: root,
	})
	if err != nil {
		return nil, a.planError(err, s)
	}
	notes = slices.Concat(s.selectorNotes(res, idx), notes, a.indexNotes(idx, res), a.skipNotes(plan))
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
		a.ui.Warn(n.msg, n.hint)
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
func (a *App) planError(err error, s selection) error {
	if ee, ok := errors.AsType[*load.EditedError](err); ok {
		return errs.Hint(&shownError{Err: err, Text: a.shown(ee.Path) + " has changes not saved back"},
			"move it aside, then skl "+s.command())
	}
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
	return ui.CommandLine(append([]string{s.verb}, words...))
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
func (a *App) indexNotes(idx *vault.Index, res resolve.Result) []note {
	var notes []note
	for _, w := range idx.Warnings() {
		switch {
		case w.BadName():
			notes = append(notes, note{msg: w.Message(), hint: "rename it in " + a.shown(a.paths.Vault)})
		case slices.ContainsFunc(res.Skills, func(s resolve.Skill) bool { return strings.HasPrefix(w.Path, s.String()+"/") }):
			notes = append(notes, note{msg: w.Message(), hint: "edit " + w.Path})
		}
	}
	return notes
}

// skipNotes are the skills a plan dropped because someone else's skill has the name.
func (a *App) skipNotes(p *load.Plan) []note {
	notes := make([]note, 0, len(p.Skipped))
	for _, sk := range p.Skipped {
		notes = append(notes, note{
			msg:  fmt.Sprintf("%s   skipped: %s is not in your vault", sk.Skill.Folder, a.shown(sk.Path)),
			hint: ui.CommandLine([]string{"import", ui.Clean(sk.Foreign)}),
		})
	}
	return notes
}

// resolveError gives the errors of resolving selectors their ✗ line's hint. Words
// that can't work were caught already, so these come from the vault, a set's file
// or an earlier command.
func (s selection) resolveError(err error, idx *vault.Index) error {
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
