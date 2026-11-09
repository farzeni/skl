package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/load"
	"github.com/farzeni/skl/internal/ui"
)

// docs/guide/getting-started.md "Make your vault": "? Import the 3 skills in
// ~/.claude/skills into you/?  › yes   choose   no"; "To pick which skills to
// import, answer choose: skl asks about each one."
func TestPickImports(t *testing.T) {
	three := []load.Foreign{{Folder: "pr-review"}, {Folder: "my-notes"}, {Folder: "sql-explain"}}
	for _, tt := range []struct {
		name    string
		offer   []load.Foreign
		answers []int
		asked   []string
		want    []string
		err     error
	}{
		{
			name: "yes takes all", offer: three, answers: []int{0},
			asked: []string{"Import the 3 skills in ~/.claude/skills into you/? yes/choose/no"},
			want:  []string{"pr-review", "my-notes", "sql-explain"},
		},
		{
			name: "no takes none", offer: three, answers: []int{2},
			asked: []string{"Import the 3 skills in ~/.claude/skills into you/? yes/choose/no"},
		},
		{
			name: "choose asks about each", offer: three, answers: []int{1, 0, 1, 0},
			asked: []string{
				"Import the 3 skills in ~/.claude/skills into you/? yes/choose/no",
				"Import pr-review? yes/no", "Import my-notes? yes/no", "Import sql-explain? yes/no",
			},
			want: []string{"pr-review", "sql-explain"},
		},
		{
			name: "one skill", offer: three[:1], answers: []int{0},
			asked: []string{"Import pr-review in ~/.claude/skills into you/? yes/no"},
			want:  []string{"pr-review"},
		},
		{
			name: "Ctrl-C", offer: three, answers: []int{1, 1},
			asked: []string{"Import the 3 skills in ~/.claude/skills into you/? yes/choose/no", "Import pr-review? yes/no"},
			err:   context.Canceled,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var asked []string
			ask := func(_ context.Context, q ui.Question) (int, error) {
				asked = append(asked, q.Prompt+" "+strings.Join(q.Answers, "/"))
				a := tt.answers[len(asked)-1]
				if tt.err != nil && len(asked) == len(tt.answers) {
					return a, tt.err
				}
				return a, nil
			}
			got, err := pickImports(context.Background(), ask, tt.offer, "~/.claude/skills", "you")
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			var names []string
			for _, f := range got {
				names = append(names, f.Folder)
			}
			if !slices.Equal(names, tt.want) || !slices.Equal(asked, tt.asked) {
				t.Errorf("got %q after %q, want %q after %q", names, asked, tt.want, tt.asked)
			}
		})
	}
}

// docs/reference/commands.md "Output": a change that can't be recorded as a
// generation is taken back, so the command can run again: the vault's commit,
// the folder copied in, and the loaded skill's record.
func TestImportTakenBackWithoutGeneration(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.vaulted(t)
	skill := filepath.Join(h.home, ".claude", "skills", "notes")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: notes\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head := func() string {
		out, err := exec.Command("git", "-C", h.a.paths.Vault, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := head()
	gens := filepath.Join(h.a.paths.Home, "generations")
	if err := os.RemoveAll(gens); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gens, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := h.a.Import(ctx, []string{"notes"}, "mine", false)
	if err == nil {
		t.Fatal("import made no generation, and said nothing")
	}
	// It got as far as the generation: the import itself was done.
	if !strings.Contains(h.stderr.String(), "◇ imported   notes → mine/") || !strings.Contains(err.Error(), "generations") {
		t.Fatalf("stopped before the generation: %v\n%s", err, h.stderr.String())
	}
	if head() != before {
		t.Error("the vault's commit is still there")
	}
	if _, err := os.Stat(filepath.Join(h.a.paths.Vault, "mine")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the group is still in the vault: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skill, ".skl-copy.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the folder is still adopted: %v", err)
	}
	st, err := h.a.store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range st.Scopes {
		if len(sc.Selectors) > 0 || len(sc.Copies) > 0 {
			t.Errorf("state.json keeps %+v", sc)
		}
	}

	// With generations back, it runs again.
	if err := os.Remove(gens); err != nil {
		t.Fatal(err)
	}
	if err := h.a.Import(ctx, []string{"notes"}, "mine", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skill, ".skl-copy.toml")); err != nil {
		t.Errorf("not adopted: %v", err)
	}
}
