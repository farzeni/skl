package app

import (
	"context"
	"slices"

	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/vault"
)

// Add is skl add <selectors>: the words join the end of the active skill set. A
// word already there that would change nothing stays out, so running it again
// changes nothing.
func (a *App) Add(ctx context.Context, words []string, dryRun bool) error {
	return a.loading(ctx, selection{
		verb:  "add",
		label: "add",
		words: words,
		next: func(active []string, sels []selector.Selector, idx *vault.Index) ([]string, []note, error) {
			next := slices.Clone(active)
			for i, word := range words {
				held := slices.ContainsFunc(next, func(l string) bool { return sameLine(l, sels[i]) })
				if held && noEffect(next, sels[i], idx) {
					continue
				}
				next = append(next, word)
			}
			return next, nil, nil
		},
	}, dryRun)
}

// noEffect reports whether adding sel to lines leaves the same skills. Anything
// that can't be resolved counts as an effect: the final resolve says what's wrong.
func noEffect(lines []string, sel selector.Selector, idx *vault.Index) bool {
	sels, err := parseLines(lines)
	if err != nil {
		return false
	}
	before, err := resolve.Resolve(sels, idx)
	if err != nil {
		return false
	}
	after, err := resolve.Resolve(append(sels, sel), idx)
	return err == nil && slices.Equal(before.Skills, after.Skills)
}
