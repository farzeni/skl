package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

type harness struct {
	a              *App
	home           string
	stdout, stderr bytes.Buffer
}

// newHarness gives an App a temporary HOME with a git identity.
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{home: t.TempDir()}
	t.Setenv("HOME", h.home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(h.home, ".gitconfig"))
	git(t, h.home, "config", "--global", "user.name", "Ann")
	git(t, h.home, "config", "--global", "user.email", "ann@example.com")
	a, err := h.app(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	return h
}

func (h *harness) app(ctx context.Context) (*App, error) {
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return h.home
		case "COLUMNS":
			return "80"
		}
		return ""
	}
	u := ui.New(ui.Options{Stdin: strings.NewReader(""), Stdout: &h.stdout, Stderr: &h.stderr, Getenv: getenv})
	return New(ctx, Deps{UI: u, Getenv: getenv, Now: func() time.Time {
		return time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	}})
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// vaulted runs a real skl init against a bare repo.
func (h *harness) vaulted(t *testing.T) {
	t.Helper()
	bare := filepath.Join(h.home, "you", "skills.git")
	git(t, h.home, "init", "--quiet", "--bare", bare)
	if err := h.a.Init(context.Background(), bare, nil, false); err != nil {
		t.Fatal(err)
	}
	h.stderr.Reset()
}

func hintOf(t *testing.T, err error) string {
	t.Helper()
	he, ok := errors.AsType[*errs.Hinted](err)
	if !ok {
		t.Fatalf("no hint on %v", err)
	}
	return he.Hint
}

func noBody(t *testing.T) Body {
	return func(context.Context, *Change) (bool, error) {
		t.Error("the body ran")
		return false, nil
	}
}

