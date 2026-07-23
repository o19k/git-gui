package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeFilter opens the filter on the focused pane and types term into it.
func typeFilter(t *testing.T, m Model, term string) Model {
	t.Helper()
	m = key(t, m, "/")
	if !m.filtering {
		t.Fatal("/ did not open the filter")
	}
	for _, r := range term {
		m = key(t, m, string(r))
	}
	return m
}

func TestFilterNarrowsTheList(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelFiles), "dirty")

	if got := m.panelLen(PanelFiles); got != 1 {
		t.Fatalf("filter left %d of 3 files, want 1", got)
	}
	if f, ok := m.selectedFile(); !ok || f.Path != "dirty.go" {
		t.Errorf("selection = %+v, want dirty.go", f)
	}

	view := m.View()
	if !strings.Contains(view, "dirty.go") {
		t.Error("the matching file is not shown")
	}
	if strings.Contains(view, "staged.go") {
		t.Error("a non-matching file is still listed")
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelBranches), "ORIGIN")

	if got := m.panelLen(PanelBranches); got != 1 {
		t.Errorf("case-insensitive filter matched %d refs, want 1", got)
	}
}

func TestFilterMatchesCommitsBySubjectAndSha(t *testing.T) {
	for _, term := range []string{"merge", "bbb2222"} {
		m := typeFilter(t, onPane(t, fixture(t), PanelCommits), term)
		if got := m.panelLen(PanelCommits); got != 1 {
			t.Errorf("%q matched %d commits, want 1", term, got)
		}
		if c, ok := m.selectedCommit(); !ok || c.Short != "bbb2222" {
			t.Errorf("%q selected %+v", term, c)
		}
	}
}

func TestTypingAFilterDoesNotTriggerBindings(t *testing.T) {
	// "d" is discard in Changes and "c" is commit; while filtering both are text.
	m := typeFilter(t, onPane(t, fixture(t), PanelFiles), "dc")

	if m.overlay.kind != overlayNone {
		t.Errorf("a letter typed into the filter opened overlay %d", m.overlay.kind)
	}
	if m.filter[PanelFiles] != "dc" {
		t.Errorf("filter = %q, want %q", m.filter[PanelFiles], "dc")
	}
}

func TestEnterKeepsTheFilterAndEscapeClearsIt(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelFiles), "dirty")

	kept, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := kept.(Model)
	if after.filtering {
		t.Error("enter did not stop typing")
	}
	if after.filter[PanelFiles] != "dirty" {
		t.Errorf("enter dropped the filter: %q", after.filter[PanelFiles])
	}
	// With typing over, the letters are bindings again.
	if bound, _ := press(t, after, "d"); bound.overlay.kind != overlayConfirm {
		t.Error("d did not go back to meaning discard")
	}

	cleared, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if after := cleared.(Model); after.filtering || after.filter[PanelFiles] != "" {
		t.Errorf("esc left filtering=%v filter=%q", after.filtering, after.filter[PanelFiles])
	}
}

func TestFilterKeepsTheCursorInsideTheShorterList(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m = key(t, m, "G") // last of three
	if m.cursor[PanelFiles] != 2 {
		t.Fatalf("G left the cursor at %d", m.cursor[PanelFiles])
	}

	m = typeFilter(t, m, "dirty") // one match, so index 2 no longer exists

	if m.cursor[PanelFiles] != 0 {
		t.Errorf("cursor left dangling at %d after the filter shrank the list", m.cursor[PanelFiles])
	}
	if f, ok := m.selectedFile(); !ok || f.Path != "dirty.go" {
		t.Errorf("selection = %+v, want the only match", f)
	}
}

func TestAFilterMatchingNothingSaysSo(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelFiles), "zzz")

	if _, ok := m.selectedFile(); ok {
		t.Error("an empty filtered list still reported a selection")
	}
	if !strings.Contains(m.View(), "nothing matches zzz") {
		t.Error("the pane does not say why it is empty")
	}
	// Acting on nothing must not panic or reach a hidden entry.
	if next, _ := press(t, m, "d"); next.overlay.kind != overlayNone {
		t.Error("d acted on a filtered-out file")
	}
}

func TestTheFilterTermIsVisibleInThePaneTitle(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelBranches), "main")

	if !strings.Contains(m.View(), "/main") {
		t.Error("the title does not show what is being filtered on")
	}
}

func TestEachPaneKeepsItsOwnFilter(t *testing.T) {
	m := typeFilter(t, onPane(t, fixture(t), PanelFiles), "dirty")
	m, _ = press(t, m, "enter")
	m = onPane(t, m, PanelCommits)

	if m.filter[PanelCommits] != "" {
		t.Errorf("the Changes filter leaked into Commits: %q", m.filter[PanelCommits])
	}
	if m.panelLen(PanelCommits) != 2 {
		t.Error("Commits was narrowed by another pane's filter")
	}
	if m.filter[PanelFiles] != "dirty" {
		t.Error("moving away dropped the Changes filter")
	}
}
