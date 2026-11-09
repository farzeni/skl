// Package load copies vault skills into the folders of the targets, following
// docs/design/architecture.md "Loading pipeline": Compute diffs what's selected
// against what's loaded, and Apply writes the new copies in staging and swaps
// them in. It holds no command and never touches the network; a folder without
// .skl-copy.toml is someone else's and is never changed.
package load

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/vault"
)

// SkillsDir is a target's skills folder under root: the home folder for the user
// scope, a repo's top level for its project scope.
func SkillsDir(root string, t config.Target) string {
	if t == config.Agents {
		return filepath.Join(root, ".agents", "skills")
	}
	return filepath.Join(root, ".claude", "skills")
}

// Manifest is .skl-copy.toml (docs/reference/files.md "Loaded skills").
type Manifest struct {
	Source string `toml:"source"` // the vault skill, group/folder
	Commit string `toml:"commit"` // the vault commit it was copied from
	Tree   string `toml:"tree"`   // the folder's git tree hash, as copied
}

// Kind is what a plan does to a skill.
type Kind int

// The things a plan does.
const (
	Load Kind = iota
	Replace
	Unload
)

// Change is one line of the plan: what happens to one name, in any target.
type Change struct {
	Kind   Kind
	Name   string
	Skill  string // the vault skill, group/folder: the one loaded, or the one unloaded
	Detail string // "someone/pr-review replaces review/pr-review", or "updated"
	From   string // for Unload: the one skills folder it leaves, if others keep it
}

// Skip is a skill dropped from one target because someone else's skill holds a
// name it needs.
type Skip struct {
	Skill   resolve.Skill
	Path    string // the other skill's folder
	Foreign string // that folder's name
}

// Plan is what loading a list of skills would do. Compute makes it and changes
// nothing; Apply carries it out.
type Plan struct {
	Changes []Change
	Skipped []Skip
	Copies  []state.Copy // what's loaded afterward, in every target
	vault   string
	commit  string
	steps   []step
}

// step is one folder's change. dir/old leaves, and the skill arrives as dir/name.
type step struct {
	target config.Target
	dir    string
	old    string // a folder to move out, if any
	skill  *resolve.Skill
	tree   string
}

// Empty reports whether no folder would change.
func (p *Plan) Empty() bool { return len(p.steps) == 0 }

// Input is what Compute needs.
type Input struct {
	Vault    string // the vault's folder
	Commit   string // the vault's HEAD
	Result   resolve.Result
	Current  []state.Copy
	Targets  []config.Target
	HomeRoot string // what a target's skills folder is under
}

// Compute diffs the selected skills against what's loaded. It only reads.
//
// The folders decide what's there, through their .skl-copy.toml: state.json lists
// what to unload, so a state moved aside or a crash mid-swap only costs a reload.
func Compute(ctx context.Context, in Input) (*Plan, error) {
	p := &Plan{vault: in.Vault, commit: in.Commit}
	trees := map[resolve.Skill]string{}
	treeOfSkill := func(s resolve.Skill) (string, error) {
		if t, ok := trees[s]; ok {
			return t, nil
		}
		t, err := treeOf(ctx, filepath.Join(in.Vault, s.Group, s.Folder), manifestFile, originFile)
		trees[s] = t
		return t, err
	}

	var c changes
	for _, t := range allTargets(in.Targets, in.Current) {
		if err := p.planTarget(ctx, in, t, treeOfSkill, &c); err != nil {
			return nil, err
		}
	}
	p.Changes = c.list()
	return p, nil
}

