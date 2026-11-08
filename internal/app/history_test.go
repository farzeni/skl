package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/farzeni/skl/internal/state"
)

// writeGenerations puts generations on disk as the state package writes them.
func (h *harness) writeGenerations(t *testing.T, gens ...state.Generation) {
	t.Helper()
	if err := os.MkdirAll(h.a.paths.Generations, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, g := range gens {
		data, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.a.paths.Generations, strconv.Itoa(g.N)+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// docs/guide/rollback.md "See what changed": the list, newest first, with the
// relative times of a fixed clock.
func TestHistoryText(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	now := h.a.now()
	h.writeGenerations(t,
		state.Generation{N: 9, Time: now.Add(-3*24*time.Hour - time.Hour), Command: "use @backend"},
		state.Generation{N: 10, Time: now.Add(-2*time.Hour - 10*time.Minute), Command: "saved pr-review → review/pr-review"},
		state.Generation{N: 11, Time: now.Add(-2 * time.Hour), Command: "use @backend writing/changelog"},
		state.Generation{N: 12, Time: now.Add(-20 * time.Second), Command: "learn deep-pr-review → review/pr-review"},
	)
	if err := os.Remove(filepath.Join(h.a.paths.Generations, "1.json")); err != nil {
		t.Fatal(err)
	}
	if err := h.a.History(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	want := "12  now      learn deep-pr-review → review/pr-review\n" +
		"11  2h ago   use @backend writing/changelog\n" +
		"10  2h ago   saved pr-review → review/pr-review\n" +
		" 9  3d ago   use @backend\n"
	if got := h.stdout.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if h.stderr.Len() > 0 {
		t.Errorf("stderr: %s", h.stderr.String())
	}
}

// docs/reference/commands.md "JSON": the history object, times in UTC.
func TestHistoryJSON(t *testing.T) {
	h := newHarness(t)
	h.vaulted(t)
	commit := "9b1d4e0c2f7a8e35d6b1c0f4a9e2d7b3c5f8a1e6"
	h.writeGenerations(t, state.Generation{
		N: 12, Time: time.Date(2026, 10, 22, 16, 3, 0, 0, time.FixedZone("CEST", 2*3600)),
		Command: "learn deep-pr-review → review/pr-review", VaultCommit: commit,
	})
	if err := h.a.History(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	var got historyJSON
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	first := got.Generations[0]
	if len(got.Generations) != 2 || first.Generation != 12 || first.Time != "2026-10-22T14:03:00Z" ||
		first.Command != "learn deep-pr-review → review/pr-review" || first.Commit != commit {
		t.Errorf("got %+v", got)
	}
	if init := got.Generations[1]; init.Generation != 1 || init.Command != "init "+filepath.Join(h.home, "you", "skills.git") || len(init.Commit) != 40 {
		t.Errorf("generation 1: %+v", init)
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{-time.Hour, "now"}, // a clock that went back
		{0, "now"},
		{59 * time.Second, "now"},
		{time.Minute, "1m ago"},
		{59*time.Minute + 59*time.Second, "59m ago"},
		{time.Hour, "1h ago"},
		{2*time.Hour + 59*time.Minute, "2h ago"},
		{23 * time.Hour, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{3*24*time.Hour + 23*time.Hour, "3d ago"},
		{400 * 24 * time.Hour, "400d ago"},
	} {
		if got := age(now, now.Add(-tc.ago)); got != tc.want {
			t.Errorf("age(%v) = %q, want %q", tc.ago, got, tc.want)
		}
	}
}
