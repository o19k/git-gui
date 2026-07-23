package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A patch built from part of a hunk is the one thing here git will reject
// outright if the arithmetic is wrong, so the counts in the header are checked
// against the body as well as against git's own verdict.

const mixedHunkPatch = `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,4 +1,4 @@
 one
-two
+TWO
-three
+THREE
 four
`

func TestPatchLinesKeepsOnlyTheChosenChange(t *testing.T) {
	diff := ParseFileDiff(mixedHunkPatch)

	// Body indices: 0 " one", 1 "-two", 2 "+TWO", 3 "-three", 4 "+THREE", 5 " four".
	patch, err := diff.PatchLines(0, map[int]bool{1: true, 2: true}, false)
	if err != nil {
		t.Fatalf("PatchLines: %v", err)
	}

	if !strings.Contains(patch, "+TWO") || !strings.Contains(patch, "-two") {
		t.Errorf("the chosen change is missing:\n%s", patch)
	}
	if strings.Contains(patch, "+THREE") {
		t.Errorf("an unchosen addition was carried in:\n%s", patch)
	}
	// The unchosen removal is still in the file being patched, so it has to
	// stay as context rather than disappear.
	if !strings.Contains(patch, " three") {
		t.Errorf("the unchosen removal was dropped instead of kept as context:\n%s", patch)
	}
	assertHeaderMatchesBody(t, patch)
}

// Unstaging applies the patch backwards, so the side that must survive as
// context is the other one.
func TestPatchLinesFlipsWhichSideIsContextWhenReversed(t *testing.T) {
	diff := ParseFileDiff(mixedHunkPatch)

	patch, err := diff.PatchLines(0, map[int]bool{1: true, 2: true}, true)
	if err != nil {
		t.Fatalf("PatchLines: %v", err)
	}

	if strings.Contains(patch, "-three") {
		t.Errorf("a removal the index does not hold was carried in:\n%s", patch)
	}
	if !strings.Contains(patch, " THREE") {
		t.Errorf("the unchosen addition must stay as context when reversing:\n%s", patch)
	}
	assertHeaderMatchesBody(t, patch)
}

func TestPatchLinesRefusesAnEmptySelection(t *testing.T) {
	diff := ParseFileDiff(mixedHunkPatch)
	if _, err := diff.PatchLines(0, map[int]bool{}, false); err == nil {
		t.Error("a patch with nothing chosen should be refused, not built")
	}
	if _, err := diff.PatchLines(9, map[int]bool{1: true}, false); err == nil {
		t.Error("an out-of-range hunk should error")
	}
}

// assertHeaderMatchesBody is what `git apply` checks first: the two counts in
// the @@ line against the lines that follow it.
func assertHeaderMatchesBody(t *testing.T, patch string) {
	t.Helper()

	diff := ParseFileDiff(patch)
	if len(diff.Hunks) != 1 {
		t.Fatalf("rebuilt patch has %d hunks, want 1", len(diff.Hunks))
	}
	hunk := diff.Hunks[0]

	fields := strings.Fields(hunk.Header)
	oldCount := rangeCount(t, fields[1])
	newCount := rangeCount(t, fields[2])

	gotOld, gotNew := 0, 0
	for _, line := range hunk.Lines {
		switch {
		case line == "" || (line[0] != '+' && line[0] != '-' && line[0] != '\\'):
			gotOld, gotNew = gotOld+1, gotNew+1
		case line[0] == '-':
			gotOld++
		case line[0] == '+':
			gotNew++
		}
	}

	if gotOld != oldCount || gotNew != newCount {
		t.Errorf("header says -%d +%d, body holds -%d +%d:\n%s",
			oldCount, newCount, gotOld, gotNew, patch)
	}
}

func rangeCount(t *testing.T, field string) int {
	t.Helper()
	_, count, found := strings.Cut(field[1:], ",")
	if !found {
		return 1
	}
	n := 0
	for _, r := range count {
		n = n*10 + int(r-'0')
	}
	return n
}

// The real check: git itself has to accept the patch and the index has to end
// up holding one of the two edits.
func TestStageLinesStagesOnlyTheChosenLine(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	// One hunk holding both edits, by asking for enough context to join them.
	opts := DiffOpts{Context: 20}
	raw, err := repo.Diff(ctx, "f.txt", false, opts)
	if err != nil {
		t.Fatal(err)
	}
	hunks := ParseFileDiff(raw).Hunks
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks with 20 lines of context, want them joined into 1", len(hunks))
	}

	chosen := map[int]bool{}
	for i, line := range hunks[0].Lines {
		if strings.HasPrefix(line, "-one") || strings.HasPrefix(line, "+ONE") {
			chosen[i] = true
		}
	}
	if len(chosen) != 2 {
		t.Fatalf("the first edit is not where it was expected in %v", hunks[0].Lines)
	}

	if err := repo.StageLines(ctx, "f.txt", 0, chosen, opts); err != nil {
		t.Fatalf("StageLines: %v", err)
	}

	staged, err := repo.Diff(ctx, "f.txt", true, DiffOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "+ONE") {
		t.Errorf("the chosen line was not staged:\n%s", staged)
	}
	if strings.Contains(staged, "+TEN") {
		t.Errorf("a line that was not chosen was staged too:\n%s", staged)
	}

	// And the working tree still holds both edits.
	data, err := os.ReadFile(filepath.Join(repo.Path, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "TEN") {
		t.Error("staging changed the file on disk")
	}
}

func TestUnstageLinesTakesBackOnlyTheChosenLine(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	if err := repo.Stage(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}

	opts := DiffOpts{Context: 20}
	raw, err := repo.Diff(ctx, "f.txt", true, opts)
	if err != nil {
		t.Fatal(err)
	}
	hunks := ParseFileDiff(raw).Hunks
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(hunks))
	}

	chosen := map[int]bool{}
	for i, line := range hunks[0].Lines {
		if strings.HasPrefix(line, "-one") || strings.HasPrefix(line, "+ONE") {
			chosen[i] = true
		}
	}

	if err := repo.UnstageLines(ctx, "f.txt", 0, chosen, opts); err != nil {
		t.Fatalf("UnstageLines: %v", err)
	}

	staged, err := repo.Diff(ctx, "f.txt", true, DiffOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(staged, "+ONE") {
		t.Errorf("the chosen line is still staged:\n%s", staged)
	}
	if !strings.Contains(staged, "+TEN") {
		t.Errorf("the other edit was taken out of the index too:\n%s", staged)
	}
}
