package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/o19k/git-gui/internal/git"

	"github.com/o19k/git-gui/internal/theme"
)

// The open tab was named by a fill one step off the background, which at these
// two greys reads as a smudge rather than as a selection. It is named by the
// accent and an underline now, and this asserts the two styles land on the
// tabs they belong to rather than that one of them exists somewhere.
func TestTheOpenTabIsToldApartFromTheOthers(t *testing.T) {
	withColour(t)

	for open := range tabCount {
		t.Run(tabTitles[open], func(t *testing.T) {
			m := fixture(t)
			m.tab = Tab(open)
			bar := m.tabBar()

			for other := range tabCount {
				title := " " + tabTitles[other]
				active := theme.TabActiveStyle.UnsetPadding().Render(title)
				idle := theme.TabStyle.UnsetPadding().Render(title)

				if Tab(other) == m.tab {
					if !strings.Contains(bar, active) {
						t.Errorf("the open tab %q is not drawn as open", tabTitles[other])
					}
					if strings.Contains(bar, idle) {
						t.Errorf("the open tab %q is also drawn as closed", tabTitles[other])
					}
					continue
				}
				if !strings.Contains(bar, idle) {
					t.Errorf("the closed tab %q is not drawn as closed", tabTitles[other])
				}
				if strings.Contains(bar, active) {
					t.Errorf("the closed tab %q is drawn as open", tabTitles[other])
				}
			}
		})
	}
}

// The cursors already survive a tab switch. The focus did not, so a glance at
// another tab cost a walk back across the columns on return.
func TestATabIsReopenedWhereItWasLeft(t *testing.T) {
	m := fixture(t)

	m = open(t, m, TabLog)
	m.focus = PanelCommits
	m = open(t, m, TabChanges)
	m.focus = PanelDiff
	m = open(t, m, TabLog)

	if m.focus != PanelCommits {
		t.Errorf("the Log tab reopened on %v, want the pane it was left on", m.focus)
	}
	if m = open(t, m, TabChanges); m.focus != PanelDiff {
		t.Errorf("Local Changes reopened on %v, want the pane it was left on", m.focus)
	}
}

// Nothing is remembered about a tab that has never been open, and the first
// column is the wrong answer for the Explorer.
func TestAnUnvisitedTabLandsOnItsFirstWorkingPane(t *testing.T) {
	for tab := range tabCount {
		if Tab(tab) == TabChanges {
			continue // where a fresh model already stands
		}
		m := open(t, fixture(t), Tab(tab))
		if m.focus != landingPane[tab] {
			t.Errorf("%s opened on %v, want %v", tabTitles[tab], m.focus, landingPane[tab])
		}
	}
	if landingPane[TabFiles] == tabPanes[TabFiles][0] {
		t.Error("the Explorer lands on its parent column, which is context rather than work")
	}
}

// A model built by a zero value rather than by New remembers a pane belonging
// to no tab, and restoring it would strand the focus outside the layout.
func TestAPaneFromAnotherTabIsNotRestored(t *testing.T) {
	m := fixture(t)
	m.lastFocus[TabLog] = PanelStash

	if m = open(t, m, TabLog); m.focus != landingPane[TabLog] {
		t.Errorf("focus = %v, want the Log tab's own landing pane", m.focus)
	}
}

// Revealing a path is a request for somewhere specific, which outranks
// wherever the tab happened to be left.
func TestRevealingAPathOutranksTheRememberedPane(t *testing.T) {
	m := fixture(t)
	m = open(t, m, TabChanges)
	m.focus = PanelDiff
	m = open(t, m, TabLog)

	next, _ := m.showInChanges("dirty.go")
	moved := next.(Model)
	if moved.focus != PanelFiles {
		t.Errorf("focus = %v, want the file list the path is in", moved.focus)
	}
	if got := moved.files()[moved.cursor[PanelFiles]].Path; got != "dirty.go" {
		t.Errorf("the cursor is on %q, want the revealed path", got)
	}
}

func open(t *testing.T, m Model, tab Tab) Model {
	t.Helper()
	next, _ := m.openTab(tab)
	return next.(Model)
}

// The label and the digit are styled separately, and rendering one styled
// string inside another pads it twice and closes the outer style at the inner
// one's reset.
func TestEveryTabLabelIsPaddedOnce(t *testing.T) {
	withColour(t)

	plain := ansi.Strip(fixture(t).tabBar())
	for tab := range tabCount {
		want := "  " + string(rune('1'+tab)) + " " + tabTitles[tab] + "  "
		if !strings.Contains(plain, want) {
			t.Errorf("the tab bar reads %q, want it to hold %q", plain, want)
		}
	}
}

// The key list is longer than a terminal is tall, so it scrolls rather than
// being cut to fit and hiding every section past the middle.
func TestTheKeyListReachesItsLastSection(t *testing.T) {
	all := helpLines()
	last := ansi.Strip(strings.Join(all, "\n"))
	if !strings.Contains(last, "draw the branch graph") {
		t.Fatal("the graph key is not documented at all")
	}

	m := fixture(t)
	m.height, m.overlay = 40, overlay{kind: overlayHelp}

	if strings.Contains(ansi.Strip(m.helpView()), "draw the branch graph") {
		t.Skip("the list now fits on one screen")
	}

	next, _ := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	scrolled := next.(Model)
	if scrolled.overlay.kind != overlayHelp {
		t.Fatal("a movement key closed the help instead of moving through it")
	}
	if !strings.Contains(ansi.Strip(scrolled.helpView()), "draw the branch graph") {
		t.Error("the end of the key list is unreachable")
	}
}

// Scrolling made the movement keys mean something here, and a reader who is
// done should not have to find which key is still the way out.
func TestAnyOtherKeyStillClosesTheKeyList(t *testing.T) {
	m := fixture(t)
	m.overlay = overlay{kind: overlayHelp}

	next, _ := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if next.(Model).overlay.kind != overlayNone {
		t.Error("the help stayed open")
	}
}

// A view toggle leaves no trace in the panel it belongs to, so the footer is
// the only place it can be found without opening the key list.
func TestTheGraphKeyIsInTheCommitsFooter(t *testing.T) {
	m := fixture(t)
	m.tab, m.focus = TabLog, PanelCommits

	if !strings.Contains(ansi.Strip(m.footer()), "L graph") {
		t.Errorf("the Commits footer reads %q", ansi.Strip(m.footer()))
	}
}

// A panel binding is offered a key before the navigation is, so file history on
// h swallowed the only letter that walks back out of the commit's file list.
func TestHLeavesTheCommitFileListForTheCommits(t *testing.T) {
	m := fixture(t)
	m = open(t, m, TabLog)
	m.focus = PanelCommitFiles
	m.commitFiles = []git.FileChange{{Index: 'M', Path: "touched.go"}}

	moved := key(t, m, "h")
	if moved.focus != PanelCommits {
		t.Errorf("h landed on %v, want the commit list to its left", moved.focus)
	}
}

// The same key in Local Changes, where nothing sits to the left, still has no
// business asking git for a history.
func TestHistoryMovedToShiftH(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommitFiles)
	m.commitFiles = []git.FileChange{{Index: 'M', Path: "touched.go"}}

	if _, _, handled := m.commitFilesKey("h"); handled {
		t.Error("h is still bound in the commit's file list")
	}
	if _, _, handled := m.commitFilesKey("H"); !handled {
		t.Error("H does not open the file's history")
	}
}
