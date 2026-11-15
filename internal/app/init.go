package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/errs"
	"github.com/farzeni/skl/internal/gitx"
	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/ref"
	"github.com/farzeni/skl/internal/selector"
	"github.com/farzeni/skl/internal/state"
	"github.com/farzeni/skl/internal/ui"
	"github.com/farzeni/skl/internal/vault"
)

// Init is skl init <repo> [selectors]: on a fresh machine it makes ~/.skl/vault
// from an existing or an empty repo, offers to import the skills in the targets'
// folders that aren't in a vault, and loads the selectors and what it imported
// (docs/guide/getting-started.md "Make your vault"). With the vault already
// there, it only loads the selectors, as skl use would.
//
// Making the vault runs in the frame's lock with no vault check, since making
// the vault is the point. The clone is built in staging, asked about and moved
// into place in one step, once it's complete, so an error, a question that
// can't be asked or Ctrl-C leaves no vault at all.
func (a *App) Init(ctx context.Context, word string, words []string, dryRun bool) error {
	repo, err := a.parseRepo(word)
	if err != nil {
		return err
	}
	sel := selection{
		verb: "init", fixed: []string{repo.Input}, label: "load", words: words,
		next: func([]string, []selector.Selector, *vault.Index) ([]string, []note, error) {
			return words, nil, nil
		},
	}
	if _, err := sel.parseWords(); err != nil {
		return err
	}
	root, err := a.userRoot()
	if err != nil {
		return err
	}
	if dryRun {
		return a.initDryRun(ctx, repo, sel, root)
	}
	retry := initCommand(repo.Input)
	if v, err := vault.Open(a.git, a.paths.Vault); err == nil && len(words) > 0 {
		if err := a.holdsOnly(ctx, v, repo); err != nil {
			return hinted(err, retry)
		}
		return a.loading(ctx, sel, false)
	}
	err = a.locked(ctx, sel.command(), func(ctx context.Context, c *Change) (bool, error) {
		v, err := vault.Open(a.git, a.paths.Vault)
		switch {
		case err == nil:
			return false, a.holdsOnly(ctx, v, repo)
		case errors.Is(err, vault.ErrNotClone):
			return false, a.notClone(err, retry)
		case !errors.Is(err, vault.ErrNoVault):
			return false, err
		}
		m, err := a.makeVault(ctx, repo, root)
		if err != nil {
			return false, err
		}
		c.Vault = m.vault
		undoVault := func() { _ = os.RemoveAll(a.paths.Vault) } // init made all of it just now
		c.Undo = undoVault
		if len(m.imports.found) == 0 && len(words) == 0 {
			a.reportVault(m)
			return true, nil
		}
		var between func()
		if m.cloned {
			a.reportVault(m)
		} else {
			between = func() { a.reportVault(m) }
		}
		return true, a.adoptAndLoad(ctx, c, m.imports, &sel, between, undoVault)
	})
	return hinted(err, retry)
}

func (a *App) parseRepo(word string) (ref.Repo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ref.Repo{}, fmt.Errorf("init: %w", err)
	}
	repo, err := ref.ParseRepo(word, a.refOptions(cwd))
	if err != nil {
		return ref.Repo{}, errs.Usagef("skl help init", "%w", err)
	}
	return repo, nil
}

func (a *App) refOptions(cwd string) ref.Options {
	return ref.Options{
		DefaultHost: a.cfg.DefaultHost,
		SSH:         a.cfg.Protocol == config.SSH,
		Home:        a.getenv("HOME"),
		Cwd:         cwd,
	}
}

// initDryRun prints what init would do: make or clone the vault, import the
// skills it would offer, load the selectors. It reads the remote, but it takes no
// lock and creates nothing, ~/.skl included. With no clone to look in, it lists
// every skill it would offer, even one the vault turns out to have.
func (a *App) initDryRun(ctx context.Context, repo ref.Repo, sel selection, root string) error {
	retry := initCommand(repo.Input)
	v, err := vault.Open(a.git, a.paths.Vault)
	switch {
	case err == nil:
		if err := a.holdsOnly(ctx, v, repo); err != nil {
			return hinted(err, retry)
		}
		if len(sel.words) > 0 {
			return a.loading(ctx, sel, true)
		}
		a.ui.Done("nothing to change")
		return nil
	case errors.Is(err, vault.ErrNotClone):
		return a.notClone(err, retry)
	case !errors.Is(err, vault.ErrNoVault):
		return hinted(err, retry)
	}
	empty, err := a.remoteEmpty(ctx, repo)
	if err != nil {
		return hinted(err, retry)
	}
	a.planVault(repo, empty)
	offer, err := a.importable(root, nil)
	if err != nil {
		return hinted(err, retry)
	}
	if group, err := selector.OwnerGroup(repo.Owner()); err == nil && len(offer) > 0 {
		a.ui.Plan("import %s → %s/", folderList(offer), group)
		if err := a.planImportLoad(ctx, sel, group, offer, root); err != nil {
			return hinted(err, retry)
		}
	}
	if len(sel.words) > 0 {
		a.ui.Plan("load %s", strings.Join(sel.words, " "))
	}
	return nil
}

