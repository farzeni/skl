package vault

import (
	"context"
	_ "embed" // the license texts
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/farzeni/skl/internal/gitx"
)

// NoLicense is the license setting that writes no LICENSE file.
const NoLicense = "none"

//go:embed licenses/MIT.txt
var mitText string

//go:embed licenses/Apache-2.0.txt
var apacheText string

// CheckIdentity returns ErrNoIdentity if git in this environment can't name
// the author and committer of a commit. Git's own check, so an identity from
// the environment or one git works out by itself counts too. dir "" is fine.
func CheckIdentity(ctx context.Context, git *gitx.Git, dir string) error {
	for _, who := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		if _, err := git.Run(ctx, dir, "var", who); err != nil {
			if ge, ok := errors.AsType[*gitx.Error](err); ok && ctx.Err() == nil && ge.Code > 0 {
				return fmt.Errorf("%w: %w", ErrNoIdentity, err)
			}
			return err
		}
	}
	return nil
}

// Create makes the clone of an empty repo a new vault: its LICENSE and first
// commit, on this machine only. license is an SPDX id or NoLicense, in which
// case the first commit is empty: a vault always has a HEAD for generations and
// merges to start from. paths, such as skills init imports, join the commit.
func (v *Vault) Create(ctx context.Context, license string, now time.Time, paths ...string) error {
	if err := CheckIdentity(ctx, v.git, v.dir); err != nil {
		return err
	}
	args := []string{"commit", "--quiet", "--allow-empty", "--message", "skl init: new vault"}
	if license != NoLicense {
		holder, err := v.authorName(ctx)
		if err != nil {
			return err
		}
		text := licenseText(license, holder, now.Year())
		if err := os.WriteFile(filepath.Join(v.dir, "LICENSE"), []byte(text), 0o644); err != nil {
			return fmt.Errorf("vault: %w", err)
		}
		// -f: a global gitignore may name it.
		if _, err := v.git.Run(ctx, v.dir, "add", "--force", "--", "LICENSE"); err != nil {
			return err
		}
	}
	if len(paths) > 0 {
		if _, err := v.git.Run(ctx, v.dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
			return err
		}
	}
	_, err := v.git.Run(ctx, v.dir, args...)
	return err
}

// authorName is the name git would commit as, from "Name <email> time zone".
func (v *Vault) authorName(ctx context.Context) (string, error) {
	out, err := v.git.Run(ctx, v.dir, "var", "GIT_AUTHOR_IDENT")
	if err != nil {
		return "", err
	}
	name, _, _ := strings.Cut(out, " <")
	return strings.TrimSpace(name), nil
}

// licenseText is the file for an SPDX id: the full text of the two skl carries,
// else the standard one-line tag, which names any license without skl having to
// carry or fetch its text.
func licenseText(id, holder string, year int) string {
	switch id {
	case "MIT":
		return strings.NewReplacer("<year>", fmt.Sprint(year), "<holder>", holder).Replace(mitText)
	case "Apache-2.0":
		return apacheText
	}
	return "SPDX-License-Identifier: " + id + "\n"
}
