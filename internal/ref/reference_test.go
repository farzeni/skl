package ref

import (
	"errors"
	"strings"
	"testing"
)

var (
	httpsOpts = Options{DefaultHost: "github.com", Home: "/home/u", Cwd: "/work"}
	sshOpts   = func() Options { o := httpsOpts; o.SSH = true; return o }()
)

// want is what Parse should read. An empty canonical means the word itself.
type want struct {
	url, host, repo string // the repo: what git clones, and its identity
	local           bool
	hasPath         bool
	path, set, pin  string
	canonical       string
}

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		doc, word string
		o         Options
		want      want
	}{
		// repos.md "Skills inside a repo"
		{
			doc:  `"In GitHub shorthand, the first two parts are the repo and the rest is the path"`,
			word: "someone/agent-skills/pdf", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", canonical: "https://github.com/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `shorthand with a path of several parts`,
			word: "someone/agent-skills/skills/pdf", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "skills/pdf", canonical: "https://github.com/someone/agent-skills//skills/pdf",
			},
		},
		{
			doc:  `"owner/repo and host/path forms are stored as HTTPS, whatever protocol you fetch with"`,
			word: "someone/agent-skills/pdf", o: sshOpts,
			want: want{
				url: "git@github.com:someone/agent-skills.git", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", canonical: "https://github.com/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `shorthand on another default_host is stored with that host`,
			word: "someone/agent-skills/pdf", o: Options{DefaultHost: "git.corp.example"},
			want: want{
				url: "https://git.corp.example/someone/agent-skills", host: "git.corp.example", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", canonical: "https://git.corp.example/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `shorthand also takes // before the path`,
			word: "someone/agent-skills//skills/pdf", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "skills/pdf", canonical: "https://github.com/someone/agent-skills//skills/pdf",
			},
		},
		{
			doc:  `"skl learn gitlab.com/team/sub/skills//review/pr-review"`,
			word: "gitlab.com/team/sub/skills//review/pr-review", o: httpsOpts,
			want: want{
				url: "https://gitlab.com/team/sub/skills", host: "gitlab.com", repo: "team/sub/skills",
				hasPath: true, path: "review/pr-review", canonical: "https://gitlab.com/team/sub/skills//review/pr-review",
			},
		},
		{
			doc:  `host/path with protocol ssh is stored as HTTPS too`,
			word: "gitlab.com/team/skills//pdf", o: sshOpts,
			want: want{
				url: "git@gitlab.com:team/skills.git", host: "gitlab.com", repo: "team/skills",
				hasPath: true, path: "pdf", canonical: "https://gitlab.com/team/skills//pdf",
			},
		},
		{
			doc:  `"In the host/path form, every part after the host belongs to the repo"`,
			word: "gitlab.com/team/sub/skills", o: httpsOpts,
			want: want{
				url: "https://gitlab.com/team/sub/skills", host: "gitlab.com", repo: "team/sub/skills",
				canonical: "https://gitlab.com/team/sub/skills",
			},
		},
		{
			doc:  `"skl learn git@git.example.com:team/skills.git//review/pr-review"; "A trailing .git is dropped"`,
			word: "git@git.example.com:team/skills.git//review/pr-review", o: httpsOpts,
			want: want{
				url: "git@git.example.com:team/skills.git", host: "git.example.com", repo: "team/skills",
				hasPath: true, path: "review/pr-review", canonical: "git@git.example.com:team/skills//review/pr-review",
			},
		},
		{
			doc:  `"A URL you typed is stored as you typed it": an HTTPS URL keeps its host's case and port`,
			word: "https://Git.Example.com:8443/team/skills.git//pdf", o: sshOpts,
			want: want{
				url: "https://Git.Example.com:8443/team/skills.git", host: "git.example.com", repo: "team/skills",
				hasPath: true, path: "pdf", canonical: "https://Git.Example.com:8443/team/skills//pdf",
			},
		},
		{
			doc:  `an ssh:// URL with a path`,
			word: "ssh://git@git.example.com:2222/team/skills//pdf", o: httpsOpts,
			want: want{
				url: "ssh://git@git.example.com:2222/team/skills", host: "git.example.com", repo: "team/skills",
				hasPath: true, path: "pdf",
			},
		},
		{
			doc:  `"If the whole repo is one skill, end the reference with //"`,
			word: "someone/pdf-skill//", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/pdf-skill", host: "github.com", repo: "someone/pdf-skill",
				hasPath: true, canonical: "https://github.com/someone/pdf-skill//",
			},
		},
		{
			doc:  `a URL to a repo that is one skill`,
			word: "https://git.example.com/team/pdf.git//", o: httpsOpts,
			want: want{
				url: "https://git.example.com/team/pdf.git", host: "git.example.com", repo: "team/pdf",
				hasPath: true, canonical: "https://git.example.com/team/pdf//",
			},
		},
		{
			doc:  `a repo alone, as a learn argument: no path`,
			word: "someone/agent-skills", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				canonical: "https://github.com/someone/agent-skills",
			},
		},
		{
			doc:  `a trailing slash after a path is dropped`,
			word: "someone/agent-skills/skills/pdf/", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "skills/pdf", canonical: "https://github.com/someone/agent-skills//skills/pdf",
			},
		},

		// repos.md "How a reference is stored": a repo on this machine
		{
			doc:  `"A repo on this machine is stored as its path: with ~ when it's under your home folder"`,
			word: "~/code/skills//pdf", o: httpsOpts,
			want: want{
				url: "/home/u/code/skills", repo: "/home/u/code/skills", local: true,
				hasPath: true, path: "pdf", canonical: "~/code/skills//pdf",
			},
		},
		{
			doc:  `an absolute path under home is stored with ~`,
			word: "/home/u/code/skills.git//pdf", o: httpsOpts,
			want: want{
				url: "/home/u/code/skills.git", repo: "/home/u/code/skills", local: true,
				hasPath: true, path: "pdf", canonical: "~/code/skills//pdf",
			},
		},
		{
			doc:  `home itself is stored as ~`,
			word: "~", o: httpsOpts,
			want: want{url: "/home/u", repo: "/home/u", local: true, canonical: "~"},
		},
		{
			doc:  `"else absolute"; a folder that only starts like home isn't under it`,
			word: "/home/user2/skills", o: httpsOpts,
			want: want{url: "/home/user2/skills", repo: "/home/user2/skills", local: true},
		},
		{
			doc:  `a relative path is stored absolute`,
			word: "./skills//review/pr-review", o: httpsOpts,
			want: want{
				url: "/work/skills", repo: "/work/skills", local: true,
				hasPath: true, path: "review/pr-review", canonical: "/work/skills//review/pr-review",
			},
		},
		{
			doc:  `a file:// URL is a repo on this machine, stored as its path`,
			word: "file:///home/u/code/skills.git//pdf", o: httpsOpts,
			want: want{
				url: "file:///home/u/code/skills.git", repo: "/home/u/code/skills", local: true,
				hasPath: true, path: "pdf", canonical: "~/code/skills//pdf",
			},
		},

		// repos.md "Skills inside a repo": skill sets
		{
			doc:  `"skl learn someone/agent-skills/@backend"`,
			word: "someone/agent-skills/@backend", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, set: "backend", canonical: "https://github.com/someone/agent-skills//@backend",
			},
		},
		{
			doc:  `"skl learn gitlab.com/team/sub/skills//@backend"`,
			word: "gitlab.com/team/sub/skills//@backend", o: httpsOpts,
			want: want{
				url: "https://gitlab.com/team/sub/skills", host: "gitlab.com", repo: "team/sub/skills",
				hasPath: true, set: "backend", canonical: "https://gitlab.com/team/sub/skills//@backend",
			},
		},
		{
			doc:  `"someone/agent-skills//sub/@backend reads sub/sets/backend.toml"`,
			word: "someone/agent-skills//sub/@backend", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "sub", set: "backend", canonical: "https://github.com/someone/agent-skills//sub/@backend",
			},
		},
		{
			doc:  `shorthand: a path before the @ without //`,
			word: "someone/agent-skills/sub/@backend", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "sub", set: "backend", canonical: "https://github.com/someone/agent-skills//sub/@backend",
			},
		},
		{
			doc:  `an @ last part never belongs to a repo, so a host form needs no // before it`,
			word: "gitlab.com/team/sub/skills/@backend", o: httpsOpts,
			want: want{
				url: "https://gitlab.com/team/sub/skills", host: "gitlab.com", repo: "team/sub/skills",
				hasPath: true, set: "backend", canonical: "https://gitlab.com/team/sub/skills//@backend",
			},
		},
		{
			doc:  `files.md: url = "https://github.com/someone/agent-skills//@backend"`,
			word: "https://github.com/someone/agent-skills//@backend", o: sshOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, set: "backend",
			},
		},
		{
			doc:  `a skill set in an SSH URL`,
			word: "git@git.example.com:team/skills.git//@backend", o: httpsOpts,
			want: want{
				url: "git@git.example.com:team/skills.git", host: "git.example.com", repo: "team/skills",
				hasPath: true, set: "backend", canonical: "git@git.example.com:team/skills//@backend",
			},
		},
		{
			doc:  `a skill set in a repo on this machine`,
			word: "~/code/skills/@backend", o: httpsOpts,
			want: want{
				url: "/home/u/code/skills", repo: "/home/u/code/skills", local: true,
				hasPath: true, set: "backend", canonical: "~/code/skills//@backend",
			},
		},
		{
			doc:  `a set name follows the name grammar, letters from any script included`,
			word: "someone/agent-skills/@résumé_2", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, set: "résumé_2", canonical: "https://github.com/someone/agent-skills//@résumé_2",
			},
		},

		// repos.md "Pinning a version"
		{
			doc:  `"skl learn someone/agent-skills/pdf#v1.2": a tag; "A pin is stored apart"`,
			word: "someone/agent-skills/pdf#v1.2", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", pin: "v1.2", canonical: "https://github.com/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `"Add # and a tag, branch or commit": a branch with a slash`,
			word: "someone/agent-skills/pdf#feature/new-api", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", pin: "feature/new-api", canonical: "https://github.com/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `"Add # and a tag, branch or commit": a commit`,
			word: "gitlab.com/team/skills//pdf#3f9a1c2e4b5d6f708192a3b4c5d6e7f8091a2b3c", o: httpsOpts,
			want: want{
				url: "https://gitlab.com/team/skills", host: "gitlab.com", repo: "team/skills",
				hasPath: true, path: "pdf", pin: "3f9a1c2e4b5d6f708192a3b4c5d6e7f8091a2b3c",
				canonical: "https://gitlab.com/team/skills//pdf",
			},
		},
		{
			doc:  `"everything after it is the ref's name, even another #: pdf#v1#rc pins v1#rc"`,
			word: "someone/agent-skills/pdf#v1#rc", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, path: "pdf", pin: "v1#rc", canonical: "https://github.com/someone/agent-skills//pdf",
			},
		},
		{
			doc:  `"someone/agent-skills/@backend#v1.2 copies the set and every skill it names at v1.2"`,
			word: "someone/agent-skills/@backend#v1.2", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/agent-skills", host: "github.com", repo: "someone/agent-skills",
				hasPath: true, set: "backend", pin: "v1.2", canonical: "https://github.com/someone/agent-skills//@backend",
			},
		},
		{
			doc:  `a pinned URL with a path`,
			word: "https://git.example.com/team/skills.git//review/pr-review#main", o: httpsOpts,
			want: want{
				url: "https://git.example.com/team/skills.git", host: "git.example.com", repo: "team/skills",
				hasPath: true, path: "review/pr-review", pin: "main", canonical: "https://git.example.com/team/skills//review/pr-review",
			},
		},
		{
			doc:  `a pinned repo that is one skill`,
			word: "someone/pdf-skill//#v2", o: httpsOpts,
			want: want{
				url: "https://github.com/someone/pdf-skill", host: "github.com", repo: "someone/pdf-skill",
				hasPath: true, pin: "v2", canonical: "https://github.com/someone/pdf-skill//",
			},
		},
		{
			doc:  `a pinned repo on this machine`,
			word: "~/code/skills//pdf#v1", o: httpsOpts,
			want: want{
				url: "/home/u/code/skills", repo: "/home/u/code/skills", local: true,
				hasPath: true, path: "pdf", pin: "v1", canonical: "~/code/skills//pdf",
			},
		},

		// Passwords: a URL is stored as typed, but never with its password.
		{
			doc:  `a password in a URL is never stored`,
			word: "https://you:s3cret@git.example.com/team/skills.git//pdf", o: httpsOpts,
			want: want{
				url: "https://you:s3cret@git.example.com/team/skills.git", host: "git.example.com", repo: "team/skills",
				hasPath: true, path: "pdf", canonical: "https://you@git.example.com/team/skills//pdf",
			},
		},
	} {
		t.Run(tt.word, func(t *testing.T) {
			r, err := Parse(tt.word, tt.o)
			if err != nil {
				t.Fatalf("%s: %v", tt.doc, err)
			}
			w := tt.want
			if w.canonical == "" {
				w.canonical = tt.word
			}
			got := want{
				url: r.Repo.URL, host: r.Repo.Host, repo: r.Repo.Path, local: r.Repo.Local,
				hasPath: r.HasPath, path: r.Path, set: r.Set, pin: r.Pin,
				canonical: r.Canonical(tt.o.Home),
			}
			if got != w {
				t.Errorf("%s\n got %+v\nwant %+v", tt.doc, got, w)
			}
		})
	}
}

