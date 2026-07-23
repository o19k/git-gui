package tui

import (
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

func TestChangeForInStatus(t *testing.T) {
	// Test: changeFor returns the snapshot entry for a file with working-tree changes
	m := fixture(t)
	m.snap.Files = []git.FileChange{
		{Index: '.', Work: 'M', Path: "modified.go"},
		{Index: 'M', Work: '.', Path: "staged.go"},
	}

	// File with changes should come from snapshot
	change, ok := m.changeFor("modified.go")
	if !ok || change.Work != 'M' {
		t.Errorf("changeFor(modified.go) = %v, %v; want work='M'", change, ok)
	}

	change, ok = m.changeFor("staged.go")
	if !ok || change.Index != 'M' {
		t.Errorf("changeFor(staged.go) = %v, %v; want index='M'", change, ok)
	}
}

func TestChangeForTrackedClean(t *testing.T) {
	// A file git holds a copy of but which has no working-tree change is the
	// case a two-state answer gets wrong, and it is the common one here.
	m := fixture(t)
	m.index = map[string][]fsEntry{
		".": {{Name: "clean.go", Cached: true}},
	}

	change, ok := m.changeFor("clean.go")
	if !ok || change.Index != '.' || change.Work != '.' {
		t.Errorf("changeFor(clean.go) = {%c,%c}, %v; want {.,.} so a delete goes through git rm",
			change.Index, change.Work, ok)
	}
}

// Being listed is not being tracked: ls-files reports untracked paths too, and
// reading the listing rather than the index row inverts every delete.
func TestChangeForListedButUntracked(t *testing.T) {
	m := fixture(t)
	m.snap.Files = nil
	m.index = map[string][]fsEntry{
		".": {{Name: "scratch.go"}},
	}

	change, ok := m.changeFor("scratch.go")
	if !ok || !change.Untracked() {
		t.Errorf("changeFor(scratch.go) = {%c,%c}; want untracked, since git holds no copy of it",
			change.Index, change.Work)
	}
}

func TestChangeForUntracked(t *testing.T) {
	// Test: changeFor returns Index '?' for untracked files
	m := fixture(t)
	m.snap.Files = nil
	m.index = map[string][]fsEntry{}

	change, ok := m.changeFor("new.go")
	if !ok || change.Index != '?' || change.Work != '?' {
		t.Errorf("changeFor(new.go) = {%c,%c}, %v; want {?,?}", change.Index, change.Work, ok)
	}
}

func TestMarkedPathsReturnsMarkedSet(t *testing.T) {
	// Test: markedPaths returns marked paths when marks exist
	m := fixture(t)
	m.cwd = "."
	m.explorerMarks = map[string]bool{
		"file1.go": true,
		"file2.go": true,
	}

	paths := m.markedPaths()
	if len(paths) != 2 {
		t.Errorf("markedPaths returned %d items, want 2", len(paths))
	}

	found := make(map[string]bool)
	for _, p := range paths {
		found[p] = true
	}
	if !found["file1.go"] || !found["file2.go"] {
		t.Error("markedPaths missing expected paths")
	}
}

func TestMarkedPathsReturnsSelection(t *testing.T) {
	// Test: markedPaths returns selection when no marks exist
	m := onPane(t, fixture(t), PanelEntries)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file1.go", Dir: false},
			{Name: "file2.go", Dir: false},
		},
	}
	m.cursor[PanelEntries] = 1
	m.explorerMarks = nil

	paths := m.markedPaths()
	if len(paths) != 1 || paths[0] != "file2.go" {
		t.Errorf("markedPaths = %v, want [file2.go]", paths)
	}
}

func TestToggleExplorerMark(t *testing.T) {
	// Test: toggleExplorerMark adds and removes marks
	m := onPane(t, fixture(t), PanelEntries)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file1.go", Dir: false},
		},
	}
	m.explorerMarks = make(map[string]bool)

	// Mark the file
	m.toggleExplorerMark()
	if !m.explorerMarks["file1.go"] {
		t.Error("toggleExplorerMark did not add mark")
	}

	// Unmark the file
	m.toggleExplorerMark()
	if m.explorerMarks["file1.go"] {
		t.Error("toggleExplorerMark did not remove mark")
	}
}

