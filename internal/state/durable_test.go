package state

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/config"
)

// deepStore is a store whose home is three missing folders below an existing
// one, which it returns.
func deepStore(t *testing.T) (s *Store, p config.Paths, root string) {
	t.Helper()
	root = t.TempDir()
	home := filepath.Join(root, "a", "b", ".skl")
	p, err := config.NewPaths(func(k string) string {
		if k == "SKL_HOME" {
			return home
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(p, nil), p, root
}

// syncRecorder replaces the store's fsync. It records each folder synced and
// what was in it at that moment, then does the real sync.
type syncRecorder struct {
	dirs     []string
	contents map[string][]string
}

func recordSyncs(s *Store) *syncRecorder {
	r := &syncRecorder{contents: map[string][]string{}}
	s.syncFile = func(f *os.File) error {
		r.dirs = append(r.dirs, f.Name())
		entries, err := os.ReadDir(f.Name())
		if err != nil {
			return err
		}
		for _, e := range entries {
			r.contents[f.Name()] = append(r.contents[f.Name()], e.Name())
		}
		return f.Sync()
	}
	return r
}

// take returns the folders synced since the last call.
func (r *syncRecorder) take() []string {
	dirs := r.dirs
	r.dirs = nil
	return dirs
}

func (r *syncRecorder) wantHeld(t *testing.T, dir, child string) {
	t.Helper()
	if !slices.Contains(r.contents[dir], child) {
		t.Errorf("%s was synced before it held %s: %v", dir, child, r.contents[dir])
	}
}

// Card #96 item 1: docs/reference/files.md "Local state": "generations/ and
// state.json hold your rollback history", so a power cut must not lose the
// folders that hold them.
func TestNewFoldersAreSynced(t *testing.T) {
	s, p, root := deepStore(t)
	rec := recordSyncs(s)
	ctx := context.Background()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "a", "b")

	// skl init is the first to take the lock, and creates ~/.skl.
	l, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	if got, want := rec.take(), []string{b, a, root}; !slices.Equal(got, want) {
		t.Errorf("Lock on a new home synced %v, want %v", got, want)
	}
	rec.wantHeld(t, b, ".skl")
	rec.wantHeld(t, a, "b")
	rec.wantHeld(t, root, "a")

	// The first generation creates generations/.
	if _, err := l.AddGeneration(ctx, Generation{Time: genTime}); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.take(), []string{p.Home, p.Generations}; !slices.Equal(got, want) {
		t.Errorf("first generation synced %v, want %v", got, want)
	}
	rec.wantHeld(t, p.Home, "generations")
	rec.wantHeld(t, p.Generations, "1.json")

	// Folders that exist need no sync of their parent.
	if _, err := l.AddGeneration(ctx, Generation{Time: genTime}); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.take(), []string{p.Generations}; !slices.Equal(got, want) {
		t.Errorf("second generation synced %v, want %v", got, want)
	}
	if err := l.Save(ctx, &State{}); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.take(), []string{p.Home}; !slices.Equal(got, want) {
		t.Errorf("Save synced %v, want %v", got, want)
	}
}

// A save that creates the home itself syncs the same way.
func TestSaveCreatesAndSyncsTheHome(t *testing.T) {
	s, p, root := deepStore(t)
	rec := recordSyncs(s)
	if err := s.save(context.Background(), &State{}); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(root, "a"), filepath.Join(root, "a", "b")
	if got, want := rec.take(), []string{b, a, root, p.Home}; !slices.Equal(got, want) {
		t.Errorf("synced %v, want %v", got, want)
	}
}

// Card #96 item 2. os.File.Sync returns a *fs.PathError, as the first case.
func TestFolderSyncRefused(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"ENOTSUP from Sync", &fs.PathError{Op: "sync", Path: "x", Err: syscall.ENOTSUP}},
		{"ENOTSUP", syscall.ENOTSUP},
		{"EINVAL", syscall.EINVAL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, p, _ := deepStore(t)
			var debug bytes.Buffer
			s.log = slog.New(slog.NewTextHandler(&debug, &slog.HandlerOptions{Level: slog.LevelDebug}))
			s.syncFile = func(*os.File) error { return tt.err }
			ctx := context.Background()

			l, err := s.Lock(ctx)
			if err != nil {
				t.Fatalf("Lock creating folders: %v", err)
			}
			t.Cleanup(func() { _ = l.Release() })
			if err := l.Save(ctx, &State{Scopes: map[ScopeKey]Scope{User: {Selectors: []string{"@new"}}}}); err != nil {
				t.Errorf("Save: %v", err)
			}
			if g, err := l.AddGeneration(ctx, Generation{Time: genTime}); err != nil || g.N != 1 {
				t.Errorf("AddGeneration: %+v, %v", g, err)
			}
			st, err := s.Load(ctx)
			if err != nil || st.Scopes[User].Selectors[0] != "@new" {
				t.Errorf("Load: %+v, %v", st, err)
			}
			// Logged for each folder it couldn't sync, the state.json one included.
			if !strings.Contains(debug.String(), "folder sync not supported") ||
				!strings.Contains(debug.String(), "dir="+p.Home) {
				t.Errorf("debug log: %q", debug.String())
			}
		})
	}
}

