package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// blameFixture puts annotations on screen with the content pane focused, which
// is where the annotation keys apply.
func blameFixture(t *testing.T) Model {
	t.Helper()
	m := fixture(t)
	m.focus = PanelFiles
	m.cursor[PanelFiles] = 1 // dirty.go, a tracked file
	m, _ = press(t, m, "b")

	next, _ := m.Update(blameMsg{
		path: "dirty.go",
		lines: []git.BlameLine{
			{Short: "aaa1111", Author: "ada", When: "2026-01-01", Text: "one"},
			{Short: "bbb2222", Author: "bob", When: "2026-02-02", Text: "two"},
			{Short: "aaa1111", Author: "ada", When: "2026-01-01", Text: "three"},
		},
	})
	m = next.(Model)
	m.focus = PanelDiff
	return m
}

func TestBlameKeysOnlyApplyToThePaneHoldingThem(t *testing.T) {
	m := blameFixture(t)
	m.focus = PanelFiles

	// With the file list focused, j is the list's own key: the annotations
	// follow the selection rather than scrolling under it.
	before := m.cursor[PanelFiles]
	m, _ = press(t, m, "j")
	if m.cursor[PanelFiles] == before {
		t.Error("j stopped moving between files while annotations were on")
	}
}

func TestBlameCursorWalksTheAnnotatedFile(t *testing.T) {
	m := blameFixture(t)

	m, _ = press(t, m, "j")
	if m.blameCursor != 1 {
		t.Fatalf("blame cursor = %d, want 1", m.blameCursor)
	}
	if short, _ := m.selectedBlame(); short != "bbb2222" {
		t.Errorf("selected line's commit = %q", short)
	}

	// It stops at the ends rather than wrapping.
	m, _ = press(t, m, "k")
	m, _ = press(t, m, "k")
	if m.blameCursor != 0 {
		t.Errorf("blame cursor = %d, want it to stop at the top", m.blameCursor)
	}
}

func TestEnterOpensTheCommitBehindTheLine(t *testing.T) {
	m := blameFixture(t)

	m, _ = press(t, m, "enter")

	if m.tab != TabLog || m.focus != PanelCommits {
		t.Fatalf("enter landed on tab %d pane %d", m.tab, m.focus)
	}
	if m.blameOn {
		t.Error("the annotations are still on, and they share the pane with the patch")
	}
	if m.cursor[PanelCommits] != 0 {
		t.Errorf("selected commit index = %d, want the one the line names", m.cursor[PanelCommits])
	}
}

// A commit older than the list the tab holds cannot be selected in it, and
// saying so beats jumping somewhere arbitrary.
func TestEnterSaysWhenTheCommitIsOutOfReach(t *testing.T) {
	m := blameFixture(t)
	// A commit older than the list the tab holds.
	m.blameLines = append(m.blameLines, git.BlameLine{Short: "fff9999", Text: "old"})
	m.blameCursor = len(m.blameLines) - 1

	m, _ = press(t, m, "enter")

	if m.tab == TabLog {
		t.Error("the tab changed for a commit that is not in the list")
	}
	if !strings.Contains(m.status, "fff9999") {
		t.Errorf("status = %q, want it to name the commit it could not reach", m.status)
	}
}

func TestParentWalkRebasesTheAnnotationOnTheCommitBefore(t *testing.T) {
	m := blameFixture(t)

	m, cmd := press(t, m, "<")
	if cmd == nil {
		t.Fatal("< read nothing")
	}
	if m.blameRev != "aaa1111^" {
		t.Errorf("blame revision = %q, want the parent of the line's commit", m.blameRev)
	}
	if m.blameLines != nil {
		t.Error("the old annotations were left on screen while the new ones are read")
	}
	if !strings.Contains(m.blameTitle(), "aaa1111^") {
		t.Errorf("the title does not say what is being annotated: %q", m.blameTitle())
	}

	// And the way back.
	m, _ = press(t, m, ">")
	if m.blameRev != "" {
		t.Errorf("blame revision = %q, want the working copy", m.blameRev)
	}
}

// A reply for a revision already walked past would otherwise overwrite the one
// being waited for.
func TestStaleBlameRepliesAreDropped(t *testing.T) {
	m := blameFixture(t)
	m.blameRev = "aaa1111^"

	next, _ := m.Update(blameMsg{path: "dirty.go", rev: "", lines: []git.BlameLine{{Text: "stale"}}})
	m = next.(Model)

	for _, line := range m.blameLines {
		if line.Text == "stale" {
			t.Error("a reply for the revision walked away from was drawn")
		}
	}
}
