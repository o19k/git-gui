package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// worktreePanelFixture puts the model on the Log tab's Worktrees pane, with the
// tool itself open in the first checkout. The window is wide, so the narrow
// first column still has room for the paths beside the branch names.
func worktreePanelFixture(t *testing.T) Model {
	t.Helper()
	m := fixture(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m = next.(Model)
	m.repo = &git.Repo{Path: "/home/x/project"}
	m.snap.Worktrees = []git.Worktree{
		{Path: "/home/x/project", Branch: "main"},
		{Path: "/home/x/wt/auth", Branch: "agent/auth"},
		{Path: "/home/x/wt/detached", Head: "deadbeefcafe"},
	}
	return onPane(t, m, PanelWorktrees)
}

func TestWorktreePanelListsCheckoutsAndMarksTheOpenOne(t *testing.T) {
	m := worktreePanelFixture(t)
	view := m.View()

	for _, want := range []string{"Worktrees — 3 worktrees", "agent/auth", "/home/x/wt/auth"} {
		if !strings.Contains(view, want) {
			t.Errorf("the worktrees pane is missing %q", want)
		}
	}
	// The open checkout carries the marker; the others do not.
	if !strings.Contains(view, "▸") {
		t.Error("the open checkout is not marked")
	}
}

func TestWorktreePanelAimsTheCommitListAtTheSelectedCheckout(t *testing.T) {
	m := worktreePanelFixture(t)

	// The second checkout: a branch, so the commit list should point at its ref.
	m.cursor[PanelWorktrees] = 1
	m.refreshPreview()

	if m.previewKey != "worktree:/home/x/wt/auth" {
		t.Errorf("previewKey = %q, want the checkout's own key", m.previewKey)
	}
	if m.logRef != "refs/heads/agent/auth" {
		t.Errorf("logRef = %q, want the checkout's branch", m.logRef)
	}
}

func TestADetachedWorktreeAimsAtItsCommit(t *testing.T) {
	m := worktreePanelFixture(t)
	m.cursor[PanelWorktrees] = 2
	m.refreshPreview()

	if m.logRef != "deadbeefcafe" {
		t.Errorf("logRef = %q, want the detached HEAD's commit", m.logRef)
	}
}

func TestEnterOnAnotherCheckoutOpensIt(t *testing.T) {
	m := worktreePanelFixture(t)
	m.cursor[PanelWorktrees] = 1

	_, cmd, handled := m.worktreesKey("enter")
	if !handled {
		t.Fatal("enter was not handled by the worktrees pane")
	}
	if cmd == nil {
		t.Error("opening another checkout queued no command")
	}
}

func TestEnterOnTheOpenCheckoutSaysSo(t *testing.T) {
	m := worktreePanelFixture(t)
	m.cursor[PanelWorktrees] = 0

	next, _, _ := m.worktreesKey("enter")
	if s := next.(Model).status; !strings.Contains(s, "already open") {
		t.Errorf("status = %q, want it to say the checkout is already open", s)
	}
}

func TestRemovingTheOpenWorktreeIsRefused(t *testing.T) {
	m := worktreePanelFixture(t)
	m.cursor[PanelWorktrees] = 0

	next, _, _ := m.worktreesKey("d")
	nm := next.(Model)
	if nm.overlay.kind == overlayConfirm {
		t.Error("removing the worktree in use was offered anyway")
	}
	if nm.status == "" {
		t.Error("nothing said why the removal was refused")
	}
}

func TestRemovingAnotherWorktreeAsksFirst(t *testing.T) {
	m := worktreePanelFixture(t)
	m.cursor[PanelWorktrees] = 1

	next, _, _ := m.worktreesKey("d")
	nm := next.(Model)
	if nm.overlay.kind != overlayConfirm {
		t.Fatalf("no confirm: overlay kind = %d", nm.overlay.kind)
	}
	if !nm.overlay.danger {
		t.Error("removing a checkout is not marked destructive")
	}
}

func TestNewWorktreeFromThePaneAsksForThePath(t *testing.T) {
	m := worktreePanelFixture(t)

	_, cmd, handled := m.worktreesKey("n")
	if !handled || cmd == nil {
		t.Fatal("n did not start adding a worktree")
	}
	// The command asks for a new worktree, which the model turns into an input.
	next, _ := m.Update(cmd())
	if next.(Model).overlay.kind != overlayInput {
		t.Errorf("adding a worktree did not open an input: kind = %d", next.(Model).overlay.kind)
	}
}
