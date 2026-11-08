package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SetFile is a named skill set's file, from the vault's root: sets/<name>.toml.
func SetFile(name string) string { return "sets/" + name + ".toml" }

// FindSet returns the name of the set a name means, as it is written on disk: the
// exact one, else the only one that folds the same (docs/guide/skill-sets.md
// "Names and clashes"). None is resolve.ErrNoSet, two or more an
// *AmbiguousSetError.
func (i *Index) FindSet(name string) (string, error) { return i.findSet(name) }

// FormatSet is a set file as skl writes it, one selector per line
// (docs/reference/files.md "sets/<name>.toml"), so two machines that change
// different lines merge without a conflict. skl writes it itself, not through a
// TOML encoder, to keep that layout exact.
func FormatSet(lines []string) []byte {
	if len(lines) == 0 {
		return []byte("select = []\n")
	}
	var b strings.Builder
	b.WriteString("select = [\n")
	for _, l := range lines {
		b.WriteString("  " + tomlString(l) + ",\n")
	}
	b.WriteString("]\n")
	return []byte(b.String())
}

// tomlString is s as a TOML basic string. A selector never needs escaping, but
// the file must stay valid TOML whatever the line holds.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// WriteSet writes the set name with lines, replacing its file in one step: a
// temp file next to it, renamed over it. It returns what puts the file back as it
// was, or removes it if there was none.
func (v *Vault) WriteSet(name string, lines []string) (restore func() error, err error) {
	file := filepath.Join(v.dir, filepath.FromSlash(SetFile(name)))
	old, err := os.ReadFile(file)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("vault: %w", err)
	}
	info, err := os.Lstat(file)
	mode := fs.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	if err := replaceFile(file, FormatSet(lines), mode); err != nil {
		return nil, err
	}
	return func() error {
		if !existed {
			if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("vault: %w", err)
			}
			return nil
		}
		return replaceFile(file, old, mode)
	}, nil
}

// replaceFile writes data to file through a temp file in its folder. A crash
// leaves the old file or the new one; a leftover temp file starts with a dot, so
// Index never takes it for a set.
func replaceFile(file string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".*.tmp")
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), file)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("vault: %w", err)
	}
	return nil
}
