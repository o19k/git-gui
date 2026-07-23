package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// click builds a press event, the only kind handleMouse acts on.
func click(b tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress}
}

func mouse(t *testing.T, m Model, msg tea.MouseMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.handleMouse(msg)
	return next.(Model), cmd
}

// longDiff is diff content taller than the pane, so scrolling has somewhere to go.
func longDiff(lines int) string {
	var b strings.Builder
	for i := range lines {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	return b.String()
}

// panelTop is p's first screen row, which is the top of the body unless p is
// stacked below another pane in its column.
func panelTop(m Model, p Panel) int {
	top := 1 + m.bannerHeight()
	for _, column := range tabColumns[m.tab] {
		heights := m.paneHeights(column)
		row := top
		for i, pane := range column {
			if pane == p {
				return row
			}
			row += heights[i]
		}
	}
	return top
}

// panelLeft is the first screen column of p, mirroring the renderer's split.
func panelLeft(m Model, p Panel) int {
	widths := m.paneWidths()
	left := 0
	for i, column := range tabColumns[m.tab] {
		for _, pane := range column {
			if pane == p {
				return left
			}
		}
		left += widths[i]
	}
	return left
}

// logFixture is a model on the Log tab, whose three columns exercise the
// routing more than the Changes tab's two.
func logFixture(t *testing.T) Model {
	t.Helper()
	next, _ := fixture(t).openTab(TabLog)
	return next.(Model)
}

func TestClickFocusesThePaneUnderThePointer(t *testing.T) {
	m := logFixture(t)

	x := panelLeft(m, PanelCommits) + 2
	m, _ = mouse(t, m, click(tea.MouseButtonLeft, x, panelTop(m, PanelCommits)+1))

	if m.focus != PanelCommits {
		t.Errorf("focus = %v, want PanelCommits", m.focus)
	}
	if m.cursor[PanelCommits] != 0 {
		t.Errorf("clicking the first row selected %d", m.cursor[PanelCommits])
	}
}

func TestClickSelectsTheRowItLandsOn(t *testing.T) {
	m := fixture(t)

	y := panelTop(m, PanelFiles) + 1 + 2 // third content row
	m, _ = mouse(t, m, click(tea.MouseButtonLeft, 2, y))

	if m.cursor[PanelFiles] != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor[PanelFiles])
	}
}

func TestClickPastTheLastEntryKeepsTheSelection(t *testing.T) {
	m := fixture(t)
	m.cursor[PanelFiles] = 1

	// Changes holds three entries but the column runs the full height.
	y := panelTop(m, PanelFiles) + 1 + 20
	m, _ = mouse(t, m, click(tea.MouseButtonLeft, 2, y))

	if m.cursor[PanelFiles] != 1 {
		t.Errorf("a click on empty space moved the cursor to %d", m.cursor[PanelFiles])
	}
}

func TestClickOnABorderDoesNotMoveTheCursor(t *testing.T) {
	m := fixture(t)
	m.focus = PanelDiff
	m.cursor[PanelFiles] = 2

	m, _ = mouse(t, m, click(tea.MouseButtonLeft, 2, panelTop(m, PanelFiles)))

	if m.focus != PanelFiles {
		t.Errorf("a click on the frame did not focus the pane: %v", m.focus)
	}
	if m.cursor[PanelFiles] != 2 {
		t.Errorf("a click on the frame moved the cursor to %d", m.cursor[PanelFiles])
	}
}

func TestClickOutsideTheBodyIsIgnored(t *testing.T) {
	m := fixture(t)
	m.focus = PanelDiff

	for _, y := range []int{0, m.height - 1} { // the tab bar and the footer
		next, _ := mouse(t, m, click(tea.MouseButtonLeft, 2, y))
		if next.focus != PanelDiff {
			t.Errorf("a click on row %d changed the focus to %v", y, next.focus)
		}
	}
}

func TestWheelOverTheDiffScrollsTheDiff(t *testing.T) {
	m := fixture(t)
	m.mainContent = longDiff(200)
	before := m.cursor[PanelFiles]

	x := panelLeft(m, PanelDiff) + 4
	m, _ = mouse(t, m, click(tea.MouseButtonWheelDown, x, panelTop(m, PanelDiff)+1))

	if m.mainOffset != wheelStep {
		t.Errorf("mainOffset = %d, want %d", m.mainOffset, wheelStep)
	}
	if m.cursor[PanelFiles] != before {
		t.Error("scrolling the diff moved a list cursor")
	}
}

