package state

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// pidWait is how long a refusal waits for the holder to record its pid,
	// which it does right after taking the file.
	pidWait = time.Second
	pidPoll = 10 * time.Millisecond
)

// LockedError means another skl holds the lock. PID is the pid it recorded,
// or 0 if it recorded none.
type LockedError struct{ PID int }

func (e *LockedError) Error() string {
	if e.PID == 0 {
		return "another skl is running"
	}
	return fmt.Sprintf("another skl is running (pid %d)", e.PID)
}

// Lock is the held lock: one skl changes things at a time on a machine.
type Lock struct {
	f *os.File
	s *Store
}

// Lock takes the lock without waiting for its holder, or returns a
// *LockedError. Commands that change something hold it until they're done.
//
// It is an flock on the lock file, which records the holder's pid. The kernel
// drops an flock when its holder dies however it dies, so a lock left by a
// killed skl is free, even if the system has given its pid to another process,
// and two skls can't both take over a stale lock. The file is opened
// close-on-exec, so git or an engine that outlives skl doesn't keep it.
//
// Taking it also removes the temp files a killed skl left.
func (s *Store) Lock(ctx context.Context) (*Lock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.mkdirAll(filepath.Dir(s.lockPath)); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(s.lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	deadline := time.Now().Add(pidWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", s.lockPath, err)
		}
		// Someone holds it. A pid that's missing or dead means the holder has
		// just taken the file and hasn't written its own pid over it, or is
		// just releasing it: look again before saying who it is.
		pid := recordedPID(f)
		if (pid != 0 && alive(pid)) || time.Now().After(deadline) {
			_ = f.Close()
			return nil, &LockedError{PID: pid}
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(pidPoll):
		}
	}
	if err := recordPID(f); err != nil {
		_ = f.Close() // closing drops the flock
		return nil, fmt.Errorf("lock %s: %w", s.lockPath, err)
	}
	s.removeLeftovers()
	return &Lock{f: f, s: s}, nil
}

// removeLeftovers removes the temp files writeAtomic left when a skl was killed
// in the middle of a write. Only the lock's holder writes, and that is us now,
// so no live skl owns one. A file from after this skl's start is left alone all
// the same, for the next skl to judge. Nothing depends on this working, so a
// failure doesn't stop the command.
func (s *Store) removeLeftovers() {
	stateFile := filepath.Base(s.statePath)
	s.removeTemps(filepath.Dir(s.statePath), func(target string) bool { return target == stateFile })
	s.removeTemps(s.generations, func(target string) bool {
		_, ok := generationNumber(target)
		return ok
	})
}

func (s *Store) removeTemps(dir string, isTarget func(string) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Debug("can't look for temp files", "dir", dir, "err", err)
		}
		return
	}
	for _, e := range entries {
		target, ok := tempTarget(e.Name())
		if !ok || !isTarget(target) || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(s.started) {
			continue
		}
		if err := os.Remove(path); err != nil {
			s.log.Debug("can't remove temp file", "path", path, "err", err)
			continue
		}
		s.log.Debug("removed temp file", "path", path)
	}
}

// tempTarget returns the file a temp file of writeAtomic was going to replace:
// "state.json" for ".state.json.123.tmp". os.CreateTemp puts digits in the
// middle. Any other name isn't one of skl's temp files.
func tempTarget(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, ".")
	if !ok {
		return "", false
	}
	rest, ok = strings.CutSuffix(rest, ".tmp")
	if !ok {
		return "", false
	}
	i := strings.LastIndexByte(rest, '.')
	if i < 0 {
		return "", false
	}
	if _, err := strconv.ParseUint(rest[i+1:], 10, 64); err != nil {
		return "", false
	}
	return rest[:i], true
}

// Release gives the lock up. Calling it again does nothing. The file stays:
// removing it could leave two skls holding two different files.
func (l *Lock) Release() error {
	if l.f == nil {
		return nil
	}
	f := l.f
	l.f = nil
	return errors.Join(f.Truncate(0), f.Close())
}

func recordPID(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return err
}

// recordedPID reads the pid in the lock file, 0 if there is none. The
// newline is written with the digits, so its absence means a partial write.
func recordedPID(f *os.File) int {
	buf := make([]byte, 32)
	n, err := f.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0
	}
	digits, ok := strings.CutSuffix(string(buf[:n]), "\n")
	if !ok {
		return 0
	}
	pid, err := strconv.Atoi(digits)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// alive says whether a process with this pid exists. EPERM means it does, as
// another user's.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
