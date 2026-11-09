package load

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/vault"
)

// Saved is one copy saved back into the vault.
type Saved struct {
	Target config.Target
	Name   string   // the copy's folder
	Skill  string   // the vault skill, group/folder
	Files  []string // what changed, by path inside the skill's folder, sorted
	Commit string   // the vault's HEAD once it's saved, which the copy now records
	Tree   string   // the copy's tree, which it now records
}

// SaveInput is what SaveBack needs.
type SaveInput struct {
	Vault    *vault.Vault
	HomeRoot string
	Targets  []config.Target
	Current  []state.Copy // what state.json says is loaded, for a copy whose record skl can't read
}

// SaveBack merges each loaded copy that changed into the vault, one commit each,
// following docs/design/architecture.md "Saving back". It reads every target that's
// configured or holds a copy, in turn, so with two targets the second copy merges
// into the vault the first one already changed. The vault's work tree must be clean.
//
// A copy's folder that's gone, or holds no file, is not a deletion: the next load
// copies it in again. Only the files inside a copy's folder count, never
// .skl-copy.toml. A conflict stops at once with a *vault.ConflictError and the
// merge in progress; the copies saved before it are returned with it.
func SaveBack(ctx context.Context, in SaveInput) ([]Saved, error) {
	var saved []Saved
	for _, t := range allTargets(in.Targets, in.Current) {
		dir := SkillsDir(in.HomeRoot, t)
		h, err := scan(dir)
		if err != nil {
			return saved, err
		}
		for _, c := range h.copies {
			s, ok, err := saveCopy(ctx, in, t, dir, c)
			if err != nil {
				return saved, err
			}
			if ok {
				saved = append(saved, s)
			}
		}
	}
	return saved, nil
}

// saveCopy saves one copy if it changed.
func saveCopy(ctx context.Context, in SaveInput, t config.Target, dir string, c ownedCopy) (Saved, bool, error) {
	m := recordOf(c, t, in.Current)
	if m.Commit == "" || m.Tree == "" || !vaultSkill(m.Source) {
		return Saved{}, false, nil // nothing says what it was copied from: the next load replaces it
	}
	folder := filepath.Join(dir, c.folder)
	snap, err := snapshotOf(ctx, folder)
	if err != nil {
		return Saved{}, false, err
	}
	if snap.tree == m.Tree || snap.tree == "" {
		return Saved{}, false, nil
	}
	if err := writeSnapshot(ctx, in.Vault, snap); err != nil {
		return Saved{}, false, err
	}
	before, err := in.Vault.Files(ctx, m.Commit, m.Source)
	if err != nil {
		return Saved{}, false, err
	}
	head, err := in.Vault.SaveSkill(ctx, vault.Save{
		Path:    m.Source,
		Base:    m.Commit,
		Tree:    snap.tree,
		Message: "skl: save " + c.folder + " → " + m.Source,
		Copy:    string(t) + " " + c.folder,
	})
	if err != nil {
		return Saved{}, false, err
	}
	// The copy now holds what the vault got from it. If the vault had changes of
	// its own, the next load sees a tree that differs and copies the merge in.
	m = Manifest{Source: m.Source, Commit: head, Tree: snap.tree}
	if err := replaceManifest(folder, m); err != nil {
		return Saved{}, false, err
	}
	return Saved{
		Target: t, Name: c.folder, Skill: m.Source,
		Files: changedFiles(before, snap.files), Commit: head, Tree: snap.tree,
	}, true, nil
}

// recordOf is what a copy was made from: its .skl-copy.toml, else what state.json
// says about the copy in that folder.
func recordOf(c ownedCopy, t config.Target, current []state.Copy) Manifest {
	if c.manifest.Commit != "" && c.manifest.Tree != "" {
		return c.manifest
	}
	if cp := currentCopy(current, t, selector.Fold(c.folder)); cp != nil {
		return Manifest{Source: cp.Skill, Commit: cp.Commit, Tree: cp.Tree}
	}
	return c.manifest
}

// vaultSkill reports whether source names a skill folder in the vault, group/folder,
// so a record can't make skl write anywhere else.
func vaultSkill(source string) bool {
	group, folder, ok := strings.Cut(source, "/")
	valid := func(s string) bool {
		return s != "" && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "/\\\x00")
	}
	return ok && valid(group) && valid(folder)
}

// writeSnapshot puts a copy's objects in the vault, and checks git names them as
// skl did.
func writeSnapshot(ctx context.Context, v *vault.Vault, s *snapshot) error {
	names, err := v.WriteObjects(ctx, s.objects)
	if err != nil {
		return err
	}
	want := slices.Concat(s.fileSums, s.linkSums, s.treeSums)
	if !slices.Equal(names, want) {
		return fmt.Errorf("load: git stored a copy's files under other names than skl computed")
	}
	return nil
}

// changedFiles lists what differs between two versions of a skill's files.
func changedFiles(before, after map[string]vault.File) []string {
	var out []string
	for name, f := range after {
		if before[name] != f {
			out = append(out, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// replaceManifest writes a copy's .skl-copy.toml in one step.
func replaceManifest(folder string, m Manifest) error {
	file := filepath.Join(folder, manifestFile)
	tmp := file + ".tmp"
	if err := writeManifest(tmp, m); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file)
}
