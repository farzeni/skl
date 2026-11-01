// Package gitx is the only package that runs git. Every call runs the same
// way on every host: without prompts (GIT_TERMINAL_PROMPT=0, ssh in batch
// mode), logged before it runs when SKL_DEBUG=1, and with git's error turned
// into an *Error (docs/design/architecture.md "Fetching from remotes").
//
// A command that may contact a remote has a timeout and leads its own process
// group, so stopping it stops all it started. A local command keeps the
// terminal, for a pinentry, and has no timeout (see mayUseRemote).
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

// DefaultTimeout bounds each git call that may contact a remote: "each git
// call times out after 30 seconds" (docs/design/architecture.md "Fetching from
// remotes").
const DefaultTimeout = 30 * time.Second

// waitDelay is how long skl waits for git's output after git exits or is
// stopped, in case a child it started keeps the pipes open.
const waitDelay = time.Second

// Git runs git. Its zero value is not usable; call New. A Git is safe for
// concurrent use.
type Git struct {
	// Timeout bounds each call that may contact a remote; zero means
	// DefaultTimeout.
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
	if dir == "" {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "))
	} else {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "), "dir", dir)
	}

	runCtx, timeout := ctx, time.Duration(0)
	if mayUseRemote(args) {
		timeout = g.Timeout
		if timeout == 0 {
			timeout = DefaultTimeout
		}
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeoutCause(ctx, timeout, errTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if timeout != 0 {
		// git leads its own process group, so stopping it stops what it
		// started, such as ssh or a credential helper, which would otherwise
		// outlive skl. A background group can't read the terminal: a child that
		// tries stops, until the timeout.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	} else {
		// git stays in skl's group, in the terminal's foreground, where Ctrl-C
		// reaches all of it. Interrupted, git removes its lock files; killed,
		// it would leave index.lock in the way of the next commit. WaitDelay
		// kills a git that ignores the interrupt.
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	}
	// A child that left the group, such as an ssh ControlMaster, may keep the
	// pipes open; stop waiting for it.
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// exec only returns this when git itself exited 0, and git wrote
		// everything before it exited.
		g.log.DebugContext(ctx, "git "+subcommand(args)+" exited 0, but something it started still holds its output")
		return stdout.String(), nil
	}
	e := &Error{Args: args, Stderr: stderr.String(), Code: -1, err: err}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		e.Code = exit.ExitCode()
	}
	switch {
	case ctx.Err() != nil:
		e.err = ctx.Err()
	case interrupted(err):
		// Ctrl-C reached git through the terminal before skl's own signal
		// handler canceled ctx.
		e.err = context.Canceled
	case errors.Is(context.Cause(runCtx), errTimeout):
		e.err, e.kind, e.reason = errTimeout, ErrUnreachable, "timed out"
		e.msg = fmt.Sprintf("timed out after %s", timeout)
	case errors.Is(err, exec.ErrNotFound):
		return "", g.notFound()
	default:
		e.reason, e.msg, e.kind = classify(e.Stderr)
	}
	return "", e
}

// interrupted reports whether git died of SIGINT.
func interrupted(err error) bool {
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		return false
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGINT
}

// localCommands don't contact a remote in the way skl runs them. They include
// every command that signs or runs a hook, so a pinentry or a slow hook can take
// its time, except push and pull, which contact a remote anyway.
//
// checkout, switch, restore, reset, sparse-checkout, cat-file, show, diff,
// grep, blame and archive are left out on purpose: in a partial clone, as in
// skl's source cache, they fetch missing blobs from the remote.
var localCommands = map[string]bool{
	"add": true, "am": true, "apply": true, "branch": true, "check-attr": true,
	"check-ignore": true, "cherry-pick": true, "commit": true, "commit-tree": true,
	"config": true, "diff-index": true, "for-each-ref": true, "hash-object": true,
	"init": true, "log": true, "ls-files": true, "ls-tree": true, "merge": true,
	"merge-base": true, "merge-file": true, "merge-tree": true, "mktree": true,
	"mv": true, "read-tree": true, "rebase": true, "reflog": true, "rev-list": true,
	"rev-parse": true, "revert": true, "rm": true, "show-ref": true, "stash": true,
	"status": true, "symbolic-ref": true, "tag": true, "update-index": true,
	"update-ref": true, "var": true, "version": true, "worktree": true,
	"write-tree": true,
}

// mayUseRemote reports whether git with args may contact a remote. What isn't
// listed as local counts as remote: clone, fetch, pull, push and ls-remote, but
// also a command skl starts using later. Wrongly remote, it gets a timeout and
// loses the terminal; wrongly local, a dead remote could hang it for good.
func mayUseRemote(args []string) bool {
	return !localCommands[subcommand(args)]
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
