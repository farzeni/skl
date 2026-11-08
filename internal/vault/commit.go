package vault

import (
	"context"
	"errors"

	"github.com/farzeni/skl/internal/gitx"
)

// Commit stages paths (git add -A, so the vault's .gitignore applies) and
// commits only them, with message. With no paths it stages and commits
// everything. It reports false, and commits nothing, when the paths hold no
// change.
//
// git commit can be stopped from the terminal even under
// context.WithoutCancel (gitx's terminal mode). On any error, Ctrl-C included,
// Commit takes back what it did: a commit that went through is undone with the
// working tree kept, and the paths are unstaged, so the caller only has to put
// its files back.
func (v *Vault) Commit(ctx context.Context, message string, paths ...string) (committed bool, err error) {
	before, err := v.Head(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			v.Uncommit(context.WithoutCancel(ctx), before, paths...)
		}
	}()
	if _, err := v.git.Run(ctx, v.dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return false, err
	}
	changed, err := v.staged(ctx, paths)
	if err != nil || !changed {
		return false, err
	}
	if _, err := v.git.Run(ctx, v.dir, append([]string{"commit", "--quiet", "--message", message, "--"}, paths...)...); err != nil {
		return false, err
	}
	return true, nil
}

// staged reports whether the index differs from HEAD in paths.
func (v *Vault) staged(ctx context.Context, paths []string) (bool, error) {
	_, err := v.git.Run(ctx, v.dir, append([]string{"diff-index", "--cached", "--quiet", "HEAD", "--"}, paths...)...)
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return true, nil // --quiet's "there are differences"
	}
	return false, err
}

// Uncommit puts HEAD back on before, if a commit got through since, and unstages
// paths (everything, with none), leaving the working tree as it is. It does its
// best: the error that brought the caller here is the one to report.
func (v *Vault) Uncommit(ctx context.Context, before string, paths ...string) {
	if head, err := v.Head(ctx); err == nil && head != before {
		_, _ = v.git.Run(ctx, v.dir, "update-ref", "HEAD", before, head)
	}
	_, _ = v.git.Run(ctx, v.dir, append([]string{"reset", "--quiet", "--"}, paths...)...)
}