// planImportLoad prints what loading the imports would change, as the real run
// does after "◇ imported": each folder stays where it is as the loaded copy, and
// with two targets the skill is loaded into the other one too. The plan is made
// against a vault holding only the imports, written to a temporary folder, with
// each folder read as the copy it would become.
func (a *App) planImportLoad(ctx context.Context, sel selection, group string, offer []load.Foreign, root string) error {
	tmp, err := os.MkdirTemp("", "skl-init-")
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	dir := filepath.Join(tmp, "vault")
	adopted := make([]state.Copy, 0, len(offer))
	for _, f := range offer {
		dst := filepath.Join(dir, group, f.Folder)
		if err := load.CopyIn(ctx, f.Path(), dst); err != nil {
			return a.shownPaths(err)
		}
		tree, err := load.SkillTree(ctx, dst)
		if err != nil {
			return err
		}
		adopted = append(adopted, state.Copy{Name: f.Folder, Target: f.Target, Skill: group + "/" + f.Folder, Tree: tree})
	}
	idx, err := vault.IndexAt(ctx, dir)
	if err != nil {
		return err
	}
	// The group holds only the imports, so the real run adds the group.
	sel.saved = adopted
	sel.next = func([]string, []selector.Selector, *vault.Index) ([]string, []note, error) {
		return []string{group}, nil, nil
	}
	next, err := a.prepareAt(ctx, vaultView{dir: dir, idx: idx}, &state.State{}, sel, nil, root)
	if err != nil {
		return err
	}
	a.printChanges(next.plan.Changes)
	for _, n := range next.notes {
		a.ui.Warn(n.msg, ui.Clean(n.hint))
	}
	return nil
}

func (a *App) planVault(repo ref.Repo, empty bool) {
	if empty {
		a.ui.Plan("new vault %s", repo.Input)
		return
	}
	a.ui.Plan("clone %s", repo.Input)
}

// initCommand is "skl init <word>", as a hint.
func initCommand(word string) string { return ui.CommandLine([]string{"init", word}) }

// holdsOnly is nil if the vault's origin is repo: init again is fine. A machine
// has one vault, so any other repo stops.
func (a *App) holdsOnly(ctx context.Context, v *vault.Vault, repo ref.Repo) error {
	origin, err := v.Origin(ctx)
	if err != nil {
		return err
	}
	vaultDir := a.shown(a.paths.Vault)
	if origin == "" {
		return errs.Hint(fmt.Errorf("%s has no origin", vaultDir), "git -C "+vaultDir+" remote add origin "+ref.HidePassword(repo.URL))
	}
	held, err := ref.ParseRepo(origin, a.refOptions(a.paths.Vault))
	if err == nil && held.Same(repo) {
		return nil
	}
	name := ref.HidePassword(origin) // an origin skl can't read is shown as it is, password aside
	if err == nil {
		name = a.shortName(held)
	}
	return errs.Hint(fmt.Errorf("%s already holds %s", vaultDir, name), a.secondSetup(repo))
}

// shortName is a repo's shortest name, with ~ on this machine.
func (a *App) shortName(r ref.Repo) string {
	return a.shown(r.Short(a.cfg.DefaultHost))
}

// secondSetup is the hint for keeping a second vault apart: another SKL_HOME,
// named after the repo's owner, and not the one in use.
func (a *App) secondSetup(repo ref.Repo) string {
	owner := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, repo.Owner())
	name := ".skl-" + owner
	home := a.getenv("HOME")
	if filepath.Join(home, name) == a.paths.Home {
		name += "-2"
	}
	return "SKL_HOME=~/" + name + " " + initCommand(repo.Input)
}

// madeVault is what makeVault made: the vault in place, and the skills it
// imported, committed but not adopted yet.
type madeVault struct {
	vault   *vault.Vault
	cloned  bool
	counts  *vault.Counts // what the clone held, nil if it couldn't be counted
	imports imports
}

