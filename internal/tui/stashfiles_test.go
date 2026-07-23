package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// stashFixture puts the model on the Stash tab's file pane, with a loaded entry.
func stashFixture(t *testing.T) Model {
	t.Helper()
	m := onPane(t, fixture(t), PanelStashFiles)
	m.stashRef = "stash@{0}"
	m.stashFiles = []git.FileChange{
		{Index: 'M', Work: '.', Path: "a.txt"},
		{Index: 'M', Work: '.', Path: "b.txt"},
		{Index: 'A', Work: '.', Path: "c.txt"},
	}
	return m
}

func TestStashFilesPaneListsTheEntrysPaths(t *testing.T) {
	view := stashFixture(t).View()

	for _, want := range []string{"Files — 3 files", "a.txt", "b.txt", "c.txt"} {
		if !strings.Contains(view, want) {
			t.Errorf("the stash files pane is missing %q", want)
		}
	}
}

func TestSpaceMarksAndUnmarksAFile(t *testing.T) {
	m := stashFixture(t)

	m, _ = press(t, m, "space")
	if !m.stashMarks["a.txt"] {
		t.Fatal("space did not mark the selected file")
	}
	if !strings.Contains(m.View(), "✓") {
		t.Error("a marked file is not shown as marked")
	}

	m, _ = press(t, m, "space")
	if m.stashMarks["a.txt"] {
		t.Error("space did not unmark the file")
	}
}

func TestMarkingDoesNotMutateTheEarlierModel(t *testing.T) {
	// The marks are a map, so a shallow copy of the model would share it and
	// let a keystroke rewrite history the update loop still holds.
	before := stashFixture(t)
	after, _ := press(t, before, "space")

	if before.stashMarks["a.txt"] {
		t.Error("marking a file reached back into the model it came from")
	}
	if !after.stashMarks["a.txt"] {
		t.Error("the mark did not land on the new model")
	}
}

func TestRestoreActsOnTheMarkedFilesOnly(t *testing.T) {
	m := stashFixture(t)
	m, _ = press(t, m, "space") // a.txt
	m = key(t, m, "j")
	m = key(t, m, "j")          // c.txt
	m, _ = press(t, m, "space") // and c.txt

	if got := m.markedStashPaths(); len(got) != 2 || got[0] != "a.txt" || got[1] != "c.txt" {
		t.Fatalf("marked paths = %v, want [a.txt c.txt]", got)
	}

	m, cmd := press(t, m, "u")
	if cmd != nil {
		t.Fatal("restoring ran before the confirm was answered")
	}
	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Fatalf("overwriting the working tree should ask first: kind=%d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.body, "a.txt") || !strings.Contains(m.overlay.body, "c.txt") {
		t.Errorf("the confirm does not name what it will overwrite: %q", m.overlay.body)
	}
	if strings.Contains(m.overlay.body, "b.txt") {
		t.Errorf("the confirm names an unmarked file: %q", m.overlay.body)
	}
}

func TestRestoreWithNothingMarkedUsesTheSelection(t *testing.T) {
	m := stashFixture(t)
	m = key(t, m, "j") // b.txt

	if got := m.markedStashPaths(); len(got) != 1 || got[0] != "b.txt" {
		t.Fatalf("with no marks the target should be the selection, got %v", got)
	}
	if _, cmd := press(t, m, "u"); cmd != nil {
		t.Error("restoring ran without asking")
	}
}

func TestMovingToAnotherStashEntryDropsTheMarks(t *testing.T) {
	m := stashFixture(t)
	m, _ = press(t, m, "space")

	m = onPane(t, m, PanelStash)
	m.snap.Stashes = []git.Stash{
		{Ref: "stash@{0}", Subject: "first"},
		{Ref: "stash@{1}", Subject: "second"},
	}
	m = key(t, m, "j") // the other entry

	if len(m.stashMarks) != 0 {
		t.Errorf("marks survived onto a different entry: %v", m.stashMarks)
	}
	if m.stashRef != "stash@{1}" {
		t.Errorf("stashRef = %q, want the newly selected entry", m.stashRef)
	}
	if m.stashFiles != nil {
		t.Error("the previous entry's file list was left on screen")
	}
}

func TestAStaleStashFileListIsDiscarded(t *testing.T) {
	m := stashFixture(t)

	next, _ := m.Update(stashFilesMsg{
		ref:   "stash@{9}",
		files: []git.FileChange{{Index: 'M', Path: "wrong.txt"}},
	})
	after := next.(Model)

	if len(after.stashFiles) != 3 {
		t.Error("a file list for another entry was painted over the current one")
	}
}

func TestRestoringNothingDoesNothing(t *testing.T) {
	m := onPane(t, fixture(t), PanelStashFiles)
	m.stashRef, m.stashFiles = "", nil

	next, cmd := press(t, m, "u")
	if cmd != nil || next.overlay.kind != overlayNone {
		t.Error("u acted with no stash entry loaded")
	}
}
