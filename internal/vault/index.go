package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"

	"github.com/farzeni/skl/internal/resolve"
	"github.com/farzeni/skl/internal/selector"
)

var _ resolve.Index = (*Index)(nil)

// Index is what the vault's folder holds, as docs/reference/files.md "Your vault"
// lays it out. It reports names as they are on disk; resolve decides what they
// mean. The slices it returns are its own: don't change them.
type Index struct {
	dir          string
	groups       []string
	skills       map[string][]resolve.Skill
	descriptions map[resolve.Skill]string
	sets         []string
	warnings     []Warning
}

// Warning is something in the vault that skl works around. It holds no command:
// app builds the hint.
type Warning struct {
	Path   string // from the vault's root, with any invalid UTF-8 replaced
	Reason string // one line
}

// Message returns the warning without a hint: "review/x/SKILL.md   no frontmatter".
func (w Warning) Message() string { return w.Path + "   " + w.Reason }

// BadName reports a folder or file skipped for its name, as opposed to a skill that
// loads with something wrong in it.
func (w Warning) BadName() bool { return w.Reason == badNameReason }

// SetError is a named skill set whose file can't be used.
type SetError struct {
	File string // from the vault's root: sets/<name>.toml
	Err  error
}

func (e *SetError) Error() string { return e.File + ": " + e.Err.Error() }

func (e *SetError) Unwrap() error { return e.Err }

// AmbiguousSetError is a name that matches two or more skill sets that differ
// only by case or Unicode form, and no set exactly.
type AmbiguousSetError struct {
	Name string   // as typed
	Sets []string // the vault's sets, as written
}

func (e *AmbiguousSetError) Error() string {
	return fmt.Sprintf("@%s matches %d skill sets: @%s", e.Name, len(e.Sets), strings.Join(e.Sets, ", @"))
}

// Index reads the vault's folder: every top-level folder except the reserved
// and hidden ones is a group, and every folder in a group that holds a SKILL.md
// is a skill; a hidden folder is neither. Set files are listed here and read by
// Set, so a broken one stops only what uses it. A folder that can't be listed is
// an error; anything else wrong is a warning.
func (v *Vault) Index(ctx context.Context) (*Index, error) {
	top, err := os.ReadDir(v.dir)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	idx := &Index{
		dir:          v.dir,
		skills:       make(map[string][]resolve.Skill),
		descriptions: make(map[resolve.Skill]string),
	}
	for _, g := range top {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := g.Name()
		if !g.IsDir() || selector.ReservedGroup(name) || strings.HasPrefix(name, ".") {
			continue
		}
		if !utf8.ValidString(name) {
			idx.warnName(name)
			continue
		}
		if err := idx.readGroup(name); err != nil {
			return nil, err
		}
	}
	if err := idx.listSets(); err != nil {
		return nil, err
	}
	return idx, nil
}

// Groups returns the vault's groups, sorted by their bytes.
func (i *Index) Groups() []string { return i.groups }

// Skills returns a group's skills, sorted by their folder's bytes, or nothing if
// the group doesn't exist.
func (i *Index) Skills(group string) []resolve.Skill { return i.skills[group] }

// Description returns the description in a skill's frontmatter, "" if it has none.
func (i *Index) Description(s resolve.Skill) string { return i.descriptions[s] }

// Sets returns the names of the named skill sets, sorted by their bytes.
func (i *Index) Sets() []string { return i.sets }

// Warnings returns what's wrong in the vault, in the order it was read.
func (i *Index) Warnings() []Warning { return i.warnings }

// Set returns the lines of a named skill set. It reads the file each time. A
// name is found in any case or Unicode form, as a group or a skill is. A set
// that doesn't exist is resolve.ErrNoSet, and a file it can't use a *SetError.
func (i *Index) Set(name string) ([]string, error) {
	found, err := i.findSet(name)
	if err != nil {
		return nil, err
	}
	file := "sets/" + found + ".toml"
	lines, err := readSet(filepath.Join(i.dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, &SetError{File: file, Err: readable(err)}
	}
	return lines, nil
}

// findSet picks the set a name means: the exact one, which keeps every set
// nameable, else the only one that folds the same. Looking in the list, never
// building a path from the name, is what keeps .origin.toml files and other
// folders out of reach.
func (i *Index) findSet(name string) (string, error) {
	if slices.Contains(i.sets, name) {
		return name, nil
	}
	want := selector.Fold(name)
	var found []string
	for _, s := range i.sets {
		if selector.Fold(s) == want {
			found = append(found, s)
		}
	}
	switch len(found) {
	case 0:
		return "", resolve.ErrNoSet
	case 1:
		return found[0], nil
	}
	return "", &AmbiguousSetError{Name: name, Sets: found}
}

func (i *Index) readGroup(group string) error {
	entries, err := os.ReadDir(filepath.Join(i.dir, group))
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	i.groups = append(i.groups, group)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			i.readSkill(group, e.Name())
		}
	}
	return nil
}

