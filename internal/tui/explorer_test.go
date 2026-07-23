package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"

	"github.com/o19k/git-gui/internal/theme"
)

// A lipgloss.Style holds a function, so two of them cannot be compared with
// ==, and comparing what they render proves nothing here: the tests run with
// no terminal, so the colour profile strips every escape and each style
// renders its input unchanged. The foreground itself is what carries the
// meaning, so that is what is compared.
func foreground(s lipgloss.Style) string {
	return fmt.Sprint(s.GetForeground())
}

func TestEntryStyleDimsIgnoredEntries(t *testing.T) {
	style := entryStyle(fsEntry{Name: "node_modules", Dir: true, Ignored: true})
	if got, want := foreground(style), foreground(theme.DimStyle); got != want {
		t.Errorf("an ignored entry is %s, want the dim %s", got, want)
	}
}

func TestEntryStyleLeavesCleanEntriesAlone(t *testing.T) {
	style := entryStyle(fsEntry{Name: "file.txt"})
	if got, want := foreground(style), foreground(theme.NormalStyle); got != want {
		t.Errorf("a clean entry is %s, want the normal %s", got, want)
	}
}

func TestEntryStyleColoursEachStatus(t *testing.T) {
	clean := foreground(theme.NormalStyle)
	for _, status := range []byte{'U', 'D', 'M', 'A', '?'} {
		t.Run(string(status), func(t *testing.T) {
			got := foreground(entryStyle(fsEntry{Name: "file.txt", Status: status}))
			if want := fmt.Sprint(theme.StatusColor(status)); got != want {
				t.Errorf("status %c is %s, want %s", status, got, want)
			}
			if got == clean {
				t.Errorf("status %c is the same colour as a clean entry", status)
			}
		})
	}
}

// The rollup is what makes a status useful on a collapsed directory, and its
// order is the whole of it: a directory holding both a conflict and an
// untracked file has to say conflict.
func TestRollUpTakesTheWorstStatusBeneathADirectory(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]byte
		want   byte
	}{
		{"conflict outranks untracked", map[string]byte{"a/x": 'U', "a/y": '?'}, 'U'},
		{"deleted outranks modified", map[string]byte{"a/x": 'D', "a/y": 'M'}, 'D'},
		{"modified outranks added", map[string]byte{"a/x": 'M', "a/y": 'A'}, 'M'},
		{"added outranks untracked", map[string]byte{"a/x": 'A', "a/y": '?'}, 'A'},
		{"untracked alone still shows", map[string]byte{"a/x": '?'}, '?'},
		{"nothing beneath stays clean", map[string]byte{"b/x": 'M'}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := map[string][]fsEntry{".": {{Name: "a", Dir: true}}}
			rollUp(index, tt.status)
			if got := index["."][0].Status; got != tt.want {
				t.Errorf("directory a rolled up to %q, want %q", got, tt.want)
			}
		})
	}
}

// A change any distance down still reaches the top, or a collapsed tree hides
// it just as effectively as no rollup at all.
func TestRollUpReachesEveryAncestor(t *testing.T) {
	index := map[string][]fsEntry{
		".":     {{Name: "a", Dir: true}},
		"a":     {{Name: "b", Dir: true}},
		"a/b":   {{Name: "c", Dir: true}},
		"a/b/c": {{Name: "deep.txt"}},
	}
	rollUp(index, map[string]byte{"a/b/c/deep.txt": 'M'})

	for dir, want := range map[string]byte{".": 'M', "a": 'M', "a/b": 'M'} {
		if got := index[dir][0].Status; got != want {
			t.Errorf("%s rolled up to %q, want %q", dir, got, want)
		}
	}
	if got := index["a/b/c"][0].Status; got != 0 {
		t.Errorf("the file itself was given %q by the rollup; it carries its own status", got)
	}
}

// TestExplorerLenEmpty tests that an empty Explorer reports 0 length.
func TestExplorerLenEmpty(t *testing.T) {
	m := Model{
		index:   make(map[string][]fsEntry),
		fsIndex: make(map[string][]fsEntry),
		cwd:     ".",
	}

	if len := m.explorerLen(PanelEntries); len != 0 {
		t.Errorf("explorerLen(empty) = %d, want 0", len)
	}
}

