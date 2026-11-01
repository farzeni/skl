package gitx

import (
	"errors"
	"strings"
)

var (
	// ErrUnreachable is a remote that can't be reached: the host doesn't
	// resolve or refuses, access is denied, the repo isn't there, or the
	// call timed out (docs/reference/commands.md "Errors").
	ErrUnreachable = errors.New("can't reach the remote")
	// ErrNotRepo is a directory that isn't in a git repository.
	ErrNotRepo = errors.New("not a git repository")

	errTimeout = errors.New("timed out")
)

// Error is a git call that failed. It matches ErrUnreachable or ErrNotRepo
// with errors.Is when git's message says so, and ctx's error when the call
// was canceled.
type Error struct {
	Args   []string // the arguments after "git"
	Stderr string   // all of git's standard error
	Code   int      // git's exit code; -1 if git didn't exit on its own

	kind   error  // ErrUnreachable, ErrNotRepo or nil
	reason string // the short cause, for an unreachable remote
	msg    string // git's message, or what went wrong when git said nothing
	err    error  // what exec or the context returned
}

// Error is "git <subcommand>: <git's message>": the line that names the
// reason for an unreachable remote, else git's last fatal or error line.
func (e *Error) Error() string {
	msg := e.msg
	if msg == "" {
		msg = e.err.Error()
	}
	return "git " + subcommand(e.Args) + ": " + msg
}

// subcommand is the first argument that isn't one of git's own options,
// such as clone in "-c k=v clone …".
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-c", "-C", "--git-dir", "--work-tree", "--namespace", "--config-env":
			i++
		default:
			if !strings.HasPrefix(a, "-") {
				return a
			}
		}
	}
	return ""
}

func (e *Error) Unwrap() []error {
	if e.kind == nil {
		return []error{e.err}
	}
	return []error{e.kind, e.err}
}

// Reason is the short cause of an unreachable remote, in git's words, such
// as "Could not resolve host", for "✗ can't reach github.com (Could not
// resolve host)". It is "" for other failures.
func (e *Error) Reason() string { return e.reason }

// Message returns git's own message from its standard error: the last line
// starting with "fatal:" or "error:", without that prefix, else the last
// line that isn't empty.
func Message(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		for _, p := range []string{"fatal: ", "error: "} {
			if msg, ok := strings.CutPrefix(lines[i], p); ok {
				return strings.TrimSpace(msg)
			}
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// unreachable lists the phrases git, curl and ssh use when a remote can't be
// reached, most specific first, with the reason shown to the user (the
// phrase itself when reason is "").
var unreachable = []struct{ phrase, reason string }{
	{"Could not resolve host", ""}, // curl, and ssh's "Could not resolve hostname"
	{"Connection refused", ""},
	{"Connection timed out", ""},
	{"Operation timed out", ""},
	{"Network is unreachable", ""},
	{"No route to host", ""},
	{"Failed to connect", ""},
	{"terminal prompts disabled", ""}, // a password was needed
	{"Authentication failed", ""},
	// ssh's "Permission denied (publickey)."; a bare "Permission denied" is
	// a local file, such as "cannot mkdir x: Permission denied".
	{"Permission denied (", "Permission denied"},
	{"Host key verification failed", ""},
	{"Repository not found", ""},
	{"does not appear to be a git repository", ""},
	{"The requested URL returned error", ""},
	{"Could not read from remote repository", ""},
	{"unable to access", ""},
}

// classify reads git's standard error and returns the reason for an
// unreachable remote, the message to show (the line naming the reason, else
// git's last fatal or error line) and the sentinel it matches.
func classify(stderr string) (reason, msg string, kind error) {
	for _, u := range unreachable {
		if before, _, ok := strings.Cut(stderr, u.phrase); ok {
			start := strings.LastIndexByte(before, '\n') + 1
			line, _, _ := strings.Cut(stderr[start:], "\n")
			reason = u.reason
			if reason == "" {
				reason = u.phrase
			}
			return reason, Message(line), ErrUnreachable
		}
	}
	// git clone of a local path that isn't there: "fatal: repository
	// '/x' does not exist". Other "'…' does not exist" lines, such as
	// "path 'x' does not exist in 'HEAD'", are not about a remote.
	for line := range strings.Lines(stderr) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "fatal: repository '") && strings.HasSuffix(line, "' does not exist") {
			return "does not exist", Message(line), ErrUnreachable
		}
	}
	if strings.Contains(stderr, "not a git repository") {
		kind = ErrNotRepo
	}
	return "", Message(stderr), kind
}
