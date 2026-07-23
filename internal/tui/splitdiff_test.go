package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

const rewritePatch = `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,4 +1,4 @@
 one
-two
-three
+TWO
+THREE
 four
`

func TestSplitPairsARemovalWithWhatReplacedIt(t *testing.T) {
	rows := splitRows(rewritePatch)

	// Find the row holding the first rewrite.
	var paired *splitRow
	for i := range rows {
		if strings.Contains(rows[i].left, "two") {
			paired = &rows[i]
			break
		}
	}
	if paired == nil {
		t.Fatalf("the removed line is missing from the split: %+v", rows)
	}
	if !strings.Contains(paired.right, "TWO") {
		t.Errorf("the removal sits opposite %q, want the line that replaced it", paired.right)
	}
}

func TestSplitPutsContextOnBothSides(t *testing.T) {
	for _, r := range splitRows(rewritePatch) {
		if strings.HasSuffix(r.left, "one") {
			if r.left != r.right {
				t.Errorf("context line differs across the split: %q vs %q", r.left, r.right)
			}
			return
		}
	}
	t.Error("the context line is missing from the split")
}

func TestSplitTreatsFileHeadersAsHeaders(t *testing.T) {
	// "--- a/f.txt" and "+++ b/f.txt" only look like removals and additions.
	for _, r := range splitRows(rewritePatch) {
		if strings.HasPrefix(r.left, "---") || strings.HasPrefix(r.right, "+++") {
			t.Errorf("a file header was paired as content: %+v", r)
		}
	}
	headers := 0
	for _, r := range splitRows(rewritePatch) {
		if r.header != "" {
			headers++
		}
	}
	if headers != 5 { // diff, index, ---, +++, @@
		t.Errorf("got %d headers, want 5", headers)
	}
}

func TestSplitHandlesUnevenRuns(t *testing.T) {
	// Two lines removed, one added: the extra removal must still get a row.
	patch := "@@ -1,3 +1,2 @@\n-a\n-b\n+c\n"
	rows := splitRows(patch)

	var lefts, rights []string
	for _, r := range rows {
		if r.header != "" {
			continue
		}
		lefts = append(lefts, r.left)
		rights = append(rights, r.right)
	}
	if len(lefts) != 2 {
		t.Fatalf("got %d content rows, want 2: %+v", len(lefts), rows)
	}
	if !strings.Contains(lefts[1], "b") || rights[1] != "" {
		t.Errorf("the unmatched removal is wrong: left=%q right=%q", lefts[1], rights[1])
	}
}

func TestSplitOnAnEmptyPatch(t *testing.T) {
	if got := splitRows(""); len(got) != 0 {
		t.Errorf("an empty patch produced rows: %+v", got)
	}
}

func TestSplitLinesFitTheWidthExactly(t *testing.T) {
	const width = 61 // odd, so the halves cannot be equal
	lines := splitDiffLines(rewritePatch, 0, 20, width)

	// Headers span the pane and are padded by the frame; the two-column rows
	// are the ones that have to add up on their own.
	rows := 0
	for i, line := range lines {
		if !strings.Contains(line, splitDivider) {
			continue
		}
		rows++
		if w := lipgloss.Width(line); w != width {
			t.Errorf("line %d is %d columns, want %d: %q", i, w, width, line)
		}
	}
	if rows == 0 {
		t.Fatal("no two-column rows were rendered")
	}
}

func TestSplitFallsBackWhenTooNarrow(t *testing.T) {
	// Two unreadable columns are worse than one readable one.
	lines := splitDiffLines(rewritePatch, 0, 20, 3)
	for _, line := range lines {
		if strings.Contains(line, splitDivider) {
			t.Errorf("a divider was drawn in a 3-column pane: %q", line)
		}
	}
}

func TestVTogglesTheDiffLayout(t *testing.T) {
	m := fixture(t)
	m.mainContent = rewritePatch
	m.focus = PanelDiff

	m = key(t, m, "v")
	if !m.splitDiff {
		t.Fatal("v did not switch to the side-by-side view")
	}
	if !strings.Contains(m.View(), splitDivider) {
		t.Error("the split view is not on screen")
	}

	m = key(t, m, "v")
	if m.splitDiff {
		t.Error("v did not switch back")
	}
}

func TestScrollBoundsFollowTheLayout(t *testing.T) {
	m := fixture(t)
	m.mainContent = rewritePatch
	m.focus = PanelDiff

	// The split view is shorter: paired lines share a row.
	unified := m.mainLines()
	m = key(t, m, "v")
	if split := m.mainLines(); split >= unified {
		t.Errorf("split view is %d rows, unified is %d — pairing saved nothing", split, unified)
	}
}

func TestTheLayoutToggleIsRefusedInHunkMode(t *testing.T) {
	// Hunk ranges are offsets into the unified text.
	m, _ := press(t, hunkFixture(t), "enter")

	m = key(t, m, "v")
	if m.splitDiff {
		t.Error("the layout changed under hunk mode")
	}
	if !strings.Contains(m.status, "hunk mode") {
		t.Errorf("status = %q, want an explanation", m.status)
	}
}

// The layout toggle is one setting for the whole program, so the Explorer's
// patch obeys it too.
func TestVAlsoLaysOutTheExplorerPatch(t *testing.T) {
	m := fixture(t)
	m.tab = TabFiles
	m.focus = PanelPreview
	m.previewFor = previewID{path: "src/main.go", kind: previewDiff}
	m.previewContent = rewritePatch

	m = key(t, m, "v")
	if !m.splitDiff {
		t.Fatal("v did not switch to the side-by-side view")
	}
	if !strings.Contains(strings.Join(m.previewPaneLines(20, 100), "\n"), splitDivider) {
		t.Error("the Explorer preview is still drawing the unified patch")
	}

	// Pairing makes the view shorter, so scrolling has to stop sooner.
	unified := len(strings.Split(strings.TrimRight(rewritePatch, "\n"), "\n"))
	if n := m.previewLen(); n >= unified {
		t.Errorf("the split preview is %d rows, unified is %d — pairing saved nothing", n, unified)
	}

	m = key(t, m, "v")
	if strings.Contains(strings.Join(m.previewPaneLines(20, 100), "\n"), splitDivider) {
		t.Error("v did not switch the Explorer preview back")
	}
}

// A history is a list of commits, not a patch: there is no second side to lay
// it against, and pairing its lines would invent one.
func TestTheLayoutToggleLeavesAHistoryAlone(t *testing.T) {
	m := fixture(t)
	m.tab = TabFiles
	m.focus = PanelPreview
	m.previewFor = previewID{path: "src/main.go", kind: previewHistory}
	m.previewContent = rewritePatch

	m = key(t, m, "v")
	if strings.Contains(strings.Join(m.previewPaneLines(20, 100), "\n"), splitDivider) {
		t.Error("the history was drawn in two columns")
	}
}
