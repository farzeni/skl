// Package resolve turns selectors into an ordered list of vault skills,
// following docs/guide/skill-sets.md "Selectors" and "Names and clashes". It
// is pure: everything it knows about the vault comes through an Index.
package resolve

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/selector"
)

// Skill is a skill in the vault.
type Skill struct {
	Group  string
	Folder string
	Name   string // the frontmatter name; empty if it has none
}

// String returns the skill as "group/folder".
func (s Skill) String() string { return s.Group + "/" + s.Folder }

// names holds a skill's names in the form they compare in, selector.Fold.
type names struct {
	folder, name string
}

func foldNames(s Skill) names {
	return names{folder: selector.Fold(s.Folder), name: selector.Fold(s.Name)}
}

// clashes reports whether two skills share a name. A skill's names are its
// folder name and its frontmatter name.
func (n names) clashes(o names) bool {
	return n.folder == o.folder || n.folder == o.name || n.name == o.folder || (n.name != "" && n.name == o.name)
}

// ErrNoSet is what Index.Set returns for a skill set that doesn't exist.
var ErrNoSet = errors.New("no such skill set")

// Index is what Resolve reads from the vault.
type Index interface {
	// Groups returns the vault's groups, in a stable order.
	Groups() []string
	// Skills returns a group's skills, in a stable order, or nothing if the
	// group doesn't exist.
	Skills(group string) []Skill
	// Set returns the lines of a named skill set, or ErrNoSet.
	Set(name string) ([]string, error)
}

// Result is what a list of selectors resolves to.
type Result struct {
	Skills   []Skill // in the order they were first selected; no two clash
	Replaced []Replacement
	Warnings []Warning

	selected []selection // what's left after the removals, before clashes settle
}

// Selected returns what the selectors select, clashes or not: the skills removals
// left, in the order they were first selected.
func (r Result) Selected() []Skill {
	skills := make([]Skill, len(r.selected))
	for i, o := range r.selected {
		skills[i] = o.skill
	}
	return skills
}

// Skipping settles the clashes again for a place where skip's skills can't load. A
// skill loses a clash only to a later skill that stays loaded, so what a skipped
// skill would have replaced stays. It also returns the skipped skills that lost no
// clash: the ones that would have loaded.
func (r Result) Skipping(skip func(Skill) bool) (Result, []Skill) {
	res, skipped := settle(r.selected, skip)
	res.Warnings = r.Warnings
	return res, skipped
}

// Replacement is a skill that lost a clash to a skill in the result.
type Replacement struct {
	Old, New Skill
}

// Detail returns "someone/pr-review replaces review/pr-review".
func (r Replacement) Detail() string { return r.New.String() + " replaces " + r.Old.String() }

// Problem is why a selector matched nothing.
type Problem int

// The ways a selector matches nothing.
const (
	NoGroup Problem = iota
	NoSkill
	NoSet
)

func (p Problem) String() string {
	return [...]string{NoGroup: "matches no group", NoSkill: "matches no skill", NoSet: "matches no skill set"}[p]
}

// Warning is a selector that matched nothing. It holds no command: app builds
// the hint, for the command being run.
type Warning struct {
	Selector selector.Selector
	Problem  Problem
	Set      string // the named skill set that holds the line, if any
	Suggest  string // what a one-word name probably meant: a selector that resolves
	Skill    Skill  // the skill Suggest selects, if any
}

// Message returns the warning without a hint:
// "review/old-name   matches no skill (in @backend)".
func (w Warning) Message() string {
	msg := w.Selector.String() + "   " + w.Problem.String()
	if w.Set != "" {
		msg += " (in @" + w.Set + ")"
	}
	return msg
}

// CycleError is a skill set that includes itself.
type CycleError struct {
	Cycle []string // the set names, starting and ending with the same one
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("skill set @%s includes itself: @%s", e.Cycle[0], strings.Join(e.Cycle, " → @"))
}

// SetLineError is a line of a named skill set that can't be used.
type SetLineError struct {
	Set string // the set that holds the line, as named
	Err error
}

func (e *SetLineError) Error() string { return "@" + e.Set + ": " + e.Err.Error() }

func (e *SetLineError) Unwrap() error { return e.Err }

// ClashError is two skills that one group selector selected and that share a name.
// Nothing on the command line says which one is meant, and the disk may hold them
// as one folder, so skl stops.
type ClashError struct {
	A, B Skill
}

func (e *ClashError) Error() string {
	return e.A.String() + " and " + e.B.String() + " have the same name"
}

// AmbiguousGroupError is a selector whose group matches two or more vault
// groups that differ only by case or Unicode form.
type AmbiguousGroupError struct {
	Group  string   // as typed
	Groups []string // the vault's groups, as written
}

func (e *AmbiguousGroupError) Error() string {
	return fmt.Sprintf("%s matches %d groups: %s", e.Group, len(e.Groups), strings.Join(e.Groups, ", "))
}

