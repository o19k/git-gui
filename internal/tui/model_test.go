package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
)

// fixture builds a sized model holding a known snapshot, without touching git.
func fixture(t *testing.T) Model {
	t.Helper()
	m := New(context.Background(), nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(snapshotMsg(git.Snapshot{
		Branch: "main",
		Files: []git.FileChange{
			{Index: 'M', Work: '.', Path: "staged.go"},
			{Index: '.', Work: 'M', Path: "dirty.go"},
			{Index: '?', Work: '?', Path: "new.go"},
		},
		Branches: []git.Branch{
			{Name: "main", Kind: git.RefLocal, Head: true, Upstream: "origin/main", Ahead: 2, Behind: 1},
			{Name: "origin/main", Kind: git.RefRemote},
		},
		Commits: []git.Commit{
			{SHA: "aaa", Short: "aaa1111", Subject: "first", Parents: []string{"p"}},
			{SHA: "bbb", Short: "bbb2222", Subject: "merge", Parents: []string{"p", "q"}},
		},
		Stashes: []git.Stash{{Ref: "stash@{0}", Subject: "WIP"}},
	}))
	return next.(Model)
}

// onPane opens the tab that owns p and focuses it, the way the number key and
// then tab would.
func onPane(t *testing.T, m Model, p Panel) Model {
	t.Helper()
	next, _ := m.openTab(tabOf(p))
	m = next.(Model)
	m.focus = p
	return m
}

func key(t *testing.T, m Model, k string) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return next.(Model)
}

func TestTabBarNamesEveryTabAndTheRepositoryState(t *testing.T) {
	view := fixture(t).View()

	for i, want := range tabTitles {
		if !strings.Contains(view, want) {
			t.Errorf("the tab bar is missing %q", want)
		}
		if !strings.Contains(view, fmt.Sprint(i+1)) {
			t.Errorf("tab %q does not show the key that opens it", want)
		}
	}
	// The corner carries what the Status panel used to.
	for _, want := range []string{"main", "↑2", "↓1"} {
		if !strings.Contains(view, want) {
			t.Errorf("the corner is missing %q", want)
		}
	}
}

func TestEachTabShowsItsOwnPanes(t *testing.T) {
	cases := []struct {
		tab  Tab
		want []string
	}{
		{TabChanges, []string{"Changes — 3 files", "staged.go", "dirty.go", "new.go"}},
		{TabLog, []string{"Branches — 2 refs", "Commits — 2 commits", "first", "merge"}},
		{TabStash, []string{"Stash — 1 entry", "WIP"}},
	}
	for _, c := range cases {
		next, _ := fixture(t).openTab(c.tab)
		view := next.(Model).View()
		for _, want := range c.want {
			if !strings.Contains(view, want) {
				t.Errorf("tab %d is missing %q", c.tab, want)
			}
		}
	}
}

func TestViewFillsTerminalExactly(t *testing.T) {
	m := fixture(t)
	view := m.View()

	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Errorf("view has %d lines, terminal is %d tall", len(lines), m.height)
	}
	// Every row must be exactly the terminal width, or the frame tears.
	for i, line := range lines {
		if w := lipgloss.Width(line); w != m.width {
			t.Errorf("line %d is %d columns wide, want %d", i, w, m.width)
		}
	}
}

func TestLongContentDoesNotTearTheFrame(t *testing.T) {
	// A wrapped row would push every panel below it down.
	long := strings.Repeat("very long commit subject ", 20)
	m := fixture(t)
	next, _ := m.Update(snapshotMsg(git.Snapshot{
		Branch:  strings.Repeat("branch-", 30),
		Files:   []git.FileChange{{Index: 'M', Work: '.', Path: strings.Repeat("deep/path/", 20) + "file.go"}},
		Commits: []git.Commit{{SHA: "aaa", Short: "aaa1111", Subject: long, Parents: []string{"p"}}},
	}))
	m = next.(Model)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Errorf("long content made the view %d lines tall, terminal is %d", len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != m.width {
			t.Errorf("line %d is %d columns, want %d", i, w, m.width)
		}
	}
}

func TestPaneWidthsFillTheTerminalExactly(t *testing.T) {
	m := fixture(t)

	for _, w := range []int{60, 100, 200} {
		m.width = w
		for tab := range tabCount {
			m.tab = Tab(tab)
			widths := m.paneWidths()

			if len(widths) != len(tabColumns[tab]) {
				t.Fatalf("tab %d: %d widths for %d columns", tab, len(widths), len(tabColumns[tab]))
			}
			total := 0
			for i, cw := range widths {
				if cw < minPaneW {
					t.Errorf("tab %d at width %d: column %d is %d wide", tab, w, i, cw)
				}
				total += cw
			}
			if total != w {
				t.Errorf("tab %d: columns total %d, terminal is %d", tab, total, w)
			}

			// A column of stacked panes must fill its height exactly too.
			for _, column := range tabColumns[tab] {
				rows := 0
				for _, h := range m.paneHeights(column) {
					rows += h
				}
				if rows != m.bodyHeight() {
					t.Errorf("tab %d: a column totals %d rows, body is %d", tab, rows, m.bodyHeight())
				}
			}
		}
	}
}

