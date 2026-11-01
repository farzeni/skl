package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/gitx"
	"github.com/farzeni/skl/internal/ref"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Init is skl init <repo>: on a fresh machine it makes ~/.skl/vault from an
// existing or an empty repo (docs/guide/getting-started.md "Make your vault").
//
// It runs in the frame's lock with no vault check, since making the vault is
// the point. The clone is built in staging and moved into place in one step,
// once it's complete, so an error or Ctrl-C leaves no vault at all.
func (a *App) Init(ctx context.Context, word string, dryRun bool) error {
	repo, err := a.parseRepo(word)
	if err != nil {
		return err
	}
	if dryRun {
		return a.initDryRun(ctx, repo)
	}
	retry := initCommand(word)
	err = a.locked(ctx, "init "+word, func(ctx context.Context, c *Change) (bool, error) {
		v, err := vault.Open(a.git, a.paths.Vault)
		switch {
		case err == nil:
			return false, a.holdsOnly(ctx, v, repo)
		case errors.Is(err, vault.ErrNotClone):
			return false, a.notClone(err, retry)
		case !errors.Is(err, vault.ErrNoVault):
			return false, err
		}
		v, err = a.makeVault(ctx, repo)
		if err != nil {
			return false, err
		}
		c.Vault = v
		return true, nil
	})
	return hinted(err, retry)
}

func (a *App) parseRepo(word string) (ref.Repo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ref.Repo{}, fmt.Errorf("init: %w", err)
	}
	repo, err := ref.ParseRepo(word, a.refOptions(cwd))
	if err != nil {
		return ref.Repo{}, errs.Usagef("skl help init", "%w", err)
	}
	return repo, nil
}

func (a *App) refOptions(cwd string) ref.Options {
	return ref.Options{
		DefaultHost: a.cfg.DefaultHost,
		SSH:         a.cfg.Protocol == config.SSH,
		Home:        a.getenv("HOME"),
		Cwd:         cwd,
	}
}

// initDryRun prints what init would do. It reads the remote, but it takes no
// lock and creates nothing, ~/.skl included.
func (a *App) initDryRun(ctx context.Context, repo ref.Repo) error {
	retry := initCommand(repo.Input)
	v, err := vault.Open(a.git, a.paths.Vault)
	switch {
	case err == nil:
		if err := a.holdsOnly(ctx, v, repo); err != nil {
			return hinted(err, retry)
		}
		a.ui.Done("nothing to change")
		return nil
	case errors.Is(err, vault.ErrNotClone):
		return a.notClone(err, retry)
	case !errors.Is(err, vault.ErrNoVault):
		return hinted(err, retry)
	}
	empty, err := vault.RemoteEmpty(ctx, a.git, repo.URL)
	if err != nil {
		return hinted(a.initReachError(err, repo), retry)
	}
	a.planVault(repo, empty)
	return nil
}

func (a *App) planVault(repo ref.Repo, empty bool) {
	if empty {
		a.ui.Plan("new vault %s", repo.Input)
		return
	}
	a.ui.Plan("clone %s", repo.Input)
}

// initCommand is "skl init <word>", as a hint.
func initCommand(word string) string { return ui.CommandLine([]string{"init", word}) }

// hinted gives an error with no command of its own the one that tries again.
func hinted(err error, retry string) error {
	var h *errs.Hinted
	if err == nil || errors.As(err, &h) || errors.Is(err, context.Canceled) {
		return err
	}
	return errs.Hint(err, retry)
}

// holdsOnly is nil if the vault's origin is repo: init again is fine. A machine
// has one vault, so any other repo stops.
func (a *App) holdsOnly(ctx context.Context, v *vault.Vault, repo ref.Repo) error {
	origin, err := v.Origin(ctx)
	if err != nil {
		return err
	}
	vaultDir := a.shown(a.paths.Vault)
	if origin == "" {
		return errs.Hint(fmt.Errorf("%s has no origin", vaultDir), "git -C "+vaultDir+" remote add origin "+repo.URL)
	}
	held, err := ref.ParseRepo(origin, a.refOptions(a.paths.Vault))
	if err == nil && held.Same(repo) {
		return nil
	}
	name := origin // an origin skl can't read is shown as it is
	if err == nil {
		name = a.shortName(held)
	}
	return errs.Hint(fmt.Errorf("%s already holds %s", vaultDir, name), a.secondSetup(repo))
}

