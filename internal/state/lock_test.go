package state

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/config"
)

// TestMain lets the test binary act as a second skl process: with
// SKL_STATE_HELPER set it runs that helper instead of the tests.
func TestMain(m *testing.M) {
	if h := os.Getenv("SKL_STATE_HELPER"); h != "" {
		os.Exit(helper(h))
	}
	os.Exit(m.Run())
}

// helper is what the second process does.
func helper(name string) int {
	ctx := context.Background()
	p, err := config.NewPaths(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	s := New(p, nil)
	switch name {
	case "hold": // take the lock, say so, hold it until stdin closes
		l, err := s.Lock(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		fmt.Println("locked")
		_, _ = io.Copy(io.Discard, os.Stdin)
		if err := l.Release(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		return 0
	case "hold-with-child": // the same, and start a child that outlives us
		if _, err := s.Lock(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		child := exec.Command("sleep", "30")
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		fmt.Println("locked", child.Process.Pid)
		_, _ = io.Copy(io.Discard, os.Stdin)
		return 0
	case "try": // say whether we got the lock, and from whom we didn't
		l, err := s.Lock(ctx)
		if err != nil {
			var held *LockedError
			if !errors.As(err, &held) {
				fmt.Fprintln(os.Stderr, err)
				return 3
			}
			fmt.Println("refused", held.PID)
			return 1
		}
		fmt.Println("locked")
		if err := l.Release(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 3
		}
		return 0
	case "save-loop": // write state.json until killed
		for i := 0; ; i++ {
			tag := string(rune('a' + i%2))
			if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: bigScope(tag)}}); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 3
			}
		}
	}
	fmt.Fprintln(os.Stderr, "unknown helper", name)
	return 3
}

func helperCmd(t *testing.T, p config.Paths, name string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"SKL_STATE_HELPER="+name, "SKL_HOME="+p.Home,
		"GORACE=atexit_sleep_ms=0") // under -race a process otherwise sleeps 1 s as it exits
	return cmd
}

// startHelper starts a helper that runs until killed.
func startHelper(t *testing.T, p config.Paths, name string) *exec.Cmd {
	t.Helper()
	cmd := helperCmd(t, p, name)
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}

// startHolder starts a helper that holds the lock and returns once it does,
// with the words it printed after "locked".
func startHolder(t *testing.T, p config.Paths, name string) (*exec.Cmd, io.WriteCloser, []string) {
	t.Helper()
	cmd := helperCmd(t, p, name)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("holder: %v, stderr: %s", err, stderr.String())
	}
	words := strings.Fields(line)
	if len(words) == 0 || words[0] != "locked" {
		t.Fatalf("holder said %q", line)
	}
	return cmd, stdin, words[1:]
}

