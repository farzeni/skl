// Package gitx is the only package that runs git. Every call runs the same
// way on every host: without prompts (GIT_TERMINAL_PROMPT=0, ssh in batch
// mode), with a timeout that, like Ctrl-C, stops git and all it started,
// logged before it runs when SKL_DEBUG=1, and with git's error turned into
// an *Error (docs/design/architecture.md "Fetching from remotes").
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultTimeout bounds each git call: "each git call times out after 30
// seconds" (docs/design/architecture.md "Fetching from remotes").
const DefaultTimeout = 30 * time.Second

// Git runs git. Its zero value is not usable; call New. A Git is safe for
// concurrent use.
type Git struct {
	// Timeout bounds each call; zero means DefaultTimeout.
	Timeout time.Duration

	log *slog.Logger

	mu      sync.Mutex
	checked bool
	verErr  error

	sshMu  sync.Mutex
	sshCfg map[string]string // dir → core.sshCommand there, "" if unset
}

// New returns a Git that logs each command to log at debug level before it
// runs. A nil log logs nothing.
func New(log *slog.Logger) *Git {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Git{log: log}
}

// Run runs git with args in dir ("" for the current directory) and returns
// its standard output. The first call checks git's version once for the
// whole Git. A failure is an *Error; an unreachable remote or a timeout also
// matches ErrUnreachable, and a dir outside a repo ErrNotRepo. If ctx is
// canceled, the error matches ctx's error.
func (g *Git) Run(ctx context.Context, dir string, args ...string) (string, error) {
	if err := g.checkVersion(ctx); err != nil {
		return "", err
	}
	return g.run(ctx, dir, args)
}

func (g *Git) run(ctx context.Context, dir string, args []string) (string, error) {
	env := baseEnv()
	if ssh := g.sshCommand(ctx, dir); ssh != "" {
		env = append(env, "GIT_SSH_COMMAND="+ssh)
	} else {
		// git would run even an empty GIT_SSH_COMMAND instead of GIT_SSH.
		env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "GIT_SSH_COMMAND=") })
	}
	return g.exec(ctx, dir, args, env)
}

// exec runs git with env as its whole environment.
func (g *Git) exec(ctx context.Context, dir string, args, env []string) (string, error) {
	timeout := g.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	tctx, cancel := context.WithTimeoutCause(ctx, timeout, errTimeout)
	defer cancel()

	if dir == "" {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "))
	} else {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "), "dir", dir)
	}

	cmd := exec.CommandContext(tctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	// git leads its own process group, so stopping it stops what it started,
	// such as ssh or a credential helper, which would otherwise outlive skl.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	// A child that left the group, such as an ssh ControlMaster, may keep the
	// pipes open; stop waiting for it.
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	e := &Error{Args: args, Stderr: stderr.String(), Code: -1, err: err}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		e.Code = exit.ExitCode()
	}
	switch {
	case ctx.Err() != nil:
		e.err = ctx.Err()
	case errors.Is(context.Cause(tctx), errTimeout):
		e.err, e.kind, e.reason = errTimeout, ErrUnreachable, "timed out"
		e.msg = fmt.Sprintf("timed out after %s", timeout)
	case errors.Is(err, exec.ErrNotFound):
		return "", g.notFound()
	default:
		e.reason, e.msg, e.kind = classify(e.Stderr)
	}
	return "", e
}

// killGroup kills the process group that pid leads. A group with nothing
// left to kill is ESRCH, or EPERM on macOS when only zombies remain.
func killGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
		return os.ErrProcessDone
	}
	return err
}

// baseEnv is the process's environment plus GIT_TERMINAL_PROMPT=0, so a
// missing credential fails at once instead of waiting for input. LC_ALL=C
// keeps git's messages in English, so classify can read them.
func baseEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

// sshCommand is the ssh command git would use in dir, in batch mode, so an
// unknown host key fails at once: the user's GIT_SSH_COMMAND, else their
// core.sshCommand (which a GIT_SSH_COMMAND set by skl would override), else
// plain ssh. It is "" when git should run the user's GIT_SSH, which comes
// after both in git's order and takes no options. core.sshCommand is read
// once per dir.
func (g *Git) sshCommand(ctx context.Context, dir string) string {
	const batch = " -o BatchMode=yes"
	if own := os.Getenv("GIT_SSH_COMMAND"); own != "" {
		return own + batch
	}
	g.sshMu.Lock()
	defer g.sshMu.Unlock()
	cfg, ok := g.sshCfg[dir]
	if !ok {
		// Exit 1 means unset. Any other failure, such as a dir that doesn't
		// exist, is the main call's to report.
		out, _ := g.exec(ctx, dir, []string{"config", "--get", "core.sshCommand"}, baseEnv())
		cfg = strings.TrimSpace(out)
		if ctx.Err() == nil {
			if g.sshCfg == nil {
				g.sshCfg = map[string]string{}
			}
			g.sshCfg[dir] = cfg
		}
	}
	if cfg == "" {
		if os.Getenv("GIT_SSH") != "" {
			return ""
		}
		cfg = "ssh"
	}
	return cfg + batch
}
