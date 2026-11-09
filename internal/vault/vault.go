// Package vault is the git clone that holds your skills, at ~/.skl/vault:
// finding it, making it (Clone, Create), and what it holds. Only skl's own
// commits go through it; it never pushes.
package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/farzeni/skl/internal/gitx"
)

var (
	// ErrNoVault means this machine has no vault yet.
	ErrNoVault = errors.New("no vault on this machine")
	// ErrNotClone is a vault folder that holds files but isn't a git clone.
	ErrNotClone = errors.New("isn't a git repo")
	// ErrNoIdentity is a git that doesn't know who to commit as.
	ErrNoIdentity = errors.New("git needs your name and email for skl's commits")
	// ErrNoDefaultBranch is a repo with branches whose HEAD names none of them,
	// so a clone checks nothing out.
	ErrNoDefaultBranch = errors.New("has branches but no default branch")
	// ErrNoBranches is a repo with tags or other refs but no branch.
	ErrNoBranches = errors.New("has no branches")
)

// Vault is a git clone of a vault.
type Vault struct {
	git  *gitx.Git
	dir  string
	head string // a Snapshot's commit: it has no git of its own
}

// Open finds the vault at dir. It looks for dir/.git itself rather than asking
// git, which would happily answer for a repo further up, such as a home folder
// kept in git. A missing or empty dir is ErrNoVault; one that holds something
// else is ErrNotClone.
func Open(git *gitx.Git, dir string) (*Vault, error) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return &Vault{git: git, dir: dir}, nil
	}
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, ErrNoVault
	case err == nil && len(entries) == 0:
		return nil, ErrNoVault
	}
	return nil, ErrNotClone
}

// Dir is the clone's folder.
func (v *Vault) Dir() string { return v.dir }

// RemoteEmpty reports whether the repo at url has no refs at all. It changes
// nothing, so it's how a dry run tells a new vault from a clone. It fails with
// gitx.ErrUnreachable if the repo can't be reached, and with ErrNoDefaultBranch
// or ErrNoBranches if a clone would check nothing out: git lists no HEAD then.
func RemoteEmpty(ctx context.Context, git *gitx.Git, url string) (bool, error) {
	out, err := git.Run(ctx, "", "ls-remote", "--", url)
	if err != nil {
		return false, err
	}
	var refs []string
	for line := range strings.Lines(out) {
		_, name, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if name == "HEAD" {
			return false, nil
		}
		refs = append(refs, name)
	}
	if len(refs) == 0 {
		return true, nil
	}
	return false, headless(refs)
}

// headless says what's wrong with a repo that has refs but no HEAD to check out.
func headless(refs []string) error {
	for _, r := range refs {
		if strings.HasPrefix(r, "refs/heads/") || strings.HasPrefix(r, "refs/remotes/") {
			return ErrNoDefaultBranch
		}
	}
	return ErrNoBranches
}

// Clone clones url into dir, which must not exist yet.
func Clone(ctx context.Context, git *gitx.Git, url, dir string) (*Vault, error) {
	if _, err := git.Run(ctx, "", "clone", "--quiet", "--", url, dir); err != nil {
		return nil, err
	}
	return &Vault{git: git, dir: dir}, nil
}

// Born reports whether the clone has a commit checked out. A clone of an empty
// repo doesn't; neither does one of a repo whose HEAD names no branch, which is
// ErrNoDefaultBranch or ErrNoBranches, because a first commit there would start
// a new history next to the repo's own.
func (v *Vault) Born(ctx context.Context) (bool, error) {
	if _, err := v.Head(ctx); err == nil {
		return true, nil
	} else if ctx.Err() != nil {
		return false, ctx.Err()
	}
	refs, err := v.git.Run(ctx, v.dir, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return false, err
	}
	if refs = strings.TrimSpace(refs); refs != "" {
		return false, headless(strings.Split(refs, "\n"))
	}
	return false, nil
}

// Head is the commit checked out.
func (v *Vault) Head(ctx context.Context) (string, error) {
	if v.head != "" {
		return v.head, nil
	}
	out, err := v.git.Run(ctx, v.dir, "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(out), err
}

// Origin is the URL of the clone's origin as configured, "" if it has none.
func (v *Vault) Origin(ctx context.Context) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "config", "--get", "remote.origin.url")
	if err != nil {
		if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 {
			return "", nil // git config's "not set"
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Ahead counts the commits on HEAD that origin doesn't have: the changes not
// pushed.
func (v *Vault) Ahead(ctx context.Context) (int, error) {
	out, err := v.git.Run(ctx, v.dir, "rev-list", "--count", "HEAD", "--not", "--remotes=origin")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("vault: reading git rev-list: %w", err)
	}
	return n, nil
}

// Move moves the clone to dir, in one step. The caller makes sure dir is free:
// missing, or an empty folder, which goes first because os.Rename won't replace
// a folder. os.Remove can't take one that isn't empty.
func (v *Vault) Move(dir string) error {
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("vault: %w", err)
	}
	if err := os.Rename(v.dir, dir); err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	v.dir = dir
	return nil
}

// Counts is what a vault holds.
type Counts struct{ Groups, Skills, Sets int }

// Count counts the vault's groups, skills and named skill sets: "Every other
// top-level folder is a group, and every folder in a group that holds a
// SKILL.md is a skill." A folder with no skill isn't counted as a group, and a
// SKILL.md at any other depth is ignored.
func (v *Vault) Count(ctx context.Context) (Counts, error) {
	idx, err := v.Index(ctx)
	if err != nil {
		return Counts{}, err
	}
	c := Counts{Sets: len(idx.Sets())}
	for _, g := range idx.Groups() {
		if n := len(idx.Skills(g)); n > 0 {
			c.Groups++
			c.Skills += n
		}
	}
	return c, nil
}