// AmbiguousSkillError is a selector whose name matches two or more skills in its
// group, and no folder exactly.
type AmbiguousSkillError struct {
	Name   string  // the selector's group/name, as typed
	Skills []Skill // the group's skills, as written
}

func (e *AmbiguousSkillError) Error() string {
	list := make([]string, len(e.Skills))
	for i, s := range e.Skills {
		list[i] = s.String()
	}
	return fmt.Sprintf("%s matches %d skills: %s", e.Name, len(list), strings.Join(list, ", "))
}

// Resolve applies sels left to right: each removal takes away what the earlier
// selectors selected. Then, among what's left, the later of two skills that
// clash wins. A selector that matches nothing is a warning. A set that includes
// itself, a set line that isn't a selector, a set that can't be read, a group
// that matches two vault groups, or a name that matches two skills in its group
// is an error.
func Resolve(sels []selector.Selector, idx Index) (Result, error) {
	r := resolver{idx: idx}
	if err := r.apply(sels, ""); err != nil {
		return Result{}, err
	}
	if err := sameSelector(r.selected); err != nil {
		return Result{}, err
	}
	res, _ := settle(r.selected, func(Skill) bool { return false })
	res.Warnings = r.warnings
	return res, nil
}

// selection is a selected skill, its folded names and when it was last selected.
// A skill selected again keeps its place in the list, and counts as selected then
// in a clash. Settling compares every pair, so the names are folded only here.
// group is the group selector that last selected it, counted from 1, or 0 if no
// group selector did.
type selection struct {
	skill Skill
	names names
	last  int
	group int
}

func (s selection) clashes(o selection) bool { return s.names.clashes(o.names) }

type resolver struct {
	idx      Index
	selected []selection
	warnings []Warning
	count    int      // selections so far
	groups   int      // group selectors applied so far
	stack    []string // the sets being expanded, outermost first
}

// apply applies selectors that come from the set named in, or from the
// command if in is empty.
func (r *resolver) apply(sels []selector.Selector, in string) error {
	for _, sel := range sels {
		if sel.Kind == selector.Set {
			if err := r.applySet(sel, in); err != nil {
				return err
			}
			continue
		}
		skills, err := r.match(sel, in)
		if err != nil {
			return inSet(err, in)
		}
		group := 0
		if sel.Kind == selector.Group && !sel.Remove {
			r.groups++
			group = r.groups
		}
		for _, s := range skills {
			if sel.Remove {
				r.remove(s)
			} else {
				r.add(s, group)
			}
		}
	}
	return nil
}

// applySet applies a set's lines in place of @name. -@name removes what the
// set selects on its own, which is what the rest of the selector names.
func (r *resolver) applySet(sel selector.Selector, in string) error {
	want := selector.Fold(sel.Name)
	if i := slices.IndexFunc(r.stack, func(s string) bool { return selector.Fold(s) == want }); i >= 0 {
		return &CycleError{Cycle: append(slices.Clone(r.stack[i:]), sel.Name)}
	}
	lines, err := r.idx.Set(sel.Name)
	if errors.Is(err, ErrNoSet) {
		r.warn(sel, NoSet, in, suggestion{})
		return nil
	}
	if err != nil {
		return inSet(err, in)
	}
	sels := make([]selector.Selector, len(lines))
	for i, line := range lines {
		if sels[i], err = selector.Parse(line); err != nil {
			return inSet(err, sel.Name)
		}
	}

	r.stack = append(r.stack, sel.Name)
	defer func() { r.stack = r.stack[:len(r.stack)-1] }()
	if !sel.Remove {
		return r.apply(sels, sel.Name)
	}
	own := resolver{idx: r.idx, warnings: r.warnings, stack: r.stack}
	if err := own.apply(sels, sel.Name); err != nil {
		return err
	}
	r.warnings = own.warnings
	for _, o := range own.selected {
		r.remove(o.skill)
	}
	return nil
}

// inSet says which set holds the line that failed; in is empty for the command's
// own words.
func inSet(err error, in string) error {
	if in == "" {
		return err
	}
	return &SetLineError{Set: in, Err: err}
}

// match returns the skills a group or skill selector names, and warns if
// there are none.
func (r *resolver) match(sel selector.Selector, in string) ([]Skill, error) {
	group, exists, err := r.group(sel.Group)
	if err != nil {
		return nil, err
	}
	skills := r.idx.Skills(group)
	if sel.Kind == selector.Skill {
		found := find(skills, sel.Name)
		switch len(found) {
		case 0:
			r.warn(sel, NoSkill, in, suggestion{})
			return nil, nil
		case 1:
			return found, nil
		}
		return nil, &AmbiguousSkillError{Name: sel.Group + "/" + sel.Name, Skills: found}
	}
	if len(skills) > 0 {
		return skills, nil
	}
	if exists {
		r.warn(sel, NoSkill, in, suggestion{})
		return nil, nil
	}
	r.warn(sel, NoGroup, in, r.suggest(sel.Group))
	return nil, nil
}

