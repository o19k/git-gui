package tui

import (
	"strings"
	"testing"
)

func TestOpenPickerGathersIndexPaths(t *testing.T) {
	// Test: openPicker creates a list of all indexed paths
	m := fixture(t)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file1.go", Dir: false},
			{Name: "file2.go", Dir: false},
		},
		"subdir": {
			{Name: "file3.go", Dir: false},
		},
	}

	m, _ = m.openPicker()

	if m.overlay.kind != overlayList {
		t.Errorf("openPicker did not open list overlay: %v", m.overlay.kind)
	}

	if len(m.overlay.items) < 2 {
		t.Errorf("openPicker found %d items, want at least 2", len(m.overlay.items))
	}
}

func TestOpenPickerFiltersAsTyped(t *testing.T) {
	// Test: openPicker filtering (handled by listMatches)
	// This is tested through the overlay list filtering mechanism
	m := fixture(t)
	m.cwd = "."
	m.index = map[string][]fsEntry{
		".": {
			{Name: "foo.go", Dir: false},
			{Name: "bar.go", Dir: false},
		},
	}

	m, _ = m.openPicker()
	items := m.listMatches()

	if len(items) < 2 {
		t.Errorf("openPicker returned %d items", len(items))
	}
}

func TestStartGrepCreatesCommand(t *testing.T) {
	// Test: startGrep creates an async grep command
	m := fixture(t)
	m.repo = nil // Don't actually run git

	// We can't easily test the actual grep without a real repo,
	// but we can verify it returns a command
	cmd := m.startGrep("test")
	if cmd == nil {
		t.Error("startGrep did not return a command")
	}
}

func TestHandleGrepWithMatches(t *testing.T) {
	// Test: handleGrep presents matches in a picker
	m := fixture(t)
	msg := grepMsg{
		query: "test",
		hits: []grepHit{
			{Path: "file.go", Line: 42, Text: "test string"},
			{Path: "other.go", Line: 100, Text: "test again"},
		},
		err: nil,
	}

	result, _ := m.handleGrep(msg)
	m = result.(Model)

	if m.overlay.kind != overlayList {
		t.Errorf("handleGrep did not open list overlay: %v", m.overlay.kind)
	}

	if len(m.overlay.items) != 2 {
		t.Errorf("handleGrep loaded %d items, want 2", len(m.overlay.items))
	}
}

func TestHandleGrepWithNoMatches(t *testing.T) {
	// Test: handleGrep shows "no matches" for empty results
	m := fixture(t)
	msg := grepMsg{
		query: "notfound",
		hits:  []grepHit{},
		err:   nil,
	}

	result, _ := m.handleGrep(msg)
	m = result.(Model)

	if m.overlay.kind != overlayList {
		t.Errorf("handleGrep did not open list overlay: %v", m.overlay.kind)
	}

	// Should show one item saying "no matches"
	if len(m.overlay.items) != 1 {
		t.Errorf("handleGrep with no matches loaded %d items, want 1", len(m.overlay.items))
	}

	if m.overlay.items[0].label != "no matches" {
		t.Errorf("handleGrep shows %q, want 'no matches'", m.overlay.items[0].label)
	}
}

func TestHandleGrepWithError(t *testing.T) {
	// Test: handleGrep displays error message
	m := fixture(t)
	msg := grepMsg{
		query: "test",
		hits:  nil,
		err:   errTest("git error"),
	}

	result, _ := m.handleGrep(msg)
	m = result.(Model)

	if m.status != "git error" {
		t.Errorf("handleGrep status = %q, want 'git error'", m.status)
	}
}

func TestExplorerSearchKeyOOpensPicke(t *testing.T) {
	// Test: o key opens the path picker
	m := fixture(t)
	m.index = map[string][]fsEntry{
		".": {
			{Name: "file.go", Dir: false},
		},
	}

	next, _, handled := m.explorerSearchKey("o")
	m = next.(Model)
	if !handled {
		t.Error("o key was not handled")
	}

	if m.overlay.kind != overlayList {
		t.Errorf("o did not open list overlay: %v", m.overlay.kind)
	}
}

func TestExplorerSearchKeySOpensSearchDialog(t *testing.T) {
	// Test: s key opens the search dialog
	m := fixture(t)

	next, _, handled := m.explorerSearchKey("s")
	m = next.(Model)
	if !handled {
		t.Error("s key was not handled")
	}

	if m.overlay.kind != overlayInput {
		t.Errorf("s did not open input overlay: %v", m.overlay.kind)
	}
}

func TestExplorerSearchKeyUnknownNotHandled(t *testing.T) {
	// Test: unknown key is not handled
	m := fixture(t)
	next, _, handled := m.explorerSearchKey("q")
	m = next.(Model)
	if handled {
		t.Error("unknown key was handled")
	}
}

// errTest is a test error type
type errTest string

func (e errTest) Error() string {
	return string(e)
}

