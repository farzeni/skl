//go:build linux || darwin

package spec

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/charmbracelet/x/ansi"
)

// keys are the names ptyrun takes for what a user types at a question.
var keys = map[string]string{
	"enter":  "\r",
	"right":  "\x1b[C",
	"left":   "\x1b[D",
	"tab":    "\t",
	"ctrl-c": "\x03",
}

// ptyrun is "ptyrun <keys> <program> [args…]", a program of the test binary
// that a script runs with exec or exitcode: it runs the program with its
// standard input and standard error on a pseudo-terminal 80 columns wide, as
// a user's terminal is, and its standard output on ptyrun's. Once a question
// ("? ") shows, it types keys, comma-separated names such as right,enter or
// ctrl-c ("-" for none). What the terminal showed goes to ptyrun's standard
// error, without escape sequences or carriage returns, so a script matches it
// with regexps; ptyrun exits with the program's exit code.
func ptyrun() int {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: ptyrun <keys> <program> [args…]")
		return 2
	}
	var typed []string
	if os.Args[1] != "-" {
		for k := range strings.SplitSeq(os.Args[1], ",") {
			s, ok := keys[k]
			if !ok {
				fmt.Fprintf(os.Stderr, "ptyrun: unknown key %q\n", k)
				return 2
			}
			typed = append(typed, s)
		}
	}
	master, tty, err := openPTY()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyrun:", err)
		return 2
	}
	defer func() { _ = master.Close() }()
	ws := struct{ rows, cols, x, y uint16 }{24, 80, 0, 0}
	if err := ioctl(tty.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))); err != nil {
		fmt.Fprintln(os.Stderr, "ptyrun: window size:", err)
		return 2
	}
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, os.Stdout, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "ptyrun:", err)
		return 2
	}
	_ = tty.Close()

	var mu sync.Mutex
	var shown bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			mu.Lock()
			shown.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return // EIO once the program and its terminal are gone
			}
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	asked := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(ansi.Strip(shown.String()), "? ")
	}
	var werr error
	waiting := true
	for deadline := time.Now().Add(30 * time.Second); waiting && len(typed) > 0 && time.Now().Before(deadline); {
		select {
		case werr = <-exited:
			waiting = false
			typed = nil
		case <-time.After(20 * time.Millisecond):
			if asked() {
				time.Sleep(200 * time.Millisecond) // let the question finish drawing
				for _, k := range typed {
					_, _ = master.WriteString(k)
					time.Sleep(150 * time.Millisecond)
				}
				typed = nil
			}
		}
	}
	if waiting {
		werr = <-exited
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	mu.Lock()
	out := strings.ReplaceAll(ansi.Strip(shown.String()), "\r", "")
	mu.Unlock()
	fmt.Fprint(os.Stderr, out)

	if exit, ok := errors.AsType[*exec.ExitError](werr); ok {
		if st, ok := exit.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			return 128 + int(st.Signal())
		}
		return exit.ExitCode()
	}
	if werr != nil {
		fmt.Fprintln(os.Stderr, "ptyrun:", werr)
		return 2
	}
	return 0
}

func ioctl(fd, req, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); errno != 0 {
		return errno
	}
	return nil
}