// The canonical form is what .skl-origin.toml stores, so reading it back gives
// the same reference, on a machine with any default_host and protocol.
func TestCanonicalRoundTrip(t *testing.T) {
	other := Options{DefaultHost: "git.corp.example", SSH: true, Home: "/home/u", Cwd: "/elsewhere"}
	for _, word := range []string{
		"someone/agent-skills/pdf",
		"someone/agent-skills//skills/pdf#v1.2",
		"someone/agent-skills/@backend#v1.2",
		"someone/agent-skills//sub/@backend",
		"someone/pdf-skill//",
		"someone/agent-skills",
		"gitlab.com/team/sub/skills//review/pr-review",
		"gitlab.com/team/sub/skills",
		"https://Git.Example.com:8443/team/skills.git//pdf",
		"https://you:s3cret@git.example.com/team/skills//pdf",
		"git@git.example.com:team/skills.git//review/pr-review",
		"ssh://git@git.example.com:2222/team/skills/@backend",
		"~/code/skills.git//pdf",
		"./skills//pdf",
		"/srv/git/skills.git/@backend",
		"file:///home/u/code/skills//pdf",
	} {
		r, err := Parse(word, httpsOpts)
		if err != nil {
			t.Fatalf("Parse(%q): %v", word, err)
		}
		c := r.Canonical(httpsOpts.Home)
		for _, o := range []Options{httpsOpts, other} {
			back, err := Parse(c, o)
			if err != nil {
				t.Fatalf("Parse(%q) of the canonical form of %q: %v", c, word, err)
			}
			if again := back.Canonical(o.Home); again != c {
				t.Errorf("%q → %q → %q", word, c, again)
			}
			if !back.Repo.Same(r.Repo) || back.HasPath != r.HasPath || back.Path != r.Path || back.Set != r.Set {
				t.Errorf("%q → %q reads as %+v, want %+v", word, c, back, r)
			}
			if back.Pin != "" {
				t.Errorf("%q → %q: the canonical form holds the pin %q", word, c, back.Pin)
			}
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, tt := range []struct{ word, why string }{
		{"", "empty"},
		{"pdf", "one word is a selector"},
		{"someone/@backend", "a set needs a repo before it"},
		{"@backend", "a set in your vault is a selector"},
		{"-someone/agent-skills/pdf", "an option, not a reference"},
		{"someone/agent-skills/pdf#", "an empty pin"},
		{"someone/agent-skills/pdf#-upload-pack=evil", "a pin that git would read as an option"},
		{"someone/agent-skills/pdf#v 1", "a pin with a space"},
		{"someone/agent-skills/pdf#v1..v2", "a range, not a ref"},
		{"someone/agent-skills/pdf#HEAD~1", "a revision expression, not a ref"},
		{"someone/agent-skills/pdf#main@{1}", "a reflog expression"},
		{"someone/agent-skills//a//b", "an empty part in the path"},
		{"someone/agent-skills//../pdf", "a path out of the repo"},
		{"someone/agent-skills/./pdf", "a . part"},
		{"someone/agent-skills//pdf/@a.b", "a set name with a dot"},
		{"someone/agent-skills//@", "an empty set name"},
		{"someone/agent-skills//@backend/pdf", "@ only as the last part"},
		{"someone/agent-skills/@-x", "a set name starting with -"},
		{"someone/agent-skills//p df", "a space in the path"},
		{"gitlab.com//pdf", "no repo before //"},
		{"//pdf", "no repo before //"},
		{"https://x.com//pdf", "no repo before //"},
		{"ftp://x.com/team/skills//pdf", "not a git URL"},
		{"ext::sh -c evil//x", "git's remote helpers"},
		{"~/x//pdf", "no home to read ~ from"},
	} {
		o := httpsOpts
		if strings.HasPrefix(tt.word, "~") {
			o = Options{}
		}
		if r, err := Parse(tt.word, o); !errors.Is(err, ErrNotRef) {
			t.Errorf("Parse(%q) = %+v, %v; want ErrNotRef (%s)", tt.word, r, err, tt.why)
		}
	}
}

// repos.md "Repos": "If a repo with three or more parts after the host can't
// be reached, the error says how to write it as a path instead".
func TestFolderHint(t *testing.T) {
	for _, tt := range []struct{ word, hint string }{
		{"gitlab.com/team/skills/pdf", "gitlab.com/team/skills//pdf"},
		{"gitlab.com/team/sub/skills/pdf#v1", "gitlab.com/team/sub/skills//pdf#v1"},
		{"gitlab.com/team/skills/pdf/@backend", "gitlab.com/team/skills//pdf/@backend"},
		{"GitLab.com/team/skills/pdf", "gitlab.com/team/skills//pdf"},
		// Nothing to suggest.
		{"gitlab.com/team/skills", ""},      // two parts after the host
		{"gitlab.com/team/skills//pdf", ""}, // the path is already given
		{"someone/agent-skills/pdf", ""},    // shorthand already splits
		{"https://gitlab.com/a/b/c", ""},    // a URL is used as given
		{"git@gitlab.com:a/b/c", ""},        // same
		{"~/code/team/skills/x", ""},        // a path on this machine
	} {
		r, err := Parse(tt.word, httpsOpts)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.word, err)
		}
		hint, ok := r.FolderHint()
		if ok != (tt.hint != "") || hint != tt.hint {
			t.Errorf("FolderHint(%q) = %q, %v; want %q", tt.word, hint, ok, tt.hint)
		}
	}
}