func TestEveryTabRendersWithoutTearing(t *testing.T) {
	m := fixture(t)
	for tab := range tabCount {
		next, _ := m.openTab(Tab(tab))
		tabbed := next.(Model)

		lines := strings.Split(tabbed.View(), "\n")
		if len(lines) != m.height {
			t.Errorf("tab %d: view is %d lines, terminal is %d", tab, len(lines), m.height)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w != m.width {
				t.Errorf("tab %d line %d: %d columns, want %d", tab, i, w, m.width)
			}
		}
	}
}

func TestShortTerminalKeepsEveryPanelFramed(t *testing.T) {
	m := fixture(t)
	for _, h := range []int{10, 14, 17, 20} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: h})
		m = next.(Model)

		lines := strings.Split(m.View(), "\n")
		if len(lines) != h {
			t.Errorf("height %d: view is %d lines", h, len(lines))
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w != 100 {
				t.Errorf("height %d, line %d: %d columns, want 100", h, i, w)
			}
		}
	}
}

func TestNumberKeysOpenTabs(t *testing.T) {
	m := fixture(t)
	for i, k := range []string{"2", "3", "1"} {
		m = key(t, m, k)
		want := Tab(k[0] - '1')
		if m.tab != want {
			t.Errorf("key %q opened tab %d, want %d", k, m.tab, want)
		}
		if m.focus != tabPanes[want][0] {
			t.Errorf("key %q (step %d) left focus on pane %d, want the tab's first", k, i, m.focus)
		}
	}
}

func TestTabWalksThePanesAndStopsAtTheEnds(t *testing.T) {
	m := key(t, fixture(t), "2") // Log: branches | commits | details
	panes := tabPanes[TabLog]

	for i := 1; i < len(panes); i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if m.focus != panes[i] {
			t.Fatalf("after %d tabs focus is %d, want %d", i, m.focus, panes[i])
		}
	}

	// Past the last column there is nowhere to go: it must not wrap into
	// another tab's panes.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m = next.(Model); m.focus != panes[len(panes)-1] {
		t.Errorf("tab past the last pane moved focus to %d", m.focus)
	}
	for range len(panes) + 2 {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		m = next.(Model)
	}
	if m.focus != panes[0] {
		t.Errorf("shift+tab past the first pane moved focus to %d", m.focus)
	}
}

func TestOpeningATabAgainKeepsTheSelection(t *testing.T) {
	m := key(t, fixture(t), "2") // Log
	m = key(t, m, "j")           // second branch
	m = key(t, m, "1")
	m = key(t, m, "2")

	if m.cursor[PanelBranches] != 1 {
		t.Errorf("returning to the Log tab reset the selection to %d", m.cursor[PanelBranches])
	}
}

func TestTheContentPaneScrollsInsteadOfSelecting(t *testing.T) {
	m := fixture(t)
	m.mainContent = longDiff(200)
	before := m.cursor[PanelFiles]

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // into the diff
	m = next.(Model)
	if m.focus != PanelDiff {
		t.Fatalf("tab landed on pane %d, want the diff", m.focus)
	}

	m = key(t, m, "j")
	if m.mainOffset != 1 {
		t.Errorf("j in the diff scrolled to %d, want 1", m.mainOffset)
	}
	if m.cursor[PanelFiles] != before {
		t.Error("j in the diff moved the file selection")
	}

	m = key(t, m, "G")
	if m.mainOffset == 0 {
		t.Error("G in the diff did not jump to the bottom")
	}
}

func TestCursorMovementIsBounded(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles) // 3 entries

	for range 10 {
		m = key(t, m, "j")
	}
	if got := m.cursor[PanelFiles]; got != 2 {
		t.Errorf("cursor ran to %d, want it pinned at the last entry (2)", got)
	}

	for range 10 {
		m = key(t, m, "k")
	}
	if got := m.cursor[PanelFiles]; got != 0 {
		t.Errorf("cursor ran to %d, want it pinned at 0", got)
	}
}

