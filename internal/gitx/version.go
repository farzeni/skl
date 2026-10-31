package gitx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/farzeni/skl/internal/errs"
)

// MinVersion is the oldest git skl runs with: save-back and sync need
// git merge-tree --write-tree, from git 2.38 (docs/guide/getting-started.md
// "Install").
var MinVersion = Version{2, 38}

// Version is a git release, major and minor.
type Version struct{ Major, Minor int }

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// Less reports whether v is older than w.
func (v Version) Less(w Version) bool {
	return v.Major < w.Major || v.Major == w.Major && v.Minor < w.Minor
}

// ErrOldGit is a git older than MinVersion, or no git at all.
var ErrOldGit = errors.New("git too old")

// VersionError is the git that was found when it is too old or missing.
type VersionError struct {
	Found string // what "git version" printed after "git version ", or "" if there is no git
}

func (e *VersionError) Error() string {
	if e.Found == "" {
		return "skl needs git"
	}
	return fmt.Sprintf("skl needs git %s or later (found %s)", MinVersion, e.Found)
}

// Is makes every VersionError match ErrOldGit.
func (e *VersionError) Is(target error) bool { return target == ErrOldGit }

// ParseVersion reads the output of "git version", such as "git version
// 2.39.5 (Apple Git-154)" or "git version 2.45.1.windows.1". found is the
// text after "git version ", for the error message.
func ParseVersion(out string) (v Version, found string, err error) {
	found = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "git version "))
	num, _, _ := strings.Cut(found, " ")
	parts := strings.SplitN(num, ".", 3)
	if len(parts) < 2 {
		return Version{}, found, fmt.Errorf("can't read git's version from %q", out)
	}
	if v.Major, err = strconv.Atoi(parts[0]); err == nil {
		v.Minor, err = strconv.Atoi(parts[1])
	}
	if err != nil {
		return Version{}, found, fmt.Errorf("can't read git's version from %q", out)
	}
	return v, found, nil
}

// checkVersion runs "git version" on the first call and remembers the
// answer. A canceled check isn't remembered, so the next call checks again.
// D3 (user, 2026-10-25): "✗ skl needs git 2.38 or later (found X) · update
// git".
func (g *Git) checkVersion(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.checked {
		return g.verErr
	}
	out, err := g.exec(ctx, "", []string{"version"}, baseEnv())
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		g.checked, g.verErr = true, err
		return err
	}
	v, found, err := ParseVersion(out)
	switch {
	case err != nil:
		g.verErr = err
	case v.Less(MinVersion):
		g.verErr = errs.Hint(&VersionError{Found: found}, "update git")
	}
	g.checked = true
	return g.verErr
}

// notFound is the error for a git that isn't on PATH: "✗ skl needs git ·
// install git 2.38 or later" (docs/guide/getting-started.md "Install").
func (g *Git) notFound() error {
	return errs.Hint(&VersionError{}, "install git "+MinVersion.String()+" or later")
}
