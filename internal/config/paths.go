package config

import (
	"errors"
	"fmt"
	"path/filepath"
)

// ErrNoHome means neither SKL_HOME nor HOME is set, so skl has nowhere to
// keep its files.
var ErrNoHome = errors.New("neither SKL_HOME nor HOME is set")

// Paths are skl's local files on this machine (docs/reference/files.md
// "Local state"). They all live in one folder, ~/.skl, which SKL_HOME
// moves as a whole, config file included. Every path is absolute.
type Paths struct {
	Home        string // ~/.skl, or $SKL_HOME
	Config      string // config.toml: this machine's settings
	Vault       string // vault/: the clone of your vault
	Cache       string // cache/: sparse clones for learn, safe to delete
	State       string // state.json: what's loaded where
	Generations string // generations/: one snapshot per change
	Lock        string // lock: held while a command changes something
	Staging     string // staging/: new copies, before they're swapped in
}

// NewPaths computes the paths from SKL_HOME, else $HOME/.skl. A relative
// SKL_HOME is taken from the working directory and made absolute.
func NewPaths(getenv func(string) string) (Paths, error) {
	home := getenv("SKL_HOME")
	if home == "" {
		h := getenv("HOME")
		if h == "" {
			return Paths{}, ErrNoHome
		}
		home = filepath.Join(h, ".skl")
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return Paths{}, fmt.Errorf("SKL_HOME: %w", err)
	}
	return Paths{
		Home:        home,
		Config:      filepath.Join(home, "config.toml"),
		Vault:       filepath.Join(home, "vault"),
		Cache:       filepath.Join(home, "cache"),
		State:       filepath.Join(home, "state.json"),
		Generations: filepath.Join(home, "generations"),
		Lock:        filepath.Join(home, "lock"),
		Staging:     filepath.Join(home, "staging"),
	}, nil
}