// planTarget diffs one target's folder. A target no longer configured wants nothing.
func (p *Plan) planTarget(ctx context.Context, in Input, t config.Target, treeOfSkill func(resolve.Skill) (string, error), c *changes) error {
	dir := SkillsDir(in.HomeRoot, t)
	held, err := scan(dir)
	if err != nil {
		return err
	}
	var wanted []resolve.Skill
	if slices.Contains(in.Targets, t) {
		res, skipped := in.Result.Skipping(func(s resolve.Skill) bool {
			_, ok := held.foreignFor(s)
			return ok
		})
		for _, s := range skipped {
			f, _ := held.foreignFor(s)
			p.Skipped = append(p.Skipped, Skip{Skill: s, Path: filepath.Join(dir, f), Foreign: f})
		}
		wanted = res.Skills
		c.settled(res)
	}
	kept := map[string]bool{} // what stays under a name, by folded name
	for _, s := range wanted {
		if err := ctx.Err(); err != nil {
			return err
		}
		tree, err := treeOfSkill(s)
		if err != nil {
			return err
		}
		name := selector.Fold(s.Folder)
		kept[name] = true
		onDisk, there := held.owned[name]
		current := there && onDisk.folder == s.Folder && onDisk.manifest.Source == s.String() && onDisk.manifest.Tree == tree
		if current {
			// Save-back leaves a folder emptied by hand alone, so it would stay
			// empty: it's reloaded (docs/guide/skills.md "Edit as usual").
			if current, err = hasContent(ctx, filepath.Join(dir, onDisk.folder)); err != nil {
				return err
			}
		}
		if current {
			p.Copies = append(p.Copies, state.Copy{
				Name: s.Folder, Target: t, Skill: s.String(), Commit: onDisk.manifest.Commit, Tree: tree,
			})
			continue
		}
		st := step{target: t, dir: dir, skill: &s, tree: tree}
		if there {
			st.old = onDisk.folder
		}
		p.steps = append(p.steps, st)
		p.Copies = append(p.Copies, state.Copy{
			Name: s.Folder, Target: t, Skill: s.String(), Commit: in.Commit, Tree: tree,
		})
		c.loaded(s, currentCopy(in.Current, t, name))
	}
	for _, cp := range in.Current {
		name := selector.Fold(cp.Name)
		if cp.Target != t || kept[name] {
			continue
		}
		onDisk, there := held.owned[name]
		if !there {
			continue // gone, or no longer ours: there is nothing to unload
		}
		p.steps = append(p.steps, step{target: t, dir: dir, old: onDisk.folder})
		c.unloaded(cp, dir)
	}
	return nil
}

// currentCopy is what state.json says a target holds under a folded name.
func currentCopy(current []state.Copy, t config.Target, name string) *state.Copy {
	for i, cp := range current {
		if cp.Target == t && selector.Fold(cp.Name) == name {
			return &current[i]
		}
	}
	return nil
}

// allTargets is the configured targets, then any others that hold copies: taking a
// target out of the config unloads what's there.
func allTargets(targets []config.Target, current []state.Copy) []config.Target {
	all := slices.Clone(targets)
	for _, cp := range current {
		if !slices.Contains(all, cp.Target) {
			all = append(all, cp.Target)
		}
	}
	return all
}

// changes builds the lines of a plan, one per name in any number of targets.
type changes struct {
	skills   []resolve.Skill       // what some target keeps
	replaced []resolve.Replacement // what some target settled
	added    []resolve.Skill
	details  map[string]string // an added skill's own reason for ~
	removed  []state.Copy
	dirs     map[string]string // a removed copy's skills folder
}

// settled adds what one target settled. Targets can differ, since someone else's
// skill holds a name in one folder only.
func (c *changes) settled(res resolve.Result) {
	for _, s := range res.Skills {
		if !slices.Contains(c.skills, s) {
			c.skills = append(c.skills, s)
		}
	}
	for _, r := range res.Replaced {
		if !slices.Contains(c.replaced, r) {
			c.replaced = append(c.replaced, r)
		}
	}
}

func (c *changes) loaded(s resolve.Skill, cur *state.Copy) {
	if !slices.Contains(c.added, s) {
		c.added = append(c.added, s)
	}
	if cur == nil {
		return
	}
	if c.details == nil {
		c.details = map[string]string{}
	}
	if cur.Skill == s.String() {
		c.details[s.Folder] = "updated"
	} else {
		c.details[s.Folder] = s.String() + " replaces " + cur.Skill
	}
}

func (c *changes) unloaded(cp state.Copy, dir string) {
	if c.dirs == nil {
		c.dirs = map[string]string{}
	}
	c.removed = append(c.removed, cp)
	c.dirs[string(cp.Target)+"/"+cp.Name] = dir
}

