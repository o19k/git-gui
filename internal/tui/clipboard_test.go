package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// The selection means a path in four panes and nothing in the rest, and a key
// that quietly copied the wrong one would be worse than a key that says no.
func TestWhatCountsAsAPath(t *testing.T) {
	m := revealModel()
	m.commitFiles = []git.FileChange{{Path: "committed.go"}}
	m.stashFiles = []git.FileChange{{Path: "stashed.go"}}
	m.cwd = "src"

	cases := []struct {
		focus Panel
		want  string
	}{
		{PanelFiles, "src/main.go"},
		{PanelCommitFiles, "committed.go"},
		{PanelStashFiles, "stashed.go"},
		{PanelEntries, "src/main.go"},
	}
	for _, c := range cases {
		m.focus = c.focus
		got, ok := m.selectedPath()
		if !ok || got != c.want {
			t.Errorf("pane %d: got %q (%t), want %q", c.focus, got, ok, c.want)
		}
	}

	for _, focus := range []Panel{PanelBranches, PanelCommits, PanelStash} {
		m.focus = focus
		if got, ok := m.selectedPath(); ok {
			t.Errorf("pane %d handed back %q, which is not a path", focus, got)
		}
	}
}

// An empty directory is still somewhere, and its own path is the useful answer.
func TestAnEmptyListingCopiesTheDirectoryItself(t *testing.T) {
	m := revealModel()
	m.focus = PanelEntries
	m.cwd = "src"
	m.index["src"] = nil

	got, ok := m.selectedPath()
	if !ok || got != "src" {
		t.Errorf("got %q (%t), want src", got, ok)
	}
}

// Repo-relative is what a git command wants pasted back; absolute is what
// another program wants. Both are one key.
func TestCopyingSaysWhichPathItTook(t *testing.T) {
	m := revealModel()
	m.focus = PanelEntries
	m.cwd = "src"

	next, cmd := m.copyPath(false)
	if cmd == nil {
		t.Error("nothing was written to the terminal")
	}
	if !strings.Contains(next.(Model).status, "src/main.go") {
		t.Errorf("status = %q", next.(Model).status)
	}
}

func TestCopyingNothingSaysSo(t *testing.T) {
	m := fixture(t)
	m.focus = PanelBranches

	next, cmd := m.copyPath(false)
	if cmd != nil {
		t.Error("a branch was copied as a path")
	}
	if !strings.Contains(next.(Model).status, "path") {
		t.Errorf("status = %q, want it to explain", next.(Model).status)
	}
}
