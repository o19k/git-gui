package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

func TestReflogListsWhereHeadHasBeen(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(reflogMsg{entries: []git.ReflogEntry{
		{SHA: "aaa", Short: "aaa1111", Selector: "HEAD@{0}", Action: "commit: first", When: "now"},
		{SHA: "bbb", Short: "bbb2222", Selector: "HEAD@{1}", Action: "reset: moving to HEAD~1", When: "a minute ago"},
	}})
	m = next.(Model)

	if m.overlay.kind != overlayList {
		t.Fatalf("the reflog did not open a picker: overlay kind = %d", m.overlay.kind)
	}
	if len(m.overlay.items) != 2 {
		t.Fatalf("items = %d, want 2", len(m.overlay.items))
	}
	// What the choice is made on has to be visible: the position, the commit
	// and what moved it.
	if label := m.overlay.items[1].label; !strings.Contains(label, "HEAD@{1}") ||
		!strings.Contains(label, "reset") || !strings.Contains(label, "bbb2222") {
		t.Errorf("label = %q", label)
	}
}

func TestEmptyReflogSaysSoRatherThanOpeningNothing(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(reflogMsg{})
	m = next.(Model)

	if m.overlay.kind != overlayNone {
		t.Error("an empty reflog opened a picker with nothing in it")
	}
	if m.status == "" {
		t.Error("nothing said why the picker did not open")
	}
}

// The safe way out comes first, and the one that overwrites the working tree
// is marked as such.
func TestReflogOffersTheSafeWayOutFirst(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(reflogPickMsg{entry: git.ReflogEntry{
		SHA: "aaa", Short: "aaa1111", Selector: "HEAD@{3}", Action: "commit: the lost one",
	}})
	m = next.(Model)

	if m.overlay.kind != overlayChoice {
		t.Fatalf("picking an entry did not ask what to do: kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.choices[0].label, "branch") {
		t.Errorf("the first choice is %q, want the one that moves nothing", m.overlay.choices[0].label)
	}

	reset := m.overlay.choices[len(m.overlay.choices)-1]
	if !reset.danger {
		t.Error("resetting the branch is not marked destructive")
	}
}

func TestReflogResetConfirmsBeforeOverwritingTheTree(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(reflogResetMsg{entry: git.ReflogEntry{SHA: "aaa", Short: "aaa1111"}})
	m = next.(Model)

	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Fatalf("no destructive confirm: kind = %d danger = %v", m.overlay.kind, m.overlay.danger)
	}
	if !strings.Contains(m.overlay.body, "aaa1111") {
		t.Errorf("the confirm does not name where it moves to: %q", m.overlay.body)
	}
}

func TestReflogBranchAsksForAName(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(reflogBranchMsg{entry: git.ReflogEntry{SHA: "aaa", Short: "aaa1111"}})
	m = next.(Model)

	if m.overlay.kind != overlayInput {
		t.Fatalf("no prompt for the branch name: kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.title, "aaa1111") {
		t.Errorf("prompt title = %q", m.overlay.title)
	}
}
