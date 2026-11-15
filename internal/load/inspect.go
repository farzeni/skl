package load

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
)

// Inspection is what the skills folders hold, against what's selected: what skl
// status reports. The folders decide what's loaded, through their .skl-copy.toml,
// as they do in Compute.
type Inspection struct {
	Dirs    []string // the skills folders it read
	Loaded  []Loaded // one per skill, however many targets hold it
	Gone    []Gone
	Skipped []Skip
	Foreign []string // someone else's skills that no Skip names, sorted
}

// Loaded is one loaded skill.
type Loaded struct {
	Name    string // its folder
	Skill   string // the vault skill it came from, "" if its manifest can't say
	Changed bool   // some target's copy differs from what was copied
}

// Gone is a copy state.json lists and a selector still wants, with no folder.
type Gone struct {
	Name string
	Path string
}

// Inspect reads the folders of every target that's configured or holds a copy. It
// changes nothing, takes no lock and needs no vault commit: only Result, Current,
// Targets and HomeRoot of in count.
func Inspect(ctx context.Context, in Input) (*Inspection, error) {
	out := &Inspection{}
	loaded := map[Loaded]bool{} // by Name and Skill, with Changed false
	foreign := map[string]bool{}
	var skippedFolders []string
	for _, t := range allTargets(in.Targets, in.Current) {
		dir := SkillsDir(in.HomeRoot, t)
		out.Dirs = append(out.Dirs, dir)
		h, err := scan(dir)
		if err != nil {
			return nil, err
		}
		for _, c := range h.copies {
			tree, err := Tree(ctx, filepath.Join(dir, c.folder))
			if err != nil {
				return nil, err
			}
			key := Loaded{Name: c.folder, Skill: c.manifest.Source}
			loaded[key] = loaded[key] || tree != c.manifest.Tree
		}
		for _, name := range h.others {
			if isSkill(filepath.Join(dir, name)) {
				foreign[name] = true
			}
		}
		wanted := map[string]bool{}
		if slices.Contains(in.Targets, t) {
			res, skipped := in.Result.Skipping(func(s resolve.Skill) bool {
				_, ok := h.foreignFor(s)
				return ok
			})
			for _, s := range skipped {
				f, _ := h.foreignFor(s)
				out.Skipped = append(out.Skipped, Skip{Skill: s, Path: filepath.Join(dir, f)})
				skippedFolders = append(skippedFolders, f)
			}
			for _, s := range res.Skills {
				wanted[s.String()] = true
			}
		}
		out.Gone = append(out.Gone, gone(in.Current, t, dir, h, wanted)...)
	}
	for key, changed := range loaded {
		key.Changed = changed
		out.Loaded = append(out.Loaded, key)
	}
	sortLoaded(out.Loaded, in.Current)
	for name := range foreign {
		if !slices.Contains(skippedFolders, name) {
			out.Foreign = append(out.Foreign, name)
		}
	}
	slices.Sort(out.Foreign)
	return out, nil
}

// gone is what t should hold by state.json, and a selector still wants, with
// nothing at its name. A name someone else's skill took is a Skip, not this.
func gone(current []state.Copy, t config.Target, dir string, h held, wanted map[string]bool) []Gone {
	var out []Gone
	for _, cp := range current {
		name := selector.Fold(cp.Name)
		_, owned := h.owned[name]
		_, other := h.foreign[name]
		if cp.Target == t && !owned && !other && wanted[cp.Skill] {
			out = append(out, Gone{Name: cp.Name, Path: filepath.Join(dir, cp.Name)})
		}
	}
	return out
}

// isSkill reports whether a folder someone else made is a skill: it holds a
// SKILL.md, through a link or not. A work folder next to a skill isn't one.
func isSkill(path string) bool {
	info, err := os.Stat(filepath.Join(path, "SKILL.md"))
	return err == nil && info.Mode().IsRegular()
}

// sortLoaded puts what state.json lists first, in its order, which is the order
// the selectors chose; the rest follow by name.
func sortLoaded(loaded []Loaded, current []state.Copy) {
	rank := func(l Loaded) int {
		i := slices.IndexFunc(current, func(c state.Copy) bool { return c.Name == l.Name && c.Skill == l.Skill })
		if i < 0 {
			return len(current)
		}
		return i
	}
	slices.SortFunc(loaded, func(a, b Loaded) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Skill, b.Skill))
	})
}
