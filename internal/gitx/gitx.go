// Package gitx is the only package that runs git. Every call runs the same
// way on every host: without prompts (GIT_TERMINAL_PROMPT=0, ssh in batch
// mode), with a timeout, logged before it runs when SKL_DEBUG=1, and with
// git's error turned into an *Error (docs/design/architecture.md "Fetching
// from remotes").
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	return g.exec(ctx, dir, args, append(baseEnv(), "GIT_SSH_COMMAND="+g.sshCommand(ctx, dir)))
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
	// A child git started, such as ssh or a hook, may keep the pipes open
	// after git is killed; stop waiting for it.
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
		e.err = context.Cause(ctx)
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

// baseEnv is the process's environment plus GIT_TERMINAL_PROMPT=0, so a
// missing credential fails at once instead of waiting for input. LC_ALL=C
// keeps git's messages in English, so classify can read them.
func baseEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

// sshCommand is the ssh command git would use in dir, in batch mode, so an
// unknown host key fails at once: the user's GIT_SSH_COMMAND, else their
// core.sshCommand (which a GIT_SSH_COMMAND set by skl would override), else
// plain ssh. core.sshCommand is read once per dir.
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
		cfg = "ssh"
	}
	return cfg + batch
}
