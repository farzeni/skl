package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Generation is one change: a snapshot of what's loaded plus the vault commit
// (docs/design/architecture.md "Generations"). It lives in
// generations/<n>.json and is never rewritten. Numbers are local to a machine.
type Generation struct {
	N           int       `json:"n"`
	Time        time.Time `json:"time"`
	Command     string    `json:"command"` // as skl history shows it
	VaultCommit string    `json:"vault_commit"`
	// Scopes is what was loaded after the change: the user scope and the
	// project scope of the repo it ran in, and any other scope it changed.
	Scopes map[ScopeKey]Scope `json:"scopes,omitempty"`
	// Changed lists the scopes the change touched, which are the ones a
	// rollback restores and the repo skl history shows.
	Changed []ScopeKey `json:"changed,omitempty"`
}

// NoGenerationError is a generation that doesn't exist.
type NoGenerationError struct{ N int }

func (e *NoGenerationError) Error() string { return fmt.Sprintf("no generation %d", e.N) }

// AddGeneration writes g as the next generation and returns it with its
// number. The caller holds the lock and sets everything but N.
func (s *Store) AddGeneration(ctx context.Context, g Generation) (Generation, error) {
	if err := ctx.Err(); err != nil {
		return Generation{}, err
	}
	numbers, err := s.numbers()
	if err != nil {
		return Generation{}, err
	}
	g.N = 1
	if len(numbers) > 0 {
		g.N = slices.Max(numbers) + 1
	}
	data, err := marshal(g)
	if err != nil {
		return Generation{}, fmt.Errorf("generations: %w", err)
	}
	if err := writeAtomic(s.generationPath(g.N), data); err != nil {
		return Generation{}, fmt.Errorf("generations: %w", err)
	}
	return g, nil
}

// Generation reads generation n, or returns a *NoGenerationError.
func (s *Store) Generation(ctx context.Context, n int) (Generation, error) {
	if err := ctx.Err(); err != nil {
		return Generation{}, err
	}
	path := s.generationPath(n)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Generation{}, &NoGenerationError{N: n}
	}
	if err != nil {
		return Generation{}, fmt.Errorf("generations: %w", err)
	}
	var g Generation
	if err := json.Unmarshal(data, &g); err != nil {
		return Generation{}, fmt.Errorf("%w %s: %w", ErrCorrupt, path, err)
	}
	g.N = n // the file's name is what the numbering goes by
	return g, nil
}

// Generations returns every generation, newest first.
func (s *Store) Generations(ctx context.Context) ([]Generation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	numbers, err := s.numbers()
	if err != nil {
		return nil, err
	}
	slices.Sort(numbers)
	slices.Reverse(numbers)
	gens := make([]Generation, 0, len(numbers))
	for _, n := range numbers {
		g, err := s.Generation(ctx, n)
		if err != nil {
			return nil, err
		}
		gens = append(gens, g)
	}
	return gens, nil
}

func (s *Store) generationPath(n int) string {
	return filepath.Join(s.generations, strconv.Itoa(n)+".json")
}

// numbers lists the generations on disk. Only a file named exactly
// <n>.json counts, so a temp file left by a crash doesn't.
func (s *Store) numbers() ([]int, error) {
	entries, err := os.ReadDir(s.generations)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generations: %w", err)
	}
	var numbers []int
	for _, e := range entries {
		stem, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !e.Type().IsRegular() {
			continue
		}
		if n, err := strconv.Atoi(stem); err == nil && n > 0 && strconv.Itoa(n) == stem {
			numbers = append(numbers, n)
		}
	}
	return numbers, nil
}
