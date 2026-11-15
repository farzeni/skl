package ref

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/farzeni/skl/internal/selector"
)

// ErrNotRef is a word that is none of the forms of a reference.
var ErrNotRef = errors.New("isn't a reference")

// Ref is a reference to someone else's repo or to something in it, read
// (repos.md "Skills inside a repo" and "Pinning a version").
type Ref struct {
	Input string // as typed, with any password hidden: safe to print
	Repo  Repo

	// HasPath reports that the reference names something inside the repo: a
	// folder, a skill set, or the repo itself as one skill ("repo//").
	HasPath bool
	Path    string // the folder, slash-separated, no slash at either end; "" is the repo's root
	Set     string // a skill set's name, after @; Path is where their vault starts
	Pin     string // the tag, branch or commit after #; "" follows the default branch

	split bool // written with //
}

// Parse reads word as a reference to a repo, a folder or a skill set in it,
// with an optional pin, or returns an error matching ErrNotRef. A two-part
// word is a repo on the default host: whether it names a vault skill instead
// is the caller's call.
func Parse(word string, o Options) (Ref, error) {
	r := Ref{Input: hidePassword(word)}
	fail := func(why string) error {
		return fmt.Errorf("%s %w: %s", r.Input, ErrNotRef, why)
	}

	rest, pin, pinned := strings.Cut(word, "#")
	if pinned && !validPin(pin) {
		return Ref{}, fail("#" + pin + " isn't a tag, branch or commit")
	}
	repoWord, inner, split := cutPath(rest)
	var hasSet bool
	if split {
		inner, r.Set, hasSet = cutSet(strings.TrimSuffix(inner, "/"))
		r.HasPath = true
	} else {
		repoWord, r.Set, hasSet = cutSet(repoWord)
		repoWord, inner = cutShorthand(repoWord)
		r.HasPath = hasSet || inner != ""
	}
	r.Pin, r.split = pin, split

	if hasSet && !selector.ValidName(r.Set) {
		return Ref{}, fail("@" + r.Set + " isn't a valid skill set name")
	}
	if inner != "" {
		for part := range strings.SplitSeq(inner, "/") {
			if !validPathPart(part) {
				return Ref{}, fail(inner + " isn't a path inside a repo")
			}
		}
	}
	r.Path = inner

	repo, err := ParseRepo(repoWord, o)
	if err != nil {
		return Ref{}, fail("use owner/repo/path, host/path//path, a URL or a path")
	}
	r.Repo = repo
	return r, nil
}

// cutPath splits word at the // that ends the repo, past a URL's scheme.
func cutPath(word string) (repo, inner string, found bool) {
	start := 0
	if i := strings.Index(word, "://"); i >= 0 {
		start = i + len("://")
	}
	j := strings.Index(word[start:], "//")
	if j < 0 {
		return word, "", false
	}
	return word[:start+j], word[start+j+2:], true
}

// cutSet splits a last part that starts with @ off s. An @ part never
// belongs to a repo, so no // is needed before it.
func cutSet(s string) (rest, set string, found bool) {
	i := strings.LastIndex(s, "/")
	last := s[i+1:]
	if !strings.HasPrefix(last, "@") {
		return s, "", false
	}
	return s[:max(i, 0)], last[1:], true
}

// cutShorthand splits owner/repo/path on the default host: the first two
// parts are the repo, the rest is the path. Every other form is all repo.
func cutShorthand(word string) (repo, inner string) {
	if isPath(word) || strings.Contains(word, "://") {
		return word, ""
	}
	if _, _, isSCP := cutSCP(word); isSCP {
		return word, ""
	}
	parts := strings.Split(strings.TrimSuffix(word, "/"), "/")
	if len(parts) < 3 || strings.Contains(parts[0], ".") {
		return word, ""
	}
	return parts[0] + "/" + parts[1], strings.Join(parts[2:], "/")
}