// shortName is a repo's shortest name, with ~ on this machine.
func (a *App) shortName(r ref.Repo) string {
	return a.shown(r.Short(a.cfg.DefaultHost))
}

// secondSetup is the hint for keeping a second vault apart: another SKL_HOME,
// named after the repo's owner, and not the one in use.
func (a *App) secondSetup(repo ref.Repo) string {
	owner := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, repo.Owner())
	name := ".skl-" + owner
	home := a.getenv("HOME")
	if filepath.Join(home, name) == a.paths.Home {
		name += "-2"
	}
	return "SKL_HOME=~/" + name + " " + initCommand(repo.Input)
}

// makeVault clones repo into staging, gives a new vault its first commit, and
// moves it into place. Until the move, stopping leaves only staging behind, and
// the deferred removal or the next init clears that.
func (a *App) makeVault(ctx context.Context, repo ref.Repo) (*vault.Vault, error) {
	empty, err := vault.RemoteEmpty(ctx, a.git, repo.URL)
	if err != nil {
		return nil, a.initReachError(err, repo)
	}
	if empty {
		if err := vault.CheckIdentity(ctx, a.git, ""); err != nil {
			return nil, identityError(err)
		}
	}
	a.planVault(repo, empty)

	staged := filepath.Join(a.paths.Staging, "vault")
	if err := os.RemoveAll(staged); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if err := os.MkdirAll(a.paths.Staging, 0o755); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	defer func() { _ = os.RemoveAll(staged) }() // gone already once moved

	v, err := vault.Clone(ctx, a.git, repo.URL, staged)
	if err != nil {
		return nil, a.initReachError(err, repo)
	}
	// What the clone holds decides, not what ls-remote saw a moment ago: a
	// repo that got its first commit since then must not get a LICENSE.
	born, err := v.Born(ctx)
	if errors.Is(err, vault.ErrNoDefaultBranch) {
		return nil, errs.Hint(fmt.Errorf("%s %w", repo.Input, err),
			"set one on your git host, then "+initCommand(repo.Input))
	}
	if err != nil {
		return nil, err
	}
	if !born {
		if err := v.Create(ctx, a.cfg.License, a.now()); err != nil {
			return nil, identityError(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := v.Move(a.paths.Vault); err != nil {
		return nil, err
	}
	a.reportVault(v, born)
	return v, nil
}

// identityError gives vault.ErrNoIdentity the commands that set an identity.
func identityError(err error) error {
	if errors.Is(err, vault.ErrNoIdentity) {
		return errs.Hint(vault.ErrNoIdentity,
			`git config --global user.name "You" && git config --global user.email you@example.com`)
	}
	return err
}

// initReachError is a repo init can't reach. skl never creates one, and hosts
// answer "not found" for a missing repo and a private one alike, so it says
// both. A network failure says that instead.
func (a *App) initReachError(err error, repo ref.Repo) error {
	retry := initCommand(repo.Input)
	if !errors.Is(err, gitx.ErrUnreachable) {
		return err
	}
	if reason := networkReason(err); reason != "" && repo.Host != "" {
		return errs.Hint(fmt.Errorf("can't reach %s (%s)", repo.Host, reason), retry)
	}
	return errs.Hint(fmt.Errorf("can't reach %s", repo.Input), "create it or check access, then "+retry)
}

// reportVault prints what init made: "◇ LICENSE" for a new vault, what it holds
// for a clone.
func (a *App) reportVault(v *vault.Vault, cloned bool) {
	if !cloned {
		if a.cfg.License != vault.NoLicense {
			a.ui.StepHint("LICENSE", a.cfg.License+", for the skills you write", "edit LICENSE to change it")
		}
		return
	}
	c, err := v.Count()
	if err != nil {
		return // only a report: the clone is in place
	}
	a.ui.Step("cloned", countsLine(c))
}

// countsLine is "3 groups · 14 skills · 2 skill sets", or "no skills yet".
func countsLine(c vault.Counts) string {
	if c.Skills == 0 && c.Sets == 0 {
		return "no skills yet"
	}
	var parts []string
	for _, p := range []struct {
		n          int
		one, other string
	}{{c.Groups, "group", "groups"}, {c.Skills, "skill", "skills"}, {c.Sets, "skill set", "skill sets"}} {
		switch {
		case p.n == 1:
			parts = append(parts, "1 "+p.one)
		case p.n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.other))
		}
	}
	return strings.Join(parts, " · ")
}
