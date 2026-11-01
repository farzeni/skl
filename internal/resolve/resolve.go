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

// clashes reports whether two skills share a name. A skill's names are its
// folder name and its frontmatter name.
func (s Skill) clashes(o Skill) bool {
	return s.Folder == o.Folder || s.Folder == o.Name || s.Name == o.Folder || (s.Name != "" && s.Name == o.Name)
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
	Suggest  string // the skill a one-word name probably meant, as group/folder
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

// Resolve applies sels left to right: each removal takes away what the earlier
// selectors selected. Then, among what's left, the later of two skills that
// clash wins. A selector that matches nothing is a warning. A set that includes
// itself, a set line that isn't a selector, or a set that can't be read is an
// error.
func Resolve(sels []selector.Selector, idx Index) (Result, error) {
	r := resolver{idx: idx}
	if err := r.apply(sels, ""); err != nil {
		return Result{}, err
	}
	res := settle(r.selected)
	res.Warnings = r.warnings
	return res, nil
}

// selection is a selected skill and when it was last selected. A skill selected
// again keeps its place in the list, and counts as selected then in a clash.
type selection struct {
	skill Skill
	last  int
}

func (s selection) clashes(o selection) bool { return s.skill.clashes(o.skill) }

type resolver struct {
	idx      Index
	selected []selection
	warnings []Warning
	count    int      // selections so far
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
		for _, s := range r.match(sel, in) {
			if sel.Remove {
				r.remove(s)
			} else {
				r.add(s)
			}
		}
	}
	return nil
}

// applySet applies a set's lines in place of @name. -@name removes what the
// set selects on its own, which is what the rest of the selector names.
func (r *resolver) applySet(sel selector.Selector, in string) error {
	if i := slices.Index(r.stack, sel.Name); i >= 0 {
		return &CycleError{Cycle: append(slices.Clone(r.stack[i:]), sel.Name)}
	}
	lines, err := r.idx.Set(sel.Name)
	if errors.Is(err, ErrNoSet) {
		r.warn(sel, NoSet, in, "")
		return nil
	}
	if err != nil {
		return fmt.Errorf("@%s: %w", sel.Name, err)
	}
	sels := make([]selector.Selector, len(lines))
	for i, line := range lines {
		if sels[i], err = selector.Parse(line); err != nil {
			return fmt.Errorf("@%s: %w", sel.Name, err)
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

// match returns the skills a group or skill selector names, and warns if
// there are none.
func (r *resolver) match(sel selector.Selector, in string) []Skill {
	skills := r.idx.Skills(sel.Group)
	if sel.Kind == selector.Skill {
		if s, ok := find(skills, sel.Name); ok {
			return []Skill{s}
		}
		r.warn(sel, NoSkill, in, "")
		return nil
	}
	if len(skills) > 0 {
		return skills
	}
	if slices.Contains(r.idx.Groups(), sel.Group) {
		r.warn(sel, NoSkill, in, "")
		return nil
	}
	suggest := ""
	if s, ok := r.skillNamed(sel.Group); ok {
		suggest = s.String()
	}
	r.warn(sel, NoGroup, in, suggest)
	return nil
}

// find looks a name up in one group: a folder name first, then a frontmatter
// name. Folders are unique in a group; of two equal frontmatter names, the
// first wins.
func find(skills []Skill, name string) (Skill, bool) {
	if i := slices.IndexFunc(skills, func(s Skill) bool { return s.Folder == name }); i >= 0 {
		return skills[i], true
	}
	if i := slices.IndexFunc(skills, func(s Skill) bool { return s.Name == name }); i >= 0 {
		return skills[i], true
	}
	return Skill{}, false
}

// skillNamed looks a name up in every group, in the index's order.
func (r *resolver) skillNamed(name string) (Skill, bool) {
	for _, g := range r.idx.Groups() {
		if s, ok := find(r.idx.Skills(g), name); ok {
			return s, true
		}
	}
	return Skill{}, false
}

// warn adds a warning once.
func (r *resolver) warn(sel selector.Selector, problem Problem, in, suggest string) {
	w := Warning{Selector: sel, Problem: problem, Set: in, Suggest: suggest}
	if !slices.Contains(r.warnings, w) {
		r.warnings = append(r.warnings, w)
	}
}

func (r *resolver) add(s Skill) {
	r.count++
	if i := slices.IndexFunc(r.selected, func(o selection) bool { return o.skill == s }); i >= 0 {
		r.selected[i].last = r.count
		return
	}
	r.selected = append(r.selected, selection{skill: s, last: r.count})
}

func (r *resolver) remove(s Skill) {
	r.selected = slices.DeleteFunc(r.selected, func(o selection) bool { return o.skill == s })
}

// settle resolves clashes: going from the last selected to the first, a skill
// stays if it clashes with none that stayed. So a skill is unloaded only by a
// later skill in the result, and the report names every such skill. It says
// nothing about a skill that lost a clash and then lost its own.
func settle(selected []selection) Result {
	byLast := slices.Clone(selected)
	slices.SortFunc(byLast, func(a, b selection) int { return b.last - a.last })
	var kept []selection
	for _, o := range byLast {
		if !slices.ContainsFunc(kept, o.clashes) {
			kept = append(kept, o)
		}
	}

	var res Result
	for _, o := range selected {
		if slices.Contains(kept, o) {
			res.Skills = append(res.Skills, o.skill)
		}
	}
	for _, o := range selected {
		if slices.Contains(kept, o) {
			continue
		}
		for _, w := range selected {
			if slices.Contains(kept, w) && w.last > o.last && o.clashes(w) {
				res.Replaced = append(res.Replaced, Replacement{Old: o.skill, New: w.skill})
			}
		}
	}
	return res
}
