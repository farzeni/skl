package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// statusReport is what skl status found. It only reads: it takes no lock, so it
// runs while another skl changes things, and it never goes online.
type statusReport struct {
	selectors []string
	found     *load.Inspection
	notes     []note // the ! lines, in the order they print
	sets      []string
	ahead     int
}

// Status is skl status (docs/guide/skill-sets.md "What's loaded").
func (a *App) Status(ctx context.Context, asJSON bool) error {
	r, err := a.readStatus(ctx)
	if err != nil {
		return hinted(err, "skl status")
	}
	if asJSON {
		return hinted(a.printStatusJSON(ctx, r), "skl status")
	}
	a.printStatus(r)
	return hinted(a.ui.ReportErr(), "skl status")
}

func (a *App) readStatus(ctx context.Context) (*statusReport, error) {
	v, err := a.openVault()
	if err != nil {
		return nil, err
	}
	st, err := a.store.Load(ctx)
	if err != nil {
		return nil, a.stateError(err)
	}
	home, err := a.homeRoot()
	if err != nil {
		return nil, err
	}
	idx, err := v.Index(ctx)
	if err != nil {
		return nil, err
	}
	ahead, err := a.notPushed(ctx, v)
	if err != nil {
		return nil, err
	}
	current := st.Scopes[state.User]
	res, notes := activeSkills(current.Selectors, idx, "edit "+a.shown(a.paths.State))
	found, err := load.Inspect(ctx, load.Input{Result: res, Current: current.Copies, Targets: a.cfg.Targets, HomeRoot: home})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, a.shownPaths(err)
	}
	for _, g := range found.Gone {
		notes = append(notes, note{
			msg:  fmt.Sprintf("%s   folder is gone: %s", g.Name, a.shown(g.Path)),
			hint: ui.CommandLine(append([]string{"use"}, current.Selectors...)),
		})
	}
	plan := load.Plan{Skipped: found.Skipped}
	notes = append(notes, a.skipNotes(&plan)...)
	for _, w := range idx.Warnings() {
		hint := "edit " + w.Path
		if w.BadName() {
			hint = "rename it in " + a.shown(a.paths.Vault)
		}
		notes = append(notes, note{msg: w.Message(), hint: hint})
	}
	return &statusReport{selectors: current.Selectors, found: found, notes: notes, sets: idx.Sets(), ahead: ahead}, nil
}

// notPushed is the vault's commits origin doesn't have. A vault with no commit yet
// has none, and that's for the first command that changes something to sort out.
// Any other failure stops: a report that left this out would look clean.
func (a *App) notPushed(ctx context.Context, v *vault.Vault) (int, error) {
	born, err := v.Born(ctx)
	if err != nil || !born {
		return 0, err
	}
	return v.Ahead(ctx)
}

// activeSkills resolves the active skill set for a report, which doesn't stop for
// a line that can't be used: each such line is a warning, and the rest still count.
func activeSkills(lines []string, idx *vault.Index, editState string) (resolve.Result, []note) {
	s := selection{verb: "status"}
	var notes []note
	var kept []selector.Selector
	for _, line := range lines {
		sel, err := selector.Parse(line)
		if err != nil { // only a hand edit of state.json puts one here, and drop can't parse it either
			notes = append(notes, note{msg: err.Error(), hint: editState})
			continue
		}
		if _, err := resolve.Resolve([]selector.Selector{sel}, idx); err != nil {
			notes = append(notes, lineNote(s, err, idx))
			continue
		}
		kept = append(kept, sel)
	}
	res, err := resolve.Resolve(kept, idx)
	if err != nil { // each line works alone, and together they don't
		return resolve.Result{}, append(notes, lineNote(s, err, idx))
	}
	return res, append(notes, s.selectorNotes(res, idx)...)
}

// lineNote is the ✗ line a load would stop with, as a warning. What a named set's
// line says ends like its other warnings: "(in @backend)".
func lineNote(s selection, err error, idx *vault.Index) note {
	msg := err.Error()
	if sl, ok := errors.AsType[*resolve.SetLineError](err); ok {
		if _, isFile := errors.AsType[*vault.SetError](err); isFile {
			msg = sl.Err.Error() // it names the file
		} else {
			msg = fmt.Sprintf("%s (in @%s)", sl.Err, sl.Set)
		}
	}
	hint := ""
	if he, ok := errors.AsType[*errs.Hinted](s.resolveError(err, idx)); ok {
		hint = he.Hint
	}
	return note{msg: msg, hint: hint}
}