// list is every loaded skill, then every unloaded one. A clash the selectors
// settled says which skill replaced which, on the line of the one that won, and
// the loser needs no line of its own.
func (c *changes) list() []Change {
	var out []Change
	replacedBy := map[string]bool{} // the vault skills a ~ line already names
	for _, s := range c.added {
		var details []string
		for _, r := range c.replaced {
			if r.New == s {
				details = append(details, r.Detail())
				replacedBy[r.Old.String()] = true
			}
		}
		if len(details) == 0 {
			if d, ok := c.details[s.Folder]; ok {
				details = []string{d}
			}
		}
		if len(details) == 0 {
			out = append(out, Change{Kind: Load, Name: s.Folder, Skill: s.String()})
			continue
		}
		for _, d := range details {
			out = append(out, Change{Kind: Replace, Name: s.Folder, Skill: s.String(), Detail: d})
		}
	}
	var gone []Change
	for _, cp := range c.removed {
		if replacedBy[cp.Skill] || slices.ContainsFunc(gone, func(g Change) bool { return g.Name == cp.Name }) {
			continue
		}
		from := ""
		for _, s := range c.skills {
			if selector.Fold(s.Folder) == selector.Fold(cp.Name) {
				from = c.dirs[string(cp.Target)+"/"+cp.Name]
			}
		}
		gone = append(gone, Change{Kind: Unload, Name: cp.Name, Skill: cp.Skill, From: from})
	}
	slices.SortFunc(gone, func(a, b Change) int { return strings.Compare(a.Name, b.Name) })
	return append(out, gone...)
}

// held is what a skills folder holds.
type held struct {
	owned   map[string]ownedCopy // skl's copies, by folded folder name
	foreign map[string]string    // every name of someone else's skills → its folder
	copies  []ownedCopy          // skl's copies, each one, in folder order
	others  []string             // the folders and files of someone else's, in order
}

type ownedCopy struct {
	folder   string
	manifest Manifest
}

// foreignFor returns the folder of someone else's skill that shares a name with s.
// Its own folder name comes first: that's the folder the copy would land on.
func (h held) foreignFor(s resolve.Skill) (string, bool) {
	if f, ok := h.foreign[selector.Fold(s.Folder)]; ok {
		return f, true
	}
	if s.Name == "" {
		return "", false
	}
	f, ok := h.foreign[selector.Fold(s.Name)]
	return f, ok
}

// scan reads a skills folder. A folder that doesn't exist yet holds nothing. Hidden
// entries aren't skills, so they take no name.
func scan(dir string) (held, error) {
	h := held{owned: map[string]ownedCopy{}, foreign: map[string]string{}}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		if m, ok := readManifest(e, path); ok {
			h.owned[selector.Fold(name)] = ownedCopy{folder: name, manifest: m}
			h.copies = append(h.copies, ownedCopy{folder: name, manifest: m})
			continue
		}
		h.others = append(h.others, name)
		h.foreign[selector.Fold(name)] = name
		if fm, err := readForeignName(path); err == nil && fm != "" {
			if _, taken := h.foreign[selector.Fold(fm)]; !taken {
				h.foreign[selector.Fold(fm)] = name
			}
		}
	}
	return h, nil
}

// readManifest reports whether e is a real folder with a .skl-copy.toml in it,
// the mark of a copy skl made. A link is another tool's, whatever it points at. A
// manifest that doesn't parse still marks the folder as skl's, with no tree.
func readManifest(e fs.DirEntry, path string) (Manifest, bool) {
	if !e.IsDir() {
		return Manifest{}, false
	}
	file := filepath.Join(path, manifestFile)
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() {
		return Manifest{}, false
	}
	var m Manifest
	if _, err := toml.DecodeFile(file, &m); err != nil {
		return Manifest{}, true
	}
	return m, true
}

// readForeignName reads the frontmatter name of someone else's skill. Following a
// link is fine; a pipe named SKILL.md would hang the read.
func readForeignName(path string) (string, error) {
	file := filepath.Join(path, "SKILL.md")
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a file")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	fm, err := vault.ParseFrontmatter(data)
	return fm.Name, err
}

// Loader carries plans out. It keeps its work in Staging, under the lock.
type Loader struct {
	Staging string
	rename  func(from, to string) error // os.Rename; tests replace it
}

// NewLoader returns a loader that works in staging.
func NewLoader(staging string) *Loader {
	return &Loader{Staging: staging, rename: os.Rename}
}

