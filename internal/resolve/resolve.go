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

// clashes reports whether two skills have the same folder name or the same
// frontmatter name.
func (s Skill) clashes(o Skill) bool {
	return s.Folder == o.Folder || (s.Name != "" && s.Name == o.Name)
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
	Skills   []Skill // in the order they were selected; no two clash
	Replaced []Replacement
	Warnings []Warning
}

// Replacement is a skill that lost a clash to a later selector. Only clashes
// that still hold in the result are reported, against the final winner.
type Replacement struct {
	Old, New Skill
}

// Detail returns "someone/pr-review replaces review/pr-review".
func (r Replacement) Detail() string { return r.New.String() + " replaces " + r.Old.String() }

// Warning is a selector that matched nothing.
type Warning struct {
	Selector string // as written, "-" included
	Problem  string // "matches no skill"
	Set      string // the named skill set that holds the line, if any
	Hint     string // the command that deals with it
}

// Message returns the warning without its hint:
// "review/old-name   matches no skill (in @backend)".
func (w Warning) Message() string {
	msg := w.Selector + "   " + w.Problem
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

// Resolve applies sels left to right. A selector that matches nothing is a
// warning. A set that includes itself, a set line that isn't a selector, or a
// set that can't be read is an error.
func Resolve(sels []selector.Selector, idx Index) (Result, error) {
	r := resolver{idx: idx}
	if err := r.apply(sels, ""); err != nil {
		return Result{}, err
	}
	return r.Result, nil
}

type resolver struct {
	Result
	idx   Index
	stack []string // the sets being expanded, outermost first
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

// applySet applies a set's lines in place of @name. -@name removes the skills
// the set would add on its own, which is what the rest of the selector names.
func (r *resolver) applySet(sel selector.Selector, in string) error {
	if i := slices.Index(r.stack, sel.Name); i >= 0 {
		return &CycleError{Cycle: append(slices.Clone(r.stack[i:]), sel.Name)}
	}
	lines, err := r.idx.Set(sel.Name)
	if errors.Is(err, ErrNoSet) {
		r.warn(sel, "matches no skill set", in, "")
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
	own := resolver{idx: r.idx, stack: r.stack}
	own.Warnings = r.Warnings
	if err := own.apply(sels, sel.Name); err != nil {
		return err
	}
	r.Warnings = own.Warnings
	for _, s := range own.Skills {
		r.remove(s)
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
		r.warn(sel, "matches no skill", in, "")
		return nil
	}
	if len(skills) > 0 {
		return skills
	}
	if slices.Contains(r.idx.Groups(), sel.Group) {
		r.warn(sel, "matches no skill", in, "")
		return nil
	}
	suggest := ""
	if s, ok := r.skillNamed(sel.Group); ok {
		suggest = s.String()
		if sel.Remove {
			suggest = "-" + suggest
		}
	}
	r.warn(sel, "matches no group", in, suggest)
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

// warn adds a warning once. suggest is the selector the word probably meant.
func (r *resolver) warn(sel selector.Selector, problem, in, suggest string) {
	w := Warning{Selector: sel.String(), Problem: problem, Set: in}
	switch {
	case in != "" && suggest != "":
		w.Hint = fmt.Sprintf("edit sets/%s.toml to say %s", in, suggest)
	case in != "":
		w.Hint = fmt.Sprintf("edit sets/%s.toml", in)
	case suggest != "":
		w.Hint = "skl use " + suggest
	default:
		w.Hint = "skl drop " + sel.String()
	}
	if !slices.Contains(r.Warnings, w) {
		r.Warnings = append(r.Warnings, w)
	}
}

// add appends s, replacing every skill it clashes with. A skill already there
// keeps its place.
func (r *resolver) add(s Skill) {
	if slices.Contains(r.Skills, s) {
		return
	}
	var lost []Skill
	r.Skills = slices.DeleteFunc(r.Skills, func(o Skill) bool {
		if !o.clashes(s) {
			return false
		}
		lost = append(lost, o)
		return true
	})
	r.Skills = append(r.Skills, s)

	// Whoever the losers had replaced, s has now replaced.
	for i, rep := range r.Replaced {
		if slices.Contains(lost, rep.New) {
			r.Replaced[i].New = s
		}
	}
	for _, o := range lost {
		r.Replaced = append(r.Replaced, Replacement{Old: o, New: s})
	}
	// A skill that came back is no longer replaced by anything.
	r.Replaced = slices.DeleteFunc(r.Replaced, func(rep Replacement) bool { return rep.Old == s })
}

// remove drops s, and the clashes it took part in: the replacement either
// reports no longer holds.
func (r *resolver) remove(s Skill) {
	r.Skills = slices.DeleteFunc(r.Skills, func(o Skill) bool { return o == s })
	r.Replaced = slices.DeleteFunc(r.Replaced, func(rep Replacement) bool { return rep.Old == s || rep.New == s })
}
