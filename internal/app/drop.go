package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/vault"
)

// Drop is skl drop <selectors>. A word that is a line of the active skill set takes
// that line out. If what it names is still loaded through another line, such as one
// skill of a loaded group, a removal line goes in too. A word that does neither
// isn't in the active skill set, which is only a warning
// (docs/reference/repos.md "Grammar").
func (a *App) Drop(ctx context.Context, words []string, dryRun bool) error {
	return a.loading(ctx, selection{
		verb:  "drop",
		label: "drop",
		words: words,
		next: func(active []string, sels []selector.Selector, idx *vault.Index) ([]string, []note, error) {
			next := slices.Clone(active)
			var notes []note
			for i, word := range words {
				var n *note
				var err error
				next, n, err = dropWord(next, word, sels[i], idx)
				if err != nil {
					return nil, nil, err
				}
				if n != nil {
					notes = append(notes, *n)
				}
			}
			return next, notes, nil
		},
	}, dryRun)
}

func dropWord(lines []string, word string, sel selector.Selector, idx *vault.Index) ([]string, *note, error) {
	kept := slices.DeleteFunc(slices.Clone(lines), func(l string) bool { return sameLine(l, sel) })
	removed := len(kept) < len(lines)
	if sel.Remove && removed {
		return kept, nil, nil // a removal taken back: what it held off is loaded again
	}
	loaded, err := resolved(kept, idx)
	if err != nil {
		return nil, nil, err
	}
	positive := sel
	positive.Remove = false
	named, err := resolve.Resolve([]selector.Selector{positive}, idx)
	if _, clash := errors.AsType[*resolve.ClashError](err); clash {
		// A group whose skills clash can still be dropped: that's one way out.
		if removed {
			return kept, nil, nil
		}
		return append(kept, "-"+word), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !sel.Remove && slices.ContainsFunc(named.Skills, func(s resolve.Skill) bool { return slices.Contains(loaded, s) }) {
		return append(kept, "-"+word), nil, nil
	}
	if removed {
		return kept, nil, nil
	}
	return kept, notInActive(word, named, loaded), nil
}

// notInActive is the warning for a word the active skill set doesn't hold. When the
// word is one skill's name and that skill is loaded, it says how to drop that.
func notInActive(word string, named resolve.Result, loaded []resolve.Skill) *note {
	for _, w := range named.Warnings {
		if w.Suggest != "" && slices.ContainsFunc(loaded, func(s resolve.Skill) bool { return s.String() == w.Suggest }) {
			return &note{msg: word + "   " + w.Problem.String(), hint: "skl drop " + w.Suggest}
		}
	}
	return &note{msg: word + "   isn't in the active skill set", hint: "skl status"}
}

// resolved is the skills lines select, clashes or not: a skill that lost a clash
// can still be the one loaded, where someone else's skill holds the winner's name.
func resolved(lines []string, idx *vault.Index) ([]resolve.Skill, error) {
	sels, err := parseLines(lines)
	if err != nil {
		return nil, err
	}
	res, err := resolve.Resolve(sels, idx)
	if err != nil {
		return nil, err
	}
	return res.Selected(), nil
}

// sameLine reports whether a line of the active skill set is the selector, in any
// case or Unicode form.
func sameLine(line string, sel selector.Selector) bool {
	got, err := selector.Parse(line)
	if err != nil {
		return strings.EqualFold(line, sel.String())
	}
	return selector.Fold(got.String()) == selector.Fold(sel.String())
}