// Swap is an applied plan. It lasts until the caller knows the change stands.
type Swap struct {
	l     *Loader
	done  []move
	roots []string
}

type move struct{ from, to string }

// Apply writes the plan's new copies in staging, then swaps them in. Stopping
// while it writes changes nothing. The swap itself runs to its end whatever
// happens to ctx. A step that fails moves back what it moved, so the previous set
// stays in place.
func (l *Loader) Apply(ctx context.Context, p *Plan) (*Swap, error) {
	s := &Swap{l: l}
	staged, err := s.stage(ctx, p)
	if err != nil {
		s.Close()
		return nil, err
	}
	for i, st := range p.steps {
		if err := s.swap(st, staged[i]); err != nil {
			s.Undo()
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

// stage writes each new copy with its manifest. It returns where each step's copy
// and displaced folder go.
func (s *Swap) stage(ctx context.Context, p *Plan) ([]stagedStep, error) {
	roots := map[string]string{}
	out := make([]stagedStep, len(p.steps))
	for i, st := range p.steps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root, ok := roots[st.dir]
		if !ok {
			var err error
			if root, err = s.prepare(st); err != nil {
				return nil, err
			}
			roots[st.dir] = root
		}
		out[i].old = filepath.Join(root, "old", fmt.Sprint(i))
		if st.skill == nil {
			continue
		}
		out[i].fresh = filepath.Join(root, "new", fmt.Sprint(i))
		src := filepath.Join(p.vault, st.skill.Group, st.skill.Folder)
		if err := copyFolder(ctx, src, out[i].fresh, 0, []string{manifestFile, originFile}); err != nil {
			return nil, err
		}
		m := Manifest{Source: st.skill.String(), Commit: p.commit, Tree: st.tree}
		if err := writeManifest(filepath.Join(out[i].fresh, manifestFile), m); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type stagedStep struct{ fresh, old string }

// prepare makes a target's folder and the staging root for it: ~/.skl/staging, or
// a hidden folder inside the skills folder when that is on another disk, where a
// rename can't reach. It clears what a killed skl left.
func (s *Swap) prepare(st step) (string, error) {
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		return "", err
	}
	root := filepath.Join(s.l.Staging, "load", string(st.target))
	if err := s.claim(root); err != nil {
		return "", err
	}
	probe := filepath.Join(st.dir, ".skl-probe")
	_ = os.RemoveAll(probe)
	err := s.l.rename(root, probe)
	if err == nil {
		if err := s.l.rename(probe, root); err != nil {
			return "", err
		}
		return root, nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return "", err
	}
	root = filepath.Join(st.dir, ".skl-staging")
	if err := s.claim(root); err != nil {
		return "", err
	}
	return root, nil
}

// claim empties a staging root and remembers to remove it.
func (s *Swap) claim(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	s.roots = append(s.roots, root)
	return os.MkdirAll(filepath.Join(root, "old"), 0o755)
}

func (s *Swap) swap(st step, staged stagedStep) error {
	if st.old != "" {
		if err := s.move(filepath.Join(st.dir, st.old), staged.old); err != nil {
			return err
		}
	}
	if st.skill != nil {
		return s.move(staged.fresh, filepath.Join(st.dir, st.skill.Folder))
	}
	return nil
}

func (s *Swap) move(from, to string) error {
	if err := s.l.rename(from, to); err != nil {
		return err
	}
	s.done = append(s.done, move{from, to})
	return nil
}

// Undo moves everything back, newest first. It does its best: what fails here has
// no further way back, and the next load reads what's on disk.
func (s *Swap) Undo() {
	for _, m := range slices.Backward(s.done) {
		_ = s.l.rename(m.to, m.from)
	}
	s.done = nil
}

// Close removes the staging folders, and with them the copies that were replaced.
func (s *Swap) Close() {
	for _, root := range s.roots {
		_ = os.RemoveAll(root)
	}
	if len(s.roots) > 0 {
		_ = os.Remove(filepath.Join(s.l.Staging, "load"))
	}
	s.roots = nil
}

func writeManifest(file string, m Manifest) error {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(m); err != nil {
		return err
	}
	return os.WriteFile(file, b.Bytes(), 0o644)
}