func TestCursorOnEmptyPanelDoesNotMove(t *testing.T) {
	m := New(context.Background(), nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m = onPane(t, m, PanelFiles)
	m = key(t, m, "j")

	if m.cursor[PanelFiles] != 0 {
		t.Errorf("cursor moved in an empty panel: %d", m.cursor[PanelFiles])
	}
	if !strings.Contains(m.View(), "working tree clean") {
		t.Error("an empty Files panel should say so")
	}
}

func TestStalePreviewIsDiscarded(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m = key(t, m, "j") // now on dirty.go; previewKey follows the selection

	// A reply for the previously selected file arrives late.
	next, _ := m.Update(previewMsg{key: "file:staged.go", title: "Diff — staged.go", content: "stale"})
	m = next.(Model)

	if m.mainContent == "stale" {
		t.Error("a preview for an abandoned selection was painted over the current one")
	}

	next, _ = m.Update(previewMsg{key: m.previewKey, title: "Diff — dirty.go", content: "fresh"})
	if m = next.(Model); m.mainContent != "fresh" {
		t.Errorf("preview for the current selection was dropped: %q", m.mainContent)
	}
}

func TestSnapshotShrinkClampsCursor(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits)
	m = key(t, m, "G") // last commit
	if m.cursor[PanelCommits] != 1 {
		t.Fatalf("G left cursor at %d, want 1", m.cursor[PanelCommits])
	}

	// A refresh where history got rewritten out from under the cursor.
	next, _ := m.Update(snapshotMsg(git.Snapshot{Branch: "main"}))
	if m = next.(Model); m.cursor[PanelCommits] != 0 {
		t.Errorf("cursor left dangling at %d after the list emptied", m.cursor[PanelCommits])
	}
	if strings.Contains(m.View(), "panic") {
		t.Error("view broke after the list emptied")
	}
}

// j closed the help before the key list could be scrolled. Now that the list
// is longer than the screen, the movement keys move through it and everything
// else is still the way out.
func TestHelpOpensAndAnyKeyButAMoveCloses(t *testing.T) {
	m := key(t, fixture(t), "?")
	if m.overlay.kind != overlayHelp {
		t.Fatal("? did not open help")
	}
	if !strings.Contains(m.View(), "open a tab") {
		t.Error("help view is missing its key list")
	}

	if m = key(t, m, "j"); m.overlay.kind != overlayHelp {
		t.Error("a movement key closed the help instead of moving through it")
	}
	if m = key(t, m, "esc"); m.overlay.kind != overlayNone {
		t.Error("help should close on any key that is not a move")
	}
}

func TestSmallTerminalDoesNotPanic(t *testing.T) {
	m := New(context.Background(), nil)
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 6}, {Width: 4, Height: 3}, {Width: 200, Height: 60}} {
		next, _ := m.Update(size)
		m = next.(Model)
		_ = m.View() // must not panic at any size
	}
}

// A tab is registered in ten places, and every one of them tolerates an
// unknown tab quietly: a missing layout draws nothing, a missing title draws an
// empty string, a missing weight slices out of range and takes the process with
// it. Half-registering a tab is therefore something you find by using it, one
// symptom at a time. This walks all ten instead, so the tab after this one is
// found by a test rather than by a user.
func TestEveryTabIsFullyRegistered(t *testing.T) {
	for tab := range tabCount {
		tab := Tab(tab)
		t.Run(tabTitles[tab], func(t *testing.T) {
			if tabTitles[tab] == "" {
				t.Error("the tab has no title, so the tab bar shows a gap")
			}
			if len(tabColumns[tab]) == 0 {
				t.Fatal("the tab has no layout, so it draws nothing at all")
			}
			if got, want := len(tabWeights[tab]), len(tabColumns[tab]); got != want {
				t.Fatalf("the tab has %d column weights for %d columns; split slices out of range on a short one", got, want)
			}
			if len(tabPanes[tab]) == 0 {
				t.Fatal("the tab has no panes in focus order")
			}

			// The tab is reached by its number key, not only by openTab.
			opened := key(t, fixture(t), string(rune('1'+int(tab))))
			if opened.tab != tab {
				t.Fatalf("pressing %d opened tab %v", tab+1, opened.tab)
			}

			for _, p := range tabPanes[tab] {
				if title := opened.panelTitle(p); title == "" {
					t.Errorf("pane %v has no title, so it draws an unlabelled box", p)
				}
				if lines := opened.panelLines(p, 10, 40); lines == nil {
					t.Errorf("pane %v draws nothing, not even a placeholder", p)
				}
				if hints := opened.panelKeyHints(p); hints == nil {
					t.Errorf("pane %v offers no footer keys", p)
				}
				opened.panelLen(p) // must not panic on an unknown pane
			}

			// The whole frame renders, which is where a nil weight panics.
			if view := opened.View(); view == "" {
				t.Error("the tab renders an empty view")
			}
		})
	}
}

// The help is the only place the tabs are enumerated in prose, and it was
// written when there were three of them.
func TestHelpNamesEveryTab(t *testing.T) {
	m := fixture(t)
	m.overlay = overlay{kind: overlayHelp}
	help := m.View()

	for tab := range tabCount {
		if !strings.Contains(help, tabTitles[tab]) {
			t.Errorf("the help does not mention the %s tab", tabTitles[tab])
		}
	}
}