// At the root the left column stands for the repository itself. Listing the
// root's children there instead would draw the same rows as the middle column,
// side by side, which reads as a rendering fault rather than as a position.
func TestParentEntriesAtRootShowTheRepositoryItself(t *testing.T) {
	m := Model{
		index:   map[string][]fsEntry{".": {{Name: "file.txt"}, {Name: "dir", Dir: true}}},
		fsIndex: make(map[string][]fsEntry),
		cwd:     ".",
	}

	parents := m.parentEntries()
	if len(parents) != 1 || parents[0].Name != "." {
		t.Errorf("parentEntries() at root = %v, want the single root entry", parents)
	}
}

// Both columns hide the same names, or a file is invisible in one and present
// in the other at the same moment.
func TestParentEntriesHideWhatTheListingHides(t *testing.T) {
	m := Model{
		index: map[string][]fsEntry{
			".":   {{Name: ".gitignore"}, {Name: "src", Dir: true}},
			"src": {{Name: "main.go"}},
		},
		fsIndex: make(map[string][]fsEntry),
		cwd:     "src",
	}

	for _, e := range m.parentEntries() {
		if e.Name == ".gitignore" {
			t.Error("the parent column shows a dotfile the middle column hides")
		}
	}

	m.showHidden = true
	var found bool
	for _, e := range m.parentEntries() {
		found = found || e.Name == ".gitignore"
	}
	if !found {
		t.Error("the parent column still hides a dotfile after showHidden")
	}
}

// TestExplorerTitleWithFromFS tests that title includes " (from disk)" when listing came from ReadDir.
func TestExplorerTitleWithFromFS(t *testing.T) {
	m := Model{
		cwd:     "dir",
		fsIndex: make(map[string][]fsEntry),
	}
	m.fsIndex["dir"] = []fsEntry{{Name: "file.txt"}}

	title := m.explorerTitle(PanelEntries)
	if !contains(title, "(from disk)") {
		t.Errorf("explorerTitle with FromFS = %q, want to contain '(from disk)'", title)
	}
}

