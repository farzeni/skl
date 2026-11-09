package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/ref"
	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Import is skl import <name>… [--group <group>]: skills skl didn't create, in the
// targets' folders, are copied into the vault and committed, and each folder stays
// where it is as the loaded skill (docs/guide/skills.md "Add a skill you made").
// Each joins the active skill set as skl add would. group "" is the vault owner's.
func (a *App) Import(ctx context.Context, names []string, group string, dryRun bool) error {
	names, err := importNames(names, group)
	if err != nil {
		return err
	}
	root, err := a.userRoot()
	if err != nil {
		return err
	}
	line := append([]string{"import"}, names...)
	if group != "" {
		line = append(line, "--group", group)
	}
	command := strings.TrimPrefix(ui.CommandLine(line), "skl ")
	if dryRun {
		return hinted(a.importDryRun(ctx, names, group, root), "skl "+command)
	}
	return a.Changing(ctx, command, func(ctx context.Context, c *Change) (bool, error) {
		idx, err := c.Vault.Index(ctx)
		if err != nil {
			return false, err
		}
		found, err := a.findImports(idx, root, names)
		if err != nil {
			return false, err
		}
		g, err := a.importGroup(ctx, c.Vault, group, names)
		if err != nil {
			return false, err
		}
		before, err := c.Vault.Head(ctx)
		if err != nil {
			return false, err
		}
		folders, err := copyImports(ctx, c.Vault, g, found)
		if err != nil {
			return false, err
		}
		undoVault := func() { uncopy(c.Vault, before, folders) }
		if _, err := c.Vault.Commit(ctx, importMessage(g, found), folders...); err != nil {
			undoVault()
			return false, err
		}
		if err := a.adoptAndLoad(ctx, c, imports{group: g, found: found}, nil, nil, undoVault); err != nil {
			return false, err
		}
		return true, nil
	})
}

// importNames checks the words before anything else: each must be a name a vault
// skill can have, and the group one a group can have. A name typed twice counts once.
func importNames(names []string, group string) ([]string, error) {
	var out []string
	for _, n := range names {
		if !selector.ValidName(n) {
			return nil, errs.Usagef("rename it in ~/.claude/skills, then skl import it",
				"%s isn't a valid skill name: %s", ui.Clean(n), nameRule)
		}
		if !slices.ContainsFunc(out, func(o string) bool { return selector.Fold(o) == selector.Fold(n) }) {
			out = append(out, n)
		}
	}
	if group != "" {
		switch {
		case !selector.ValidName(group):
			return nil, errs.Usagef("skl help import", "%s isn't a valid group name: %s", ui.Clean(group), nameRule)
		case selector.ReservedGroup(group):
			return nil, errs.Usagef("skl help import", "%s can't be a group: sets, lessons and h are reserved", ui.Clean(group))
		}
	}
	return out, nil
}

// importDryRun prints what import would copy, and where. It takes no lock and
// writes nothing.
func (a *App) importDryRun(ctx context.Context, names []string, group, root string) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	if err := a.notMerging(ctx, v); err != nil {
		return err
	}
	idx, err := v.Index(ctx)
	if err != nil {
		return err
	}
	found, err := a.findImports(idx, root, names)
	if err != nil {
		return err
	}
	g, err := a.importGroup(ctx, v, group, names)
	if err != nil {
		return err
	}
	a.ui.Plan("import %s → %s/", folderList(found), g)
	return nil
}

// findImports finds each name in the targets' folders, the first target first. A
// name the vault already has, as a folder or a frontmatter name, stops the import.
func (a *App) findImports(idx *vault.Index, root string, names []string) ([]load.Foreign, error) {
	all, err := load.Foreigners(root, a.cfg.Targets)
	if err != nil {
		return nil, a.shownPaths(err)
	}
	var found []load.Foreign
	for _, name := range names {
		i := slices.IndexFunc(all, func(f load.Foreign) bool { return selector.Fold(f.Folder) == selector.Fold(name) })
		if i < 0 {
			if s, ok := vaultSkillNamed(idx, name, ""); ok {
				return nil, a.alreadyInVault(name, s, load.SkillsDir(root, a.cfg.Targets[0]))
			}
			return nil, errs.Hint(fmt.Errorf("%s isn't in %s", ui.Clean(name), a.targetDirs(root)), "skl status")
		}
		f := all[i]
		if s, ok := vaultSkillNamed(idx, f.Folder, f.Name); ok {
			return nil, a.alreadyInVault(name, s, f.Dir)
		}
		// Two of them sharing a name would clash in the group: stop before
		// anything is copied, while renaming one is still the way out.
		if j := slices.IndexFunc(found, f.Clashes); j >= 0 {
			return nil, errs.Hint(fmt.Errorf("%s and %s have the same name", ui.Clean(found[j].Folder), ui.Clean(f.Folder)),
				"rename one in "+a.shown(f.Dir)+", then skl import it")
		}
		found = append(found, f)
	}
	return found, nil
}

