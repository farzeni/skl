package vault

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/farzeni/skl/internal/gitx"
)

// A merge in progress (docs/design/architecture.md "Safety") is either sync's
// merge of origin, or a save-back's merge of a synthetic commit that names the
// copy it came from. skl sync --continue finishes either one; skl sync --abort,
// or Unmerge, takes it back.

// OpenMerge is the merge in progress in the vault.
type OpenMerge struct {
	Theirs   string // MERGE_HEAD
	Incoming int    // the commits it brings that HEAD doesn't have
	// Copy is the loaded copy a save-back merge came from, "<target> <folder>"
	// (the synthetic commit's Skl-Copy trailer), or "" for sync's merge.
	Copy string
}

// Open reads the merge in progress. Call it only when Merging says there is one.
func (v *Vault) Open(ctx context.Context) (OpenMerge, error) {
	theirs, err := v.rev(ctx, "MERGE_HEAD")
	if err != nil {
		return OpenMerge{}, err
	}
	out, err := v.git.Run(ctx, v.dir, "rev-list", "--count", theirs, "--not", "HEAD")
	if err != nil {
		return OpenMerge{}, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return OpenMerge{}, err
	}
	copied, err := v.git.Run(ctx, v.dir, "log", "-1", "--format=%(trailers:key=Skl-Copy,valueonly,separator=%x00)", theirs)
	if err != nil {
		return OpenMerge{}, err
	}
	return OpenMerge{Theirs: theirs, Incoming: n, Copy: strings.TrimSpace(copied)}, nil
}

// Markers lists the files in the work tree that still hold conflict markers the
// merge brought, as git diff --check sees them against HEAD.
func (v *Vault) Markers(ctx context.Context) ([]string, error) {
	out, err := v.git.Run(ctx, v.dir, "diff", "--check", "HEAD")
	ge, ok := errors.AsType[*gitx.Error](err)
	if err != nil && (!ok || ge.Code != 2 || ctx.Err() != nil) {
		return nil, err
	}
	if ok {
		out = ge.Stdout // 2 is --check's "problems found": markers or whitespace
	}
	var paths []string
	for line := range strings.SplitSeq(out, "\n") {
		path, rest, ok := strings.Cut(line, ":")
		if !ok || !strings.HasSuffix(rest, "leftover conflict marker") {
			continue // whitespace problems aren't skl's to report
		}
		if len(paths) == 0 || paths[len(paths)-1] != path {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Continue commits the merge in progress with every change in the work tree, as
// the user resolved it, under git's own merge message. On any error, Ctrl-C
// included, the merge stays in progress as it was.
func (v *Vault) Continue(ctx context.Context) error {
	if _, err := v.git.Run(ctx, v.dir, "add", "-A"); err != nil {
		return err
	}
	_, err := v.git.Run(ctx, v.dir, "commit", "--quiet", "--no-edit")
	if err != nil {
		if merging, merr := v.Merging(context.WithoutCancel(ctx)); merr == nil && !merging {
			return nil // the commit went through before the error came
		}
	}
	return err
}

// Side is one version of a file in a merge: absent when that side has no such
// file.
type Side struct {
	Data   []byte
	Absent bool
}

// Sides are a conflicted file as the merge in progress started from it (Base),
// as HEAD has it (Ours) and as MERGE_HEAD has it (Theirs).
type Sides struct{ Base, Ours, Theirs Side }

// SidesOf reads path's three versions in the merge in progress.
func (v *Vault) SidesOf(ctx context.Context, path string) (Sides, error) {
	base, err := v.git.Run(ctx, v.dir, "merge-base", "HEAD", "MERGE_HEAD")
	if err != nil {
		return Sides{}, err
	}
	var s Sides
	for _, side := range []struct {
		rev string
		to  *Side
	}{{strings.TrimSpace(base), &s.Base}, {"HEAD", &s.Ours}, {"MERGE_HEAD", &s.Theirs}} {
		if *side.to, err = v.blob(ctx, side.rev, path); err != nil {
			return Sides{}, err
		}
	}
	return s, nil
}

// blob is path's content at rev, or an absent Side if rev has no such file.
func (v *Vault) blob(ctx context.Context, rev, path string) (Side, error) {
	obj, err := v.git.Run(ctx, v.dir, "rev-parse", "--verify", "--quiet", rev+":"+path)
	if ge, ok := errors.AsType[*gitx.Error](err); ok && ge.Code == 1 && ctx.Err() == nil {
		return Side{Absent: true}, nil
	}
	if err != nil {
		return Side{}, err
	}
	data, err := v.git.Run(ctx, v.dir, "cat-file", "blob", strings.TrimSpace(obj))
	if err != nil {
		return Side{}, err
	}
	return Side{Data: []byte(data)}, nil
}

// TreeAt is the git tree of path at rev: for a save-back merge, the copy's
// folder as it was saved.
func (v *Vault) TreeAt(ctx context.Context, rev, path string) (string, error) {
	return v.rev(ctx, rev+":"+path)
}
