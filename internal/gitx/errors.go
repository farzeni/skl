package gitx

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	// ErrUnreachable is a remote that can't be reached: the host doesn't
	// resolve or refuses, access is denied, the repo isn't there, or the
	// call timed out (docs/reference/commands.md "Errors").
	ErrUnreachable = errors.New("can't reach the remote")
	// ErrNoAccess is the kind of ErrUnreachable where the repo isn't there or
	// access is denied, as opposed to a network or server failure. Hosts answer
	// those two alike, so skl can't tell them apart. It also matches
	// ErrUnreachable.
	ErrNoAccess = fmt.Errorf("%w: not there or access denied", ErrUnreachable)
	// ErrNotRepo is a directory that isn't in a git repository.
	ErrNotRepo = errors.New("not a git repository")

	errTimeout = errors.New("timed out")
)

// Error is a git call that failed. It matches ErrUnreachable, ErrNoAccess or
// ErrNotRepo with errors.Is when git's message says so, and ctx's error when the
// call was canceled.
type Error struct {
	Args   []string // the arguments after "git"
	Stdout string   // all of git's standard output: merge-tree reports a conflict there
	Stderr string   // all of git's standard error
	Code   int      // git's exit code; -1 if git didn't exit on its own

	kind   error  // ErrUnreachable, ErrNoAccess, ErrNotRepo or nil
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
// resolve host)", or "The requested URL returned error: 500". It is "" for other
// failures.
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
// reached, most specific first, with the reason shown to the user (the phrase
// itself when reason is "") and whether the remote answered no, rather than the
// network or the server failing.
var unreachable = []struct {
	phrase, reason string
	noAccess       bool
}{
	{phrase: "Could not resolve host"}, // curl, and ssh's "Could not resolve hostname"
	{phrase: "Connection refused"},
	{phrase: "Connection timed out"},
	{phrase: "Operation timed out"},
	{phrase: "Network is unreachable"},
	{phrase: "No route to host"},
	{phrase: "Failed to connect"},
	{phrase: "Connection closed by", reason: "Connection closed by remote host"}, // or "by 192.0.2.1 port 22"
	{phrase: "Connection reset"},
	{phrase: "terminal prompts disabled", noAccess: true}, // a password was needed
	{phrase: "Authentication failed", noAccess: true},
	// ssh's "Permission denied (publickey)."; a bare "Permission denied" is
	// a local file, such as "cannot mkdir x: Permission denied".
	{phrase: "Permission denied (", reason: "Permission denied", noAccess: true},
	{phrase: "Host key verification failed", noAccess: true},
	{phrase: "Repository not found", noAccess: true},
	{phrase: "does not appear to be a git repository", noAccess: true},
	{phrase: urlError},
	// git's last word when ssh or the remote side failed in any other way: a bad
	// ssh config, no git-upload-pack, a host saying no in its own words. Too
	// little to tell which, so the line before it is the reason.
	{phrase: couldNotRead},
	{phrase: "unable to access"},
}

const (
	urlError     = "The requested URL returned error"
	couldNotRead = "Could not read from remote repository"
)

// causeBefore is the last line of stderr that says something, without the
// "remote:" git puts before the far side's words. Progress and warnings say
// nothing about the failure.
func causeBefore(stderr string) string {
	lines := strings.Split(stderr, "\n")
	for _, line := range slices.Backward(lines) {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "remote:"))
		if line == "" || strings.HasPrefix(line, "Cloning into") || strings.HasPrefix(strings.ToLower(line), "warning:") {
			continue
		}
		return line
	}
	return ""
}

// classify reads git's standard error and returns the reason for an
// unreachable remote, the message to show (the line naming the reason, else
// git's last fatal or error line) and the sentinel it matches.
func classify(stderr string) (reason, msg string, kind error) {
	for _, u := range unreachable {
		before, _, ok := strings.Cut(stderr, u.phrase)
		if !ok {
			continue
		}
		start := strings.LastIndexByte(before, '\n') + 1
		line, _, _ := strings.Cut(stderr[start:], "\n")
		reason = u.reason
		if reason == "" {
			reason = u.phrase
		}
		noAccess := u.noAccess
		switch u.phrase {
		case urlError:
			// "… returned error: 500": the code decides.
			_, status, _ := strings.Cut(line, u.phrase+": ")
			status = strings.TrimSpace(status)
			reason += ": " + status
			code, _, _ := strings.Cut(status, " ")
			noAccess = code == "401" || code == "403" || code == "404"
		case couldNotRead:
			if cause := causeBefore(stderr[:start]); cause != "" {
				return cause, cause, ErrUnreachable
			}
		case "unable to access":
			// "unable to access 'url': <curl's reason>", such as an SSL problem.
			if _, why, ok := strings.Cut(line, "': "); ok {
				reason = strings.TrimSpace(why)
			}
		}
		if noAccess {
			return reason, Message(line), ErrNoAccess
		}
		return reason, Message(line), ErrUnreachable
	}
	// Other "'…' does not exist" or "not found" lines, such as "path 'x' does
	// not exist in 'HEAD'", are not about a remote.
	for line := range strings.Lines(stderr) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "fatal: repository '") {
			continue
		}
		switch {
		case strings.HasSuffix(line, "' does not exist"): // git clone of a local path
			return "does not exist", Message(line), ErrNoAccess
		case strings.HasSuffix(line, "' not found"): // a host answering 404
			return "Repository not found", Message(line), ErrNoAccess
		}
	}
	if strings.Contains(stderr, "not a git repository") {
		kind = ErrNotRepo
	}
	return "", Message(stderr), kind
}
