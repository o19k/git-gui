package tui

import (
	"strings"
	"testing"
)

// lineFixture is hunkFixture with the hunks open and lines being picked.
func lineFixture(t *testing.T) Model {
	t.Helper()
	m := hunkFixture(t)
	m, _ = press(t, m, "enter") // hunk mode
	m, _ = press(t, m, "enter") // line mode
	return m
}

func TestEnterOpensTheHunkForPickingLines(t *testing.T) {
	m := lineFixture(t)

	if !m.hunkMode || !m.lineMode {
		t.Fatalf("line mode did not open: hunk=%v line=%v", m.hunkMode, m.lineMode)
	}
	// The cursor lands on something that can be picked, never on context.
	hunks := m.currentHunks()
	if line := hunks[m.hunkCursor].Lines[m.lineCursor]; !changedLine(line) {
		t.Errorf("the cursor started on %q, which cannot be staged", line)
	}
	if !strings.Contains(m.View(), "lines") {
		t.Error("the title does not say lines are being picked")
	}
}

func TestPickingWalksOnlyOverChangedLines(t *testing.T) {
	m := lineFixture(t)
	hunks := m.currentHunks()

	for range len(hunks[m.hunkCursor].Lines) {
		m, _ = press(t, m, "j")
		if line := hunks[m.hunkCursor].Lines[m.lineCursor]; !changedLine(line) {
			t.Fatalf("j landed on %q, which cannot be staged", line)
		}
	}
}

func TestSpaceMarksLinesAndEnterStagesThem(t *testing.T) {
	// The second hunk, which holds a removal and an addition: marking the one
	// changed line of the first has nowhere to move on to.
	m := hunkFixture(t)
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "enter")

	first := m.lineCursor
	m, _ = press(t, m, " ")
	if !m.lineMarks[first] {
		t.Error("space did not mark the line under the cursor")
	}
	if m.lineCursor == first {
		t.Error("marking should move on, so a run of lines is one key each")
	}
	if !strings.Contains(m.View(), "picked") {
		t.Error("the title does not say how much is picked")
	}

	// Marking the same line again takes it back off.
	m.lineCursor = first
	m, _ = press(t, m, " ")
	if m.lineMarks[first] {
		t.Error("space did not unmark a marked line")
	}
}

// Nothing marked means the line under the cursor, so the common case is one
// keystroke rather than two.
func TestNothingMarkedStagesTheLineUnderTheCursor(t *testing.T) {
	m := lineFixture(t)

	picked := m.pickedLines()
	if len(picked) != 1 || !picked[m.lineCursor] {
		t.Errorf("picked = %v, want just the cursor at %d", picked, m.lineCursor)
	}
}

func TestEnterLeavesLineModeAfterStaging(t *testing.T) {
	m := lineFixture(t)

	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Error("enter staged nothing")
	}
	if m.lineMode {
		t.Error("line mode stayed open after staging")
	}
	if !m.hunkMode {
		t.Error("staging lines should leave the hunks open, not the file list")
	}
}

func TestEscLeavesLineModeForTheHunks(t *testing.T) {
	m := lineFixture(t)

	m, _ = press(t, m, "esc")
	if m.lineMode {
		t.Error("esc did not leave line mode")
	}
	if !m.hunkMode {
		t.Error("esc left hunk mode as well, which is one step too far")
	}
}

func TestLeavingHunkModeForgetsTheMarks(t *testing.T) {
	m := lineFixture(t)
	m, _ = press(t, m, " ")

	m, _ = press(t, m, "esc") // back to hunks
	m, _ = press(t, m, "esc") // back to the files

	if m.lineMarks != nil || m.lineMode {
		t.Errorf("marks survived leaving: marks=%v mode=%v", m.lineMarks, m.lineMode)
	}
}

// The staged patch is what is applied, so a diff generated with whitespace
// hidden cannot be the one being read.
func TestHunkModeIgnoresTheWhitespaceSetting(t *testing.T) {
	m := hunkFixture(t)
	m.settings.IgnoreWhitespace = true

	if !m.diffOpts().IgnoreWhitespace {
		t.Error("the setting does not reach a patch that is only read")
	}
	m.hunkMode = true
	if m.diffOpts().IgnoreWhitespace {
		t.Error("the setting reached the patch that gets applied")
	}
}
