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
	// saved is the generation the frame made for edits it saved first, or 0.
	saved int
}

// Body is a command's own work. It returns whether it changed anything: if not,
// no generation is made and the command ends "✓ nothing to change".
//
// A body must leave nothing half done when ctx is canceled: finish the step it
// is in under context.WithoutCancel, or undo it, then return ctx's error.
type Body func(ctx context.Context, c *Change) (bool, error)

// Changing runs a command that changes something and needs the vault:
//
//	vault check → lock → merge guard → save first (its own generation) → body →
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
		if err := a.notMerging(ctx, v); err != nil {
			return false, err
		}
		if err := a.saveFirst(ctx, c); err != nil {
			return false, err
		}
		return body(ctx, c)
	})
	return hinted(err, "skl "+command)
}

// notMerging is the ✗ line for a vault with a merge in progress: until skl sync
// --continue or --abort, nothing changes it (docs/reference/commands.md "Errors").
func (a *App) notMerging(ctx context.Context, v *vault.Vault) error {
	merging, err := v.Merging(ctx)
	if err != nil {
		return err
	}
	if merging {
		return errs.Hint(vault.ErrMerging, "skl sync --continue")
	}
	return nil
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
	switch {
	case changed:
		return a.record(ctx, command, c)
	case c.saved > 0:
		// The save was the change: it ends like one (docs/guide/skills.md "Edit as
		// usual").
		a.ui.DropHeld()
		return a.ended(context.WithoutCancel(ctx), c, c.saved)
	default:
		a.ui.Done("nothing to change")
		return nil
	}
}

// record ends a change: the body has done it, so from here on Ctrl-C waits.
func (a *App) record(ctx context.Context, command string, c *Change) error {
	ctx = context.WithoutCancel(ctx)
	g, err := a.addGeneration(ctx, command, c)
	if err != nil {
		c.undo()
		return err
	}
	return a.ended(ctx, c, g.N)
}

// ended is the end of a change that made generation n.
func (a *App) ended(ctx context.Context, c *Change, n int) error {
	ahead, err := c.Vault.Ahead(ctx)
	if err != nil {
		return err
	}
	if ahead > 0 {
		a.ui.Warn(notPushed(ahead), "skl sync")
	}
	a.ui.Done(fmt.Sprintf("generation %d · skl rollback to undo", n))
	return nil
}

// addGeneration records the vault's HEAD and the scopes as they are now.
func (a *App) addGeneration(ctx context.Context, command string, c *Change) (state.Generation, error) {
	head, err := c.Vault.Head(ctx)
	if err != nil {
		return state.Generation{}, err
	}
	return c.Lock.AddGeneration(ctx, state.Generation{
		Time:        a.now(),
		Command:     command,
		VaultCommit: head,
		Scopes:      snapshot(c.State, c.Scopes),
		Changed:     c.Scopes,
	})
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
