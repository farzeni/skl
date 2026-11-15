package vault

import (
	"context"
	"slices"
	"testing"
)

// docs/guide/sync.md "By hand": skl leaves git's markers; skl sync --continue
// finishes the merge once they're gone. Open says what the merge brings, SidesOf
// what each side holds.
func TestOpenMergeContinue(t *testing.T) {
	ctx := context.Background()
	v, other := twoClones(t)
	commitFile(t, other, "a.txt", "one\ntheirs\nthree\n")
	run(t, other, "push", "--quiet", "origin", "main")
	commitFile(t, v.dir, "a.txt", "one\nmine\nthree\n")
	if err := v.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.Merge(ctx, "refs/remotes/origin/main"); err == nil {
		t.Fatal("Merge: no conflict")
	}
	open, err := v.Open(ctx)
	if err != nil || open.Incoming != 1 || open.Copy != "" || open.Theirs == "" {
		t.Errorf("Open = %+v, %v", open, err)
	}
	s, err := v.SidesOf(ctx, "a.txt")
	if err != nil || string(s.Base.Data) != "one\ntwo\nthree\n" || string(s.Ours.Data) != "one\nmine\nthree\n" ||
		string(s.Theirs.Data) != "one\ntheirs\nthree\n" {
		t.Errorf("SidesOf = %+v, %v", s, err)
	}
	if s, err := v.SidesOf(ctx, "gone.txt"); err != nil || !s.Base.Absent || !s.Ours.Absent || !s.Theirs.Absent {
		t.Errorf("SidesOf(gone.txt) = %+v, %v", s, err)
	}

	// The markers are still there.
	if left, err := v.Markers(ctx); err != nil || !slices.Equal(left, []string{"a.txt"}) {
		t.Errorf("Markers = %v, %v", left, err)
	}
	// Whitespace isn't a marker.
	write(t, v.dir+"/a.txt", "one\nboth  \nthree\n")
	if left, err := v.Markers(ctx); err != nil || len(left) != 0 {
		t.Errorf("Markers after the edit = %v, %v", left, err)
	}
	// docs/guide/sync.md "By hand": a line that is exactly ======= once the
	// conflict is resolved is a Markdown heading's underline, not a marker. Nor is
	// a lone >>>>>>> or <<<<<<<: only the pair is.
	write(t, v.dir+"/a.txt", "Title\n=======\none\nboth\n>>>>>>> quoted\nthree\n<<<<<<< open\n")
	if left, err := v.Markers(ctx); err != nil || len(left) != 0 {
		t.Errorf("Markers with a setext heading = %v, %v", left, err)
	}
	if err := v.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	if merging, err := v.Merging(ctx); err != nil || merging {
		t.Errorf("Merging = %v, %v", merging, err)
	}
	if parents := run(t, v.dir, "log", "-1", "--format=%P"); len(parents) < 81 {
		t.Errorf("not a merge commit: %q", parents)
	}
	if st := run(t, v.dir, "status", "--porcelain"); st != "" {
		t.Errorf("work tree not clean:\n%s", st)
	}
}

// Markers counts only the pair a change brings: a skill that shows git's markers
// as an example, committed, isn't a conflict when a heading is added above it.
func TestMarkersOnlyNewPairs(t *testing.T) {
	ctx := context.Background()
	v, _ := twoClones(t)
	example := "<<<<<<< ours\nmine\n=======\ntheirs\n>>>>>>> theirs\n"
	commitFile(t, v.dir, "doc.md", example)
	write(t, v.dir+"/doc.md", "Title\n=======\n"+example)
	write(t, v.dir+"/a.txt", "one\n<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> origin/main\n")
	if left, err := v.Markers(ctx); err != nil || !slices.Equal(left, []string{"a.txt"}) {
		t.Errorf("Markers = %v, %v", left, err)
	}
}

// A save-back's merge names its copy in the synthetic commit's Skl-Copy trailer.
func TestOpenSaveBack(t *testing.T) {
	ctx := context.Background()
	v, _ := twoClones(t)
	base := run(t, v.dir, "rev-parse", "HEAD")
	commitFile(t, v.dir, "a.txt", "one\nvault\nthree\n")
	write(t, v.dir+"/a.txt", "one\nloaded\nthree\n")
	run(t, v.dir, "add", "a.txt")
	tree := run(t, v.dir, "write-tree") // the loaded copy's side
	run(t, v.dir, "reset", "--quiet", "--hard", "HEAD")
	synthetic := run(t, v.dir, "commit-tree", tree, "-p", base, "-m", "skl: x as loaded\n\nSkl-Copy: claude pr-review\n")
	if err := v.leaveConflict(ctx, synthetic, "skl: save pr-review → review/pr-review", ""); err == nil {
		t.Fatal("leaveConflict: no conflict")
	}
	open, err := v.Open(ctx)
	if err != nil || open.Copy != "claude pr-review" || open.Theirs != synthetic {
		t.Errorf("Open = %+v, %v", open, err)
	}
}