// makeVault clones repo into staging, asks which skills to import, copies them
// in, gives a new vault its first commit (the imported skills with it) or a clone
// a commit of the imports, and moves it into place. Until the move, stopping
// leaves only staging behind, and the deferred removal or the next init clears
// that.
func (a *App) makeVault(ctx context.Context, repo ref.Repo, root string) (*madeVault, error) {
	empty, err := a.remoteEmpty(ctx, repo)
	if err != nil {
		return nil, err
	}
	if empty {
		if err := vault.CheckIdentity(ctx, a.git, ""); err != nil {
			return nil, identityError(err)
		}
	}
	a.planVault(repo, empty)

	staged := filepath.Join(a.paths.Staging, "vault")
	if err := os.RemoveAll(staged); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if err := os.MkdirAll(a.paths.Staging, 0o755); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	defer func() { _ = os.RemoveAll(staged) }() // gone already once moved

	v, err := vault.Clone(ctx, a.git, repo.URL, staged)
	if err != nil {
		return nil, a.initReachError(err, repo)
	}
	// What the clone holds decides, not what ls-remote saw a moment ago: a
	// repo that got its first commit since then must not get a LICENSE.
	born, err := v.Born(ctx)
	if err != nil {
		return nil, branchError(err, repo)
	}
	var counts *vault.Counts
	if c, err := v.Count(ctx); err == nil && born {
		counts = &c
	}
	im, err := a.offerImports(ctx, v, repo, root)
	if err != nil {
		return nil, err
	}
	folders, err := copyImports(ctx, v, im.group, im.found)
	if err != nil {
		return nil, err
	}
	if !born {
		if err := v.Create(ctx, a.cfg.License, a.now(), folders...); err != nil {
			return nil, identityError(err)
		}
	} else if len(folders) > 0 {
		if _, err := v.Commit(ctx, importMessage(im.group, im.found), folders...); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := v.Move(a.paths.Vault); err != nil {
		return nil, err
	}
	return &madeVault{vault: v, cloned: born, counts: counts, imports: im}, nil
}

// offerImports asks whether to import the skills in the targets' folders that
// aren't in a vault, and which, into the group named after the repo's owner
// (docs/guide/getting-started.md "Make your vault"). A skill the clone already has
// isn't offered: its name is taken. v is the clone, still in staging.
func (a *App) offerImports(ctx context.Context, v *vault.Vault, repo ref.Repo, root string) (imports, error) {
	idx, err := v.Index(ctx)
	if err != nil {
		return imports{}, err
	}
	offer, err := a.importable(root, idx)
	if err != nil || len(offer) == 0 {
		return imports{}, err
	}
	group, err := selector.OwnerGroup(repo.Owner())
	if err != nil {
		a.ui.Warn(err.Error(), ui.CommandLine([]string{"import", offer[0].Folder, "--group", "<group>"}))
		return imports{}, nil
	}
	var dirs []string
	for _, f := range offer {
		if d := a.shown(f.Dir); !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	chosen, err := pickImports(ctx, a.ui.Ask, offer, strings.Join(dirs, " and "), group)
	if err != nil {
		return imports{}, err
	}
	// The group gets the selector when it holds only what's imported now.
	return imports{group: group, found: chosen, whole: len(idx.Skills(group)) == 0}, nil
}

// importable is the skills skl could import: someone else's skills in the
// targets' folders, one per folder name, the first target's first, whose names
// are valid and, with idx, not taken in the vault.
func (a *App) importable(root string, idx *vault.Index) ([]load.Foreign, error) {
	all, err := load.Foreigners(root, a.cfg.Targets)
	if err != nil {
		return nil, a.shownPaths(err)
	}
	var out []load.Foreign
	for _, f := range all {
		if !selector.ValidName(f.Folder) || slices.ContainsFunc(out, func(o load.Foreign) bool {
			return selector.Fold(o.Folder) == selector.Fold(f.Folder)
		}) {
			continue
		}
		if idx != nil {
			if _, taken := vaultSkillNamed(idx, f.Folder, f.Name); taken {
				continue
			}
		}
		// One that shares a name with a skill already offered would clash with it
		// in the group: it isn't offered, and the warning says how to get it in.
		if j := slices.IndexFunc(out, f.Clashes); j >= 0 {
			a.ui.Warn(fmt.Sprintf("%s   not offered: same name as %s", ui.Clean(f.Folder), ui.Clean(out[j].Folder)),
				"rename it in "+a.shown(f.Dir)+", then skl import it")
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// pickImports asks "Import the 3 skills in ~/.claude/skills into you/?": yes takes
// them all, choose asks about each, no takes none.
func pickImports(ctx context.Context, ask func(context.Context, ui.Question) (int, error),
	offer []load.Foreign, dirs, group string,
) ([]load.Foreign, error) {
	q := ui.Question{
		Prompt:  fmt.Sprintf("Import the %d skills in %s into %s/?", len(offer), dirs, group),
		Answers: []string{"yes", "choose", "no"},
	}
	if len(offer) == 1 {
		q = ui.Question{
			Prompt:  fmt.Sprintf("Import %s in %s into %s/?", ui.Clean(offer[0].Folder), dirs, group),
			Answers: []string{"yes", "no"},
		}
	}
	answer, err := ask(ctx, q)
	if err != nil {
		return nil, err
	}
	switch q.Answers[answer] {
	case "yes":
		return offer, nil
	case "no":
		return nil, nil
	}
	var chosen []load.Foreign
	for _, f := range offer {
		answer, err := ask(ctx, ui.Question{Prompt: "Import " + ui.Clean(f.Folder) + "?", Answers: []string{"yes", "no"}})
		if err != nil {
			return nil, err
		}
		if answer == 0 {
			chosen = append(chosen, f)
		}
	}
	return chosen, nil
}

// remoteEmpty reads the remote before anything is made, with the ✗ line for one
// that can't be a vault.
func (a *App) remoteEmpty(ctx context.Context, repo ref.Repo) (bool, error) {
	empty, err := vault.RemoteEmpty(ctx, a.git, repo.URL)
	if err != nil {
		return false, branchError(a.initReachError(err, repo), repo)
	}
	return empty, nil
}

// branchError is a repo with nothing to check out, with what to do about it.
func branchError(err error, repo ref.Repo) error {
	switch {
	case errors.Is(err, vault.ErrNoDefaultBranch):
		return errs.Hint(fmt.Errorf("%s %w", repo.Input, err), "set one on your git host, then "+initCommand(repo.Input))
	case errors.Is(err, vault.ErrNoBranches):
		return errs.Hint(fmt.Errorf("%s %w", repo.Input, err), "push a branch, then "+initCommand(repo.Input))
	}
	return err
}

// identityError gives vault.ErrNoIdentity the commands that set an identity.
func identityError(err error) error {
	if errors.Is(err, vault.ErrNoIdentity) {
		return errs.Hint(vault.ErrNoIdentity,
			`git config --global user.name "You"`+"\n    git config --global user.email you@example.com")
	}
	return err
}

// initReachError is a repo init can't reach. skl never creates one, and hosts
// answer "not found" for a missing repo and a private one alike, so it says
// both. Any other failure of the network or the remote says that instead.
func (a *App) initReachError(err error, repo ref.Repo) error {
	retry := initCommand(repo.Input)
	if !errors.Is(err, gitx.ErrUnreachable) {
		return err
	}
	if reason := networkReason(err); reason != "" && repo.Host != "" {
		return errs.Hint(fmt.Errorf("can't reach %s (%s)", repo.Host, reason), retry)
	}
	return errs.Hint(fmt.Errorf("can't reach %s", repo.Input), "create it or check access, then "+retry)
}

// reportVault prints what init made: "◇ LICENSE" for a new vault, what it holds
// for a clone, as cloned, before anything was imported.
func (a *App) reportVault(m *madeVault) {
	switch {
	case !m.cloned:
		if a.cfg.License != vault.NoLicense {
			a.ui.StepHint("LICENSE", a.cfg.License+", for the skills you write", "edit LICENSE to change it")
		}
	case m.counts != nil: // only a report: without it, the clone is in place all the same
		a.ui.Step("cloned", countsLine(*m.counts))
	}
}

// countsLine is "3 groups · 14 skills · 2 skill sets", or "no skills yet".
func countsLine(c vault.Counts) string {
	if c.Skills == 0 && c.Sets == 0 {
		return "no skills yet"
	}
	var parts []string
	for _, p := range []struct {
		n          int
		one, other string
	}{{c.Groups, "group", "groups"}, {c.Skills, "skill", "skills"}, {c.Sets, "skill set", "skill sets"}} {
		switch {
		case p.n == 1:
			parts = append(parts, "1 "+p.one)
		case p.n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.other))
		}
	}
	return strings.Join(parts, " · ")
}