func (a *App) alreadyInVault(name string, s resolve.Skill, dir string) error {
	return errs.Hint(fmt.Errorf("%s is already in your vault: %s", ui.Clean(name), ui.Clean(s.String())),
		"rename it in "+a.shown(dir)+", then skl import it")
}

// targetDirs names the targets' skills folders: "~/.claude/skills", or two with "or".
func (a *App) targetDirs(root string) string {
	dirs := make([]string, len(a.cfg.Targets))
	for i, t := range a.cfg.Targets {
		dirs[i] = a.shown(load.SkillsDir(root, t))
	}
	return strings.Join(dirs, " or ")
}

// vaultSkillNamed returns the vault skill whose folder or frontmatter name is one
// of names, in any case or form.
func vaultSkillNamed(idx *vault.Index, names ...string) (resolve.Skill, bool) {
	var want []string
	for _, n := range names {
		if n != "" {
			want = append(want, selector.Fold(n))
		}
	}
	for _, g := range idx.Groups() {
		for _, s := range idx.Skills(g) {
			if slices.Contains(want, selector.Fold(s.Folder)) || s.Name != "" && slices.Contains(want, selector.Fold(s.Name)) {
				return s, true
			}
		}
	}
	return resolve.Skill{}, false
}

// importGroup is the group to import into: the one given, else the vault owner's
// (docs/reference/repos.md "Grammar").
func (a *App) importGroup(ctx context.Context, v *vault.Vault, group string, names []string) (string, error) {
	if group != "" {
		return group, nil
	}
	origin, err := v.Origin(ctx)
	if err != nil {
		return "", err
	}
	retry := ui.CommandLine(append(append([]string{"import"}, names...), "--group", "<group>"))
	if origin == "" {
		return "", errs.Hint(errors.New("your vault has no origin to name a group after"), retry)
	}
	repo, err := ref.ParseRepo(origin, a.refOptions(a.paths.Vault))
	if err != nil {
		return "", errs.Hint(fmt.Errorf("can't name a group after your vault's origin: %w", err), retry)
	}
	g, err := selector.OwnerGroup(repo.Owner())
	if err != nil {
		return "", errs.Hint(err, retry)
	}
	return g, nil
}

// copyImports copies each skill into the vault's group, and returns the folders,
// from the vault's root. It stops, and removes what it copied, at the first error.
func copyImports(ctx context.Context, v *vault.Vault, group string, found []load.Foreign) ([]string, error) {
	var folders []string
	for _, f := range found {
		rel := path.Join(group, f.Folder)
		dst := filepath.Join(v.Dir(), filepath.FromSlash(rel))
		if _, err := os.Lstat(dst); err == nil {
			removeAll(v.Dir(), folders)
			return nil, fmt.Errorf("import: %s is in the way in your vault", rel)
		}
		folders = append(folders, rel)
		if err := load.CopyIn(ctx, f.Path(), dst); err != nil {
			removeAll(v.Dir(), folders)
			return nil, err
		}
	}
	return folders, nil
}

func removeAll(dir string, folders []string) {
	for _, f := range folders {
		_ = os.RemoveAll(filepath.Join(dir, filepath.FromSlash(f)))
	}
	if len(folders) > 0 {
		_ = os.Remove(filepath.Join(dir, path.Dir(folders[0]))) // the group, if import made it
	}
}

// uncopy takes an import's commit and folders back out of the vault.
func uncopy(v *vault.Vault, before string, folders []string) {
	v.Uncommit(context.Background(), before, folders...)
	removeAll(v.Dir(), folders)
}

