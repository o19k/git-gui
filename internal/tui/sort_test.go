package tui

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func names(entries []fsEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func sortFixture() []fsEntry {
	return []fsEntry{
		{Name: "zeta.go", Status: 'M'},
		{Name: "alpha.md"},
		{Name: "src", Dir: true},
		{Name: "beta.go", Status: '?'},
		{Name: "docs", Dir: true},
	}
}

// A file listing that scattered directories through the names would be harder
// to read rather than differently ordered, so they stay above in every order —
// reversed included, which is the one that would silently break the rule.
func TestDirectoriesStayAboveFilesInEveryOrder(t *testing.T) {
	stats := map[string]fileMeta{
		"zeta.go":  {size: 10, mtime: time.Unix(300, 0)},
		"alpha.md": {size: 30, mtime: time.Unix(100, 0)},
		"beta.go":  {size: 20, mtime: time.Unix(200, 0)},
	}

	for _, mode := range []sortMode{sortName, sortStatus, sortExtension, sortSize, sortTime} {
		for _, reverse := range []bool{false, true} {
			entries := sortFixture()
			sortListing(entries, ".", mode, reverse, stats)

			for i, e := range entries {
				if e.Dir && i >= 2 {
					t.Errorf("%s reverse=%t: directory %q sank to row %d: %v",
						sortNames[mode], reverse, e.Name, i, names(entries))
				}
			}
		}
	}
}

func TestSortByName(t *testing.T) {
	entries := sortFixture()
	sortListing(entries, ".", sortName, false, nil)

	want := []string{"docs", "src", "alpha.md", "beta.go", "zeta.go"}
	if got := names(entries); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The status order matches the ranking the rows are already coloured by, so
// what the order picks out is what the eye was picking out.
func TestSortByStatusPutsWhatChangedFirst(t *testing.T) {
	entries := sortFixture()
	sortListing(entries, ".", sortStatus, false, nil)

	files := names(entries)[2:]
	if files[len(files)-1] != "alpha.md" {
		t.Errorf("the clean file is not last: %v", files)
	}
	if !slices.Contains(files[:2], "zeta.go") || !slices.Contains(files[:2], "beta.go") {
		t.Errorf("the changed files are not first: %v", files)
	}
}

func TestSortByExtension(t *testing.T) {
	entries := sortFixture()
	sortListing(entries, ".", sortExtension, false, nil)

	want := []string{"beta.go", "zeta.go", "alpha.md"}
	if got := names(entries)[2:]; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A dotfile is a name that begins with a dot, not a file of type "gitignore".
func TestADotfileHasNoExtension(t *testing.T) {
	if got := extensionOf(".gitignore"); got != "" {
		t.Errorf("extensionOf(.gitignore) = %q", got)
	}
	if got := extensionOf("main.go"); got != "go" {
		t.Errorf("extensionOf(main.go) = %q", got)
	}
	if got := extensionOf("Makefile"); got != "" {
		t.Errorf("extensionOf(Makefile) = %q", got)
	}
}

// Asking for size is asking what is big, and asking for a time is asking what
// is recent. Both lead with the answer.
func TestSizeAndTimeLeadWithTheLargestAndTheNewest(t *testing.T) {
	stats := map[string]fileMeta{
		"zeta.go":  {size: 10, mtime: time.Unix(300, 0)},
		"alpha.md": {size: 30, mtime: time.Unix(100, 0)},
		"beta.go":  {size: 20, mtime: time.Unix(200, 0)},
	}

	entries := sortFixture()
	sortListing(entries, ".", sortSize, false, stats)
	if got := names(entries)[2]; got != "alpha.md" {
		t.Errorf("largest first put %q at the top: %v", got, names(entries))
	}

	entries = sortFixture()
	sortListing(entries, ".", sortTime, false, stats)
	if got := names(entries)[2]; got != "zeta.go" {
		t.Errorf("newest first put %q at the top: %v", got, names(entries))
	}
}

// The numbers are read one directory at a time, so a listing is sorted before
// they arrive. Falling back to name order keeps that from looking shuffled.
func TestAnOrderWithNoNumbersYetFallsBackToName(t *testing.T) {
	entries := sortFixture()
	sortListing(entries, ".", sortSize, false, map[string]fileMeta{})

	want := []string{"alpha.md", "beta.go", "zeta.go"}
	if got := names(entries)[2:]; !slices.Equal(got, want) {
		t.Errorf("got %v, want it in name order: %v", got, want)
	}
}

// Ordering by name costs nothing to answer, and reaching the filesystem for a
// listing nobody asked to size would be work no key requested.
func TestOnlySizeAndTimeReachTheFilesystem(t *testing.T) {
	for _, mode := range []sortMode{sortName, sortStatus, sortExtension} {
		if mode.needsStat() {
			t.Errorf("%s asks for a stat it does not use", sortNames[mode])
		}
	}
	for _, mode := range []sortMode{sortSize, sortTime} {
		if !mode.needsStat() {
			t.Errorf("%s sorts by a number nothing reads", sortNames[mode])
		}
	}

	m := navModel()
	m.sortMode = sortName
	if cmd := m.statDir("."); cmd != nil {
		t.Error("name order went to the filesystem")
	}
}

// The picker's action runs after Update has returned, against a copy, so the
// choice travels as a message. This is the whole reason for sortMsg.
func TestChoosingAnOrderAppliesIt(t *testing.T) {
	m := navModel()
	m.stats = make(map[string]fileMeta)

	next, _ := m.handleSort(sortMsg{mode: sortExtension})
	after := next.(Model)

	if after.sortMode != sortExtension {
		t.Fatalf("sortMode = %v", after.sortMode)
	}
	if !strings.Contains(after.explorerTitle(PanelEntries), "extension") {
		t.Errorf("the title does not name the order: %q", after.explorerTitle(PanelEntries))
	}
}

// Name order is what a listing looks like anyway, so it says nothing; anything
// else has to, or a surprising arrangement reads as a fault.
func TestOnlyAnUnusualOrderIsAnnounced(t *testing.T) {
	m := navModel()
	if got := m.sortLabel(); got != "" {
		t.Errorf("name order announced itself as %q", got)
	}

	m.sortReverse = true
	if !strings.Contains(m.sortLabel(), "name") {
		t.Errorf("reversed name order says %q", m.sortLabel())
	}
}

// The listings are shared with every earlier copy of the model, so re-ordering
// has to replace them rather than sort them where they lie.
func TestReorderingDoesNotWriteThroughASharedListing(t *testing.T) {
	m := navModel()
	m.stats = make(map[string]fileMeta)
	before := m.index["."]
	snapshot := names(before)

	next, _ := m.handleSort(sortMsg{mode: sortName, flip: true})
	after := next.(Model)

	if got := names(before); !slices.Equal(got, snapshot) {
		t.Errorf("the listing an earlier model holds was re-ordered under it: %v", got)
	}
	if slices.Equal(names(after.index["."]), snapshot) {
		t.Error("reversing changed nothing")
	}
}

// Stats arrive per directory and merge into what is already known; replacing
// the map wholesale would forget every other directory visited.
func TestArrivingNumbersMergeRatherThanReplace(t *testing.T) {
	m := navModel()
	m.sortMode = sortSize
	m.stats = map[string]fileMeta{"src/main.go": {size: 5}}

	next, _ := m.handleStats(statsMsg{dir: ".", meta: map[string]fileMeta{
		"a.txt": {size: 10},
		"b.txt": {size: 20},
	}})
	after := next.(Model)

	for _, path := range []string{"src/main.go", "a.txt", "b.txt"} {
		if _, ok := after.stats[path]; !ok {
			t.Errorf("%s was forgotten", path)
		}
	}
	if got := names(after.entries()); got[1] != "b.txt" {
		t.Errorf("the listing was not re-ordered by the numbers that arrived: %v", got)
	}
}
