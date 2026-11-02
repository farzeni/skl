package app

import (
	"context"

	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/vault"
)

// Use is skl use <selectors>: the words become the active skill set
// (docs/guide/skill-sets.md "Load, add and drop").
func (a *App) Use(ctx context.Context, words []string, dryRun bool) error {
	return a.loading(ctx, selection{
		verb:  "use",
		label: "load",
		words: words,
		next: func([]string, []selector.Selector, *vault.Index) ([]string, []note, error) {
			return words, nil, nil
		},
	}, dryRun)
}
