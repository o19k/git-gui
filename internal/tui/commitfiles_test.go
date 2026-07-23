package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// commitFilesFixture puts the model on the Log tab with a commit's files loaded.
func commitFilesFixture(t *testing.T) Model {
	t.Helper()
	m := onPane(t, fixture(t), PanelCommitFiles)
	m.commitSHA = "aaa"
	m.commitFiles = []git.FileChange{
		{Index: 'M', Work: '.', Path: "one.go"},
		{Index: 'A', Work: '.', Path: "two.go"},
	}
	return m
}

func TestLogTabStacksTheFilesOverTheCommit(t *testing.T) {
	m := commitFilesFixture(t)
	view := m.View()

	for _, want := range []string{"Files — 2 files", "one.go", "two.go"} {
		if !strings.Contains(view, want) {
			t.Errorf("the commit files pane is missing %q", want)
		}
	}

	// The two share a column, so neither may take the whole body.
	filesH := m.paneHeight(PanelCommitFiles)
	detailsH := m.paneHeight(PanelDetails)
	if filesH+detailsH != m.bodyHeight() {
		t.Errorf("stacked panes are %d+%d rows, body is %d", filesH, detailsH, m.bodyHeight())
	}
	if filesH >= m.bodyHeight() || detailsH >= m.bodyHeight() {
		t.Error("a stacked pane took the whole column")
	}
}

func TestTabReachesTheStackedPanesInOrder(t *testing.T) {
	m := key(t, fixture(t), "2") // Log
	want := []Panel{PanelBranches, PanelWorktrees, PanelCommits, PanelCommitFiles, PanelDetails}

	for i := 1; i < len(want); i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if m.focus != want[i] {
			t.Fatalf("after %d tabs focus is %d, want %d", i, m.focus, want[i])
		}
	}
}

func TestSelectingAFileNarrowsThePatchToIt(t *testing.T) {
	m := commitFilesFixture(t)

	cmd := m.refreshPreview()
	if cmd == nil {
		t.Fatal("selecting a file queued no diff")
	}
	if !strings.HasPrefix(m.previewKey, "commitfile:aaa:one.go") {
		t.Errorf("previewKey = %q, want the selected path", m.previewKey)
	}

	m = key(t, m, "j")
	m.refreshPreview()
	if !strings.HasPrefix(m.previewKey, "commitfile:aaa:two.go") {
		t.Errorf("moving down did not follow to the next path: %q", m.previewKey)
	}
}

func TestMovingBackToCommitsRestoresTheWholePatch(t *testing.T) {
	m := commitFilesFixture(t)
	m.refreshPreview()

	next, _ := m.movePane(-1)
	m = next.(Model)
	if m.focus != PanelCommits {
		t.Fatalf("shift+tab landed on %d", m.focus)
	}
	m.refreshPreview()

	if !strings.HasPrefix(m.previewKey, "commit:") {
		t.Errorf("previewKey = %q, want the whole commit", m.previewKey)
	}
}

func TestMovingToAnotherCommitDropsTheOldFileList(t *testing.T) {
	m := commitFilesFixture(t)
	m = onPane(t, m, PanelCommits)
	m.cursor[PanelCommits] = 0

	m = key(t, m, "j") // the second commit
	if m.commitSHA != "bbb" {
		t.Errorf("commitSHA = %q, want the newly selected commit", m.commitSHA)
	}
	if m.commitFiles != nil {
		t.Error("the previous commit's file list was left on screen")
	}
}

func TestAStaleCommitFileListIsDiscarded(t *testing.T) {
	m := commitFilesFixture(t)

	next, _ := m.Update(commitFilesMsg{
		sha:   "zzz",
		files: []git.FileChange{{Index: 'M', Path: "wrong.go"}},
	})
	if after := next.(Model); len(after.commitFiles) != 2 {
		t.Error("a file list for another commit was painted over the current one")
	}
}

func TestACommitWithNoFilesSaysSo(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommitFiles)
	m.commitSHA, m.commitFiles = "aaa", nil

	if !strings.Contains(m.View(), "no files") {
		t.Error("an empty file list does not say why it is empty")
	}
	if _, ok := m.selectedCommitFile(); ok {
		t.Error("an empty list still reported a selection")
	}
}
