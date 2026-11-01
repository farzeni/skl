// Package selector parses the words that name things in a vault, following
// docs/reference/repos.md "Grammar". It is pure: no disk, no git.
package selector

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Kind is what a selector names.
type Kind int

// The three kinds of selector.
const (
	Group Kind = iota // review
	Skill             // review/pr-review
	Set               // @backend
)

// Selector is one parsed word.
type Selector struct {
	Remove bool // the word starts with "-"
	Kind   Kind
	Group  string // for Group and Skill
	Name   string // the skill's or the set's name
}

// String returns the word the selector was parsed from.
func (s Selector) String() string {
	var word string
	switch s.Kind {
	case Skill:
		word = s.Group + "/" + s.Name
	case Set:
		word = "@" + s.Name
	default:
		word = s.Group
	}
	if s.Remove {
		return "-" + word
	}
	return word
}

// ErrInvalid is a word that is neither a selector nor a reference.
var ErrInvalid = errors.New("invalid selector")

// ErrReference is a word that names something in someone else's repo. Parse
// returns it as a *ReferenceError.
var ErrReference = errors.New("reference")

// ReferenceError is a reference given where a selector belongs.
type ReferenceError struct {
	Ref string // the word, without a leading "-"
}

func (e *ReferenceError) Error() string { return e.Ref + " is someone else's skill" }

// Is reports whether target is ErrReference.
func (e *ReferenceError) Is(target error) bool { return target == ErrReference }

// Parse reads one word. It returns a *ReferenceError for a reference and an
// ErrInvalid error for anything else that isn't a selector.
func Parse(word string) (Selector, error) {
	rest, remove := strings.CutPrefix(word, "-")
	if isReference(rest) {
		return Selector{}, &ReferenceError{Ref: rest}
	}
	if name, ok := strings.CutPrefix(rest, "@"); ok {
		if !ValidName(name) {
			return Selector{}, invalid(word, badName)
		}
		return Selector{Remove: remove, Kind: Set, Name: name}, nil
	}
	group, name, two := strings.Cut(rest, "/")
	if !ValidName(group) || (two && !ValidName(name)) {
		return Selector{}, invalid(word, badName)
	}
	if ReservedGroup(group) {
		return Selector{}, invalid(word, "a group can't be named "+group)
	}
	if two {
		return Selector{Remove: remove, Kind: Skill, Group: group, Name: name}, nil
	}
	return Selector{Remove: remove, Kind: Group, Group: group}, nil
}

const badName = "a name has letters, digits, - and _, and starts with a letter or a digit"

func invalid(word, why string) error {
	return fmt.Errorf("%w %s: %s", ErrInvalid, word, why)
}

// isReference follows the Grammar's "Anything else is a reference", with word
// already free of its "-". A "#" counts too: it only ever pins a reference.
// A "@" word with a dot is a bad set name, not a host.
func isReference(word string) bool {
	if strings.HasPrefix(word, "~") || strings.HasPrefix(word, "/") || strings.HasPrefix(word, ".") {
		return true
	}
	if strings.Contains(word, "://") || isSCP(word) {
		return true
	}
	parts := strings.Split(word, "/")
	if len(parts) >= 3 {
		return true
	}
	if strings.HasPrefix(word, "@") {
		return false
	}
	return strings.Contains(parts[0], ".") || strings.Contains(word, "#")
}

// isSCP reports an SSH URL in the form git@host:path.
func isSCP(word string) bool {
	at := strings.Index(word, "@")
	return at > 0 && strings.Contains(word[at:], ":")
}

// ValidName reports whether name fits the grammar's name: letters, digits,
// "-" and "_", starting with a letter or a digit.
func ValidName(name string) bool {
	for i, r := range name {
		alnum := unicode.IsLetter(r) || unicode.IsDigit(r)
		if !alnum && (i == 0 || (r != '-' && r != '_')) {
			return false
		}
	}
	return name != ""
}

// ReservedGroup reports whether a group can't have this name.
func ReservedGroup(name string) bool {
	return slices.Contains([]string{"sets", "lessons", "h"}, name)
}

// OwnerGroup returns the group for a new skill whose group would be owner:
// the owner's name, or <owner>-skills if that is reserved or isn't a valid
// name. It returns an ErrInvalid error if <owner>-skills isn't valid either.
func OwnerGroup(owner string) (string, error) {
	if ValidName(owner) && !ReservedGroup(owner) {
		return owner, nil
	}
	group := owner + "-skills"
	if !ValidName(group) {
		return "", fmt.Errorf("%w group for owner %s: %s", ErrInvalid, owner, badName)
	}
	return group, nil
}
