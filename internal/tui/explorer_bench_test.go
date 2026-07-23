package tui

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

// A directory of 50,000 entries is the case the listing has to stay cheap in,
// and the guard is that only the visible window is ever styled. A regression
// here would not show up as a wrong pixel, only as a slow one — which is why
// it is a budget and not an eyeball.
func BenchmarkExplorerListing50k(b *testing.B) {
	entries := make([]fsEntry, 50_000)
	for i := range entries {
		entries[i] = fsEntry{Name: "file_" + strconv.Itoa(i) + ".go", Status: 'M'}
	}

	m := Model{
		index:      map[string][]fsEntry{".": entries},
		fsIndex:    map[string][]fsEntry{},
		cwd:        ".",
		showHidden: true,
	}
	m.focus = PanelEntries

	b.ReportAllocs()
	for b.Loop() {
		if lines := m.explorerLines(PanelEntries, 40, 80); len(lines) == 0 {
			b.Fatal("rendered nothing")
		}
	}
}

// The frame's cost must scale with the window drawn, not with the directory
// behind it. Timing that would be flaky; what actually regresses is the copy,
// so the copy is what is asserted.
func TestUnfilteredListingIsNotCopied(t *testing.T) {
	entries := make([]fsEntry, 4)
	m := Model{
		index:      map[string][]fsEntry{".": entries},
		cwd:        ".",
		showHidden: true,
	}

	got := m.entries()
	if len(got) != len(entries) || &got[0] != &entries[0] {
		t.Error("an unfiltered listing was copied; a large directory then pays that on every frame")
	}

	m.showHidden = false
	m.index["."] = []fsEntry{{Name: ".hidden"}, {Name: "shown"}}
	if got := m.entries(); len(got) != 1 || got[0].Name != "shown" {
		t.Errorf("filtering still has to work: got %v", got)
	}
}

// The listing is rebuilt on every frame, so a directory big enough to be slow
// makes every keystroke slow. The budget is a frame's worth of work; the
// fastest of several runs is used, since a loaded machine measures the machine
// rather than the code.
func TestALargeListingRendersWithinAFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}

	const (
		entries = 50_000
		budget  = 5 * time.Millisecond
	)

	listing := make([]fsEntry, entries)
	for i := range listing {
		listing[i] = fsEntry{Name: fmt.Sprintf("file%05d.go", i), Status: 'M'}
	}
	m := Model{
		index:      map[string][]fsEntry{".": listing},
		fsIndex:    make(map[string][]fsEntry),
		dirCursor:  make(map[string]int),
		cwd:        ".",
		focus:      PanelEntries,
		width:      120,
		height:     40,
		showHidden: true,
	}

	best := time.Duration(1<<62 - 1)
	for range 5 {
		start := time.Now()
		lines := m.explorerLines(PanelEntries, 38, 40)
		if elapsed := time.Since(start); elapsed < best {
			best = elapsed
		}
		if len(lines) == 0 {
			t.Fatal("the listing rendered nothing, so this measures nothing")
		}
	}

	if best > budget {
		t.Errorf("a %d-entry listing takes %v to draw, over the %v budget", entries, best, budget)
	}
}
