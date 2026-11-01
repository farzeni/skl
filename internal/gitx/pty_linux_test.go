package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// openPty opens a new pseudo-terminal and returns its two ends.
func openPty(t *testing.T) (master, tty *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty here: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		t.Fatal(e)
	}
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		t.Fatal(e)
	}
	tty, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return master, tty
}

// fakeGPG is a gpg.program that asks for a passphrase on /dev/tty, as
// pinentry-tty does, keeps the answer in dir/answer and signs. It leaves its
// pid in dir/pid.
func fakeGPG(t *testing.T, dir string) string {
	t.Helper()
	return writeScript(t, "gpg", "echo $$ > '"+dir+"/pid'\ncat >/dev/null\n"+
		"printf 'passphrase: ' >/dev/tty\nIFS= read -r answer </dev/tty || exit 1\n"+
		"echo \"$answer\" > '"+dir+"/answer'\n"+
		"echo '[GNUPG:] SIG_CREATED D 1 8 00 0 0 0' >&2\n"+
		"printf -- '-----BEGIN PGP SIGNATURE-----\\n\\nfake\\n-----END PGP SIGNATURE-----\\n'\n")
}

// ptyChild is skl under a real terminal: the session leader, in the terminal's
// foreground group, with ctx canceled by SIGINT as in cli.Run. It runs git with
// args in GITX_PTY_DIR and reports the outcome on stdout.
func TestPtyChild(t *testing.T) {
	timeout, ok := os.LookupEnv("GITX_PTY_TIMEOUT")
	if !ok {
		t.Skip("runs under a pty, started by the other tests")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if os.Getenv("GITX_PTY_NOCANCEL") != "" {
		ctx = context.WithoutCancel(ctx) //nolint:fatcontext // a step that must not stop
	}
	g := New(nil)
	g.Timeout, _ = time.ParseDuration(timeout)
	args := strings.Split(os.Getenv("GITX_PTY_ARGS"), "\x1f")
	_, err := g.Run(ctx, os.Getenv("GITX_PTY_DIR"), args...)
	fmt.Printf("\nresult: unreachable=%v canceled=%v err=%v\n", errors.Is(err, ErrUnreachable), errors.Is(err, context.Canceled), err)
}

// underPty runs git with args under a new terminal and returns the child's
// report. onPrompt, if set, runs once when "passphrase: " appears on the
// terminal, with the terminal to answer on.
func underPty(t *testing.T, dir string, timeout time.Duration, onPrompt func(term io.Writer), args ...string) string {
	t.Helper()
	master, tty := openPty(t)

	// Closed once onPrompt has run.
	answered := make(chan struct{})
	go func() {
		var screen bytes.Buffer
		buf := make([]byte, 256)
		pending := onPrompt != nil
		for {
			n, err := master.Read(buf)
			screen.Write(buf[:n])
			if err != nil {
				return
			}
			if pending && strings.Contains(screen.String(), "passphrase: ") {
				pending = false
				onPrompt(master)
				close(answered)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPtyChild$", "-test.v")
	cmd.Env = append(os.Environ(),
		"GITX_PTY_TIMEOUT="+timeout.String(),
		"GITX_PTY_DIR="+dir,
		"GITX_PTY_ARGS="+strings.Join(args, "\x1f"))
	cmd.Stdin = tty
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Run(); err != nil {
		t.Fatalf("the child under the pty: %v\n%s", err, out.String())
	}
	if onPrompt != nil {
		select {
		case <-answered:
		default:
			t.Fatalf("no prompt on the terminal\n%s", out.String())
		}
	}
	_, report, ok := strings.Cut(out.String(), "result: ")
	if !ok {
		t.Fatalf("no result from the child:\n%s", out.String())
	}
	report, _, _ = strings.Cut(report, "\n")
	return report
}

// docs/design/architecture.md "Fetching from remotes": "A git that may sign or
// run a hook, such as `git commit`, keeps your terminal, so gpg can ask for a
// passphrase, and it has no timeout".
func TestCommitSignsOnTerminal(t *testing.T) {
	isolate(t)
	g := New(nil)
	dir := newRepo(t, g)
	state := t.TempDir()
	if _, err := g.Run(t.Context(), dir, "config", "gpg.program", fakeGPG(t, state)); err != nil {
		t.Fatal(err)
	}
	// Slower than the timeout: an answer that takes a while still lands.
	report := underPty(t, dir, 100*time.Millisecond, func(term io.Writer) {
		time.Sleep(500 * time.Millisecond)
		_, _ = io.WriteString(term, "secret\n")
	}, "commit", "-S", "--allow-empty", "-m", "signed")
	if !strings.HasSuffix(report, "err=<nil>") {
		t.Fatalf("commit -S under a terminal: %s", report)
	}
	if b, err := os.ReadFile(filepath.Join(state, "answer")); err != nil || string(b) != "secret\n" {
		t.Errorf("gpg's answer = %q, %v", b, err)
	}
	out, err := g.Run(t.Context(), dir, "cat-file", "commit", "HEAD")
	if err != nil || !strings.Contains(out, "gpgsig ") {
		t.Errorf("HEAD = %q, %v, want a signed commit", out, err)
	}
}

// docs/design/architecture.md "Fetching from remotes": "Ctrl-C reaches it
// through the terminal": gpg stops, git removes its lock, and skl gets
// context.Canceled, under context.WithoutCancel too, so that step must undo.
func TestCommitCtrlCOnTerminal(t *testing.T) {
	t.Run("cancelable", testCommitCtrlC)
	t.Run("WithoutCancel", func(t *testing.T) {
		t.Setenv("GITX_PTY_NOCANCEL", "1")
		testCommitCtrlC(t)
	})
}

func testCommitCtrlC(t *testing.T) {
	isolate(t)
	g := New(nil)
	dir := newRepo(t, g)
	state := t.TempDir()
	if _, err := g.Run(t.Context(), dir, "config", "gpg.program", fakeGPG(t, state)); err != nil {
		t.Fatal(err)
	}
	report := underPty(t, dir, 100*time.Millisecond, func(term io.Writer) {
		_, _ = term.Write([]byte{3}) // ^C, as the keyboard sends it
	}, "commit", "-S", "--allow-empty", "-m", "signed")
	if !strings.Contains(report, "canceled=true") {
		t.Fatalf("Ctrl-C at the pinentry: %s", report)
	}
	b, err := os.ReadFile(filepath.Join(state, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	deadline := time.Now().Add(5 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatalf("gpg %d outlived Ctrl-C", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "index.lock")); err == nil {
		t.Error("git left .git/index.lock behind")
	}
}

// The calibration: a command that may contact a remote is in a background
// group, so a child reading the terminal stops, and the timeout ends it.
func TestNetworkTTYReaderTimesOut(t *testing.T) {
	isolate(t)
	answer := filepath.Join(t.TempDir(), "answer")
	script := "!IFS= read -r a </dev/tty; echo \"$a\" > '" + answer + "'"
	report := underPty(t, "", time.Second, nil, "-c", "alias.ttyread="+script, "ttyread")
	if !strings.Contains(report, "unreachable=true") || !strings.Contains(report, "timed out after 1s") {
		t.Fatalf("a tty reader in a network command: %s", report)
	}
	if _, err := os.Stat(answer); err == nil {
		t.Error("the child read the terminal from a background group")
	}
}

// quietRepo is a repo whose reference-transaction hook prints "passphrase: " on
// the terminal, as underPty waits for, then sleeps, so update-ref is running
// when ^C comes.
func quietRepo(t *testing.T) (dir string, args []string) {
	t.Helper()
	g := New(nil)
	dir = newRepo(t, g)
	if _, err := g.Run(t.Context(), dir, "commit", "--quiet", "--allow-empty", "-m", "first"); err != nil {
		t.Fatal(err)
	}
	hook := writeScript(t, "reference-transaction", "cat >/dev/null\n[ \"$1\" = prepared ] || exit 0\nprintf 'passphrase: ' >/dev/tty\nsleep 1\n")
	return dir, []string{"-c", "core.hooksPath=" + filepath.Dir(hook), "update-ref", "refs/heads/x", "HEAD"}
}

// docs/design/architecture.md "Fetching from remotes": "Any other local git, such
// as `git status`, has no timeout and doesn't share your terminal: Ctrl-C stops it
// only if the step it's in can stop." This one can't, so ^C doesn't cut it, and it
// takes longer than the timeout.
func TestQuietIgnoresTerminalCtrlC(t *testing.T) {
	isolate(t)
	t.Setenv("GITX_PTY_NOCANCEL", "1")
	dir, args := quietRepo(t)
	report := underPty(t, dir, 100*time.Millisecond, func(term io.Writer) {
		_, _ = term.Write([]byte{3})
	}, args...)
	if !strings.HasSuffix(report, "err=<nil>") {
		t.Fatalf("^C during update-ref under WithoutCancel: %s", report)
	}
	if _, err := New(nil).Run(t.Context(), dir, "rev-parse", "--verify", "refs/heads/x"); err != nil {
		t.Errorf("the ref didn't land: %v", err)
	}
}

// The same step when it can stop: ^C reaches skl, skl interrupts git, and git
// removes its lock.
func TestQuietStopsThroughContext(t *testing.T) {
	isolate(t)
	dir, args := quietRepo(t)
	report := underPty(t, dir, 100*time.Millisecond, func(term io.Writer) {
		_, _ = term.Write([]byte{3})
	}, args...)
	if !strings.Contains(report, "canceled=true") {
		t.Fatalf("^C during update-ref: %s", report)
	}
	if _, err := New(nil).Run(t.Context(), dir, "rev-parse", "--verify", "--quiet", "refs/heads/x"); err == nil {
		t.Error("the ref landed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "refs", "heads", "x.lock")); err == nil {
		t.Error("git left x.lock behind")
	}
}

// A hook that reads the terminal in a background group stops. Stopping it through
// ctx must still end it, so git removes its lock.
func TestQuietStoppedHookLeavesNoLock(t *testing.T) {
	isolate(t)
	g := New(nil)
	dir := newRepo(t, g)
	if _, err := g.Run(t.Context(), dir, "commit", "--quiet", "--allow-empty", "-m", "first"); err != nil {
		t.Fatal(err)
	}
	hook := writeScript(t, "reference-transaction", "cat >/dev/null\n[ \"$1\" = prepared ] || exit 0\nprintf 'passphrase: ' >/dev/tty\nread -r a </dev/tty\n")
	report := underPty(t, dir, 100*time.Millisecond, func(term io.Writer) {
		time.Sleep(200 * time.Millisecond) // the hook is stopped by now
		_, _ = term.Write([]byte{3})
	}, "-c", "core.hooksPath="+filepath.Dir(hook), "update-ref", "refs/heads/x", "HEAD")
	if !strings.Contains(report, "canceled=true") {
		t.Fatalf("^C with the hook stopped: %s", report)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "refs", "heads", "x.lock")); err == nil {
		t.Error("git left x.lock behind")
	}
}