// printStatus is the text report. The label column fits the longest name, and
// "skill sets" and "vault" line up with it.
func (a *App) printStatus(r *statusReport) {
	labels := len("skill sets") + 2
	skills := 0
	for _, l := range r.found.Loaded {
		labels = max(labels, 2+ui.Width(l.Name)+2)
		skills = max(skills, ui.Width(l.Skill))
	}
	for _, f := range r.found.Foreign {
		labels = max(labels, 2+ui.Width(f)+2)
	}
	if len(r.selectors) > 0 || len(r.found.Loaded) > 0 {
		a.ui.ReportLine("loaded (user)  "+strings.Join(r.selectors, " "), "")
		for _, l := range r.found.Loaded {
			row := "  " + ui.Pad(l.Name, labels-2) + ui.Pad(l.Skill, skills)
			if l.Changed {
				a.ui.ReportLine(row+"  changed", "skl sync")
				continue
			}
			a.ui.ReportLine(row, "")
		}
	} else {
		a.ui.ReportLine("nothing loaded", "skl use <selectors>")
	}
	for _, n := range r.notes {
		a.ui.ReportWarn(n.msg, ui.Clean(n.hint))
	}
	if len(r.found.Foreign) > 0 {
		a.ui.ReportLine("not in your vault", "")
		for _, f := range r.found.Foreign {
			// " · " supplies the last of the column's spaces.
			a.ui.ReportLine("  "+ui.Pad(f, labels-3), importCommand(f))
		}
	}
	if len(r.sets) > 0 {
		at := make([]string, len(r.sets))
		for i, s := range r.sets {
			at[i] = "@" + s
		}
		a.ui.ReportLine(ui.Pad("skill sets", labels)+strings.Join(at, "  "), "")
	}
	if r.ahead > 0 {
		a.ui.ReportLine(ui.Pad("vault", labels)+notPushed(r.ahead), "skl sync")
	}
}

func importCommand(name string) string {
	return ui.CommandLine([]string{"import", ui.Clean(name)})
}

// The JSON of docs/reference/commands.md "JSON".
type (
	statusJSON struct {
		Generation int         `json:"generation"`
		Scopes     []scopeJSON `json:"scopes"`
		Sets       []string    `json:"sets"`
		Needs      []needJSON  `json:"needs"`
	}
	scopeJSON struct {
		Scope     string      `json:"scope"`
		Paths     []string    `json:"paths"`
		Selectors []string    `json:"selectors"`
		Skills    []skillJSON `json:"skills"`
	}
	skillJSON struct {
		Name  string `json:"name"`
		Skill string `json:"skill"`
	}
	needJSON struct {
		Skill   string `json:"skill,omitempty"`
		Name    string `json:"name,omitempty"`
		State   string `json:"state"`
		Command string `json:"command"`
	}
)

func (a *App) printStatusJSON(ctx context.Context, r *statusReport) error {
	gen, err := a.store.LastGeneration(ctx)
	if err != nil {
		return err
	}
	out := statusJSON{Generation: gen, Scopes: []scopeJSON{}, Sets: slices.Clone(r.sets), Needs: statusNeeds(r)}
	if out.Sets == nil {
		out.Sets = []string{}
	}
	if len(r.selectors) > 0 || len(r.found.Loaded) > 0 {
		sc := scopeJSON{Scope: "user", Paths: r.found.Dirs, Selectors: slices.Clone(r.selectors), Skills: []skillJSON{}}
		if sc.Selectors == nil {
			sc.Selectors = []string{}
		}
		for _, l := range r.found.Loaded {
			sc.Skills = append(sc.Skills, skillJSON{Name: l.Name, Skill: l.Skill})
		}
		out.Scopes = append(out.Scopes, sc)
	}
	return a.ui.ReportJSON(out)
}

// statusNeeds is the lines of the report that need you, in the same order.
func statusNeeds(r *statusReport) []needJSON {
	needs := []needJSON{}
	for _, l := range r.found.Loaded {
		if !l.Changed {
			continue
		}
		n := needJSON{Skill: l.Skill, State: "changed", Command: "skl sync"}
		if l.Skill == "" {
			n.Name = l.Name
		}
		needs = append(needs, n)
	}
	for _, n := range r.notes {
		need := needJSON{State: n.msg, Command: n.hint}
		// The warnings that name something write it first: "lint   skipped: …".
		if name, state, ok := strings.Cut(n.msg, "   "); ok {
			need.Name, need.State = name, state
		}
		needs = append(needs, need)
	}
	for _, f := range r.found.Foreign {
		needs = append(needs, needJSON{Name: f, State: "not in your vault", Command: importCommand(f)})
	}
	if r.ahead > 0 {
		needs = append(needs, needJSON{State: notPushed(r.ahead), Command: "skl sync"})
	}
	return needs
}
