package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoHunkPatch = `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,4 @@
 one
+inserted near the top
 two
 three
@@ -8,3 +9,3 @@
 eight
-nine
+NINE
 ten
`

func TestParseFileDiffSplitsHunks(t *testing.T) {
	diff := ParseFileDiff(twoHunkPatch)

	if len(diff.Preamble) != 4 {
		t.Errorf("preamble = %q", diff.Preamble)
	}
	if len(diff.Hunks) != 2 {
		t.Fatalf("got %d hunks, want 2", len(diff.Hunks))
	}
	if !strings.HasPrefix(diff.Hunks[0].Header, "@@ -1,3 +1,4 @@") {
		t.Errorf("first header = %q", diff.Hunks[0].Header)
	}
	if got := diff.Hunks[0].Added(); got != 1 {
		t.Errorf("first hunk adds %d lines, want 1", got)
	}
	if got := diff.Hunks[1].Added(); got != 1 {
		t.Errorf("second hunk adds %d, want 1", got)
	}
	if got := diff.Hunks[1].Removed(); got != 1 {
		t.Errorf("second hunk removes %d, want 1", got)
	}
}

func TestPatchCarriesOnlyTheChosenHunk(t *testing.T) {
	diff := ParseFileDiff(twoHunkPatch)

	first, err := diff.Patch(0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "inserted near the top") {
		t.Error("hunk 0's own change is missing")
	}
	if strings.Contains(first, "NINE") {
		t.Error("hunk 0's patch leaked hunk 1's change")
	}
	if !strings.HasPrefix(first, "diff --git") || !strings.Contains(first, "+++ b/f.txt") {
		t.Errorf("patch is missing the preamble git needs:\n%s", first)
	}
	if !strings.HasSuffix(first, "\n") {
		t.Error("patch must end with a newline or git apply rejects it")
	}

	if _, err := diff.Patch(2); err == nil {
		t.Error("an out-of-range hunk should error")
	}
	if _, err := diff.Patch(-1); err == nil {
		t.Error("a negative hunk index should error")
	}
}

func TestParseEmptyDiff(t *testing.T) {
	diff := ParseFileDiff("")
	if len(diff.Hunks) != 0 {
		t.Errorf("an empty diff produced hunks: %+v", diff.Hunks)
	}
}

// multiHunkRepo makes a file with two well-separated edits, so staging one
// leaves the other behind.
func multiHunkRepo(t *testing.T) *Repo {
	t.Helper()
	repo, dir := newRepo(t)

	lines := []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.Stage(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add f.txt"); err != nil {
		t.Fatal(err)
	}

	// Two edits, far enough apart to land in separate hunks.
	edited := []string{"ONE", "two", "three", "four", "five", "six", "seven", "eight", "nine", "TEN"}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(edited, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestStageHunkStagesOnlyThatHunk(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	raw, err := repo.Diff(ctx, "f.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ParseFileDiff(raw).Hunks); n != 2 {
		t.Fatalf("fixture produced %d hunks, want 2:\n%s", n, raw)
	}

	if err := repo.StageHunk(ctx, "f.txt", 0); err != nil {
		t.Fatalf("StageHunk: %v", err)
	}

	staged, err := repo.Diff(ctx, "f.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "+ONE") {
		t.Errorf("the chosen hunk did not reach the index:\n%s", staged)
	}
	if strings.Contains(staged, "+TEN") {
		t.Errorf("the other hunk was staged too:\n%s", staged)
	}

	unstaged, err := repo.Diff(ctx, "f.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unstaged, "+TEN") {
		t.Errorf("the unchosen hunk vanished from the working tree diff:\n%s", unstaged)
	}
	if strings.Contains(unstaged, "+ONE") {
		t.Errorf("the staged hunk is still listed as unstaged:\n%s", unstaged)
	}
}

func TestStageHunkLeavesTheWorkingTreeAlone(t *testing.T) {
	// --cached, not --index: the file on disk must not move.
	repo := multiHunkRepo(t)
	ctx := context.Background()

	before, err := os.ReadFile(filepath.Join(repo.Path, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StageHunk(ctx, "f.txt", 0); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(repo.Path, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("staging a hunk rewrote the file on disk:\n%q\n%q", before, after)
	}
}

func TestUnstageHunkReversesOneHunk(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	// Stage both hunks, then take one back out.
	if err := repo.Stage(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}
	staged, _ := repo.Diff(ctx, "f.txt", true)
	if n := len(ParseFileDiff(staged).Hunks); n != 2 {
		t.Fatalf("expected 2 staged hunks, got %d", n)
	}

	if err := repo.UnstageHunk(ctx, "f.txt", 1); err != nil {
		t.Fatalf("UnstageHunk: %v", err)
	}

	staged, _ = repo.Diff(ctx, "f.txt", true)
	if !strings.Contains(staged, "+ONE") {
		t.Errorf("the kept hunk left the index:\n%s", staged)
	}
	if strings.Contains(staged, "+TEN") {
		t.Errorf("the reversed hunk is still staged:\n%s", staged)
	}

	unstaged, _ := repo.Diff(ctx, "f.txt", false)
	if !strings.Contains(unstaged, "+TEN") {
		t.Errorf("the reversed hunk did not come back as unstaged:\n%s", unstaged)
	}
}

func TestStageEveryHunkOneByOneMatchesStagingTheFile(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	// Staging hunk 0 must not shift the numbering for hunk 1.
	if err := repo.StageHunk(ctx, "f.txt", 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.StageHunk(ctx, "f.txt", 0); err != nil {
		t.Fatalf("staging the remaining hunk failed: %v", err)
	}

	if unstaged, _ := repo.Diff(ctx, "f.txt", false); strings.TrimSpace(unstaged) != "" {
		t.Errorf("changes left unstaged:\n%s", unstaged)
	}
	staged, _ := repo.Diff(ctx, "f.txt", true)
	if !strings.Contains(staged, "+ONE") || !strings.Contains(staged, "+TEN") {
		t.Errorf("staging hunk by hunk lost a change:\n%s", staged)
	}
}
