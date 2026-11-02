package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var genTime = time.Date(2026, 10, 22, 14, 3, 0, 0, time.UTC)

// docs/design/architecture.md "Generations": "A generation is a snapshot of
// what's loaded in each scope plus the vault commit, kept in
// ~/.skl/generations/."
func TestGenerationRoundTrip(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	repo := Project("/home/u/code/p1")
	want := Generation{
		Time:        genTime,
		Command:     "use @frontend",
		VaultCommit: "9b1d4e0c2f7a8e35d6b1c0f4a9e2d7b3c5f8a1e6",
		Scopes: map[ScopeKey]Scope{
			User: sampleScope("@reviews"),
			repo: sampleScope("@frontend"),
		},
		Changed: []ScopeKey{repo},
	}
	added, err := s.addGeneration(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	want.N = 1
	if !reflect.DeepEqual(added, want) {
		t.Errorf("added:\n got %+v\nwant %+v", added, want)
	}
	if _, err := os.Stat(filepath.Join(p.Generations, "1.json")); err != nil {
		t.Error(err)
	}
	got, err := s.Generation(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read:\n got %+v\nwant %+v", got, want)
	}
}

// docs/guide/rollback.md: generations are numbered on each machine, and
// "skl history lists every generation, newest first".
func TestGenerationNumbering(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()

	if gens, err := s.Generations(ctx); err != nil || len(gens) != 0 {
		t.Fatalf("no generations yet: %v, %v", gens, err)
	}
	// Past 9, so numeric order and file-name order differ.
	for want := 1; want <= 12; want++ {
		g, err := s.addGeneration(ctx, Generation{Time: genTime, Command: "use"})
		if err != nil {
			t.Fatal(err)
		}
		if g.N != want {
			t.Fatalf("number = %d, want %d", g.N, want)
		}
	}
	gens, err := s.Generations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 12 {
		t.Fatalf("%d generations", len(gens))
	}
	for i, g := range gens {
		if g.N != 12-i {
			t.Errorf("gens[%d].N = %d", i, g.N)
		}
	}
}

// A gap never brings a number back, and other files in the folder are not
// generations.
func TestGenerationNumberingIgnoresOtherFiles(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := os.MkdirAll(p.Generations, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".7.json.123.tmp", "notes.txt", "x.json", "0.json", "-3.json", "07.json"} {
		if err := os.WriteFile(filepath.Join(p.Generations, name), []byte("junk"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(p.Generations, "4.json"), []byte(`{"n":4,"command":"use"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := s.addGeneration(ctx, Generation{Time: genTime})
	if err != nil {
		t.Fatal(err)
	}
	if g.N != 5 {
		t.Errorf("number = %d, want 5", g.N)
	}
	gens, err := s.Generations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 2 || gens[0].N != 5 || gens[1].N != 4 {
		t.Errorf("gens = %+v", gens)
	}
}

// docs/guide/rollback.md: "✗ no generation 40 · skl history".
func TestNoGeneration(t *testing.T) {
	s, _ := testStore(t)
	_, err := s.Generation(context.Background(), 40)
	var no *NoGenerationError
	if !errors.As(err, &no) || no.N != 40 {
		t.Fatalf("err = %#v", err)
	}
	if got, want := err.Error(), "no generation 40"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestCorruptGeneration(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := os.MkdirAll(p.Generations, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Generations, "1.json"), []byte("{ nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Generation(ctx, 1); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Generation: %v", err)
	}
	if _, err := s.Generations(ctx); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Generations: %v", err)
	}
}

func TestLastGeneration(t *testing.T) {
	s, _ := testStore(t)
	ctx := t.Context()
	if n, err := s.LastGeneration(ctx); err != nil || n != 0 {
		t.Fatalf("no generations: %d, %v", n, err)
	}
	for _, want := range []int{1, 2} {
		if _, err := s.addGeneration(ctx, Generation{Command: "x"}); err != nil {
			t.Fatal(err)
		}
		if n, err := s.LastGeneration(ctx); err != nil || n != want {
			t.Fatalf("LastGeneration = %d, %v, want %d", n, err, want)
		}
	}
}
