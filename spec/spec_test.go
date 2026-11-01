// Package spec checks the docs' statements against the skl binary. Each
// script in testdata/ belongs to one doc page and runs skl as a separate
// process, through the real main path, in its own HOME with local bare
// repos standing in for GitHub.
package spec

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"skl": runSkl,
	})
}

// TestSpec runs testdata/<page>[-<section>].txtar.
func TestSpec(t *testing.T) {
	testscript.Run(t, params("testdata"))
}

// TestHarness checks the harness itself: the fake github.com and the
// helper commands.
func TestHarness(t *testing.T) {
	testscript.Run(t, params("testdata/harness"))
}

func params(dir string) testscript.Params {
	return testscript.Params{
		Dir:                 dir,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		UpdateScripts:       os.Getenv("UPDATE") == "1",
		Setup:               setup,
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"remote":   cmdRemote,
			"exitcode": cmdExitcode,
		},
	}
}

// setup gives each script HOME=$WORK/home and $REMOTES, and points
// https://github.com/ at $REMOTES/github.com/ through git's own url.insteadOf,
// so skl's real URLs reach local bare repos with no test hook in skl.
func setup(e *testscript.Env) error {
	home := filepath.Join(e.WorkDir, "home")
	remotes := filepath.Join(e.WorkDir, "remotes")
	for _, d := range []string{home, remotes} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	e.Setenv("HOME", home)
	e.Setenv("REMOTES", remotes)
	e.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	e.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	e.Setenv("COLUMNS", "80")
	gitconfig := "[user]\n\tname = Test\n\temail = test@example.com\n" +
		"[init]\n\tdefaultBranch = main\n" +
		"[uploadpack]\n\tallowFilter = true\n" +
		"[url \"file://" + filepath.ToSlash(remotes) + "/github.com/\"]\n\tinsteadOf = https://github.com/\n"
	return os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o644)
}

// cmdRemote is "remote <owner/repo> [dir]": it creates the bare repo that
// https://github.com/<owner>/<repo> reaches, with one commit holding dir's
// files if dir is given.
func cmdRemote(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 1 || len(args) > 2 || strings.Count(args[0], "/") != 1 {
		ts.Fatalf("usage: remote <owner/repo> [dir]")
	}
	bare := filepath.Join(ts.Getenv("REMOTES"), "github.com", args[0]+".git")
	ts.Check(ts.Exec("git", "init", "--quiet", "--bare", bare))
	if len(args) == 1 {
		return
	}
	git := []string{"--git-dir", bare, "--work-tree", ts.MkAbs(args[1])}
	ts.Check(ts.Exec("git", append(git, "add", "--all")...))
	ts.Check(ts.Exec("git", append(git, "commit", "--quiet", "--message", "seed")...))
}

// cmdExitcode is "exitcode <n> <program> [args…]": it runs the program like
// exec, keeping its stdout and stderr for the next commands, and fails
// unless it exits with code n.
func cmdExitcode(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exitcode <n> <program> [args…]")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			ts.Fatalf("%s: %v", args[1], err)
		}
		got = exit.ExitCode()
	}
	if got != want {
		ts.Fatalf("%s exited with code %d, want %d", args[1], got, want)
	}
}