// runTry runs a one-shot skl that tries to take the lock.
func runTry(t *testing.T, p config.Paths) (stdout, stderr string, code int) {
	t.Helper()
	cmd := helperCmd(t, p, "try")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// docs/reference/commands.md "Errors": "One skl runs at a time on a machine".
// docs/design/architecture.md "Safety": the lock "records its pid". The refusal
// is a *LockedError naming the holder; the ✗ line it becomes is for ui and cli.
func TestLockContentionBetweenProcesses(t *testing.T) {
	_, p := testStore(t)
	holder, stdin, _ := startHolder(t, p, "hold")
	pid := holder.Process.Pid

	data, err := os.ReadFile(p.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(pid) {
		t.Errorf("lock file holds %q, want the pid %d", got, pid)
	}

	stdout, stderr, code := runTry(t, p)
	if code != 1 {
		t.Fatalf("second skl: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if want := fmt.Sprintf("refused %d\n", pid); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}

	// Once the first one is done, the next one runs.
	_ = stdin.Close()
	if err := holder.Wait(); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runTry(t, p)
	if code != 0 || stdout != "locked\n" {
		t.Errorf("after release: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// Card #61: "A lock whose pid is dead is taken over." A skl killed with
// SIGKILL can't clean up: the file still holds its pid.
func TestStaleLockFromKilledProcess(t *testing.T) {
	s, p := testStore(t)
	holder, _, _ := startHolder(t, p, "hold")
	pid := holder.Process.Pid
	if err := holder.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()

	data, err := os.ReadFile(p.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(pid) {
		t.Fatalf("killed holder left %q, want its pid %d", got, pid)
	}

	l, err := s.Lock(context.Background())
	if err != nil {
		t.Fatalf("stale lock wasn't taken over: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })
	data, err = os.ReadFile(p.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock file holds %q after takeover, want %d", got, os.Getpid())
	}
}

// A lock file left over from anywhere doesn't stop skl, whatever it holds:
// no process owns it. That includes the pid of a live process, which is what
// a dead skl's pid becomes when the system reuses it.
func TestLeftoverLockFileIsTakenOver(t *testing.T) {
	for _, tt := range []struct{ name, content string }{
		{"a dead pid", "4194999\n"},
		{"a live pid", strconv.Itoa(os.Getppid()) + "\n"},
		{"empty", ""},
		{"garbage", "not a pid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, p := testStore(t)
			if err := os.MkdirAll(p.Home, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.Lock, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			l, err := s.Lock(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := l.Release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// skl starts git and the engine while it holds the lock. If skl dies, a
// child that lives on must not keep the lock.
func TestChildDoesNotInheritLock(t *testing.T) {
	s, p := testStore(t)
	holder, _, words := startHolder(t, p, "hold-with-child")
	child, err := strconv.Atoi(words[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })
	if err := holder.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()
	if err := syscall.Kill(child, 0); err != nil {
		t.Fatalf("the child should still be running: %v", err)
	}
	l, err := s.Lock(context.Background())
	if err != nil {
		t.Fatalf("a child kept the lock: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

// docs/design/architecture.md "Safety": "status, history, open and help
// don't take it." So what they read has to work while another skl holds it.
func TestReadingNeedsNoLock(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	if err := s.save(ctx, &State{Scopes: map[ScopeKey]Scope{User: {Selectors: []string{"@base"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addGeneration(ctx, Generation{Time: genTime, Command: "use @base"}); err != nil {
		t.Fatal(err)
	}
	startHolder(t, p, "hold")

	st, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Scopes[User].Selectors; len(got) != 1 || got[0] != "@base" {
		t.Errorf("Load while locked: %v", got)
	}
	gens, err := s.Generations(ctx)
	if err != nil || len(gens) != 1 {
		t.Errorf("Generations while locked: %v, %v", gens, err)
	}
}

// Many skls at once: exactly one gets the lock each time, including when
// they race for a stale one. Every process either holds it or names a holder.
func TestLockRace(t *testing.T) {
	_, p := testStore(t)
	if err := os.MkdirAll(p.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	for round := range 10 {
		if err := os.WriteFile(p.Lock, []byte("4194999\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		const n = 8
		type result struct {
			cmd    *exec.Cmd
			stdin  io.WriteCloser
			stdout io.ReadCloser
			errb   *bytes.Buffer
		}
		results := make([]result, n)
		for i := range results {
			cmd := helperCmd(t, p, "hold")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			errb := &bytes.Buffer{}
			cmd.Stderr = errb
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			results[i] = result{cmd, stdin, stdout, errb}
		}
		var winners, refused []int
		for _, r := range results {
			line, _ := bufio.NewReader(r.stdout).ReadString('\n')
			if strings.HasPrefix(line, "locked") {
				winners = append(winners, r.cmd.Process.Pid)
			} else {
				refused = append(refused, r.cmd.Process.Pid)
			}
		}
		if len(winners) != 1 {
			t.Errorf("round %d: %d winners: %v", round, len(winners), winners)
		}
		for _, r := range results {
			_ = r.stdin.Close()
			_ = r.cmd.Wait()
			if pid := r.cmd.Process.Pid; slices.Contains(refused, pid) {
				want := fmt.Sprintf("another skl is running (pid %d)\n", winners[0])
				if r.errb.String() != want {
					t.Errorf("round %d: refused with %q, want %q", round, r.errb.String(), want)
				}
			}
		}
	}
}

// flockFile holds the lock file the way a holder does, without recording a pid.
func flockFile(t *testing.T, p config.Paths) *os.File {
	t.Helper()
	if err := os.MkdirAll(p.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p.Lock, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	return f
}

// In one process too: each Lock call is a holder of its own.
func TestLockInProcess(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()

	l, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Lock(ctx)
	var held *LockedError
	if !errors.As(err, &held) || held.PID != os.Getpid() {
		t.Fatalf("second Lock: %#v", err)
	}
	if got, want := err.Error(), fmt.Sprintf("another skl is running (pid %d)", os.Getpid()); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}

	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Errorf("second Release: %v", err)
	}
	data, err := os.ReadFile(p.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("released lock still names a holder: %q", data)
	}

	l, err = s.Lock(ctx)
	if err != nil {
		t.Fatalf("Lock after Release: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

// Something holds the file but never names a pid: a holder that is stuck, or
// not a skl. The refusal waits a moment for a pid, then says what it knows.
func TestLockedWithoutARecordedPid(t *testing.T) {
	s, p := testStore(t)
	flockFile(t, p)
	start := time.Now()
	_, err := s.Lock(context.Background())
	var held *LockedError
	if !errors.As(err, &held) || held.PID != 0 {
		t.Fatalf("Lock: %#v", err)
	}
	if got, want := err.Error(), "another skl is running"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("waited %v", d)
	}
}

// The window between another skl taking the file and writing its pid: the
// refusal waits for the pid instead of printing an empty or stale one.
func TestLockWaitsForThePid(t *testing.T) {
	s, p := testStore(t)
	f := flockFile(t, p)
	// A dead skl's pid is still in the file when the new holder has it.
	if _, err := f.WriteString("4194999\n"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = f.Truncate(0)
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}()
	_, err := s.Lock(context.Background())
	var held *LockedError
	if !errors.As(err, &held) || held.PID != os.Getpid() {
		t.Fatalf("Lock: %#v", err)
	}
}

func TestLockCanceledWhileWaiting(t *testing.T) {
	s, p := testStore(t)
	flockFile(t, p)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := s.Lock(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Lock: %v", err)
	}
}

// ~/.skl may not exist yet: skl init is the first command to take the lock.
func TestLockCreatesTheFolder(t *testing.T) {
	s, p := testStore(t)
	l, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	if _, err := os.Stat(p.Lock); err != nil {
		t.Error(err)
	}
}

// Changing state goes through the lock: once released, it can't.
func TestReleasedLockChangesNothing(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	l, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Save(ctx, &State{}); err != nil {
		t.Errorf("Save while held: %v", err)
	}
	g, err := l.AddGeneration(ctx, Generation{Time: genTime})
	if err != nil || g.N != 1 {
		t.Errorf("AddGeneration while held: %+v, %v", g, err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(ctx, &State{}); !errors.Is(err, ErrReleased) {
		t.Errorf("Save after Release: %v", err)
	}
	if _, err := l.AddGeneration(ctx, Generation{Time: genTime}); !errors.Is(err, ErrReleased) {
		t.Errorf("AddGeneration after Release: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Generations, "2.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("generation 2: %v", err)
	}
}
