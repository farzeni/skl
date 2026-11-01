package gitx

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/errs"
)

// isolate gives the test its own HOME and git config, so the user's
// credential helpers and settings never take part.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	cfg := "[user]\n\tname = Test\n\temail = test@example.com\n[init]\n\tdefaultBranch = main\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func newRepo(t *testing.T, g *Git) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := g.Run(t.Context(), dir, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunOutput(t *testing.T) {
	isolate(t)
	g := New(nil)
	dir := newRepo(t, g)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# pdf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "SKILL.md"}, {"commit", "--quiet", "-m", "first"}} {
		if _, err := g.Run(t.Context(), dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	out, err := g.Run(t.Context(), dir, "log", "--format=%s")
	if err != nil || out != "first\n" {
		t.Fatalf("Run(log) = %q, %v, want %q", out, err, "first\n")
	}
}

func TestRunExitCode(t *testing.T) {
	isolate(t)
	g := New(nil)
	dir := newRepo(t, g)
	_, err := g.Run(t.Context(), dir, "rev-parse", "--verify", "--quiet", "nope")
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if e.Code != 1 || e.Args[0] != "rev-parse" {
		t.Errorf("Error = code %d args %q, want code 1, rev-parse", e.Code, e.Args)
	}
	if errors.Is(err, ErrUnreachable) || errors.Is(err, ErrNotRepo) {
		t.Errorf("err = %v matches a sentinel, want neither", err)
	}

	_, err = g.Run(t.Context(), dir, "cat-file", "-p", "nope")
	if want := "git cat-file: Not a valid object name nope"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestNotRepo(t *testing.T) {
	isolate(t)
	_, err := New(nil).Run(t.Context(), t.TempDir(), "status")
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("err = %v, want ErrNotRepo", err)
	}
}

// docs/reference/commands.md "Errors": "When the network or the remote
// fails, skl shows git's error and stops."
func TestUnreachable(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "nope.git")
	clone := filepath.Join(t.TempDir(), "clone")
	tests := []struct {
		name    string
		args    []string
		reasons []string // curl's wording depends on its version
	}{
		{"connection refused", []string{"ls-remote", "http://127.0.0.1:1/someone/agent-skills"}, []string{"Connection refused", "Failed to connect"}},
		{"ssh refused", []string{"ls-remote", "ssh://git@127.0.0.1:1/someone/agent-skills"}, []string{"Connection refused"}},
		{"missing local repo", []string{"ls-remote", missing}, []string{"does not appear to be a git repository"}},
		{"clone missing local repo", []string{"clone", "--quiet", missing, clone}, []string{"does not exist"}},
		{"clone missing file:// repo", []string{"clone", "--quiet", "file://" + filepath.ToSlash(missing), clone}, []string{"does not appear to be a git repository"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(nil).Run(t.Context(), "", tt.args...)
			var e *Error
			if !errors.Is(err, ErrUnreachable) || !errors.As(err, &e) {
				t.Fatalf("err = %v, want ErrUnreachable", err)
			}
			if !slices.Contains(tt.reasons, e.Reason()) || !strings.Contains(e.Stderr, e.Reason()) {
				t.Errorf("Reason() = %q, stderr %q; want one of %q", e.Reason(), e.Stderr, tt.reasons)
			}
			// git's own line, carrying the reason.
			if !strings.Contains(err.Error(), e.Reason()) || !strings.HasPrefix(err.Error(), "git "+tt.args[0]+": ") {
				t.Errorf("Error() = %q, want git's message", err)
			}
		})
	}
}

