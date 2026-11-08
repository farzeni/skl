package vault

import (
	"context"
	"errors"
	"strings"

	"github.com/farzeni/skl/internal/gitx"
)

// Commit stages paths (git add -A -- paths) and commits only them, with message.
// With no paths it stages and commits everything. It reports false, and commits
// nothing, when the paths hold no change. On any error, Ctrl-C included (commit
// runs in gitx's terminal mode), it unstages the paths again, so the index is as
// it was.
func (v *Vault) Commit(ctx context.Context, message string, paths ...string) (bool, error) {
	spec := append([]string{"--"}, paths...)
	if len(paths) == 0 {
		spec = []string{"--", "."}
	}
	if _, err := v.git.Run(ctx, v.dir, append([]string{"add", "-A"}, spec...)...); err != nil {
		v.unstage(ctx, spec)
		return false, err
	}
	changed, err := v.staged(ctx, spec)
	if err != nil || !changed {
		v.unstage(ctx, spec)
		return false, err
	}
	args := []string{"commit", "--quiet", "--message", message}
	if len(paths) > 0 {
		args = append(args, spec...)
	}
	if _, err := v.git.Run(ctx, v.dir, args...); err != nil {
		v.unstage(ctx, spec)
		return false, err
	}
	return true, nil
}

// staged reports whether the index differs from HEAD under spec.
func (v *Vault) staged(ctx context.Context, spec []string) (bool, error) {
	_, err := v.git.Run(ctx, v.dir, append([]string{"diff-index", "--quiet", "--cached", "HEAD"}, spec...)...)
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return true, nil
	}
	return false, err
}

// unstage puts the index back to HEAD under spec. It does its best: a failure
// here leaves staged changes, which the next commit takes.
func (v *Vault) unstage(ctx context.Context, spec []string) {
	_, _ = v.git.Run(context.WithoutCancel(ctx), v.dir, append([]string{"reset", "--quiet"}, spec...)...)
}

// HandEdits lists the files changed in the vault's work tree and not committed:
// edits made in ~/.skl/vault by hand. The vault's .gitignore applies.
func (v *Vault) HandEdits(ctx context.Context) ([]string, error) {
	out, err := v.git.Run(ctx, v.dir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	var files []string
	for entry := range strings.SplitSeq(out, "\x00") {
		if len(entry) > 3 {
			files = append(files, entry[3:])
		}
	}
	return files, nil
}

// ErrMerging is a vault with an unfinished git merge: a sync or a save-back
// conflict that waits for skl sync --continue or --abort.
var ErrMerging = errors.New("a sync is in progress")

// Merging reports whether a git merge is in progress in the vault.
func (v *Vault) Merging(ctx context.Context) (bool, error) {
	_, err := v.git.Run(ctx, v.dir, "rev-parse", "--quiet", "--verify", "MERGE_HEAD")
	if err == nil {
		return true, nil
	}
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return false, nil
	}
	return false, err
}
