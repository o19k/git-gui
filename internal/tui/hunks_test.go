package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

const filePatch = `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,4 @@
 one
+added at the top
 two
 three
@@ -8,3 +9,3 @@
 eight
-nine
+NINE
 ten
`

// hunkFixture is a model showing filePatch for a modified file.
func hunkFixture(t *testing.T) Model {
	t.Helper()
	m := fixture(t)
	m.focus = PanelFiles
	m.cursor[PanelFiles] = 1 // dirty.go: modified, not staged
	m.previewStaged = false
	m.mainContent = filePatch
	return m
}

func TestHunkRangesCoverEachHunk(t *testing.T) {
	ranges := hunkRanges(filePatch)
	if len(ranges) != 2 {
		t.Fatalf("got %d ranges, want 2: %+v", len(ranges), ranges)
	}

	lines := strings.Split(strings.TrimRight(filePatch, "\n"), "\n")
	for i, r := range ranges {
		if !strings.HasPrefix(lines[r.start], "@@") {
			t.Errorf("range %d starts at %q, not a hunk header", i, lines[r.start])
		}
		if r.end <= r.start || r.end > len(lines) {
			t.Errorf("range %d is out of bounds: %+v", i, r)
		}
	}
	if ranges[0].end != ranges[1].start {
		t.Errorf("ranges leave a gap or overlap: %+v", ranges)
	}
	if ranges[1].end != len(lines) {
		t.Errorf("the last range stops short of the patch: %+v vs %d lines", ranges[1], len(lines))
	}
}

func TestHunkRangesOnAnEmptyDiff(t *testing.T) {
	if got := hunkRanges(""); len(got) != 0 {
		t.Errorf("an empty diff produced ranges: %+v", got)
	}
}

func TestEnterOpensHunkModeAndEscapeLeaves(t *testing.T) {
	m := hunkFixture(t)

	m, cmd := press(t, m, "enter")
	if cmd != nil {
		t.Error("opening hunk mode should not run a git command")
	}
	if !m.hunkMode || m.hunkCursor != 0 {
		t.Fatalf("hunk mode did not open: mode=%v cursor=%d", m.hunkMode, m.hunkCursor)
	}
	if !strings.Contains(m.View(), "hunk 1/2") {
		t.Error("the pane title does not say which hunk is selected")
	}

	m, _ = press(t, m, "esc")
	if m.hunkMode {
		t.Error("esc did not leave hunk mode")
	}
}

func TestHunkModeMovesBetweenHunks(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")

	m, _ = press(t, m, "j")
	if m.hunkCursor != 1 {
		t.Errorf("j moved to hunk %d, want 1", m.hunkCursor)
	}
	if !strings.Contains(m.View(), "hunk 2/2") {
		t.Error("the title did not follow the selection")
	}

	// The cursor must not run off either end.
	m, _ = press(t, m, "j")
	if m.hunkCursor != 1 {
		t.Errorf("j past the last hunk moved to %d", m.hunkCursor)
	}
	m, _ = press(t, m, "k")
	m, _ = press(t, m, "k")
	if m.hunkCursor != 0 {
		t.Errorf("k past the first hunk moved to %d", m.hunkCursor)
	}
}

func TestHunkModeMarksOnlyTheSelectedHunk(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")
	m.width, m.height = 200, 40 // wide enough that nothing is truncated away

	ranges := hunkRanges(filePatch)
	marked := hunkPaneLines(filePatch, ranges, 0, 0, 40)

	bars := 0
	for _, line := range marked {
		if strings.Contains(line, "▌") {
			bars++
		}
	}
	want := ranges[0].end - ranges[0].start
	if bars != want {
		t.Errorf("%d lines marked, want %d (the first hunk's size)", bars, want)
	}
}

func TestHunkModeStagesTheSelectedHunk(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")

	_, cmd := press(t, m, " ")
	if cmd == nil {
		t.Error("space did not stage the hunk")
	}
}

func TestHunkModeUnstagesWhenShowingTheStagedDiff(t *testing.T) {
	m := hunkFixture(t)
	m.previewStaged = true
	m, _ = press(t, m, "enter")

	if !strings.Contains(m.View(), "unstage") {
		t.Error("the title should say space will unstage, not stage")
	}
	if _, cmd := press(t, m, " "); cmd == nil {
		t.Error("space produced no command")
	}
}

func TestHunkModeRefusesUntrackedFiles(t *testing.T) {
	m := fixture(t)
	m.focus = PanelFiles
	m.cursor[PanelFiles] = 2 // new.go, untracked
	m.mainContent = "brand new file contents\n"

	m, cmd := press(t, m, "enter")
	if cmd != nil || m.hunkMode {
		t.Error("an untracked file has no hunks to open")
	}
	if !strings.Contains(m.status, "untracked") {
		t.Errorf("status = %q", m.status)
	}
}

func TestHunkModeClosesWhenTheLastHunkIsStaged(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")

	// A refresh arriving with nothing left to stage for this file.
	m.mainContent = ""
	next, _ := m.Update(snapshotMsg(git.Snapshot{
		Branch: "main",
		Files:  []git.FileChange{{Index: 'M', Work: '.', Path: "staged.go"}},
	}))
	m = next.(Model)

	if m.hunkMode {
		t.Error("hunk mode stayed open with no hunks left")
	}
}

func TestHunkModeClampsAfterTheDiffShrinks(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")
	m, _ = press(t, m, "j") // on the second hunk

	// Staging it leaves a one-hunk diff behind.
	oneHunk := strings.Split(filePatch, "@@ -8,3 +9,3 @@")[0]
	next, _ := m.Update(previewMsg{key: m.previewKey, title: "Diff", content: oneHunk})
	m = next.(Model)

	if m.hunkCursor != 0 {
		t.Errorf("cursor left dangling at hunk %d after the diff shrank", m.hunkCursor)
	}
	if strings.Contains(m.View(), "panic") {
		t.Error("the view broke after the diff shrank")
	}
}

func TestHunkModeDoesNotTearTheFrame(t *testing.T) {
	m, _ := press(t, hunkFixture(t), "enter")

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Errorf("view is %d lines, terminal is %d", len(lines), m.height)
	}
}

func TestHunkModeFooterReplacesThePanelKeys(t *testing.T) {
	// j/k and space are rebound here, so the Files bindings must not show.
	m, _ := press(t, hunkFixture(t), "enter")

	footer := m.footer()
	if !strings.Contains(footer, "back to files") {
		t.Errorf("hunk mode footer is missing its own keys: %q", footer)
	}
	if strings.Contains(footer, "commit") || strings.Contains(footer, "discard") {
		t.Errorf("hunk mode footer still shows the Files bindings: %q", footer)
	}
}