// docs/reference/commands.md "Errors": "✗ no vault on this machine · skl init <repo>"
func TestChangingWithoutVault(t *testing.T) {
	h := newHarness(t)
	err := h.a.Changing(context.Background(), "use x", noBody(t))
	if err == nil || err.Error() != "no vault on this machine" || !errors.Is(err, vault.ErrNoVault) {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "skl init <repo>" {
		t.Errorf("hint = %q", got)
	}
	// Nothing was made on the way there.
	if _, err := os.Stat(h.a.paths.Home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("~/.skl: %v", err)
	}
}

func TestChangingVaultIsNotAClone(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(h.a.paths.Vault, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.a.paths.Vault, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := h.a.Changing(context.Background(), "use x", noBody(t))
	if err == nil || err.Error() != "~/.skl/vault isn't a git repo" {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "move it aside, then skl init <repo>" {
		t.Errorf("hint = %q", got)
	}
}

// commands.md "Output": "A command that changes nothing makes no generation and
// ends with ✓ nothing to change."
func TestChangingNothing(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	err := h.a.Changing(context.Background(), "use x", func(_ context.Context, c *Change) (bool, error) {
		if c.Vault == nil || c.State == nil || c.Lock == nil {
			t.Errorf("change = %+v", c)
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.stderr.String(); got != "✓ nothing to change\n" {
		t.Errorf("stderr = %q", got)
	}
	gens, err := h.a.store.Generations(context.Background())
	if err != nil || len(gens) != 1 {
		t.Errorf("generations = %v, %v", gens, err)
	}
}

// commands.md "Output": "! 2 changes not pushed · skl sync", "✓ generation 14 ·
// skl rollback to undo", the ✓ last.
func TestChangingRecordsAGeneration(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	ctx := context.Background()
	repo := state.Project("/code/p1")
	err := h.a.Changing(ctx, "use @base", func(ctx context.Context, c *Change) (bool, error) {
		git(t, c.Vault.Dir(), "commit", "--quiet", "--allow-empty", "-m", "a change")
		c.State.Scopes[state.User] = state.Scope{Selectors: []string{"@base"}}
		c.State.Scopes[repo] = state.Scope{Selectors: []string{"@frontend"}}
		c.State.Scopes[state.Project("/code/other")] = state.Scope{Selectors: []string{"@untouched"}}
		c.Scopes = []state.ScopeKey{repo}
		return true, c.Lock.Save(ctx, c.State)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "! 2 changes not pushed · skl sync\n✓ generation 2 · skl rollback to undo\n"
	if got := h.stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	g, err := h.a.store.Generation(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if g.Command != "use @base" || len(g.VaultCommit) != 40 || !g.Time.Equal(h.a.now()) {
		t.Errorf("generation = %+v", g)
	}
	if len(g.Changed) != 1 || g.Changed[0] != repo {
		t.Errorf("Changed = %v", g.Changed)
	}
	if len(g.Scopes) != 2 || g.Scopes[state.User].Selectors[0] != "@base" || g.Scopes[repo].Selectors[0] != "@frontend" {
		t.Errorf("Scopes = %+v", g.Scopes)
	}
}

// commands.md "Errors": "✗ another skl is running (pid 4182) · skl sync, once
// it's done". commands-lock.txtar proves it between two real processes.
func TestLockBusy(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	ctx := context.Background()
	held, err := h.a.store.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()
	err = h.a.Changing(ctx, "use x", noBody(t))
	if _, busy := errors.AsType[*state.LockedError](err); !busy {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "skl sync, once it's done" {
		t.Errorf("hint = %q", got)
	}
}

// commands.md "Exit codes": Ctrl-C; the frame records nothing and gives the lock
// back however the body ends.
func TestBodyCanceledOrFailed(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	ctx := context.Background()
	failure := errors.New("no")
	for _, want := range []error{context.Canceled, failure} {
		err := h.a.Changing(ctx, "use x", func(context.Context, *Change) (bool, error) { return true, want })
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
		l, err := h.a.store.Lock(ctx)
		if err != nil {
			t.Fatalf("the lock wasn't released: %v", err)
		}
		_ = l.Release()
	}
	if got := h.stderr.String(); got != "" {
		t.Errorf("stderr = %q", got)
	}
	if gens, _ := h.a.store.Generations(ctx); len(gens) != 1 {
		t.Errorf("generations = %v", gens)
	}
}

// commands.md "Output": "Every ! and ✗ line ends with the command to run"; an
// error with no hint of its own gets the one that tries again.
func TestChangingErrorsHaveAHint(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	err := h.a.Changing(context.Background(), "use x", func(context.Context, *Change) (bool, error) {
		return false, errors.New("no")
	})
	if got := hintOf(t, err); got != "skl use x" {
		t.Errorf("hint = %q", got)
	}
}

// docs/reference/config.md: "A value skl can't use stops the command, naming the
// variable or the key".
func TestNewConfigErrors(t *testing.T) {
	h := newHarness(t)
	cfg := h.a.paths.Config
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`targets = "claude"`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := h.app(context.Background())
	if err == nil || err.Error() != "config: targets must be a list of target names" {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "edit targets in ~/.skl/config.toml" {
		t.Errorf("hint = %q", got)
	}

	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	if err := os.Chmod(cfg, 0); err != nil {
		t.Fatal(err)
	}
	_, err = h.app(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "can't read ~/.skl/config.toml") {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "check its permissions" {
		t.Errorf("hint = %q", got)
	}
}

func TestCountsLine(t *testing.T) {
	for _, tt := range []struct {
		c    vault.Counts
		want string
	}{
		{vault.Counts{}, "no skills yet"},
		{vault.Counts{Groups: 1, Skills: 1}, "1 group · 1 skill"},
		{vault.Counts{Groups: 3, Skills: 14, Sets: 2}, "3 groups · 14 skills · 2 skill sets"},
		{vault.Counts{Sets: 1}, "1 skill set"},
	} {
		if got := countsLine(tt.c); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.c, got, tt.want)
		}
	}
}

// getting-started.md: "SKL_HOME=~/.skl-team skl init team/skills". The hint never
// names the setup already in use, and a word a shell would split stays quoted.
func TestSecondSetup(t *testing.T) {
	h := newHarness(t)
	repo, err := h.a.parseRepo("team/skills")
	if err != nil {
		t.Fatal(err)
	}
	if got := h.a.secondSetup(repo); got != "SKL_HOME=~/.skl-team skl init team/skills" {
		t.Errorf("got %q", got)
	}
	h.a.paths.Home = filepath.Join(h.home, ".skl-team")
	if got := h.a.secondSetup(repo); got != "SKL_HOME=~/.skl-team-2 skl init team/skills" {
		t.Errorf("got %q", got)
	}
	repo, err = h.a.parseRepo("~/my code")
	if err != nil {
		t.Fatal(err)
	}
	if got := h.a.secondSetup(repo); got != "SKL_HOME=~/.skl-mycode skl init '~/my code'" {
		t.Errorf("got %q", got)
	}
}

// getting-started.md: a vault with no origin can't be compared; it says so.
func TestInitVaultWithoutOrigin(t *testing.T) {
	h := newHarness(t)
	git(t, h.home, "init", "--quiet", h.a.paths.Vault)
	err := h.a.Init(context.Background(), "you/skills", nil, false)
	if err == nil || err.Error() != "~/.skl/vault has no origin" {
		t.Fatalf("err = %v", err)
	}
	if got := hintOf(t, err); got != "git -C ~/.skl/vault remote add origin https://github.com/you/skills" {
		t.Errorf("hint = %q", got)
	}
}