// validPathPart reports whether part can be a folder name in a path inside a
// repo. An @ part is a skill set, and only as the last part.
func validPathPart(part string) bool {
	return part != "" && part != "." && part != ".." && !strings.HasPrefix(part, "@") &&
		!strings.ContainsFunc(part, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) })
}

// validPin reports whether pin can be a tag, branch or commit name. It follows
// git check-ref-format, and never lets a pin reach git as an option or as a
// revision expression (HEAD~1, main@{1}, a..b).
func validPin(pin string) bool {
	switch {
	case pin == "", pin == "@",
		strings.HasPrefix(pin, "-"), strings.HasPrefix(pin, "/"), strings.HasPrefix(pin, "."),
		strings.HasSuffix(pin, "/"), strings.HasSuffix(pin, "."), strings.HasSuffix(pin, ".lock"),
		strings.Contains(pin, ".."), strings.Contains(pin, "@{"), strings.Contains(pin, "//"),
		strings.Contains(pin, "/."):
		return false
	}
	return !strings.ContainsFunc(pin, func(c rune) bool {
		return unicode.IsSpace(c) || unicode.IsControl(c) || strings.ContainsRune(`~^:?*[\`, c)
	})
}

// hidePassword replaces the password in a URL's user info, if any, so the
// word is safe to print. The rest stays as typed.
func hidePassword(word string) string {
	i := strings.Index(word, "://")
	if i < 0 {
		return word
	}
	authority := word[i+3:]
	if end := strings.IndexAny(authority, "/#"); end >= 0 {
		authority = authority[:end]
	}
	at := strings.LastIndex(authority, "@")
	colon := strings.Index(authority, ":")
	if at < 0 || colon < 0 || colon > at {
		return word
	}
	start := i + 3 + colon + 1
	return word[:start] + "xxxxx" + word[i+3+at:]
}

// Canonical is the reference as skl stores it (repos.md "How a reference is
// stored"): the repo as a full URL, then // and the path or skill set if any.
// The pin is stored apart. home is what ~ means.
func (r Ref) Canonical(home string) string {
	s := r.Repo.canonical(home)
	if !r.HasPath {
		return s
	}
	s += "//" + r.Path
	if r.Set != "" {
		if r.Path != "" {
			s += "/"
		}
		s += "@" + r.Set
	}
	return s
}

// FolderHint is the reference written with its repo's last part as a folder
// in the repo, for a host/path repo with three or more parts after the host
// that can't be reached: gitlab.com/team/skills/pdf → gitlab.com/team/skills//pdf.
// It reports false for any other reference.
func (r Ref) FolderHint() (string, bool) {
	i := strings.LastIndex(r.Repo.Path, "/")
	if r.Repo.form != formHost || r.split || r.Path != "" || !strings.Contains(r.Repo.Path[:max(i, 0)], "/") {
		return "", false
	}
	s := r.Repo.Host + "/" + r.Repo.Path[:i] + "//" + r.Repo.Path[i+1:]
	if r.Set != "" {
		s += "/@" + r.Set
	}
	if r.Pin != "" {
		s += "#" + r.Pin
	}
	return s, true
}

// canonical is the repo as stored: a short form as HTTPS on its host, a
// repo on this machine as its path (with ~ under home), a URL as typed but
// for its password; never a trailing .git.
func (r Repo) canonical(home string) string {
	switch r.form {
	case formLocal:
		return tildePath(r.Path, home)
	case formShort, formHost:
		return "https://" + r.Host + "/" + r.Path
	}
	s := r.URL
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.User != nil {
			if _, has := u.User.Password(); has {
				u.User = url.User(u.User.Username())
				s = u.String()
			}
		}
	}
	return strings.TrimSuffix(strings.TrimRight(s, "/"), ".git")
}

// tildePath writes an absolute path under home with ~.
func tildePath(p, home string) string {
	if home == "" {
		return p
	}
	home = filepath.Clean(home)
	switch {
	case home == "/":
		return p
	case p == home:
		return "~"
	case strings.HasPrefix(p, home+"/"):
		return "~" + p[len(home):]
	}
	return p
}
