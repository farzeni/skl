package ref

import (
	"errors"
	"testing"
)

func TestParseRepo(t *testing.T) {
	https := Options{DefaultHost: "github.com", Home: "/home/u", Cwd: "/work"}
	ssh := https
	ssh.SSH = true
	gitlab := https
	gitlab.DefaultHost = "git.corp.example"

	for _, tt := range []struct {
		doc, word  string
		o          Options
		url        string
		host, path string
		local      bool
	}{
		{
			doc: `"owner/repo: A repo on the default host, GitHub unless you change it"`, word: "you/skills", o: https,
			url: "https://github.com/you/skills", host: "github.com", path: "you/skills",
		},
		{
			doc: `"skl picks SSH or HTTPS from the protocol setting"`, word: "you/skills", o: ssh,
			url: "git@github.com:you/skills.git", host: "github.com", path: "you/skills",
		},
		{
			doc: `"default_host: The host that owner/repo refers to"`, word: "you/skills", o: gitlab,
			url: "https://git.corp.example/you/skills", host: "git.corp.example", path: "you/skills",
		},
		{
			doc: `default_host with protocol ssh`, word: "you/skills", o: func() Options { g := gitlab; g.SSH = true; return g }(),
			url: "git@git.corp.example:you/skills.git", host: "git.corp.example", path: "you/skills",
		},
		{
			doc: `"A first part containing a dot is a host."`, word: "gitlab.com/team/skills", o: https,
			url: "https://gitlab.com/team/skills", host: "gitlab.com", path: "team/skills",
		},
		{
			doc: `host/path with protocol ssh`, word: "gitlab.com/team/sub/skills", o: ssh,
			url: "git@gitlab.com:team/sub/skills.git", host: "gitlab.com", path: "team/sub/skills",
		},
		{
			doc: `a host outranks default_host`, word: "gitlab.com/team/skills", o: gitlab,
			url: "https://gitlab.com/team/skills", host: "gitlab.com", path: "team/skills",
		},
		{
			doc: `a trailing .git or slash in a short form`, word: "you/skills.git/", o: https,
			url: "https://github.com/you/skills", host: "github.com", path: "you/skills",
		},
		{
			doc: `"Any HTTPS URL, used as given"`, word: "https://git.example.com/team/skills.git", o: ssh,
			url: "https://git.example.com/team/skills.git", host: "git.example.com", path: "team/skills",
		},
		{
			doc: `"Any SSH URL, used as given"`, word: "git@git.example.com:team/skills.git", o: https,
			url: "git@git.example.com:team/skills.git", host: "git.example.com", path: "team/skills",
		},
		{
			doc: `ssh:// URL, port left out of the identity`, word: "ssh://git@Git.Example.com:2222/team/skills.git", o: https,
			url: "ssh://git@Git.Example.com:2222/team/skills.git", host: "git.example.com", path: "team/skills",
		},
		{
			doc: `"~/code/skills: A repo on this machine"`, word: "~/code/skills", o: https,
			url: "/home/u/code/skills", path: "/home/u/code/skills", local: true,
		},
		{
			doc: `an absolute path, .git left out of the identity only`, word: "/srv/git/skills.git", o: ssh,
			url: "/srv/git/skills.git", path: "/srv/git/skills", local: true,
		},
		{
			doc: `a relative path`, word: "../skills", o: https,
			url: "/skills", path: "/skills", local: true,
		},
		{
			doc: `file:// URL`, word: "file:///srv/git/skills.git", o: https,
			url: "file:///srv/git/skills.git", path: "/srv/git/skills", local: true,
		},
	} {
		t.Run(tt.word, func(t *testing.T) {
			r, err := ParseRepo(tt.word, tt.o)
			if err != nil {
				t.Fatalf("%s: %v", tt.doc, err)
			}
			if r.URL != tt.url || r.Host != tt.host || r.Path != tt.path || r.Local != tt.local {
				t.Errorf("%s: got %+v", tt.doc, r)
			}
			if r.Input != tt.word {
				t.Errorf("Input = %q", r.Input)
			}
		})
	}
}