// Ref.Input is safe to print: as typed, with any password hidden.
func TestParseInput(t *testing.T) {
	for word, input := range map[string]string{
		"someone/agent-skills/pdf#v1":                    "someone/agent-skills/pdf#v1",
		"https://you:s3cret@git.example.com/t/s//pdf#v1": "https://you:xxxxx@git.example.com/t/s//pdf#v1",
	} {
		r, err := Parse(word, httpsOpts)
		if err != nil {
			t.Fatal(err)
		}
		if r.Input != input {
			t.Errorf("Input = %q, want %q", r.Input, input)
		}
	}
}

// skl init takes a repo only: "skl init you/skills#dev is a usage error",
// and so is any path or skill set. ParseRepo keeps rejecting them.
func TestParseRepoTakesNoRefParts(t *testing.T) {
	for _, word := range []string{
		"you/skills#dev", "gitlab.com/team/skills#dev", "https://x.com/team/skills#dev",
		"you/skills//", "you/skills/@backend", "git@x.com:team/skills.git//pdf",
	} {
		if _, err := ParseRepo(word, httpsOpts); !errors.Is(err, ErrNotRepo) {
			t.Errorf("ParseRepo(%q): %v, want ErrNotRepo", word, err)
		}
	}
}

// Whatever Parse accepts, its canonical form reads back as the same
// reference, on a machine with other settings, and stays the same when
// stored again.
func FuzzCanonicalRoundTrip(f *testing.F) {
	for _, seed := range []string{
		"someone/agent-skills/pdf#v1", "someone/agent-skills//sub/@backend", "someone/pdf-skill//",
		"gitlab.com/team/sub/skills//review/pr-review", "https://you:pw@Git.Example.com:8443/t/s.git//pdf",
		"git@git.example.com:team/skills.git/@backend", "~/code/skills//pdf", "./x//y", "file:///srv/s.git//p",
		// found by fuzzing
		"%/0", "/:/.//0", "/@/", "/..", "..//", "/@/.", "@0:0://", "/..git", "0:0:.git//",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, word string) {
		r, err := Parse(word, httpsOpts)
		if err != nil {
			return
		}
		c := r.Canonical(httpsOpts.Home)
		back, err := Parse(c, sshOpts)
		if err != nil {
			t.Fatalf("%q → %q: %v", word, c, err)
		}
		if again := back.Canonical(sshOpts.Home); again != c {
			t.Fatalf("%q → %q → %q", word, c, again)
		}
		if !back.Repo.Same(r.Repo) || back.HasPath != r.HasPath || back.Path != r.Path || back.Set != r.Set || back.Pin != "" {
			t.Fatalf("%q → %q reads as %+v, want %+v", word, c, back, r)
		}
	})
}