func importMessage(group string, found []load.Foreign) string {
	return "skl: import " + folderList(found) + " → " + group + "/"
}

func folderList(found []load.Foreign) string {
	names := make([]string, len(found))
	for i, f := range found {
		names[i] = ui.Clean(f.Folder)
	}
	return strings.Join(names, ", ")
}

// imports are skills committed in the vault's group, on their way to being loaded.
type imports struct {
	group string
	found []load.Foreign
	// whole adds the group to the active skill set instead of each skill: init
	// does when the group holds only what it imported.
	whole bool
}

// adoptAndLoad ends an import, once the skills are committed in the vault: each
// folder becomes the loaded copy, then the active skill set takes words, if not
// nil, and the imported skills, and loads them, into any other target too.
// between, if not nil, prints its lines after "◇ imported". undoVault takes the
// vault's part back: on an error here, or through c.Undo when no generation can
// be made, all of it goes back.
func (a *App) adoptAndLoad(ctx context.Context, c *Change, im imports, init *selection, between, undoVault func()) error {
	head, err := c.Vault.Head(ctx)
	if err != nil {
		undoVault()
		return err
	}
	adopted, err := a.adopt(ctx, c.Vault, im.group, head, im.found)
	undo := func() { adopted.Undo(); undoVault() }
	if err != nil {
		undo()
		return a.shownPaths(err)
	}
	for _, ad := range adopted {
		if ad.Replaced() {
			a.ui.Step("replaced", "link "+a.shown(ad.Path())+" with a copy")
		}
	}
	if len(im.found) > 0 {
		a.ui.Step("imported", folderList(im.found)+" → "+im.group+"/")
	}
	if between != nil {
		between()
	}
	next, err := a.importSelectors(ctx, c, im, init)
	if err != nil {
		undo()
		return err
	}
	a.printChanges(next.plan.Changes)
	for _, n := range next.notes {
		a.ui.Warn(n.msg, ui.Clean(n.hint))
	}
	if err := a.apply(ctx, c, next); err != nil {
		undo()
		return err
	}
	undoLoad, closeLoad := c.Undo, c.Cleanup
	c.Undo = func() {
		if undoLoad != nil {
			undoLoad()
		}
		undo()
	}
	c.Cleanup = func() {
		if closeLoad != nil {
			closeLoad()
		}
		adopted.Close()
	}
	return nil
}

// adopt writes each imported folder's record: it becomes the loaded copy of the
// vault skill it was copied into.
func (a *App) adopt(ctx context.Context, v *vault.Vault, group, head string, found []load.Foreign) (load.Adoptions, error) {
	var done load.Adoptions
	for _, f := range found {
		tree, err := load.SkillTree(ctx, filepath.Join(v.Dir(), group, f.Folder))
		if err != nil {
			return done, err
		}
		ad, err := load.Adopt(ctx, f, a.paths.Staging, load.Manifest{Source: group + "/" + f.Folder, Commit: head, Tree: tree})
		if err != nil {
			return done, err
		}
		done = append(done, ad)
	}
	return done, nil
}

// importSelectors works out the active skill set with words and the imported
// skills in it, and plans its load. A selector that already covers a skill stays
// as it is (docs/guide/skills.md "Add a skill you made").
func (a *App) importSelectors(ctx context.Context, c *Change, im imports, init *selection) (*loadNext, error) {
	root, err := a.homeRoot()
	if err != nil {
		return nil, err
	}
	var joined []string
	if im.whole {
		joined = []string{im.group}
	} else {
		for _, f := range im.found {
			joined = append(joined, im.group+"/"+f.Folder)
		}
	}
	// The hints of warnings repeat the command: init's own, else skl add of what
	// import adds.
	s := selection{verb: "add", words: joined}
	if init != nil {
		s = *init
	}
	s.next = func(active []string, _ []selector.Selector, idx *vault.Index) ([]string, []note, error) {
		next := slices.Clone(active)
		if init != nil {
			next = slices.Clone(init.words) // init loads its selectors as skl use would
		}
		for _, j := range joined {
			sel, err := selector.Parse(j)
			if err != nil {
				return nil, nil, err
			}
			if !noEffect(next, sel, idx) {
				next = append(next, j)
			}
		}
		return next, nil, nil
	}
	return a.prepare(ctx, c.Vault, c.State, s, nil, root)
}