func TestParseRepoRejects(t *testing.T) {
	o := Options{DefaultHost: "github.com", Home: "/home/u", Cwd: "/work"}
	for _, word := range []string{
		"", "skills", "you/", "you",
		"someone/agent-skills/pdf", // a skill, for skl learn
		"you//skills",              // // comes with skl learn
		"gitlab.com/team//skills",  // same
		"you/skills#v1",            // # comes with skl learn
		"https://x.com/you/skills#v1",
		"https://x.com/team/skills//pdf",
		"someone/@backend",    // @set comes with skl learn
		"-upload-pack=evil/x", // an option, not a repo
		"-oProxyCommand=evil:x",
		"ext::sh -c evil", // git's remote helpers
		"ftp://x.com/you/skills",
		"https:///you/skills",
		"https://x.com",
		"git@host:",
		":path",
		"you/sk ills",
		"you/../skills",
	} {
		if r, err := ParseRepo(word, o); !errors.Is(err, ErrNotRepo) {
			t.Errorf("ParseRepo(%q) = %+v, %v; want ErrNotRepo", word, r, err)
		}
	}
	if _, err := ParseRepo("~/x", Options{}); !errors.Is(err, ErrNotRepo) {
		t.Errorf("~ with no HOME: %v", err)
	}
}

// repos.md "How a reference is stored": "two references to the same repo
// match with or without" a trailing .git. Protocol doesn't matter either: a
// vault cloned over HTTPS is the same repo when init runs with protocol = "ssh".
func TestSame(t *testing.T) {
	https := Options{DefaultHost: "github.com", Home: "/home/u", Cwd: "/work"}
	ssh := https
	ssh.SSH = true
	parse := func(w string, o Options) Repo {
		t.Helper()
		r, err := ParseRepo(w, o)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a := parse("you/skills", https)
	for _, r := range []Repo{
		parse("you/skills", ssh),
		parse("https://github.com/you/skills.git", https),
		parse("https://token@github.com/you/skills", https),
		parse("https://GitHub.com/you/skills/", https),
		parse("git@github.com:you/skills.git", https),
		parse("ssh://git@github.com/you/skills", https),
		parse("github.com/you/skills", https),
	} {
		if !a.Same(r) {
			t.Errorf("%q and %q should be the same repo", a.Input, r.Input)
		}
	}
	for _, r := range []Repo{
		parse("You/skills", https),
		parse("you/other", https),
		parse("gitlab.com/you/skills", https),
		parse("/you/skills", https),
	} {
		if a.Same(r) {
			t.Errorf("%q and %q should differ", a.Input, r.Input)
		}
	}
	if !parse("~/code/skills.git", https).Same(parse("file:///home/u/code/skills", https)) {
		t.Error("a local repo with and without .git, as a path and a file URL")
	}
}

func TestShortAndOwner(t *testing.T) {
	o := Options{DefaultHost: "github.com", Home: "/home/u", Cwd: "/work"}
	for _, tt := range []struct{ word, short, owner string }{
		{"you/skills", "you/skills", "you"},
		{"https://user:secret@github.com/you/skills.git", "you/skills", "you"},
		{"gitlab.com/team/sub/skills", "gitlab.com/team/sub/skills", "team"},
		{"git@git.example.com:team/skills.git", "git.example.com/team/skills", "team"},
		{"~/code/skills.git", "/home/u/code/skills.git", "skills"},
	} {
		r, err := ParseRepo(tt.word, o)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Short(o.DefaultHost); got != tt.short {
			t.Errorf("Short(%q) = %q, want %q", tt.word, got, tt.short)
		}
		if got := r.Owner(); got != tt.owner {
			t.Errorf("Owner(%q) = %q, want %q", tt.word, got, tt.owner)
		}
	}
}