// A local disk problem is git's error, not an unreachable remote.
func TestLocalPermissionDenied(t *testing.T) {
	isolate(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	g := New(nil)
	src := newRepo(t, g)
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	for _, args := range [][]string{
		{"init", "--quiet", filepath.Join(ro, "x")},
		{"clone", "--quiet", src, filepath.Join(ro, "y")},
	} {
		_, err := g.Run(t.Context(), "", args...)
		if err == nil || errors.Is(err, ErrUnreachable) || !strings.Contains(err.Error(), "Permission denied") {
			t.Errorf("git %s: err = %v, want git's Permission denied, not ErrUnreachable", args[0], err)
		}
	}
}

// docs/reference/commands.md "Errors": "Access problems fail the same way,
// without asking for a password." docs/design/architecture.md: "Git runs
// with GIT_TERMINAL_PROMPT=0 ... so a missing credential ... fails at once
// instead of waiting for input."
func TestNoPasswordPrompt(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	start := time.Now()
	_, err := New(nil).Run(t.Context(), "", "ls-remote", srv.URL+"/you/skills.git")
	var e *Error
	if !errors.Is(err, ErrUnreachable) || !errors.As(err, &e) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if e.Reason() != "terminal prompts disabled" {
		t.Errorf("Reason() = %q, stderr %q", e.Reason(), e.Stderr)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %s, want it to fail at once", d)
	}
}

// docs/design/architecture.md "Fetching from remotes": "Git runs with
// GIT_TERMINAL_PROMPT=0 and GIT_SSH_COMMAND="ssh -o BatchMode=yes"".
func TestEnv(t *testing.T) {
	isolate(t)
	// One name per printenv: BSD printenv (macOS) reads only the first.
	printenv := []string{"-c", "alias.env=!printenv GIT_TERMINAL_PROMPT; printenv GIT_SSH_COMMAND", "env"}
	env := func(g *Git, dir string) string {
		t.Helper()
		out, err := g.Run(t.Context(), dir, printenv...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := env(New(nil), ""), "0\nssh -o BatchMode=yes\n"; got != want {
		t.Errorf("env = %q, want %q", got, want)
	}

	// docs/design/architecture.md "Fetching from remotes": "Your own
	// GIT_SSH_COMMAND or core.sshCommand is kept, with -o BatchMode=yes
	// added."
	t.Run("core.sshCommand", func(t *testing.T) {
		g := New(nil)
		if _, err := g.Run(t.Context(), "", "config", "--global", "core.sshCommand", "ssh -i ~/.ssh/work"); err != nil {
			t.Fatal(err)
		}
		g = New(nil) // read once per dir: a fresh Git sees the new value
		if got, want := env(g, ""), "0\nssh -i ~/.ssh/work -o BatchMode=yes\n"; got != want {
			t.Errorf("env = %q, want %q", got, want)
		}
		// A repo's own core.sshCommand wins over the global one, as in git.
		repo := newRepo(t, g)
		if _, err := g.Run(t.Context(), repo, "config", "core.sshCommand", "ssh -i ~/.ssh/repo"); err != nil {
			t.Fatal(err)
		}
		if got, want := env(New(nil), repo), "0\nssh -i ~/.ssh/repo -o BatchMode=yes\n"; got != want {
			t.Errorf("env in repo = %q, want %q", got, want)
		}
	})

	t.Run("ssh runs", func(t *testing.T) {
		// git really runs the user's ssh, in batch mode (a fake ssh, offline).
		bin := t.TempDir()
		args := filepath.Join(bin, "args")
		script := "#!/bin/sh\necho \"$@\" > '" + args + "'\nexit 255\n"
		if err := os.WriteFile(filepath.Join(bin, "myssh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		g := New(nil)
		if _, err := g.Run(t.Context(), "", "config", "--global", "core.sshCommand", filepath.Join(bin, "myssh")); err != nil {
			t.Fatal(err)
		}
		g = New(nil)
		if _, err := g.Run(t.Context(), "", "ls-remote", "ssh://git@example.invalid/you/skills"); err == nil {
			t.Fatal("ls-remote through a failing ssh succeeded")
		}
		b, err := os.ReadFile(args)
		if err != nil || !strings.HasPrefix(string(b), "-o BatchMode=yes ") {
			t.Errorf("ssh args = %q, %v, want -o BatchMode=yes first", b, err)
		}
	})

	t.Run("GIT_SSH_COMMAND", func(t *testing.T) {
		// GIT_SSH_COMMAND wins over the global core.sshCommand set above,
		// as in git.
		t.Setenv("GIT_SSH_COMMAND", "ssh -i ~/.ssh/env")
		if got, want := env(New(nil), ""), "0\nssh -i ~/.ssh/env -o BatchMode=yes\n"; got != want {
			t.Errorf("env = %q, want %q", got, want)
		}
	})
}

// docs/design/architecture.md "Fetching from remotes": the ssh that runs,
// in git's own order: GIT_SSH_COMMAND, core.sshCommand, GIT_SSH, ssh. "A
// program in GIT_SSH takes no options, so if neither of those is set it runs
// as it is."
func TestSSHPrecedence(t *testing.T) {
	const batch = " -o BatchMode=yes"
	tests := []struct {
		name                string
		envCmd, cfg, gitSSH string
		want                string // GIT_SSH_COMMAND as git sees it; "" if unset
	}{
		{"nothing", "", "", "", "ssh" + batch},
		{"GIT_SSH", "", "", "/bin/myssh", ""},
		{"core.sshCommand", "", "cfgssh", "", "cfgssh" + batch},
		{"core.sshCommand over GIT_SSH", "", "cfgssh", "/bin/myssh", "cfgssh" + batch},
		{"GIT_SSH_COMMAND", "envssh", "", "", "envssh" + batch},
		{"GIT_SSH_COMMAND over GIT_SSH", "envssh", "", "/bin/myssh", "envssh" + batch},
		{"GIT_SSH_COMMAND over both", "envssh", "cfgssh", "/bin/myssh", "envssh" + batch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			g := New(nil)
			if tt.cfg != "" {
				if _, err := g.Run(t.Context(), "", "config", "--global", "core.sshCommand", tt.cfg); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("GIT_SSH_COMMAND", tt.envCmd)
			t.Setenv("GIT_SSH", tt.gitSSH)
			// ${x+set} tells an unset variable from an empty one: git would
			// run an empty GIT_SSH_COMMAND instead of GIT_SSH.
			script := `!echo "${GIT_SSH_COMMAND+set}:$GIT_SSH_COMMAND"; printenv GIT_TERMINAL_PROMPT`
			out, err := New(nil).Run(t.Context(), "", "-c", "alias.env="+script, "env")
			if err != nil {
				t.Fatal(err)
			}
			want := ":\n0\n"
			if tt.want != "" {
				want = "set:" + tt.want + "\n0\n"
			}
			if out != want {
				t.Errorf("env = %q, want %q", out, want)
			}
		})
	}

	// git really runs the program in GIT_SSH, with no options of skl's.
	t.Run("GIT_SSH runs", func(t *testing.T) {
		isolate(t)
		bin := t.TempDir()
		args := filepath.Join(bin, "args")
		script := "#!/bin/sh\necho \"$@\" > '" + args + "'\nexit 255\n"
		if err := os.WriteFile(filepath.Join(bin, "myssh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIT_SSH", filepath.Join(bin, "myssh"))
		if _, err := New(nil).Run(t.Context(), "", "ls-remote", "ssh://git@example.invalid/you/skills"); err == nil {
			t.Fatal("ls-remote through a failing ssh succeeded")
		}
		b, err := os.ReadFile(args)
		if err != nil {
			t.Fatalf("GIT_SSH didn't run: %v", err)
		}
		if strings.Contains(string(b), "BatchMode") {
			t.Errorf("ssh args = %q, want no BatchMode", b)
		}
	})
}

// docs/design/architecture.md "Fetching from remotes": "Ctrl-C or a timeout
// stops the running git and everything it started, such as ssh."
func TestStopKillsChildren(t *testing.T) {
	tests := []struct {
		name string
		// set runs before git starts.
		set func(g *Git)
		is  error
	}{
		{"Ctrl-C", nil, context.Canceled},
		{"timeout", func(g *Git) { g.Timeout = 500 * time.Millisecond }, ErrUnreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			g := New(nil)
			if tt.set != nil {
				tt.set(g)
			}
			pidFile := filepath.Join(t.TempDir(), "pid")
			// sleep is git's child's child, as ssh can be. It lets go of the
			// pipes, so only the signal can stop it.
			script := "!sleep 300 >/dev/null 2>&1 </dev/null & echo $! > '" + pidFile + "'; wait"

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var pid int
			ready := make(chan struct{})
			go func() {
				defer close(ready)
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					if b, err := os.ReadFile(pidFile); err == nil && strings.HasSuffix(string(b), "\n") {
						pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
						if g.Timeout == 0 { // else the timeout stops it
							cancel()
						}
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				cancel()
			}()

			start := time.Now()
			_, err := g.Run(ctx, "", "-c", "alias.hang="+script, "hang")
			<-ready
			if pid == 0 {
				t.Fatal("the child never started")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if !errors.Is(err, tt.is) {
				t.Errorf("err = %v, want %v", err, tt.is)
			}
			if d := time.Since(start); d > 5*time.Second {
				t.Errorf("Run took %s", d)
			}
			// The kill is asynchronous, and init reaps the orphan.
			deadline := time.Now().Add(5 * time.Second)
			for {
				err := syscall.Kill(pid, 0)
				if errors.Is(err, syscall.ESRCH) {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("child %d outlived git: kill(0) = %v", pid, err)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

// The spec harness maps https://github.com/ to local repos with
// url.insteadOf in the user's git config: gitx must not override it.
func TestKeepsUserConfig(t *testing.T) {
	home := isolate(t)
	remotes := t.TempDir()
	g := New(nil)
	if _, err := g.Run(t.Context(), "", "init", "--quiet", "--bare", filepath.Join(remotes, "you", "skills.git")); err != nil {
		t.Fatal(err)
	}
	cfg := "[url \"file://" + filepath.ToSlash(remotes) + "/\"]\n\tinsteadOf = https://github.com/\n"
	f, err := os.OpenFile(filepath.Join(home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Run(t.Context(), "", "ls-remote", "https://github.com/you/skills"); err != nil {
		t.Fatalf("ls-remote through insteadOf: %v", err)
	}
}

// docs/design/architecture.md "Fetching from remotes": "each git call times
// out after 30 seconds".
func TestTimeout(t *testing.T) {
	isolate(t)
	if New(nil).Timeout != 0 || DefaultTimeout != 30*time.Second {
		t.Fatalf("default timeout = %s, want 30s", DefaultTimeout)
	}
	g := New(nil)
	g.Timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := g.Run(t.Context(), "", "-c", "alias.hang=!sleep 5", "hang")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if want := "git hang: timed out after 300ms"; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %s, want about 300ms", d)
	}
}

// CONTRIBUTING.md "Context": Ctrl-C cancels ctx, and the command returns
// context.Canceled.
func TestCanceled(t *testing.T) {
	isolate(t)
	g := New(nil)
	if _, err := g.Run(t.Context(), "", "version"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)
	_, err := g.Run(ctx, "", "-c", "alias.hang=!sleep 5", "hang")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want context.Canceled only", err)
	}
}

// docs/reference/commands.md: "130: Interrupted with Ctrl-C." A real signal:
// cli's signal.NotifyContext cancels with a cause that isn't context.Canceled,
// and the error must still match it for cli to exit 130.
func TestInterrupted(t *testing.T) {
	isolate(t)
	g := New(nil)
	if _, err := g.Run(t.Context(), "", "version"); err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt)
	defer stop()
	time.AfterFunc(200*time.Millisecond, func() { _ = syscall.Kill(os.Getpid(), syscall.SIGINT) })
	_, err := g.Run(ctx, "", "-c", "alias.hang=!sleep 5", "hang")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// docs/reference/config.md: "SKL_DEBUG: Set to 1 to print each git and
// engine command skl runs, on standard error." gitx logs at debug level;
// cli enables that level only with SKL_DEBUG=1 (internal/cli debug.go).
func TestDebugLog(t *testing.T) {
	isolate(t)
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		var buf bytes.Buffer
		g := New(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
		dir := t.TempDir()
		if _, err := g.Run(t.Context(), dir, "init", "--quiet"); err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		if level == slog.LevelInfo {
			if got != "" {
				t.Errorf("logged %q without debug", got)
			}
			continue
		}
		for _, want := range []string{`msg="git version"`, `msg="git init --quiet" dir=` + dir} {
			if !strings.Contains(got, want) {
				t.Errorf("log = %q, want a line with %q", got, want)
			}
		}
	}
}

// fakeGit puts a git on PATH that prints "git version <v>" and counts how
// often it ran.
func fakeGit(t *testing.T, version string) (count func() int) {
	t.Helper()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho x >> '" + calls + "'\necho 'git version " + version + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return func() int {
		b, _ := os.ReadFile(calls)
		return bytes.Count(b, []byte("x"))
	}
}

// D3 (user, 2026-10-25): "✗ skl needs git 2.38 or later (found X) · update
// git".
func TestOldGit(t *testing.T) {
	count := fakeGit(t, "2.37.1")
	g := New(nil)
	for range 2 {
		_, err := g.Run(t.Context(), "", "status")
		var h *errs.Hinted
		if !errors.Is(err, ErrOldGit) || !errors.As(err, &h) {
			t.Fatalf("err = %v, want ErrOldGit with a hint", err)
		}
		if got := err.Error() + " · " + h.Hint; got != "skl needs git 2.38 or later (found 2.37.1) · update git" {
			t.Errorf("error = %q", got)
		}
	}
	if n := count(); n != 1 {
		t.Errorf("git ran %d times, want the version checked once", n)
	}
}

func TestVersionCheckedOnce(t *testing.T) {
	count := fakeGit(t, "2.39.5 (Apple Git-154)")
	g := New(nil)
	for range 3 {
		if _, err := g.Run(t.Context(), "", "status"); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(); n != 5 {
		t.Errorf("git ran %d times, want 1 version check, 1 core.sshCommand read and 3 calls", n)
	}
}

func TestNoGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := New(nil).Run(t.Context(), "", "status")
	var h *errs.Hinted
	if !errors.Is(err, ErrOldGit) || !errors.As(err, &h) {
		t.Fatalf("err = %v, want ErrOldGit with a hint", err)
	}
	// docs/guide/getting-started.md "Install": "without git, it stops with
	// ✗ skl needs git · install git 2.38 or later".
	if got := err.Error() + " · " + h.Hint; got != "skl needs git · install git 2.38 or later" {
		t.Errorf("error = %q", got)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		out   string
		want  Version
		found string
		old   bool
	}{
		{"git version 2.53.0\n", Version{2, 53}, "2.53.0", false},
		{"git version 2.39.5 (Apple Git-154)\n", Version{2, 39}, "2.39.5 (Apple Git-154)", false},
		{"git version 2.45.1.windows.1\n", Version{2, 45}, "2.45.1.windows.1", false},
		{"git version 2.38.0\n", Version{2, 38}, "2.38.0", false},
		{"git version 2.37.1\n", Version{2, 37}, "2.37.1", true},
		{"git version 1.99.9\n", Version{1, 99}, "1.99.9", true},
		{"git version 3.0.0\n", Version{3, 0}, "3.0.0", false},
	}
	for _, tt := range tests {
		v, found, err := ParseVersion(tt.out)
		if err != nil || v != tt.want || found != tt.found || v.Less(MinVersion) != tt.old {
			t.Errorf("ParseVersion(%q) = %v, %q, %v (old %v); want %v, %q, old %v",
				tt.out, v, found, err, v.Less(MinVersion), tt.want, tt.found, tt.old)
		}
	}
	for _, bad := range []string{"", "git version", "git version x.y", "hello"} {
		if _, _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) = nil error", bad)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		stderr string
		kind   error
		reason string
	}{
		// docs/reference/commands.md "Errors": "✗ can't reach github.com (Could not resolve host)"
		{"fatal: unable to access 'https://github.com/you/skills/': Could not resolve host: github.com\n", ErrUnreachable, "Could not resolve host"},
		{"ssh: Could not resolve hostname github.com: Name or service not known\nfatal: Could not read from remote repository.\n", ErrUnreachable, "Could not resolve host"},
		{"git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.\n", ErrUnreachable, "Permission denied"},
		{"Host key verification failed.\nfatal: Could not read from remote repository.\n", ErrUnreachable, "Host key verification failed"},
		{"remote: Repository not found.\nfatal: repository 'https://github.com/you/nope/' not found\n", ErrUnreachable, "Repository not found"},
		{"fatal: Authentication failed for 'https://github.com/you/skills/'\n", ErrUnreachable, "Authentication failed"},
		{"fatal: not a git repository (or any of the parent directories): .git\n", ErrNotRepo, ""},
		{"fatal: bad revision 'nope'\n", nil, ""},
		{"fatal: repository '/home/you/agent-skills' does not exist\n", ErrUnreachable, "does not exist"},
		// Local problems are not an unreachable remote.
		{"fatal: cannot mkdir /ro/x: Permission denied\n", nil, ""},
		{"Cloning into '/ro/y'...\nfatal: could not create work tree dir '/ro/y': Permission denied\n", nil, ""},
		{"fatal: path 'x' does not exist in 'HEAD'\n", nil, ""},
	}
	for _, tt := range tests {
		reason, _, kind := classify(tt.stderr)
		if !errors.Is(kind, tt.kind) || reason != tt.reason {
			t.Errorf("classify(%q) = %v, %q; want %v, %q", tt.stderr, kind, reason, tt.kind, tt.reason)
		}
	}
}

func TestMessage(t *testing.T) {
	tests := []struct{ stderr, want string }{
		{"hint: x\nfatal: bad revision 'nope'\n", "bad revision 'nope'"},
		{"error: pathspec 'x' did not match\n", "pathspec 'x' did not match"},
		{"warning: a\nsomething else\n", "something else"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := Message(tt.stderr); got != tt.want {
			t.Errorf("Message(%q) = %q, want %q", tt.stderr, got, tt.want)
		}
	}
}
