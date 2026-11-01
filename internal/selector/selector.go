// Package selector parses the words that name things in a vault, following
// docs/reference/repos.md "Grammar". It is pure: no disk, no git.
package selector

import (
	"errors"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
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

// ErrInvalid is a word that is neither a selector nor a reference, or an owner
// with no valid group name.
var ErrInvalid = errors.New("invalid name")

// invalidError is an error that is ErrInvalid and reads as its own sentence.
type invalidError string

func (e invalidError) Error() string { return string(e) }

// Is reports whether target is ErrInvalid.
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

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
	if strings.TrimSpace(rest) == "" {
		if remove {
			return Selector{}, invalid("-", "it removes nothing")
		}
		return Selector{}, invalidError("a selector can't be empty")
	}
	if strings.HasPrefix(rest, "-") {
		return Selector{}, invalid(word, badName)
	}
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
	return invalidError(word + " is not a valid selector: " + why)
}

// isReference follows the Grammar's "Anything else is a reference", with word
// already free of its "-". A "#" counts too: it only ever pins a reference.
// A "@" word with a dot is a bad set name, not a host. Past a path's first
// character, a word with a space is no reference: a path can hold one, the rest
// can't.
func isReference(word string) bool {
	if strings.HasPrefix(word, "~") || strings.HasPrefix(word, "/") || strings.HasPrefix(word, ".") {
		return true
	}
	if strings.ContainsFunc(word, unicode.IsSpace) {
		return false
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
// "-" and "_", starting with a letter or a digit. A letter may carry combining
// marks, so a name in NFD or in a script that needs them is valid.
func ValidName(name string) bool {
	afterLetter := false
	for i, r := range name {
		switch {
		case unicode.IsLetter(r):
			afterLetter = true
		case unicode.IsMark(r):
			if !afterLetter {
				return false
			}
		case unicode.IsDigit(r), i > 0 && (r == '-' || r == '_'):
			afterLetter = false
		default:
			return false
		}
	}
	return name != ""
}

// Fold returns the form names are compared in: Unicode NFC and case folding.
// Two names with the same Fold are one name (skill-sets.md "Names and clashes").
// Folding has no locale: ß folds to ss, while the Turkish ı stays its own letter.
func Fold(name string) string {
	folded := cases.Fold().String(norm.NFC.String(name))
	return norm.NFC.String(strings.Map(cherokeeUpper, folded))
}

// cherokeeUpper puts a Cherokee letter in the case Unicode folds it to, the
// upper one. x/text's Fold swaps the two cases, so a pair would never match.
func cherokeeUpper(r rune) rune {
	if unicode.Is(unicode.Cherokee, r) {
		return unicode.ToUpper(r)
	}
	return r
}

// ReservedGroup reports whether a group can't have this name, in any case or
// form: on a case-insensitive disk, Sets is the folder sets.
func ReservedGroup(name string) bool {
	return slices.Contains([]string{"sets", "lessons", "h"}, Fold(name))
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
		return "", invalidError("the owner " + owner + " gives no valid group name: " + badName)
	}
	return group, nil
}
