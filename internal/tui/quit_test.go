package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// quits reports whether a command is the one that ends the program. tea.Quit
// is a function, so it is identified by what it returns.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQuittingAsksFirst(t *testing.T) {
	m := fixture(t)

	next, cmd := press(t, m, "q")

	if quits(cmd) {
		t.Fatal("q left without asking")
	}
	if next.overlay.kind != overlayConfirm {
		t.Fatalf("no question was asked: overlay kind %d", next.overlay.kind)
	}
	if !strings.Contains(next.overlay.title, "Quit") {
		t.Errorf("the question is titled %q", next.overlay.title)
	}
}

// The question is worth more when it says what is at stake, and what is at
// stake is whatever has not been committed.
func TestTheQuitQuestionCountsWhatIsUncommitted(t *testing.T) {
	m := fixture(t) // three changed files
	next, _ := press(t, m, "q")

	if !strings.Contains(next.overlay.body, strconv.Itoa(len(m.snap.Files))) {
		t.Errorf("body = %q, want the count in it", next.overlay.body)
	}

	clean := fixture(t)
	clean.snap = git.Snapshot{Branch: "main"}
	next, _ = press(t, clean, "q")
	if strings.Contains(next.overlay.body, "uncommitted") {
		t.Errorf("a clean tree was asked about changes: %q", next.overlay.body)
	}
}

func TestAnsweringTheQuitQuestionQuits(t *testing.T) {
	m := fixture(t)
	m, _ = press(t, m, "q")

	_, cmd := press(t, m, "y")

	if !quits(cmd) {
		t.Error("answering yes did not quit")
	}
}

// ctrl+c is the terminal's own abort. A program that argues with it is a
// program you cannot get out of.
func TestCtrlCLeavesWithoutAsking(t *testing.T) {
	m := fixture(t)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if !quits(cmd) {
		t.Error("ctrl+c did not quit")
	}
	if next.(Model).overlay.kind != overlayNone {
		t.Error("ctrl+c opened a question instead of leaving")
	}
}
