package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// bigScope makes state.json large enough that a torn write would show.
func bigScope(tag string) Scope {
	sc := Scope{}
	for i := range 2000 {
		sc.Selectors = append(sc.Selectors, fmt.Sprintf("%s/skill-%d", tag, i))
	}
	return sc
}

// Card #61 "Atomic writes (temp file + rename)": a reader that doesn't take
// the lock, as skl status doesn't, never sees half a file.
func TestSaveIsAtomicForReaders(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: bigScope("a")}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error)
	stop := time.Now().Add(300 * time.Millisecond)
	go func() {
		for i := 0; time.Now().Before(stop); i++ {
			tag := string(rune('a' + i%2))
			if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: bigScope(tag)}}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	reads := 0
	for time.Now().Before(stop) {
		data, err := os.ReadFile(p.State)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(data) {
			t.Fatalf("read %d saw a torn file of %d bytes", reads, len(data))
		}
		reads++
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	t.Logf("%d reads, all whole", reads)
}

// A failed write leaves the file as it was, and no temp file behind.
func TestSaveFailureKeepsTheOldFile(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: {Selectors: []string{"@old"}}}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.State)
	if err != nil {
		t.Fatal(err)
	}

	// Renaming over a non-empty directory fails after the temp file is
	// written, so this tests the last step going wrong.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(blocked, []byte("new")); err == nil {
		t.Fatal("rename over a directory worked")
	}
	entries, err := os.ReadDir(filepath.Dir(blocked))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}

	// The temp file can't even be created: the folder is read-only.
	if err := os.Chmod(p.Home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p.Home, 0o700) })
	if os.Geteuid() != 0 {
		if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: {Selectors: []string{"@new"}}}}); err == nil {
			t.Error("Save in a read-only folder worked")
		}
	}
	after, err := os.ReadFile(p.State)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("file changed:\n%s", after)
	}
}

// A skl killed in the middle of a write leaves the last whole file.
func TestSaveSurvivesKill(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: bigScope("a")}}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		cmd, _ := startHelper(t, p, "save-loop")
		time.Sleep(30 * time.Millisecond)
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		_ = cmd.Wait()
		st, err := s.Load(ctx)
		if err != nil {
			t.Fatalf("after kill: %v", err)
		}
		if n := len(st.Scopes[User].Selectors); n != 2000 {
			t.Fatalf("after kill: %d selectors", n)
		}
	}
	// A temp file a kill left behind is never read as anything.
	entries, err := os.ReadDir(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" && !strings.HasPrefix(e.Name(), ".state.json.") {
			t.Errorf("unexpected file %s", e.Name())
		}
	}
}
