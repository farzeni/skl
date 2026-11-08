package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/farzeni/skl/internal/resolve"
)

// docs/reference/files.md "sets/<name>.toml": "skl writes one selector per line".
func TestFormatSet(t *testing.T) {
	for _, tc := range []struct {
		lines []string
		want  string
	}{
		{nil, "select = []\n"},
		{
			[]string{"@base", "review", "-review/test-gaps", "someone/pdf"},
			"select = [\n  \"@base\",\n  \"review\",\n  \"-review/test-gaps\",\n  \"someone/pdf\",\n]\n",
		},
		{[]string{"résumé"}, "select = [\n  \"résumé\",\n]\n"},
		{[]string{`a"b\c` + "\x01"}, "select = [\n  \"a\\\"b\\\\c\\u0001\",\n]\n"},
	} {
		got := string(FormatSet(tc.lines))
		if got != tc.want {
			t.Errorf("FormatSet(%q) = %q, want %q", tc.lines, got, tc.want)
		}
		// What skl writes, skl reads back as it was.
		file := filepath.Join(t.TempDir(), "x.toml")
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		back, err := readSet(file)
		if err != nil || len(back) != len(tc.lines) {
			t.Fatalf("readSet(%q) = %q, %v", got, back, err)
		}
		for i := range back {
			if back[i] != tc.lines[i] {
				t.Errorf("line %d read back as %q, want %q", i, back[i], tc.lines[i])
			}
		}
	}
}

func TestWriteSetRestores(t *testing.T) {
	v := &Vault{dir: t.TempDir()}
	file := filepath.Join(v.dir, "sets", "backend.toml")

	// A new file: restoring removes it.
	restore, err := v.WriteSet("backend", []string{"review"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != "select = [\n  \"review\",\n]\n" {
		t.Errorf("written: %q", got)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after restore: %v", err)
	}

	// An old file, hand-written and private: restoring puts its bytes and mode back.
	old := "# mine\nselect = [\"data\"]\n"
	if err := os.WriteFile(file, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	restore, err = v.WriteSet("backend", []string{"review"})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != old {
		t.Errorf("restored: %q", got)
	}
	// No temp file is left behind.
	if entries, _ := os.ReadDir(filepath.Dir(file)); len(entries) != 1 {
		t.Errorf("sets/ holds %d entries", len(entries))
	}
}

func TestFindSet(t *testing.T) {
	v := tree(t, map[string]string{
		"sets/backend.toml": "select = []\n",
		"sets/Base.toml":    "select = []\n",
	})
	idx, err := v.Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"backend": "backend", "BACKEND": "backend", "base": "Base"} {
		if got, err := idx.FindSet(name); err != nil || got != want {
			t.Errorf("FindSet(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := idx.FindSet("nope"); !errors.Is(err, resolve.ErrNoSet) {
		t.Errorf("FindSet(nope): %v", err)
	}
}
