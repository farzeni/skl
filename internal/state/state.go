// Package state keeps what skl knows on this machine and never syncs
// (docs/reference/files.md "Local state"): what's loaded where in state.json,
// one snapshot per change in generations/, and the lock that lets one skl
// change things at a time.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/farzeni/skl/internal/config"
)

// schemaVersion is the format of state.json. New fields don't change it: only
// a change an older skl would get wrong does.
const schemaVersion = 1

var (
	// ErrCorrupt is a state.json, a generation file or the generations folder that
	// can't be read, or a file that isn't valid JSON. skl writes them atomically, so
	// this comes from a hand edit, permissions or a bad disk.
	ErrCorrupt = errors.New("can't read")
	// ErrNewerState is a state.json from a newer skl. Rewriting it could lose
	// what this skl doesn't know about, so it stops.
	ErrNewerState = errors.New("written by a newer skl")
	// ErrReleased is a change attempted through a lock already given up.
	ErrReleased = errors.New("the lock was released")
)

// ScopeKey names a scope in state.json: User, or Project for one repo.
type ScopeKey string

// User is the user scope.
const User ScopeKey = "user"

const projectPrefix = "project:"

// Project is the project scope of the repo with this top-level path. A git
// worktree has its own top-level path, so it is a repo of its own.
func Project(toplevel string) ScopeKey { return ScopeKey(projectPrefix + toplevel) }

// Repo returns the repo's top-level path, and false for the user scope.
func (k ScopeKey) Repo() (string, bool) {
	repo, ok := strings.CutPrefix(string(k), projectPrefix)
	if !ok {
		return "", false
	}
	return repo, true
}

// Scope is what's loaded in one scope: its active skill set and its copies.
type Scope struct {
	Selectors []string `json:"selectors,omitempty"`
	Copies    []Copy   `json:"copies,omitempty"`
}

// Copy is one loaded skill. Each target holds its own copy of a skill.
type Copy struct {
	Name   string        `json:"name"`   // its folder in the target's skills folder
	Target config.Target `json:"target"` // claude or agents
	Skill  string        `json:"skill"`  // the vault skill it came from: group/name
	Commit string        `json:"commit"` // the vault commit it was copied from
	Tree   string        `json:"tree"`   // the folder's git tree hash, as copied
}

// State is the content of state.json. Scopes is never nil after Load.
type State struct {
	Scopes map[ScopeKey]Scope
}

// file is state.json on disk.
type file struct {
	Version int                `json:"version"`
	Scopes  map[ScopeKey]Scope `json:"scopes"`
}

// Store reads and writes the local state in the paths it was given.
//
// Reading needs no lock: writes are atomic, so a reader sees the old file or
// the new one. status and history rely on that. Changing state, adding
// generations included, is for the holder of the lock, so those are methods of
// *Lock: the compiler sees to it that a caller has taken one.
type Store struct {
	statePath   string
	generations string
	lockPath    string
	log         *slog.Logger
	started     time.Time            // this skl's start: a temp file older than it isn't ours
	syncFile    func(*os.File) error // (*os.File).Sync; tests replace it
}

// New returns the store for p. log gets debug lines only; nil discards them.
// Call it when skl starts: what it finds older than that is left from before.
func New(p config.Paths, log *slog.Logger) *Store {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Store{
		statePath:   p.State,
		generations: p.Generations,
		lockPath:    p.Lock,
		log:         log,
		started:     time.Now(),
		syncFile:    (*os.File).Sync,
	}
}

// Load reads state.json. A missing file is an empty state: nothing is loaded.
func (s *Store) Load(ctx context.Context) (*State, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var f file
	data, err := os.ReadFile(s.statePath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("%w %s: %w", ErrCorrupt, s.statePath, osReason(err))
	default:
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("%w %s: %w", ErrCorrupt, s.statePath, err)
		}
	}
	if f.Version > schemaVersion {
		return nil, fmt.Errorf("%s was %w", s.statePath, ErrNewerState)
	}
	if f.Scopes == nil {
		f.Scopes = map[ScopeKey]Scope{}
	}
	return &State{Scopes: f.Scopes}, nil
}

// osReason is err without the operation and path os puts around it.
func osReason(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}

// Save writes state.json atomically. Holding the lock is how you get to call
// it. Run it under context.WithoutCancel when the change it records has already
// happened.
func (l *Lock) Save(ctx context.Context, st *State) error {
	if l.f == nil {
		return ErrReleased
	}
	return l.s.save(ctx, st)
}

func (s *Store) save(ctx context.Context, st *State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := marshal(file{Version: schemaVersion, Scopes: st.Scopes})
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := s.writeAtomic(s.statePath, data); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}

func marshal(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writeAtomic replaces path with data: a temp file in the same folder, synced,
// then renamed over it. A crash at any point leaves the old file or the new
// one. A temp file left by a crash starts with a dot and is never read; the
// next skl to take the lock removes it (see removeLeftovers).
func (s *Store) writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := s.mkdirAll(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if err := writeAndClose(tmp, data); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return s.syncDir(dir)
}

func writeAndClose(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

// mkdirAll is os.MkdirAll that also syncs the parent of every folder it
// creates, so a power cut can't lose the folder and what's in it.
func (s *Store) mkdirAll(dir string) error {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); !errors.Is(err, fs.ErrNotExist) {
			break // it exists, or MkdirAll will say what's wrong with it
		}
		missing = append(missing, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, d := range missing {
		if err := s.syncDir(filepath.Dir(d)); err != nil {
			return err
		}
	}
	return nil
}

// syncDir makes a rename or a new folder in dir survive a power cut. Some
// filesystems, such as some network ones, can't sync a folder at all. There is
// nothing more to do on those, so it isn't an error.
func (s *Store) syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = s.syncFile(d)
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		s.log.Debug("folder sync not supported", "dir", dir, "err", err)
		err = nil
	}
	return errors.Join(err, d.Close())
}