// readSkill adds a folder to its group if it holds a SKILL.md. One that can't be
// read or parsed is still a skill, named by its folder, with a warning.
func (i *Index) readSkill(group, folder string) {
	file := filepath.Join(i.dir, group, folder, skillFile)
	info, err := os.Stat(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err == nil && !info.Mode().IsRegular():
		return // a folder or a pipe named SKILL.md holds nothing to read
	}
	rel := path.Join(group, folder)
	if !utf8.ValidString(folder) {
		i.warnName(rel)
		return
	}
	skill := resolve.Skill{Group: group, Folder: folder}
	if err == nil {
		var fm Frontmatter
		if fm, err = readFrontmatter(file); err == nil {
			skill.Name = fm.Name
			if fm.Description != "" {
				i.descriptions[skill] = fm.Description
			}
		}
	}
	if err != nil {
		i.warn(path.Join(rel, skillFile), oneLine(err))
	}
	i.skills[group] = append(i.skills[group], skill)
}

// listSets lists sets/<name>.toml. The folder is optional.
func (i *Index) listSets() error {
	entries, err := os.ReadDir(filepath.Join(i.dir, "sets"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	for _, e := range entries {
		file := e.Name()
		name, isSet := strings.CutSuffix(file, ".toml")
		if e.IsDir() || !isSet || strings.HasSuffix(file, ".origin.toml") || strings.HasPrefix(file, ".") {
			continue
		}
		if !utf8.ValidString(file) {
			i.warnName("sets/" + file)
			continue
		}
		i.sets = append(i.sets, name)
	}
	return nil
}

func (i *Index) warn(rel, reason string) {
	i.warnings = append(i.warnings, Warning{Path: strings.ToValidUTF8(rel, "�"), Reason: reason})
}

// warnName skips a name that can't be typed, printed or compared.
func (i *Index) warnName(rel string) { i.warn(rel, badNameReason) }

const badNameReason = "skipped: the name isn't valid UTF-8"

// oneLine fits an error from a parser on a warning's one line, and drops the
// absolute path a read error starts with: Warning.Path says where.
func oneLine(err error) string {
	return strings.Join(strings.Fields(readable(err).Error()), " ")
}

// readable drops the absolute path a read error starts with, for a message that
// already says which file.
func readable(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return fmt.Errorf("can't read it: %w", pe.Err)
	}
	return err
}

// readSet reads a set file's select list. A missing one is an error, not an empty
// set: skl use @x with a mistyped key would otherwise unload everything.
func readSet(file string) ([]string, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("isn't a file")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseSet(data)
}

// ParseSet reads a set file's select list from its content.
func ParseSet(data []byte) ([]string, error) {
	var set struct {
		Select []string `toml:"select"`
	}
	md, err := toml.Decode(string(data), &set)
	if err != nil {
		return nil, err
	}
	if !md.IsDefined("select") {
		return nil, errors.New(`has no select = [...]`)
	}
	return set.Select, nil
}

const skillFile = "SKILL.md"

// Frontmatter is what skl reads from the YAML at the top of a SKILL.md.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func readFrontmatter(file string) (Frontmatter, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Frontmatter{}, err
	}
	return ParseFrontmatter(data)
}

// ParseFrontmatter reads the YAML between a first line of --- and the next line
// of ---, whatever follows them. Other fields are left alone, but they must parse.
func ParseFrontmatter(data []byte) (Frontmatter, error) {
	var fm Frontmatter
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	if !isFence(first) {
		return fm, errors.New("no frontmatter")
	}
	var head []byte
	for line := range bytes.Lines(rest) {
		if isFence(line) {
			if err := yaml.Unmarshal(head, &fm); err != nil {
				return Frontmatter{}, fmt.Errorf("invalid frontmatter: %w", err)
			}
			return fm, nil
		}
		head = append(head, line...)
	}
	return fm, errors.New("frontmatter isn't closed")
}

func isFence(line []byte) bool { return string(bytes.TrimRight(line, " \t\r\n")) == "---" }