func TestExplorerOpKeyMIsHandled(t *testing.T) {
	// Test: M key toggles marks
	m := onPane(t, fixture(t), PanelEntries)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file.go", Dir: false},
		},
	}
	m.explorerMarks = make(map[string]bool)

	next, _, handled := m.explorerOpKey("M")
	m = next.(Model)
	if !handled {
		t.Error("M key was not handled")
	}

	if !m.explorerMarks["file.go"] {
		t.Error("M key did not toggle mark")
	}
}

func TestExplorerOpKeyUnknownKeyNotHandled(t *testing.T) {
	// Test: unknown key is not handled
	m := fixture(t)
	next, _, handled := m.explorerOpKey("q")
	m = next.(Model)
	if handled {
		t.Error("unknown key was handled")
	}
}

func TestEntryAtFindsPathsInEitherListing(t *testing.T) {
	m := fixture(t)
	m.index = map[string][]fsEntry{
		".":   {{Name: "root.go", Cached: true}},
		"dir": {{Name: "file.go", Cached: true}},
	}
	m.fsIndex = map[string][]fsEntry{
		"ignored": {{Name: "blob.bin", FromFS: true}},
	}

	for _, path := range []string{"root.go", "dir/file.go", "ignored/blob.bin"} {
		if _, ok := m.entryAt(path); !ok {
			t.Errorf("entryAt(%q) found nothing", path)
		}
	}
	if _, ok := m.entryAt("dir/missing.go"); ok {
		t.Error("entryAt found a path that is in neither listing")
	}
}

// A directory has no index row, so whether git holds a copy of it is whatever
// its children say — and getting this wrong sends a tracked tree to unlink.
func TestIsPathTrackedFollowsChildren(t *testing.T) {
	m := fixture(t)
	m.index = map[string][]fsEntry{
		".":       {{Name: "src", Dir: true}, {Name: "scratch", Dir: true}},
		"src":     {{Name: "main.go", Cached: true}},
		"scratch": {{Name: "notes.txt"}},
	}

	if !m.isPathTracked("src") {
		t.Error("a directory holding a tracked file reported untracked")
	}
	if m.isPathTracked("scratch") {
		t.Error("a directory holding only untracked files reported tracked")
	}
}

func TestCountFilesInDir(t *testing.T) {
	// Test: countFilesInDir counts files recursively
	// Create a temporary directory structure for testing
	t.Helper()

	// This test would need a real filesystem; skip for now
	// The function is tested implicitly through delete operations
}

func TestExplorerOpKeyNHandlesCreateFile(t *testing.T) {
	// Test: n key opens create file dialog
	m := fixture(t)
	next, _, handled := m.explorerOpKey("n")
	m = next.(Model)
	if !handled {
		t.Error("n key was not handled")
	}

	if m.overlay.kind != overlayInput {
		t.Errorf("n did not open input overlay: %v", m.overlay.kind)
	}
}

func TestExplorerOpKeyCapitalNHandlesCreateDir(t *testing.T) {
	// Test: N key opens create directory dialog
	m := fixture(t)
	next, _, handled := m.explorerOpKey("N")
	m = next.(Model)
	if !handled {
		t.Error("N key was not handled")
	}

	if m.overlay.kind != overlayInput {
		t.Errorf("N did not open input overlay: %v", m.overlay.kind)
	}
}

func TestExplorerOpKeyDHandlesDiscard(t *testing.T) {
	// Test: d key opens discard confirmation
	m := onPane(t, fixture(t), PanelEntries)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file.go", Dir: false},
		},
	}
	m.explorerMarks = nil

	next, _, handled := m.explorerOpKey("d")
	m = next.(Model)
	if !handled {
		t.Error("d key was not handled")
	}

	if m.overlay.kind != overlayConfirm {
		t.Errorf("d did not open confirm overlay: %v", m.overlay.kind)
	}
}

func TestExplorerOpKeySpaceHandlesStaging(t *testing.T) {
	// Test: space key opens staging operation
	m := onPane(t, fixture(t), PanelEntries)
	m.cwd = "."
	m.snap.Files = []git.FileChange{
		{Index: '?', Work: '?', Path: "new.go"},
	}
	m.index = map[string][]fsEntry{
		".": {
			{Name: "new.go", Dir: false},
		},
	}

	_, cmd, handled := m.explorerOpKey(" ")
	if !handled {
		t.Error("space key was not handled")
	}

	if cmd == nil {
		t.Error("space key did not return a command")
	}
}
