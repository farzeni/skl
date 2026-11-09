// Package gitx is the only package that runs git. Every call runs the same
// way on every host: without prompts (GIT_TERMINAL_PROMPT=0, ssh in batch
// mode), logged before it runs when SKL_DEBUG=1, and with git's error turned
// into an *Error (docs/design/architecture.md "Fetching from remotes").
//
// git runs in one of three ways (see modeOf). A command that may contact a
// remote has a timeout and leads its own process group, so stopping it stops
// all it started. A command that may sign or run a hook keeps the terminal, for
// a pinentry, and has no timeout. Other local commands have no timeout and lead
// their own group, so only ctx can stop them, not Ctrl-C on the terminal.
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
// matches ErrUnreachable (and ErrNoAccess if the repo isn't there or access is
// denied), and a dir outside a repo ErrNotRepo. If ctx is canceled, the error
// matches ctx's error. A git that may sign or run a hook shares the terminal, so
// Ctrl-C can stop it even under context.WithoutCancel: the error then matches
// context.Canceled while ctx is live, and the caller undoes its step.
func (g *Git) Run(ctx context.Context, dir string, args ...string) (string, error) {
	return g.RunInput(ctx, dir, "", args...)
}

// RunInput is Run with input on git's standard input, for commands such as mktree
// that read their work from it.
func (g *Git) RunInput(ctx context.Context, dir, input string, args ...string) (string, error) {
	if err := g.checkVersion(ctx); err != nil {
		return "", err
	}
	return g.run(ctx, dir, input, args)
}

func (g *Git) run(ctx context.Context, dir, input string, args []string) (string, error) {
	env := baseEnv()
	if ssh := g.sshCommand(ctx, dir); ssh != "" {
		env = append(env, "GIT_SSH_COMMAND="+ssh)
	} else {
		// git would run even an empty GIT_SSH_COMMAND instead of GIT_SSH.
		env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "GIT_SSH_COMMAND=") })
	}
	return g.exec(ctx, dir, input, args, env)
}

