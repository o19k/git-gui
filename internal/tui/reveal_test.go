package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// revealModel is a repository seen from both sides: a listing for the Explorer
// and a status list naming one of the same paths.
func revealModel() Model {
	m := navModel()
	m.index["."] = []fsEntry{
		{Name: "src", Dir: true}, {Name: "a.txt"}, {Name: "b.txt"},
	}
	m.index["src"] = []fsEntry{{Name: "main.go", Status: 'M'}, {Name: "util.go"}}
	m.snap = git.Snapshot{Files: []git.FileChange{
		{Index: '.', Work: 'M', Path: "src/main.go"},
		{Index: 'M', Work: '.', Path: "a.txt"},
	}}
	m.stats = make(map[string]fileMeta)
	m.explorerMarks = make(map[string]bool)
	return m
}

// The path is what the two views have in common, so o leaves the cursor on the
// same file the other tab was pointing at — including its directory, which is
// the part that costs the most to find by hand.
func TestOpeningAFileInTheExplorerLandsOnIt(t *testing.T) {
	m := revealModel()
	m.tab = TabChanges
	m.focus = PanelFiles
	m.cursor[PanelFiles] = 0 // src/main.go

	next, cmd := press(t, m, "o")
	next = drain(t, next, cmd)

	if next.tab != TabFiles {
		t.Fatalf("still on tab %d", next.tab)
	}
	if next.cwd != "src" {
		t.Errorf("listing %q, want the directory holding the file", next.cwd)
	}
	entries := next.entries()
	if len(entries) == 0 {
		t.Fatal("nothing listed")
	}
	if got := entries[next.cursor[PanelEntries]].Name; got != "main.go" {
		t.Errorf("the cursor is on %q, want main.go", got)
	}
}

// The listing is read when the tab opens, so the path has to survive until it
// arrives — otherwise the jump lands wherever the last visit left off.
func TestAPathSurvivesUntilTheListingArrives(t *testing.T) {
	m := revealModel()
	m.tab = TabChanges
	m.focus = PanelFiles
	m.index = map[string][]fsEntry{} // nothing read yet

	next, _ := m.revealInExplorer("src/main.go")
	moved := next.(Model)
	if moved.pendingPath != "src/main.go" {
		t.Fatalf("pendingPath = %q", moved.pendingPath)
	}

	after, _ := moved.handleLoadIndex(loadIndexMsg{entries: []git.TreeEntry{
		{Path: "src/main.go"}, {Path: "src/util.go"}, {Path: "a.txt"},
	}})
	arrived := after.(Model)

	if arrived.cwd != "src" {
		t.Errorf("listing %q after the read landed, want src", arrived.cwd)
	}
	if arrived.pendingPath != "" {
		t.Error("the path is still pending after being used")
	}
}

// Held while the tab is already open, the path would be picked up by the next
// poll and drag the cursor back long after the jump was done with.
func TestNoPathIsHeldWhenTheExplorerIsAlreadyOpen(t *testing.T) {
	m := revealModel()
	m.tab = TabFiles

	next, _ := m.revealInExplorer("src/main.go")
	if got := next.(Model).pendingPath; got != "" {
		t.Errorf("pendingPath = %q, want nothing held", got)
	}
}

func TestShowingAFileInChangesPutsTheCursorOnIt(t *testing.T) {
	m := revealModel()
	m.tab = TabFiles
	m.focus = PanelEntries
	m.cwd = "src"
	m.cursor[PanelEntries] = 0 // main.go

	next, _ := press(t, m, "O")

	if next.tab != TabChanges {
		t.Fatalf("still on tab %d, status %q", next.tab, next.status)
	}
	file, ok := next.selectedFile()
	if !ok || file.Path != "src/main.go" {
		t.Errorf("selected %v (%t), want src/main.go", file, ok)
	}
}

// A clean file is in the tree but not in the status list, and landing on
// whatever happens to be at that row would be worse than saying so.
func TestShowingACleanFileInChangesSaysThereIsNothingThere(t *testing.T) {
	m := revealModel()
	m.tab = TabFiles
	m.focus = PanelEntries
	m.cwd = "src"
	m.cursor[PanelEntries] = 1 // util.go, which git has nothing to say about

	next, _ := press(t, m, "O")

	if next.tab != TabFiles {
		t.Error("the tab changed for a file that is not in the list")
	}
	if !strings.Contains(next.status, "util.go") {
		t.Errorf("status = %q, want it to name the file", next.status)
	}
}

// A filter narrows what the cursor can address, so a path behind one is
// revealed rather than reported missing.
func TestAFilteredOutFileIsRevealedRatherThanMissed(t *testing.T) {
	m := revealModel()
	m.tab = TabFiles
	m.focus = PanelEntries
	m.cwd = "src"
	m.filter[PanelFiles] = "nothing matches this"

	next, _ := press(t, m, "O")

	if next.tab != TabChanges {
		t.Fatalf("the jump was refused: status %q", next.status)
	}
	if next.filter[PanelFiles] != "" {
		t.Errorf("the filter %q survived and still hides the file", next.filter[PanelFiles])
	}
}