func contains(s, substr string) bool {
	for i := range len(s) - len(substr) + 1 {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// navModel is a two-level listing with no git calls behind it, enough to move
// a cursor and step a directory.
func navModel() Model {
	return Model{
		index: map[string][]fsEntry{
			".":   {{Name: "src", Dir: true}, {Name: "a.txt"}, {Name: "b.txt"}},
			"src": {{Name: "main.go"}},
		},
		fsIndex:   make(map[string][]fsEntry),
		dirCursor: make(map[string]int),
		cwd:       ".",
		focus:     PanelEntries,
		width:     120,
		height:    40,
	}
}

// An arrow is a spelling of a letter, not a key of its own. Globally the two
// already agree; the Explorer taking only the letters would break that in one
// tab, which is what made stepping out look impossible from the arrow keys.
func TestExplorerArrowsMeanTheSameAsTheirLetters(t *testing.T) {
	pairs := []struct{ letter, arrow string }{
		{"l", "right"},
		{"h", "left"},
		{"j", "down"},
		{"k", "up"},
	}

	for _, pair := range pairs {
		t.Run(pair.letter+" and "+pair.arrow, func(t *testing.T) {
			// Start one level in, so h has somewhere to go and l does not.
			start := navModel()
			start.cwd = "src"

			byLetter, _, ok := start.explorerKey(pair.letter)
			if !ok {
				t.Fatalf("the Explorer does not handle %q", pair.letter)
			}
			byArrow, _, ok := start.explorerKey(pair.arrow)
			if !ok {
				t.Fatalf("the Explorer does not handle %q", pair.arrow)
			}

			letter, arrow := byLetter.(Model), byArrow.(Model)
			if letter.cwd != arrow.cwd {
				t.Errorf("%q went to %q and %q to %q", pair.letter, letter.cwd, pair.arrow, arrow.cwd)
			}
			if letter.cursor[PanelEntries] != arrow.cursor[PanelEntries] {
				t.Errorf("%q left the cursor at %d and %q at %d",
					pair.letter, letter.cursor[PanelEntries],
					pair.arrow, arrow.cursor[PanelEntries])
			}
		})
	}
}

// The parent column is context, so navigating with it focused has to move the
// listing anyway; the alternative is keys that appear dead.
func TestMovementActsOnTheListingFromEitherColumn(t *testing.T) {
	for _, focus := range []Panel{PanelParent, PanelEntries} {
		m := navModel()
		m.focus = focus

		next, _, ok := m.explorerKey("j")
		if !ok {
			t.Fatal("the Explorer does not handle j")
		}
		if got := next.(Model).cursor[PanelEntries]; got != 1 {
			t.Errorf("with focus %v, j left the listing cursor at %d, want 1", focus, got)
		}
	}
}

func TestMovementStopsAtTheEndsOfTheListing(t *testing.T) {
	m := navModel()

	next, _, _ := m.explorerKey("G")
	if got, want := next.(Model).cursor[PanelEntries], len(m.entries())-1; got != want {
		t.Errorf("G landed on %d, want the last entry %d", got, want)
	}

	next, _, _ = next.(Model).explorerKey("G")
	if got, want := next.(Model).cursor[PanelEntries], len(m.entries())-1; got != want {
		t.Errorf("G past the end landed on %d, want %d", got, want)
	}

	next, _, _ = next.(Model).explorerKey("g")
	if got := next.(Model).cursor[PanelEntries]; got != 0 {
		t.Errorf("g landed on %d, want the first entry", got)
	}
}

// With the preview focused the same keys scroll it. Before this the offset had
// no key at all: it only ever moved when a search jumped to a line.
func TestMovementScrollsThePreviewWhenItHasTheFocus(t *testing.T) {
	m := navModel()
	m.focus = PanelPreview
	m.previewFor = previewID{path: "a.txt", kind: previewContent}
	m.previewContent = strings.Repeat("line\n", 500)

	next, _, ok := m.explorerKey("j")
	if !ok {
		t.Fatal("the Explorer does not handle j")
	}
	after := next.(Model)
	if after.previewOffset != 1 {
		t.Errorf("j scrolled the preview to %d, want 1", after.previewOffset)
	}
	if after.cursor[PanelEntries] != 0 {
		t.Error("j moved the listing cursor while the preview had the focus")
	}

	next, _, _ = after.explorerKey("k")
	if got := next.(Model).previewOffset; got != 0 {
		t.Errorf("k scrolled the preview to %d, want 0", got)
	}
	next, _, _ = next.(Model).explorerKey("k")
	if got := next.(Model).previewOffset; got != 0 {
		t.Errorf("k above the top scrolled to %d, want 0", got)
	}
}

// Scrolling past the end leaves a pane of blanks that no key visibly undoes.
func TestPreviewScrollKeepsTheLastLineOnScreen(t *testing.T) {
	m := navModel()
	m.focus = PanelPreview
	m.previewFor = previewID{path: "a.txt", kind: previewContent}
	m.previewContent = strings.Repeat("line\n", 10)

	next, _, _ := m.explorerKey("G")
	got := next.(Model).previewOffset
	if want := 10 - (m.paneHeight(PanelPreview) - 2); got != max(want, 0) {
		t.Errorf("G scrolled the preview to %d, want %d", got, max(want, 0))
	}
}

// The listing is git's, so a submodule is a gitlink rather than a directory to
// walk into: its contents belong to another repository.
func TestSteppingIntoASubmoduleIsRefused(t *testing.T) {
	m := navModel()
	m.index["."] = []fsEntry{{Name: "vendor", Dir: true, Module: true}}

	next, _, _ := m.explorerKey("l")
	after := next.(Model)
	if after.cwd != "." {
		t.Errorf("l stepped into the submodule at %q", after.cwd)
	}
	if after.status == "" {
		t.Error("l on a submodule did nothing and said nothing")
	}
}

// The flag existed with no key to reach it, which is the same as not having it.
func TestTheDotKeyShowsAndHidesDotfiles(t *testing.T) {
	m := navModel()
	m.index["."] = []fsEntry{{Name: ".env"}, {Name: "a.txt"}}

	if got := len(m.entries()); got != 1 {
		t.Fatalf("the listing starts with %d entries, want the dotfile hidden", got)
	}

	next, _, ok := m.explorerKey(".")
	if !ok {
		t.Fatal("the Explorer does not handle the dot key")
	}
	shown := next.(Model)
	if got := len(shown.entries()); got != 2 {
		t.Errorf("after H the listing holds %d entries, want the dotfile shown", got)
	}
	// The parent column reads the same flag; at the root it stands for the
	// repository itself, so the check is made one level in.
	inner := shown
	inner.cwd = "src"
	var inParent bool
	for _, e := range inner.parentEntries() {
		inParent = inParent || e.Name == ".env"
	}
	if !inParent {
		t.Error("H reached the listing but not the parent column")
	}

	next, _, _ = shown.explorerKey(".")
	if got := len(next.(Model).entries()); got != 1 {
		t.Errorf("H again left %d entries, want the dotfile hidden", got)
	}
}

// H can take the entry under the cursor away with it.
func TestHidingKeepsTheCursorInsideTheShorterListing(t *testing.T) {
	m := navModel()
	m.index["."] = []fsEntry{{Name: "a.txt"}, {Name: ".env"}}
	m.showHidden = true
	m.cursor[PanelEntries] = 1

	next, _, _ := m.explorerKey(".")
	if got := next.(Model).cursor[PanelEntries]; got != 0 {
		t.Errorf("the cursor is at %d after the row under it was hidden, want 0", got)
	}
}

// A disk read is slower than a keypress, so one can land for a directory
// already left. Installing it would show another directory's contents.
func TestAStaleDiskListingIsDiscarded(t *testing.T) {
	m := navModel()
	m.cwd = "src"

	next, _ := m.handleReadDir(readDirMsg{
		path:    "other",
		entries: []fsEntry{{Name: "stale.txt", FromFS: true}},
	})
	if _, ok := next.(Model).fsIndex["other"]; ok {
		t.Error("a listing for a directory the cursor had left was installed")
	}
}

// A remembered position is a cursor too, and an operation can delete the row it
// points at. Restoring it then selects nothing and the preview goes blank.
func TestARememberedPositionIsClampedWhenItsDirectoryShrinks(t *testing.T) {
	m := navModel()
	m.dirCursor["."] = 2
	m.index["."] = []fsEntry{{Name: "a.txt"}}

	m.clampCursors()

	if got := m.dirCursor["."]; got != 0 {
		t.Errorf("the remembered position is %d in a directory of one entry, want 0", got)
	}
}

// Stepping out remembers where the cursor stood, and stepping back in restores
// it — on the synchronous path, where the listing is already in hand.
func TestSteppingBackIntoADirectoryRestoresTheCursor(t *testing.T) {
	m := navModel()
	m.index["src"] = []fsEntry{{Name: "a.go"}, {Name: "b.go"}, {Name: "c.go"}}
	m.cwd = "src"
	m.cursor[PanelEntries] = 2

	next, _, _ := m.explorerKey("h")
	out := next.(Model)
	if out.cwd != "." {
		t.Fatalf("h left the Explorer in %q", out.cwd)
	}

	for i, e := range out.entries() {
		if e.Name == "src" {
			out.cursor[PanelEntries] = i
		}
	}
	next, _, _ = out.explorerKey("l")
	if got := next.(Model).cursor[PanelEntries]; got != 2 {
		t.Errorf("stepping back into src selected entry %d, want the one left at 2", got)
	}
}

// The same, on the async path: the position is applied when the listing lands,
// not when the directory is entered, since the cursor is clamped to an empty
// list while the read is out.
func TestADiskListingRestoresTheCursorWhenItArrives(t *testing.T) {
	m := navModel()
	m.cwd = "build"
	m.dirCursor["build"] = 1

	next, _ := m.handleReadDir(readDirMsg{
		path:    "build",
		entries: []fsEntry{{Name: "one", FromFS: true}, {Name: "two", FromFS: true}},
	})
	if got := next.(Model).cursor[PanelEntries]; got != 1 {
		t.Errorf("the arriving listing selected entry %d, want the remembered 1", got)
	}
}

// While the read is out the column is empty, and saying "empty" about a
// directory whose contents are on their way is a different claim.
func TestAColumnWaitingOnADiskReadSaysSo(t *testing.T) {
	m := navModel()
	m.cwd = "build"

	lines := m.explorerLines(PanelEntries, 10, 40)
	if len(lines) != 1 || !strings.Contains(lines[0], "listing…") {
		t.Errorf("a column waiting on a disk read draws %q", lines)
	}

	next, _ := m.handleReadDir(readDirMsg{path: "build", entries: nil})
	lines = next.(Model).explorerLines(PanelEntries, 10, 40)
	if len(lines) != 1 || !strings.Contains(lines[0], "empty") {
		t.Errorf("a directory that really is empty draws %q", lines)
	}
}

// The two tabs mark for different actions — one resolves a mark through the
// status list, the other lists clean files that have no entry there — so a
// mark made in one must not be reachable from the other.
func TestMarksDoNotCrossBetweenTheTabs(t *testing.T) {
	m := navModel()
	m.fileMarks = map[string]bool{}
	m.snap.Files = []git.FileChange{{Index: 'M', Work: '.', Path: "a.txt"}}

	next, _, _ := m.explorerKey("M")
	marked := next.(Model)
	if len(marked.explorerMarks) != 1 {
		t.Fatalf("M marked %d paths in the Explorer", len(marked.explorerMarks))
	}
	if len(marked.fileMarks) != 0 {
		t.Error("marking in the Explorer reached Local Changes' marks")
	}

	marked.tab = TabChanges
	marked.focus = PanelFiles
	crossed := key(t, marked, "m")
	if len(crossed.explorerMarks) != 1 {
		t.Error("marking in Local Changes reached the Explorer's marks")
	}
}

// The preview pane is not the Explorer's second list, so the key that changes
// what it shows has to work from it as well as from the listing.
func TestTheViewKeyWorksFromEitherPane(t *testing.T) {
	for _, focus := range []Panel{PanelEntries, PanelPreview} {
		m := navModel()
		m.focus = focus
		m.previewFor = previewID{path: "a.txt", kind: previewContent}

		next, _, ok := m.explorerKey("e")
		if !ok {
			t.Fatalf("with focus %v the Explorer does not handle e", focus)
		}
		if got := next.(Model).previewFor.kind; got != previewDiff {
			t.Errorf("with focus %v, e moved to kind %v, want the diff", focus, got)
		}
	}
}

// The filter is a global key, and the listing is what it has to narrow here.
func TestTheFilterNarrowsTheListing(t *testing.T) {
	m := navModel()
	m.tab = TabFiles
	m.focus = PanelEntries

	filtered := key(t, key(t, m, "/"), "b")
	names := []string{}
	for _, e := range filtered.entries() {
		names = append(names, e.Name)
	}
	if len(names) != 1 || names[0] != "b.txt" {
		t.Errorf("filtering by b lists %v, want only b.txt", names)
	}
}

// The preview was reachable only by tab, which nothing on screen mentioned, so
// a file too long for the pane could not be read past its first screen. The
// column to the right of a file is its preview, and that is what right means.
func TestRightOnAFileEntersThePreview(t *testing.T) {
	m := fixture(t)
	m.tab, m.focus = TabFiles, PanelEntries
	m.cwd = "."
	m.index = map[string][]fsEntry{".": {{Name: "long.txt", Cached: true}}}
	m.dirCursor = map[string]int{}
	m.previewFor = previewID{path: "long.txt", kind: previewContent}
	m.previewContent = strings.Repeat("a line\n", 200)

	next, _, ok := m.explorerKey("l")
	if !ok {
		t.Fatal("the Explorer does not handle l")
	}
	moved := next.(Model)
	if moved.focus != PanelPreview {
		t.Fatalf("focus = %v, want the preview", moved.focus)
	}

	// And the movement keys now act on it rather than on the listing.
	scrolled, _, _ := moved.explorerKey("j")
	if got := scrolled.(Model).previewOffset; got == 0 {
		t.Error("j in the preview did not scroll it")
	}
	if got := scrolled.(Model).cursor[PanelEntries]; got != 0 {
		t.Errorf("j moved the listing cursor to %d as well", got)
	}
}

// Left out of the preview is back to the listing it was entered from, not out
// of the directory — otherwise reading a file would cost you your place.
func TestLeftLeavesThePreviewForTheListing(t *testing.T) {
	m := fixture(t)
	m.tab, m.focus = TabFiles, PanelPreview
	m.cwd = "docs"
	m.index = map[string][]fsEntry{"docs": {{Name: "a.md", Cached: true}}}
	m.dirCursor = map[string]int{}

	next, _, _ := m.explorerKey("h")
	back := next.(Model)

	if back.focus != PanelEntries {
		t.Errorf("focus = %v, want the file listing", back.focus)
	}
	if back.cwd != "docs" {
		t.Errorf("cwd = %q, want to still be in the directory", back.cwd)
	}
}

// A file with nothing to show is not somewhere to stand: the pane would take
// the movement keys and answer none of them.
func TestRightStaysPutWhenThereIsNothingToRead(t *testing.T) {
	m := fixture(t)
	m.tab, m.focus = TabFiles, PanelEntries
	m.cwd = "."
	m.index = map[string][]fsEntry{".": {{Name: "empty.txt", Cached: true}}}
	m.dirCursor = map[string]int{}
	m.previewFor = previewID{path: "empty.txt", kind: previewContent}

	next, _, _ := m.explorerKey("l")
	if got := next.(Model).focus; got != PanelEntries {
		t.Errorf("focus = %v, want to stay on the listing", got)
	}
}

// The listing is rebuilt on the tick while the Explorer is open, and the
// remembered position was reapplied every time — so moving to another file
// held for a second or two and then jumped back to wherever the directory had
// been entered at.
func TestTheTickDoesNotDragTheCursorBack(t *testing.T) {
	tree := []git.TreeEntry{
		{Path: "a.txt", Cached: true, Mode: "100644"},
		{Path: "b.txt", Cached: true, Mode: "100644"},
		{Path: "c.txt", Cached: true, Mode: "100644"},
	}

	m := fixture(t)
	m.tab, m.focus, m.cwd = TabFiles, PanelEntries, "."

	// Landing: the position the directory was entered at comes back, because
	// the cursor was zeroed while there was no listing to hold it.
	m.dirCursor["."] = 1
	next, _ := m.handleLoadIndex(loadIndexMsg{entries: tree})
	m = next.(Model)
	if m.cursor[PanelEntries] != 1 {
		t.Fatalf("landing put the cursor at %d, want the remembered row", m.cursor[PanelEntries])
	}

	// Reading on: the reader moves, and the next tick must leave that alone.
	m.cursor[PanelEntries] = 2
	next, _ = m.handleLoadIndex(loadIndexMsg{entries: tree})
	m = next.(Model)

	if m.cursor[PanelEntries] != 2 {
		t.Errorf("the tick moved the cursor from row 2 to %d", m.cursor[PanelEntries])
	}
}

// Leaving a directory still records where the cursor stood, so stepping back
// out and in again lands on the entry it was left on rather than on the first.
func TestSteppingBackOutStillRemembersThePosition(t *testing.T) {
	m := fixture(t)
	m.tab, m.focus, m.cwd = TabFiles, PanelEntries, "docs"
	m.index = map[string][]fsEntry{
		".":    {{Name: "docs", Dir: true}, {Name: "z.txt", Cached: true}},
		"docs": {{Name: "a.md", Cached: true}, {Name: "b.md", Cached: true}},
	}
	m.dirCursor = map[string]int{}
	m.cursor[PanelEntries] = 1

	next, _, _ := m.explorerKey("h")
	m = next.(Model)

	if m.dirCursor["docs"] != 1 {
		t.Errorf("leaving docs recorded row %d, want 1", m.dirCursor["docs"])
	}
}
