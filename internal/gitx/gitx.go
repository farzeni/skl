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
	cmd.Env = environ()
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

// environ is the process's environment plus the variables that keep git
// from waiting for input: a missing credential or an unknown host key fails
// at once. LC_ALL=C keeps git's messages in English, so classify can read
// them.
func environ() []string {
	ssh := "ssh -o BatchMode=yes"
	if own := os.Getenv("GIT_SSH_COMMAND"); own != "" {
		ssh = own + " -o BatchMode=yes"
	}
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND="+ssh,
		"LC_ALL=C",
	)
}
