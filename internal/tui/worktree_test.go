package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

func TestWorktreesListMarksTheOneBeingLookedAt(t *testing.T) {
	m := fixture(t)
	m.repo = &git.Repo{Path: "/home/x/project"}

	next, _ := m.Update(worktreesMsg{trees: []git.Worktree{
		{Path: "/home/x/project", Branch: "main"},
		{Path: "/home/x/project-fix", Branch: "fix"},
	}})
	m = next.(Model)

	if m.overlay.kind != overlayList {
		t.Fatalf("no picker: overlay kind = %d", m.overlay.kind)
	}
	// Two checkouts and the row that makes another.
	if len(m.overlay.items) != 3 {
		t.Fatalf("items = %d, want the two checkouts and a way to add one", len(m.overlay.items))
	}
	if !strings.HasPrefix(m.overlay.items[0].label, "▸") {
		t.Errorf("the open checkout is not marked: %q", m.overlay.items[0].label)
	}
	if !strings.Contains(m.overlay.items[2].label, "new worktree") {
		t.Errorf("the last row is %q", m.overlay.items[2].label)
	}
}

func TestOpeningTheCheckoutAlreadyOpenSaysSo(t *testing.T) {
	m := fixture(t)
	m.repo = &git.Repo{Path: "/home/x/project"}

	next, _ := m.Update(worktreePickMsg{tree: git.Worktree{Path: "/home/x/project/", Branch: "main"}})
	m = next.(Model)

	if m.overlay.kind == overlayChoice {
		t.Error("the checkout being looked at was offered as one to open")
	}
	if m.status == "" {
		t.Error("nothing said why the choice did not open")
	}
}

func TestPickingAnotherCheckoutOffersOpenAndRemove(t *testing.T) {
	m := fixture(t)
	m.repo = &git.Repo{Path: "/home/x/project"}

	next, _ := m.Update(worktreePickMsg{tree: git.Worktree{Path: "/home/x/project-fix", Branch: "fix"}})
	m = next.(Model)

	if m.overlay.kind != overlayChoice {
		t.Fatalf("no choice: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.choices[0].label, "Open") {
		t.Errorf("the first choice is %q", m.overlay.choices[0].label)
	}
	if !m.overlay.choices[1].danger {
		t.Error("removing a checkout is not marked destructive")
	}
}

func TestNewWorktreeAsksForThePathThenTheBranch(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(newWorktreeMsg{})
	m = next.(Model)
	if m.overlay.kind != overlayInput || !strings.Contains(m.overlay.title, "directory") {
		t.Fatalf("the path was not asked for: kind = %d title = %q", m.overlay.kind, m.overlay.title)
	}

	next, _ = m.Update(worktreeBranchMsg{path: "../project-fix"})
	m = next.(Model)
	if m.overlay.kind != overlayInput {
		t.Fatalf("the branch was not asked for: kind = %d", m.overlay.kind)
	}
	// The directory's own name is the branch name people mean nine times in
	// ten, so it is offered.
	if m.overlay.value != "project-fix" {
		t.Errorf("prefill = %q, want the directory's name", m.overlay.value)
	}
}

// Opening another checkout swaps what the whole tool is looking at, and
// nothing read from the old one means anything in the new one.
func TestOpeningACheckoutStartsFromTheNewRepository(t *testing.T) {
	m := fixture(t)
	m.settings.Light = true
	m.splitDiff = true

	opened := &git.Repo{Path: "/home/x/project-fix"}
	next, _ := m.Update(openedMsg{repo: opened})
	m = next.(Model)

	if m.repo != opened {
		t.Fatal("the model is still pointed at the old checkout")
	}
	if len(m.snap.Files) != 0 || len(m.snap.Commits) != 0 {
		t.Error("the old checkout's snapshot was carried across")
	}
	if !m.settings.Light || !m.splitDiff {
		t.Error("preferences were dropped along with the repository")
	}
	if !strings.Contains(m.status, "project-fix") {
		t.Errorf("status = %q, want it to name what was opened", m.status)
	}
}

func TestOpeningAFailedCheckoutKeepsTheOldOne(t *testing.T) {
	m := fixture(t)
	before := m.repo

	next, _ := m.Update(openedMsg{err: errNotARepo})
	m = next.(Model)

	if m.repo != before {
		t.Error("a failed open replaced the repository anyway")
	}
	if m.status == "" {
		t.Error("the failure was not reported")
	}
}

var errNotARepo = errors.New("not a git repository")