func TestWheelOverAListMovesItsCursor(t *testing.T) {
	m := logFixture(t)

	x := panelLeft(m, PanelCommits) + 2
	m, _ = mouse(t, m, click(tea.MouseButtonWheelDown, x, panelTop(m, PanelCommits)+1))

	if m.focus != PanelCommits {
		t.Fatalf("focus = %v, want PanelCommits", m.focus)
	}
	// Fewer commits than one wheel notch, so the cursor must stop at the end.
	if want := min(wheelStep, len(m.snap.Commits)-1); m.cursor[PanelCommits] != want {
		t.Errorf("cursor = %d, want %d", m.cursor[PanelCommits], want)
	}
	if m.mainOffset != 0 {
		t.Error("scrolling a list moved the diff")
	}
}

func TestWheelUpAtTheTopStaysPut(t *testing.T) {
	m := fixture(t)
	m.mainContent = longDiff(200)

	x := panelLeft(m, PanelDiff) + 4
	m, _ = mouse(t, m, click(tea.MouseButtonWheelUp, x, panelTop(m, PanelDiff)+1))

	if m.mainOffset != 0 {
		t.Errorf("mainOffset = %d, want 0", m.mainOffset)
	}
}

func TestMouseIsIgnoredBehindAnOverlay(t *testing.T) {
	m := fixture(t)
	m.focus = PanelFiles
	m.overlay = overlay{kind: overlayHelp}

	m, _ = mouse(t, m, click(tea.MouseButtonLeft, 2, panelTop(m, PanelBranches)+1))

	if m.focus != PanelFiles {
		t.Error("a click reached the panels through a modal overlay")
	}
}

func TestMouseIgnoresMotionAndRelease(t *testing.T) {
	m := fixture(t)
	m.focus = PanelFiles

	for _, action := range []tea.MouseAction{tea.MouseActionMotion, tea.MouseActionRelease} {
		next, _ := mouse(t, m, tea.MouseMsg{
			X: 2, Y: panelTop(m, PanelBranches) + 1,
			Button: tea.MouseButtonLeft, Action: action,
		})
		if next.focus != PanelFiles {
			t.Errorf("action %v changed the focus", action)
		}
	}
}

func TestPointerRoutingAgreesWithTheRenderedSplit(t *testing.T) {
	// A cell one column short of a boundary must belong to the earlier pane.
	m := logFixture(t)
	widths := m.paneWidths()

	left := 0
	for i, column := range tabColumns[m.tab] {
		for _, want := range column {
			for _, x := range []int{left, left + widths[i] - 1} {
				got, _, ok := m.panelAt(x, panelTop(m, want)+1)
				if !ok || got != want {
					t.Errorf("cell %d routed to pane %v (ok=%v), want %v", x, got, ok, want)
				}
			}
		}
		left += widths[i]
	}
}

func TestRefreshBacksOffOnASlowRepository(t *testing.T) {
	cases := []struct {
		took time.Duration
		want time.Duration
	}{
		{took: 5 * time.Millisecond, want: minRefresh}, // fast: the floor
		{took: 500 * time.Millisecond, want: 10 * time.Second},
		{took: 10 * time.Second, want: maxRefresh}, // very slow: the ceiling
	}
	for _, c := range cases {
		m := fixture(t)
		m.loadTook = c.took
		if got := m.refreshEvery(); got != c.want {
			t.Errorf("a %s snapshot polls every %s, want %s", c.took, got, c.want)
		}
	}
}

func TestTickDoesNotStackSnapshots(t *testing.T) {
	m := fixture(t)
	m.loadAt = time.Now() // a snapshot already running

	next, _ := m.Update(tickMsg(time.Now()))
	after := next.(Model)

	if !after.loadAt.Equal(m.loadAt) {
		t.Error("a tick started a second snapshot while the first was in flight")
	}
}

func TestTickStartsASnapshotWhenIdle(t *testing.T) {
	m := fixture(t)
	m.loadAt = time.Time{}

	next, cmd := m.Update(tickMsg(time.Now()))
	after := next.(Model)

	if after.loadAt.IsZero() {
		t.Error("an idle tick did not start a snapshot")
	}
	if cmd == nil {
		t.Error("an idle tick produced no command")
	}
}

func TestSnapshotClearsTheInFlightMark(t *testing.T) {
	m := fixture(t)
	m.loadAt = time.Now().Add(-200 * time.Millisecond)

	next, _ := m.Update(snapshotMsg(m.snap))
	after := next.(Model)

	if !after.loadAt.IsZero() {
		t.Error("the in-flight mark survived the snapshot it was waiting for")
	}
	if after.loadTook <= 0 {
		t.Errorf("loadTook = %s, want the measured duration", after.loadTook)
	}
}
