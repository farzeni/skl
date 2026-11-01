// Package app holds the commands, one file each, and the frame every change
// runs in (frame.go). It is the only package that adds to an error the command
// the user should run next; cli prints it.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/gitx"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
)

// Deps is what the process gives the commands.
type Deps struct {
	UI     *ui.UI
	Getenv func(string) string
	Log    *slog.Logger
	Now    func() time.Time
}

// App is one run of one command, with its settings loaded.
type App struct {
	ui     *ui.UI
	getenv func(string) string
	now    func() time.Time
	paths  config.Paths
	cfg    config.Config
	git    *gitx.Git
	store  *state.Store
}

// New finds skl's paths and reads the settings.
func New(ctx context.Context, d Deps) (*App, error) {
	paths, err := config.NewPaths(d.Getenv)
	if err != nil {
		if errors.Is(err, config.ErrNoHome) {
			return nil, errs.Hint(err, "set HOME or SKL_HOME")
		}
		return nil, err
	}
	cfg, err := config.Load(ctx, paths, d.Getenv, config.Flags{})
	if err != nil {
		return nil, configError(err)
	}
	return &App{
		ui:     d.UI,
		getenv: d.Getenv,
		now:    d.Now,
		paths:  paths,
		cfg:    cfg,
		git:    gitx.New(d.Log),
		store:  state.New(paths),
	}, nil
}

// configError gives a config error the hint it carries.
func configError(err error) error {
	if h, ok := errors.AsType[interface {
		error
		Hint() string
	}](err); ok {
		return errs.Hint(err, h.Hint())
	}
	return err
}

// shown is path as the docs write it, with ~ for HOME.
func (a *App) shown(path string) string {
	return config.Tilde(path, a.getenv("HOME"))
}

// stateError gives the errors of reading state.json the lines in
// docs/reference/files.md "Local state", with ~ for the path.
func (a *App) stateError(err error) error {
	shown := a.shown(a.paths.State)
	switch {
	case errors.Is(err, state.ErrNewerState):
		return errs.Hint(fmt.Errorf("%s was %w", shown, state.ErrNewerState), "update skl")
	case errors.Is(err, state.ErrCorrupt):
		return errs.Hint(
			&shownError{Err: err, Text: strings.ReplaceAll(err.Error(), a.paths.State, shown)},
			"move it aside, then run skl status")
	}
	return err
}

// networkReason is git's reason when the network or the remote failed, for
// "✗ can't reach github.com (Could not resolve host)". It is "" for a repo
// that's missing or denied, which a host doesn't tell apart.
func networkReason(err error) string {
	ge, ok := errors.AsType[*gitx.Error](err)
	if !ok || !errors.Is(err, gitx.ErrUnreachable) {
		return ""
	}
	switch reason := ge.Reason(); reason {
	case "Could not resolve host", "Connection refused", "Connection timed out", "Operation timed out",
		"Network is unreachable", "No route to host", "Failed to connect", "timed out":
		return reason
	case "The requested URL returned error":
		return httpReason(reason, ge.Stderr)
	case "unable to access":
		return curlReason(ge.Stderr)
	case "Could not read from remote repository":
		return sshReason(ge.Stderr)
	}
	return ""
}

// httpReason is "The requested URL returned error: 502", or "" for the codes a
// missing repo or denied access gets.
func httpReason(phrase, stderr string) string {
	_, after, _ := strings.Cut(stderr, phrase+": ")
	code, _, _ := strings.Cut(after, "\n")
	switch code = strings.TrimSpace(code); code {
	case "", "401", "403", "404":
		return ""
	}
	return phrase + ": " + code
}

// curlReason is what git says after "unable to access '<url>': ", which leaves
// out the URL and any password in it.
func curlReason(stderr string) string {
	_, after, _ := strings.Cut(stderr, "unable to access '")
	line, _, _ := strings.Cut(after, "\n")
	if i := strings.LastIndex(line, "': "); i >= 0 {
		return strings.TrimSpace(line[i+len("': "):])
	}
	return ""
}

// sshReason is ssh's line about a connection that ended, from the phrase on.
// ssh says nothing of the kind when the server turned the repo down.
func sshReason(stderr string) string {
	for _, phrase := range []string{"Connection closed by", "Connection reset by"} {
		if _, after, ok := strings.Cut(stderr, phrase); ok {
			rest, _, _ := strings.Cut(after, "\n")
			return phrase + strings.TrimRight(rest, " \r")
		}
	}
	return ""
}

// shownError is an error with its text rewritten for the user.
type shownError struct {
	Err  error
	Text string
}

func (e *shownError) Error() string { return e.Text }
func (e *shownError) Unwrap() error { return e.Err }