// With no debug logger, as without SKL_DEBUG, a refusal is silent and still fine.
func TestFolderSyncRefusedIsQuietWithoutDebug(t *testing.T) {
	s, _ := testStore(t)
	var out bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s.syncFile = func(*os.File) error { return syscall.ENOTSUP }
	if err := s.save(context.Background(), &State{}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("logged: %q", out.String())
	}
}

// Card #96 item 2: "Other errors still fail the save."
func TestFolderSyncFails(t *testing.T) {
	ctx := context.Background()
	for _, want := range []error{syscall.EIO, syscall.ENOSPC, syscall.EACCES} {
		t.Run(want.Error(), func(t *testing.T) {
			s, _ := testStore(t)
			s.syncFile = func(*os.File) error { return &fs.PathError{Op: "sync", Path: "x", Err: want} }
			if err := s.save(ctx, &State{}); !errors.Is(err, want) {
				t.Errorf("Save on a new home: %v", err)
			}
			if _, err := s.addGeneration(ctx, Generation{Time: genTime}); !errors.Is(err, want) {
				t.Errorf("AddGeneration: %v", err)
			}
		})
		t.Run(want.Error()+" on the rename", func(t *testing.T) {
			s, p := testStore(t)
			if err := os.MkdirAll(p.Home, 0o755); err != nil {
				t.Fatal(err)
			}
			s.syncFile = func(*os.File) error { return want }
			if err := s.save(ctx, &State{}); !errors.Is(err, want) {
				t.Errorf("Save: %v", err)
			}
		})
		t.Run(want.Error()+" in Lock", func(t *testing.T) {
			s, _ := testStore(t)
			s.syncFile = func(*os.File) error { return want }
			if l, err := s.Lock(ctx); !errors.Is(err, want) {
				_ = l.Release()
				t.Errorf("Lock: %v", err)
			}
		})
	}
}

// writeFile writes a file with this modification time, its folder included.
func writeFile(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Card #96 item 3: "removed the next time skl takes the lock, if older than the
// lock holder's start."
func TestLockRemovesLeftoverTempFiles(t *testing.T) {
	s, p := testStore(t)
	// A start in the future, so the folder and the link made below count as old.
	s.started = time.Now().Add(time.Minute)
	before := s.started.Add(-time.Hour)
	after := s.started.Add(time.Hour)

	gone := []string{
		filepath.Join(p.Home, ".state.json.123456.tmp"),
		filepath.Join(p.Generations, ".7.json.123456.tmp"),
	}
	kept := []string{
		// From after this skl started.
		filepath.Join(p.Home, ".state.json.777.tmp"),
		filepath.Join(p.Generations, ".8.json.777.tmp"),
		// Not skl's temp files, however old.
		filepath.Join(p.Home, ".state.json.tmp"),
		filepath.Join(p.Home, ".state.json.abc.tmp"),
		filepath.Join(p.Home, "state.json.123.tmp"),
		filepath.Join(p.Home, ".state.json.123"),
		filepath.Join(p.Home, ".other.json.123.tmp"),
		filepath.Join(p.Home, ".7.json.123.tmp"), // a generation's name, in the wrong folder
		filepath.Join(p.Home, ".config.toml.123.tmp"),
		filepath.Join(p.Generations, ".state.json.123.tmp"),
		filepath.Join(p.Generations, ".07.json.123.tmp"),
		filepath.Join(p.Generations, ".x.json.123.tmp"),
		filepath.Join(p.Generations, "4.json"),
	}
	for _, path := range gone {
		writeFile(t, path, before)
	}
	for _, path := range kept {
		mtime := before
		if strings.Contains(path, ".777.") {
			mtime = after
		}
		writeFile(t, path, mtime)
	}
	// A folder or a link with a temp file's name isn't one of ours either.
	dir := filepath.Join(p.Home, ".state.json.1.tmp")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.Home, ".state.json.2.tmp")
	if err := os.Symlink(gone[0], link); err != nil {
		t.Fatal(err)
	}
	kept = append(kept, dir, link)

	l, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	for _, path := range gone {
		if exists(path) {
			t.Errorf("still there: %s", path)
		}
	}
	for _, path := range kept {
		if !exists(path) {
			t.Errorf("removed: %s", path)
		}
	}
}