// pickerModel holds two directories and an ignored tree, so a path's own
// directory and its listed one differ.
func pickerModel() Model {
	return Model{
		index: map[string][]fsEntry{
			".":   {{Name: "src", Dir: true}, {Name: "build", Dir: true, Ignored: true}, {Name: "a.txt"}},
			"src": {{Name: "main.go"}, {Name: "util.go"}},
		},
		fsIndex:   make(map[string][]fsEntry),
		dirCursor: make(map[string]int),
		cwd:       ".",
		focus:     PanelEntries,
		width:     120,
		height:    40,
	}
}

// Every entry was joined with cwd rather than with the directory it was listed
// under, so from the root the picker offered "main.go" for "src/main.go" — a
// path that names nothing.
func TestPickerOffersEachPathUnderItsOwnDirectory(t *testing.T) {
	m, _ := pickerModel().openPicker()

	got := make(map[string]bool)
	for _, item := range m.overlay.items {
		got[item.value] = true
	}
	for _, want := range []string{"a.txt", "src", "src/main.go", "src/util.go"} {
		if !got[want] {
			t.Errorf("%q is missing from the picker: %v", want, got)
		}
	}
	if got["main.go"] {
		t.Error("the picker offers main.go, which is not a path in this repository")
	}
}

func TestPickerLeavesOutIgnoredPaths(t *testing.T) {
	m, _ := pickerModel().openPicker()
	for _, item := range m.overlay.items {
		if item.value == "build" {
			t.Error("the picker offers an ignored tree among the repository's paths")
		}
	}
}

// A map has no order, so an unsorted picker reorders itself between openings.
func TestPickerIsInAStableOrder(t *testing.T) {
	first, _ := pickerModel().openPicker()
	for range 20 {
		next, _ := pickerModel().openPicker()
		if len(next.overlay.items) != len(first.overlay.items) {
			t.Fatal("the picker offered a different number of paths")
		}
		for i := range next.overlay.items {
			if next.overlay.items[i].value != first.overlay.items[i].value {
				t.Fatalf("the picker reordered itself: %q then %q",
					first.overlay.items[i].value, next.overlay.items[i].value)
			}
		}
	}
}

// The action runs on a copy of the model that is thrown away, so the choice
// has to travel back as a message.
func TestChoosingADirectoryInThePickerMovesThere(t *testing.T) {
	m := pickerModel()
	next, _ := m.handleNavigate(navigateMsg{path: "src"})
	if got := next.(Model).cwd; got != "src" {
		t.Errorf("choosing src left the Explorer in %q", got)
	}
}

func TestChoosingAFileInThePickerSelectsItInItsDirectory(t *testing.T) {
	m := pickerModel()
	next, _ := m.handleNavigate(navigateMsg{path: "src/util.go"})
	after := next.(Model)

	if after.cwd != "src" {
		t.Fatalf("choosing src/util.go listed %q", after.cwd)
	}
	if got := after.entries()[after.cursor[PanelEntries]].Name; got != "util.go" {
		t.Errorf("the cursor landed on %q, want util.go", got)
	}
}

// A path picked deliberately is shown even when it is hidden by default.
func TestChoosingAHiddenFileRevealsIt(t *testing.T) {
	m := pickerModel()
	m.index["src"] = append(m.index["src"], fsEntry{Name: ".hidden"})

	next, _ := m.handleNavigate(navigateMsg{path: "src/.hidden"})
	after := next.(Model)
	if !after.showHidden {
		t.Fatal("choosing a hidden file left it hidden")
	}
	if got := after.entries()[after.cursor[PanelEntries]].Name; got != ".hidden" {
		t.Errorf("the cursor landed on %q, want .hidden", got)
	}
}

// A search hit names a line, and the pane has to be scrolled to it — but only
// once the content it belongs to has arrived, since the read is async and its
// handler starts the pane at the top.
func TestASearchHitScrollsThePreviewToItsLine(t *testing.T) {
	m := pickerModel()
	next, _ := m.handleNavigate(navigateMsg{path: "src/main.go", line: 40})
	after := next.(Model)

	if after.previewOffset != 0 {
		t.Errorf("the pane scrolled to %d before there was anything in it", after.previewOffset)
	}
	if after.pendingLine != 40 {
		t.Fatalf("the line was not held for the content: %d", after.pendingLine)
	}
	if after.previewFor.kind != previewContent {
		t.Error("a search hit opened on something other than the file's content, which has no such line")
	}

	arrived, _ := after.handleExplorerPreview(explorerPreviewMsg{
		id:      after.previewFor,
		title:   "src/main.go",
		content: strings.Repeat("line\n", 200),
	})
	if got := arrived.(Model).previewOffset; got != 39 {
		t.Errorf("the pane scrolled to %d once the content arrived, want line 40", got)
	}
	if arrived.(Model).pendingLine != 0 {
		t.Error("the held line was not cleared, so the next preview will jump too")
	}
}
