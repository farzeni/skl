package spec

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// openPTY opens a pseudo-terminal: its master, and the terminal itself.
func openPTY() (master, tty *os.File, err error) {
	fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	master = os.NewFile(uintptr(fd), "/dev/ptmx")
	name := make([]byte, 128) // TIOCPTYGNAME's buffer
	for _, step := range []struct{ req, arg uintptr }{
		{syscall.TIOCPTYGRANT, 0},
		{syscall.TIOCPTYUNLK, 0},
		{syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))},
	} {
		if err := ioctl(master.Fd(), step.req, step.arg); err != nil {
			_ = master.Close()
			return nil, nil, err
		}
	}
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	tty, err = os.OpenFile(string(name), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, tty, nil
}