// Card #96 item 3, end to end: a killed skl's temp files go, the file it was
// replacing stays.
func TestLockCleansUpAfterAKilledWriter(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: bigScope("a")}}); err != nil {
		t.Fatal(err)
	}
	temps := func() []string {
		found, err := filepath.Glob(filepath.Join(p.Home, ".state.json.*.tmp"))
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	// Kill the writer as soon as it has a temp file. It may still get to rename
	// that one before the signal lands, so try until one stays.
	for i := 0; len(temps()) == 0; i++ {
		if i == 100 {
			t.Fatal("100 kills left no temp file")
		}
		cmd := startHelper(t, p, "save-loop")
		for deadline := time.Now().Add(10 * time.Second); len(temps()) == 0; {
			if time.Now().After(deadline) {
				t.Fatal("the writer never made a temp file")
			}
			time.Sleep(time.Millisecond)
		}
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		_ = cmd.Wait()
	}

	// This Store started before those files: they aren't older than it.
	early, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(temps()) == 0 {
		t.Error("removed temp files from after this skl's start")
	}
	if err := early.Release(); err != nil {
		t.Fatal(err)
	}

	// The next skl starts now.
	time.Sleep(20 * time.Millisecond) // past the filesystem clock's coarse tick
	l, err := New(p, nil).Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	if left := temps(); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	st, err := s.Load(ctx)
	if err != nil || len(st.Scopes[User].Selectors) != 2000 {
		t.Errorf("state.json after cleanup: %v", err)
	}
}

// Card #96 item 3: "Never remove one another live process might own." While a
// live skl holds the lock, a refused skl removes nothing, however old the file.
func TestRefusedLockRemovesNothing(t *testing.T) {
	_, p := testStore(t)
	startHolder(t, p, "hold")
	temp := filepath.Join(p.Home, ".state.json.123456.tmp")
	writeFile(t, temp, time.Now().Add(-time.Hour))
	if stdout, stderr, code := runTry(t, p); code != 1 {
		t.Fatalf("second skl: exit %d, %q, %q", code, stdout, stderr)
	}
	if !exists(temp) {
		t.Error("a refused skl removed the holder's temp file")
	}
}

// Nothing depends on the cleanup, so a failure doesn't stop the command.
func TestLockSurvivesAFailedCleanup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	s, p := testStore(t)
	var debug bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&debug, &slog.HandlerOptions{Level: slog.LevelDebug}))
	temp := filepath.Join(p.Generations, ".7.json.123.tmp")
	writeFile(t, temp, s.started.Add(-time.Hour))
	if err := os.Chmod(p.Generations, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p.Generations, 0o700) })

	l, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	if !exists(temp) {
		t.Error("the file went despite the read-only folder")
	}
	if !strings.Contains(debug.String(), "can't remove temp file") {
		t.Errorf("debug log: %q", debug.String())
	}
}

// Card #96 item 4: the compiler checks the caller holds the lock, because only
// *Lock has these methods.
func TestChangingStateNeedsTheLock(t *testing.T) {
	store, lock := reflect.TypeFor[*Store](), reflect.TypeFor[*Lock]()
	for _, name := range []string{"Save", "AddGeneration"} {
		if _, ok := store.MethodByName(name); ok {
			t.Errorf("*Store has %s, so a caller can skip the lock", name)
		}
		if _, ok := lock.MethodByName(name); !ok {
			t.Errorf("*Lock has no %s", name)
		}
	}
}

// Once released, a lock changes nothing.
func TestReleasedLockChangesNothing(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	l, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Save(ctx, &State{}); err != nil {
		t.Errorf("Save while held: %v", err)
	}
	g, err := l.AddGeneration(ctx, Generation{Time: genTime})
	if err != nil || g.N != 1 {
		t.Errorf("AddGeneration while held: %+v, %v", g, err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(ctx, &State{}); !errors.Is(err, ErrReleased) {
		t.Errorf("Save after Release: %v", err)
	}
	if _, err := l.AddGeneration(ctx, Generation{Time: genTime}); !errors.Is(err, ErrReleased) {
		t.Errorf("AddGeneration after Release: %v", err)
	}
	if exists(filepath.Join(p.Generations, "2.json")) {
		t.Error("generation 2 was written")
	}
}

// docs/reference/files.md "Local state": "If skl can't use state.json, it stops
// instead of overwriting it". A damaged file comes through the lock and its
// cleanup as it was, and Load names it.
func TestDamagedStateIsLeftAlone(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := os.MkdirAll(p.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.State, []byte("{ nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	_, err = s.Load(ctx)
	if !errors.Is(err, ErrCorrupt) || !strings.HasPrefix(err.Error(), "can't read "+p.State+": ") {
		t.Errorf("Load: %v", err)
	}
	data, err := os.ReadFile(p.State)
	if err != nil || string(data) != "{ nope" {
		t.Errorf("state.json = %q, %v", data, err)
	}
}
