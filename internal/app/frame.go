package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/vault"
)

// Change is what the frame hands a command's body.
type Change struct {
	Lock  *state.Lock
	State *state.State
	Vault *vault.Vault // nil for init until its body has made one
	// Scopes are the ones the change touched: what a rollback restores. The body
	// fills it in, and saves State through Lock if it changed it.
	Scopes []state.ScopeKey
	// Undo, if the body sets it, takes the change back when no generation can be
	// made for it, so the command can simply run again.
	Undo func()
	// Cleanup, if the body sets it, runs once the command is over, however it ended,
	// while the lock is still held.
	Cleanup func()
}

// Body is a command's own work. It returns whether it changed anything: if not,
// no generation is made and the command ends "✓ nothing to change".
//
// A body must leave nothing half done when ctx is canceled: finish the step it
// is in under context.WithoutCancel, or undo it, then return ctx's error.
type Body func(ctx context.Context, c *Change) (bool, error)

// Changing runs a command that changes something and needs the vault:
//
//	vault check → lock → (merge guard, save-back: later cards) → body →
//	generation → "! N changes not pushed" → "✓ generation N"
//
// command is what skl history shows, such as "use @backend".
func (a *App) Changing(ctx context.Context, command string, body Body) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	err = a.locked(ctx, command, func(ctx context.Context, c *Change) (bool, error) {
		c.Vault = v
		return body(ctx, c)
	})
	return hinted(err, "skl "+command)
}

// hinted gives an error with no command of its own the one that tries again.
func hinted(err error, retry string) error {
	var h *errs.Hinted
	if err == nil || errors.As(err, &h) || errors.Is(err, context.Canceled) {
		return err
	}
	return errs.Hint(err, retry)
}

// openVault is the vault, or the ✗ line for not having one.
func (a *App) openVault() (*vault.Vault, error) {
	v, err := vault.Open(a.git, a.paths.Vault)
	switch {
	case errors.Is(err, vault.ErrNoVault):
		return nil, errs.Hint(err, "skl init <repo>")
	case errors.Is(err, vault.ErrNotClone):
		return nil, a.notClone(err, "skl init <repo>")
	}
	return v, err
}

// notClone is a vault folder that holds something else.
func (a *App) notClone(err error, init string) error {
	return errs.Hint(fmt.Errorf("%s %w", a.shown(a.paths.Vault), err), "move it aside, then "+init)
}

// locked is the frame after the vault check. The lock is released however the
// body ends, Ctrl-C included.
func (a *App) locked(ctx context.Context, command string, body Body) error {
	lock, err := a.store.Lock(ctx)
	if err != nil {
		if _, busy := errors.AsType[*state.LockedError](err); busy {
			return errs.Hint(err, "skl sync, once it's done")
		}
		return err
	}
	defer func() { _ = lock.Release() }() // closing the file drops the lock anyway

	st, err := a.store.Load(ctx)
	if err != nil {
		return a.stateError(err)
	}
	c := &Change{Lock: lock, State: st}
	defer func() {
		if c.Cleanup != nil {
			c.Cleanup()
		}
	}()
	changed, err := body(ctx, c)
	if err != nil {
		return err
	}
	if !changed {
		a.ui.Done("nothing to change")
		return nil
	}
	return a.record(ctx, command, c)
}

// record ends a change: the body has done it, so from here on Ctrl-C waits.
func (a *App) record(ctx context.Context, command string, c *Change) error {
	ctx = context.WithoutCancel(ctx)
	head, err := c.Vault.Head(ctx)
	if err != nil {
		c.undo()
		return err
	}
	g, err := c.Lock.AddGeneration(ctx, state.Generation{
		Time:        a.now(),
		Command:     command,
		VaultCommit: head,
		Scopes:      snapshot(c.State, c.Scopes),
		Changed:     c.Scopes,
	})
	if err != nil {
		c.undo()
		return err
	}
	ahead, err := c.Vault.Ahead(ctx)
	if err != nil {
		return err
	}
	if ahead > 0 {
		a.ui.Warn(notPushed(ahead), "skl sync")
	}
	a.ui.Done(fmt.Sprintf("generation %d · skl rollback to undo", g.N))
	return nil
}

func (c *Change) undo() {
	if c.Undo != nil {
		c.Undo()
	}
}

// snapshot is what a generation keeps: the user scope and every scope the
// change touched. The project scope of the repo it ran in joins them with the
// first command that takes --scope project.
func snapshot(st *state.State, changed []state.ScopeKey) map[state.ScopeKey]state.Scope {
	scopes := map[state.ScopeKey]state.Scope{}
	for key, sc := range st.Scopes {
		if key == state.User || slices.Contains(changed, key) {
			scopes[key] = sc
		}
	}
	return scopes
}

func notPushed(n int) string {
	if n == 1 {
		return "1 change not pushed"
	}
	return fmt.Sprintf("%d changes not pushed", n)
}