// exec runs git with env as its whole environment.
func (g *Git) exec(ctx context.Context, dir, input string, args, env []string) (string, error) {
	if dir == "" {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "))
	} else {
		g.log.DebugContext(ctx, "git "+strings.Join(args, " "), "dir", dir)
	}

	runCtx, timeout := ctx, time.Duration(0)
	mode := modeOf(args)
	if mode == modeRemote {
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
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	// Interrupted, git removes its lock files; killed, it would leave index.lock
	// in the way of the next commit. WaitDelay kills a git that ignores the
	// interrupt.
	switch mode {
	case modeRemote:
		// git leads its own process group, so stopping it stops what it
		// started, such as ssh or a credential helper, which would otherwise
		// outlive skl. A background group can't read the terminal: a child that
		// tries stops, until the timeout.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return signalGroup(cmd.Process.Pid, syscall.SIGKILL) }
	case modeTerminal:
		// git stays in skl's group, in the terminal's foreground, where Ctrl-C
		// reaches all of it.
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	case modeQuiet:
		// The terminal's Ctrl-C doesn't reach a background group, so a step
		// under context.WithoutCancel finishes. A hook here that reads the
		// terminal stops until ctx ends or a second Ctrl-C kills skl. SIGCONT
		// lets a stopped hook take the interrupt: git waits for it before it
		// removes its locks.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			err := signalGroup(cmd.Process.Pid, syscall.SIGINT)
			_ = signalGroup(cmd.Process.Pid, syscall.SIGCONT)
			return err
		}
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
	e := &Error{Args: args, Stdout: stdout.String(), Stderr: stderr.String(), Code: -1, err: err}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		e.Code = exit.ExitCode()
	}
	switch {
	case ctx.Err() != nil:
		e.err = ctx.Err()
	case interrupted(err):
		// Ctrl-C reached a git in modeTerminal through the terminal, before
		// skl's own signal handler canceled ctx, or under WithoutCancel.
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

// mode is how a git command runs.
type mode int

const (
	// modeRemote may contact a remote: its own group, a timeout, killed when
	// stopped.
	modeRemote mode = iota
	// modeTerminal is local and may sign or run a hook, so a pinentry or a slow
	// hook can take its time: skl's group, no timeout, interrupted when stopped.
	modeTerminal
	// modeQuiet is any other local command: its own group, no timeout,
	// interrupted when ctx ends.
	modeQuiet
)

// terminalCommands may sign or run a hook that talks to the user. checkout and
// switch run post-checkout too, but they can also fetch blobs, so they count as
// remote: see quietCommands.
var terminalCommands = map[string]bool{
	"am": true, "cherry-pick": true, "commit": true, "commit-tree": true,
	"merge": true, "rebase": true, "revert": true, "tag": true, "worktree": true,
}

// quietCommands are the other commands that don't contact a remote in the way
// skl runs them.
//
// checkout, switch, restore, reset, sparse-checkout, cat-file, show, diff, grep,
// blame and archive are left out on purpose: in a partial clone, as in skl's
// source cache, they fetch missing blobs from the remote, and with no timeout a
// dead remote could hang them for good. So they count as remote.
var quietCommands = map[string]bool{
	"add": true, "apply": true, "branch": true, "check-attr": true,
	"check-ignore": true, "config": true, "diff-index": true, "for-each-ref": true,
	"hash-object": true, "init": true, "log": true, "ls-files": true, "ls-tree": true,
	"merge-base": true, "merge-file": true, "merge-tree": true, "mktree": true,
	"mv": true, "read-tree": true, "reflog": true, "rev-list": true, "rev-parse": true,
	"rm": true, "show-ref": true, "stash": true, "status": true, "symbolic-ref": true,
	"update-index": true, "update-ref": true, "var": true, "version": true,
	"write-tree": true,
}

// modeOf says how git with args runs. What isn't listed counts as remote: clone,
// fetch, pull, push and ls-remote, but also a command skl starts using later.
// Wrongly remote, it gets a timeout and loses the terminal; wrongly local, a dead
// remote could hang it for good.
func modeOf(args []string) mode {
	sub := subcommand(args)
	switch {
	case terminalCommands[sub]:
		return modeTerminal
	case quietCommands[sub]:
		return modeQuiet
	}
	return modeRemote
}

// signalGroup sends sig to the process group that pid leads. A group with
// nothing left to signal is ESRCH, or EPERM on macOS when only zombies remain.
func signalGroup(pid int, sig syscall.Signal) error {
	err := syscall.Kill(-pid, sig)
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
		return os.ErrProcessDone
	}
	return err
}

// repoEnv names the variables that point git at a repo or at parts of one: `git
// rev-parse --local-env-vars`, plus GIT_NAMESPACE. Inside a git hook they are
// set for the repo the hook runs in, and they would send skl's git calls there
// instead of to the dir skl names. git's other local variables,
// GIT_CONFIG_PARAMETERS and GIT_CONFIG_COUNT, carry settings, not a location, and
// stay. So does GIT_CEILING_DIRECTORIES: it can stop git looking upward, but
// never points it elsewhere.
var repoEnv = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_CONFIG", "GIT_DIR",
	"GIT_GRAFT_FILE", "GIT_IMPLICIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_NAMESPACE",
	"GIT_NO_REPLACE_OBJECTS", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX",
	"GIT_REPLACE_REF_BASE", "GIT_SHALLOW_FILE", "GIT_WORK_TREE",
}

// baseEnv is the process's environment without repoEnv, plus
// GIT_TERMINAL_PROMPT=0, so a missing credential fails at once instead of waiting
// for input. LC_ALL=C keeps git's messages in English, so classify can read them.
func baseEnv() []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(repoEnv, name)
	})
	return append(env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
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
		out, _ := g.exec(ctx, dir, "", []string{"config", "--get", "core.sshCommand"}, baseEnv())
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
