package load

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/selector"
)

// Foreign is a skill in a target's folder that skl didn't create: a folder, or a
// link to one, with a SKILL.md and no .skl-copy.toml.
type Foreign struct {
	Target config.Target
	Dir    string // the target's skills folder
	Folder string // its folder's name
	Name   string // its frontmatter name, "" if it has none
	Link   bool   // the folder is a link, as another tool may leave it
}

// Path is the folder in the target's skills folder.
func (f Foreign) Path() string { return filepath.Join(f.Dir, f.Folder) }

// Foreigners lists the skills skl didn't create in the folders of targets, target
// by target, each in folder order. Hidden folders and folders without a SKILL.md,
// such as a skill's workspace, aren't skills.
func Foreigners(homeRoot string, targets []config.Target) ([]Foreign, error) {
	var out []Foreign
	for _, t := range targets {
		dir := SkillsDir(homeRoot, t)
		h, err := scan(dir)
		if err != nil {
			return nil, err
		}
		for _, name := range h.others {
			path := filepath.Join(dir, name)
			if !isSkill(path) {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			fm, _ := readForeignName(path)
			out = append(out, Foreign{
				Target: t, Dir: dir, Folder: name, Name: fm, Link: info.Mode()&os.ModeSymlink != 0,
			})
		}
	}
	return out, nil
}

// Clashes reports whether f and o share a name, in any case or form: a skill's
// names are its folder and its frontmatter name, as in the vault, where two such
// skills in one group can't be loaded together.
func (f Foreign) Clashes(o Foreign) bool {
	ff, fn := selector.Fold(f.Folder), selector.Fold(f.Name)
	of, on := selector.Fold(o.Folder), selector.Fold(o.Name)
	return ff == of || ff == on || fn == of || fn != "" && fn == on
}

// CopyIn copies a skill someone else made into dst, which must not exist: the
// files git keeps, without skl's records. A link at src is followed.
func CopyIn(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return copyFolder(ctx, src, dst, 0, []string{manifestFile, originFile})
}

// SkillTree is the tree a vault skill's folder loads with: what a copy of it
// records, so Compute finds that copy current.
func SkillTree(ctx context.Context, dir string) (string, error) {
	return treeOf(ctx, dir, manifestFile, originFile)
}

// Adoption is a folder made a loaded skill. It lasts until the caller knows the
// change stands, and Undo takes it back.
type Adoption struct {
	path    string
	link    string // where the link went, for a folder that was one
	written bool
}

// Adopt makes f the loaded copy of the vault skill source, at commit, with tree:
// it writes .skl-copy.toml in the folder. A link is first replaced with a real
// copy of what it points at, so the record never lands in another tool's folder;
// the link is kept in staging until Close. It reports whether it replaced a link.
func Adopt(ctx context.Context, f Foreign, staging string, m Manifest) (*Adoption, error) {
	a := &Adoption{path: f.Path()}
	if f.Link {
		if err := a.unlink(ctx, f, staging); err != nil {
			a.Undo()
			return nil, err
		}
	}
	if err := writeManifest(filepath.Join(a.path, manifestFile), m); err != nil {
		a.Undo()
		return nil, err
	}
	a.written = true
	return a, nil
}

// Path is the adopted folder.
func (a *Adoption) Path() string { return a.path }

// Replaced reports whether the folder was a link that Adopt replaced.
func (a *Adoption) Replaced() bool { return a.link != "" }

// unlink puts a real copy where the link was. The copy is made next to it, so the
// last step is one rename.
func (a *Adoption) unlink(ctx context.Context, f Foreign, staging string) error {
	tmp := filepath.Join(f.Dir, ".skl-adopt-"+f.Folder)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := CopyIn(ctx, a.path, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	kept := filepath.Join(staging, "adopt-"+f.Folder)
	_ = os.Remove(kept)
	// A link is a file: renaming it moves the link, never what it points at.
	if err := os.Rename(a.path, kept); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	a.link = kept
	if err := os.Rename(tmp, a.path); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return nil
}

// Undo puts the folder back as it was. It does its best: the next load reads
// what's on disk.
func (a *Adoption) Undo() {
	if a.written {
		_ = os.Remove(filepath.Join(a.path, manifestFile))
		a.written = false
	}
	if a.link == "" {
		return
	}
	if info, err := os.Lstat(a.path); err == nil && info.IsDir() {
		_ = os.RemoveAll(a.path)
	}
	if err := os.Rename(a.link, a.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	a.link = ""
}

// Close lets the replaced link go.
func (a *Adoption) Close() {
	if a.link != "" {
		_ = os.Remove(a.link)
		a.link = ""
	}
}

// Adoptions are several adoptions taken back or closed together.
type Adoptions []*Adoption

// Undo takes back every adoption, newest first.
func (as Adoptions) Undo() {
	for _, a := range slices.Backward(as) {
		a.Undo()
	}
}

// Close closes every adoption.
func (as Adoptions) Close() {
	for _, a := range as {
		a.Close()
	}
}