// group finds the vault group a selector's group means, in any case or form.
// Two matches are an error, even if one is exact.
func (r *resolver) group(typed string) (group string, exists bool, err error) {
	want := selector.Fold(typed)
	var found []string
	for _, g := range r.idx.Groups() {
		if selector.Fold(g) == want {
			found = append(found, g)
		}
	}
	switch len(found) {
	case 0:
		return typed, false, nil
	case 1:
		return found[0], true, nil
	}
	return "", false, &AmbiguousGroupError{Group: typed, Groups: found}
}

// find looks a name up in one group, in any case or form: an exact folder name
// first, then folder names, then frontmatter names. It returns every skill the
// first of those that matches finds. An exact folder name is one skill, so each
// skill can always be named.
func find(skills []Skill, name string) []Skill {
	if i := slices.IndexFunc(skills, func(s Skill) bool { return s.Folder == name }); i >= 0 {
		return skills[i : i+1]
	}
	want := selector.Fold(name)
	var byFolder, byName []Skill
	for _, s := range skills {
		if selector.Fold(s.Folder) == want {
			byFolder = append(byFolder, s)
		} else if selector.Fold(s.Name) == want {
			byName = append(byName, s)
		}
	}
	if byFolder != nil {
		return byFolder
	}
	return byName
}

// suggestion is a selector to offer instead of a word, and the skill it selects.
type suggestion struct {
	word  string
	skill Skill
}

// suggest looks a name up in every group, in the index's order, and offers the first
// skill a selector can reach: a folder or a group outside the grammar can't be typed.
func (r *resolver) suggest(name string) suggestion {
	for _, g := range r.idx.Groups() {
		skills := r.idx.Skills(g)
		for _, s := range find(skills, name) {
			for _, n := range []string{s.Folder, s.Name} {
				word := g + "/" + n
				if _, err := selector.Parse(word); n == "" || err != nil {
					continue
				}
				if found := find(skills, n); len(found) == 1 && found[0] == s {
					return suggestion{word: word, skill: s}
				}
			}
		}
	}
	return suggestion{}
}

// warn adds a warning once.
func (r *resolver) warn(sel selector.Selector, problem Problem, in string, s suggestion) {
	w := Warning{Selector: sel, Problem: problem, Set: in, Suggest: s.word, Skill: s.skill}
	if !slices.Contains(r.warnings, w) {
		r.warnings = append(r.warnings, w)
	}
}

func (r *resolver) add(s Skill, group int) {
	r.count++
	if i := slices.IndexFunc(r.selected, func(o selection) bool { return o.skill == s }); i >= 0 {
		r.selected[i].last = r.count
		if group != 0 { // naming a skill doesn't say which of a group's clashing skills
			r.selected[i].group = group
		}
		return
	}
	r.selected = append(r.selected, selection{skill: s, names: foldNames(s), last: r.count, group: group})
}

func (r *resolver) remove(s Skill) {
	r.selected = slices.DeleteFunc(r.selected, func(o selection) bool { return o.skill == s })
}

// sameSelector finds two skills left selected by one group selector that clash.
// It runs after the removals: skl use data -data/PDF says which one is meant.
func sameSelector(selected []selection) error {
	for i, a := range selected {
		for _, b := range selected[i+1:] {
			if a.group != 0 && a.group == b.group && a.clashes(b) {
				return &ClashError{A: a.skill, B: b.skill}
			}
		}
	}
	return nil
}

// settle resolves clashes: going from the last selected to the first, a skill
// stays if it clashes with none that stayed and skip doesn't take it. So a skill is
// unloaded only by a later skill in the result, and the report names every such
// skill. It says nothing about a skill that lost a clash and then lost its own.
func settle(selected []selection, skip func(Skill) bool) (Result, []Skill) {
	byLast := slices.Clone(selected)
	slices.SortFunc(byLast, func(a, b selection) int { return b.last - a.last })
	var kept, gone []selection
	for _, o := range byLast {
		switch {
		case slices.ContainsFunc(kept, o.clashes):
		case skip(o.skill):
			gone = append(gone, o)
		default:
			kept = append(kept, o)
		}
	}

	res := Result{selected: selected}
	var skipped []Skill
	for _, o := range selected {
		switch {
		case slices.Contains(kept, o):
			res.Skills = append(res.Skills, o.skill)
		case slices.Contains(gone, o):
			skipped = append(skipped, o.skill)
		default:
			for _, w := range selected {
				if slices.Contains(kept, w) && w.last > o.last && o.clashes(w) {
					res.Replaced = append(res.Replaced, Replacement{Old: o.skill, New: w.skill})
				}
			}
		}
	}
	return res, skipped
}
