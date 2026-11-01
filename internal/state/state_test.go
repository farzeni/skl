package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/config"
)

func testStore(t *testing.T) (*Store, config.Paths) {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".skl")
	p, err := config.NewPaths(func(k string) string {
		if k == "SKL_HOME" {
			return home
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(p, nil), p
}

func sampleScope(selector string) Scope {
	return Scope{
		Selectors: []string{selector, "-review/test-gaps"},
		Copies: []Copy{{
			Name:   "pr-review",
			Target: config.Claude,
			Skill:  "review/pr-review",
			Commit: "9b1d4e0c2f7a8e35d6b1c0f4a9e2d7b3c5f8a1e6",
			Tree:   "5d0e7aa2f7a8e35d6b1c0f4a9e2d7b3c5f8a1e60",
		}},
	}
}

// docs/reference/files.md "Local state": "state.json # what's loaded where".
func TestStateRoundTrip(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	want := &State{Scopes: map[ScopeKey]Scope{
		User:                       sampleScope("@backend"),
		Project("/home/u/code/p1"): sampleScope("@frontend"),
	}}
	if err := s.save(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, want)
	}
}

// The schema is keyed by scope from day one: docs/design/architecture.md
// "Generations": "kept in state.json, keyed by the repo's top-level path".
func TestStateFileIsKeyedByScope(t *testing.T) {
	s, p := testStore(t)
	st := &State{Scopes: map[ScopeKey]Scope{
		User:                       {Selectors: []string{"@reviews"}},
		Project("/home/u/code/p1"): {Selectors: []string{"@frontend"}},
	}}
	if err := s.save(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p.State)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": 1`, `"user": {`, `"project:/home/u/code/p1": {`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("state.json misses %s:\n%s", want, data)
		}
	}
}

func TestScopeKey(t *testing.T) {
	tests := []struct {
		key      ScopeKey
		wantRepo string
		wantOK   bool
	}{
		{User, "", false},
		{Project("/home/u/code/p1"), "/home/u/code/p1", true},
		// A git worktree is a repo of its own: its own path, its own key.
		{Project("/home/u/code/p1-wt"), "/home/u/code/p1-wt", true},
	}
	for _, tt := range tests {
		repo, ok := tt.key.Repo()
		if repo != tt.wantRepo || ok != tt.wantOK {
			t.Errorf("%q.Repo() = %q, %v; want %q, %v", tt.key, repo, ok, tt.wantRepo, tt.wantOK)
		}
	}
}

func TestLoadWithoutAFileIsEmpty(t *testing.T) {
	s, _ := testStore(t)
	st, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Scopes == nil || len(st.Scopes) != 0 {
		t.Errorf("Scopes = %#v, want an empty map", st.Scopes)
	}
	// What a caller does on a first run must work on what Load returned.
	st.Scopes[User] = Scope{Selectors: []string{"@base"}}
}

func TestLoadRefuses(t *testing.T) {
	tests := []struct {
		name, data string
		wantErr    error
	}{
		{"not JSON", "{ nope", ErrCorrupt},
		{"truncated", `{"version": 1, "sco`, ErrCorrupt},
		{"newer skl", `{"version": 99, "scopes": {}}`, ErrNewerState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, p := testStore(t)
			if err := os.MkdirAll(p.Home, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.State, []byte(tt.data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := s.Load(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), p.State) {
				t.Errorf("err %q doesn't name the file", err)
			}
		})
	}
}

func TestCanceledContext(t *testing.T) {
	s, _ := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Load: %v", err)
	}
	if err := s.save(ctx, &State{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Save: %v", err)
	}
	if _, err := s.addGeneration(ctx, Generation{}); !errors.Is(err, context.Canceled) {
		t.Errorf("AddGeneration: %v", err)
	}
	if _, err := s.Generation(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Generation: %v", err)
	}
	if _, err := s.Generations(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Generations: %v", err)
	}
	if _, err := s.Lock(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Lock: %v", err)
	}
}
