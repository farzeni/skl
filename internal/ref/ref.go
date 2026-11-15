// Package ref reads the references commands take, in the forms of
// docs/reference/repos.md: a repo (ParseRepo, for skl init), and a reference
// to something in a repo (Parse, for skl learn): a path inside it, a skill
// set, a pin. It never touches the disk or the network, so everything it
// needs comes in through Options. A third-party part that rewrites a
// reference (internal/ext) runs before Parse, on the word as typed.
package ref

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ErrNotRepo is a word that is none of the forms of a repo.
var ErrNotRepo = errors.New("isn't a repo: use owner/repo, host/path, a URL or a path")

// Options are the settings and places a repo's short forms depend on.
type Options struct {
	DefaultHost string // the host owner/repo refers to
	SSH         bool   // protocol = "ssh": short forms use SSH, not HTTPS
	Home        string // what ~ means
	Cwd         string // what a relative path starts from
}

// Repo is a repo reference, read.
type Repo struct {
	Input string // as typed, with any password hidden: safe to print and to keep
	URL   string // what git clones: a short form per protocol, any other as given
	Local bool   // on this machine

	// The repo's identity, whatever form or protocol reached it. Path has no
	// leading slash and no .git, unless the repo is local: then it's absolute.
	Host, Path string

	form form // how it was written: decides how it's stored
}

// form is how a repo was written.
type form int

const (
	formURL   form = iota // a URL or git's scp-like form, used as given
	formShort             // owner/repo on the default host
	formHost              // host/path
	formLocal             // a path or a file:// URL
)

// ParseRepo reads word as a repo, in any of the forms in repos.md, or returns
// an error matching ErrNotRepo.
func ParseRepo(word string, o Options) (Repo, error) {
	r := Repo{Input: word}
	var ok bool
	switch {
	case strings.HasPrefix(word, "-"):
		// Never let a word reach git as an option.
	case isPath(word):
		ok = r.parsePath(word, o)
	case strings.Contains(word, "://"):
		ok = r.parseURL(word)
	default:
		if host, path, isSCP := cutSCP(word); isSCP {
			ok = r.setRemote(host, path)
			r.URL, r.form = word, formURL
		} else {
			ok = r.parseShort(word, o)
		}
	}
	if !ok {
		return Repo{}, fmt.Errorf("%s %w", word, ErrNotRepo)
	}
	return r, nil
}

func isPath(word string) bool {
	return word == "~" || word == "." || word == ".." ||
		strings.HasPrefix(word, "~/") || strings.HasPrefix(word, "/") ||
		strings.HasPrefix(word, "./") || strings.HasPrefix(word, "../")
}

func (r *Repo) parsePath(word string, o Options) bool {
	p := word
	switch {
	case p == "~" || strings.HasPrefix(p, "~/"):
		if o.Home == "" {
			return false
		}
		p = filepath.Join(o.Home, p[1:])
	case !filepath.IsAbs(p):
		p = filepath.Join(o.Cwd, p)
	}
	p = filepath.Clean(p)
	r.URL, r.Local, r.Path, r.form = p, true, strings.TrimSuffix(p, ".git"), formLocal
	return true
}

func (r *Repo) parseURL(word string) bool {
	u, err := url.Parse(word)
	if err != nil || u.Fragment != "" || strings.Contains(u.Path, "//") {
		return false
	}
	r.URL = word
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			r.Input = u.Redacted()
		}
	}
	switch u.Scheme {
	case "file":
		p := filepath.Clean(u.Path)
		if !filepath.IsAbs(p) {
			return false
		}
		r.Local, r.Path, r.form = true, strings.TrimSuffix(p, ".git"), formLocal
		return true
	case "https", "http", "ssh", "git":
		return r.setRemote(u.Hostname(), u.Path)
	}
	return false
}

// cutSCP splits git's scp-like form, [user@]host:path. A colon after a slash
// belongs to a path, and "::" is git's remote helper syntax, not a host.
func cutSCP(word string) (host, path string, ok bool) {
	before, after, found := strings.Cut(word, ":")
	if !found || strings.Contains(before, "/") || strings.HasPrefix(after, ":") {
		return "", "", false
	}
	if _, h, hasUser := strings.Cut(before, "@"); hasUser {
		before = h
	}
	return before, after, true
}

// parseShort reads owner/repo and host/path.
func (r *Repo) parseShort(word string, o Options) bool {
	host, path := o.DefaultHost, strings.TrimSuffix(word, "/")
	r.form = formShort
	first, rest, found := strings.Cut(path, "/")
	switch {
	case !found:
		return false
	case strings.Contains(first, "."):
		host, path, r.form = first, rest, formHost
	case strings.Contains(rest, "/"):
		return false // three parts: a path inside a repo, for skl learn
	}
	if !r.setRemote(host, path) {
		return false
	}
	if o.SSH {
		r.URL = "git@" + r.Host + ":" + r.Path + ".git"
	} else {
		r.URL = "https://" + r.Host + "/" + r.Path
	}
	return true
}

// setRemote records a remote repo's identity, if its parts can be one.
func (r *Repo) setRemote(host, path string) bool {
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" || strings.ContainsAny(host+path, " \t\n#@") {
		return false
	}
	for part := range strings.SplitSeq(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	r.Host, r.Path = strings.ToLower(host), path
	return true
}

// Same reports whether r and other are one repo, whatever form or protocol
// each is written in.
func (r Repo) Same(other Repo) bool {
	return r.Local == other.Local && r.Host == other.Host && r.Path == other.Path
}

// Short is the repo's shortest name: owner/repo on the default host, else
// host/path, else its path on this machine. It never shows a password that
// a URL holds.
func (r Repo) Short(defaultHost string) string {
	switch {
	case r.Local:
		return r.URL
	case r.Host == strings.ToLower(defaultHost):
		return r.Path
	}
	return r.Host + "/" + r.Path
}

// Owner is the repo's first path part, or a local repo's folder name.
func (r Repo) Owner() string {
	if r.Local {
		return filepath.Base(r.Path)
	}
	owner, _, _ := strings.Cut(r.Path, "/")
	return owner
}
