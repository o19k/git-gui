package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAPullBlockedByLocalChangesOffersToStashThem(t *testing.T) {
	m := fixture(t)
	m.busy = "pulling…"

	next, cmd := m.Update(pullMsg{
		what: "pull",
		err:  errors.New("git pull: error: Your local changes to the following files would be overwritten by merge: f.txt"),
	})
	m = next.(Model)

	if cmd != nil {
		t.Error("a blocked pull ran something instead of asking")
	}
	if m.overlay.kind != overlayChoice {
		t.Fatalf("no offer was made: overlay kind = %d", m.overlay.kind)
	}
	if m.busy != "" {
		t.Errorf("the in-flight note survived the failure: %q", m.busy)
	}

	view := m.View()
	for _, want := range []string{"Stash", "put them back"} {
		if !strings.Contains(view, want) {
			t.Errorf("the offer does not mention %q", want)
		}
	}
}

func TestTakingTheStashOfferRunsAPull(t *testing.T) {
	m := fixture(t)
	next, _ := m.Update(pullMsg{err: errors.New("git pull: Your local changes would be overwritten")})
	m = next.(Model)

	m, cmd := press(t, m, "1")

	if cmd == nil {
		t.Fatal("choosing to stash and pull ran nothing")
	}
	if m.overlay.kind != overlayNone {
		t.Error("the offer stayed on screen after being answered")
	}
	if m.busy != "pulling…" {
		t.Errorf("busy = %q, want the footer to say a pull is running", m.busy)
	}
}

func TestADivergedPullOffersRebaseAndMerge(t *testing.T) {
	m := fixture(t)
	next, _ := m.Update(pullMsg{err: errors.New("git pull: fatal: Not possible to fast-forward, aborting.")})
	m = next.(Model)

	if m.overlay.kind != overlayChoice {
		t.Fatalf("no offer was made: overlay kind = %d", m.overlay.kind)
	}
	if len(m.overlay.choices) != 2 {
		t.Fatalf("offered %d ways forward, want rebase and merge", len(m.overlay.choices))
	}
	view := m.View()
	for _, want := range []string{"Rebase", "Merge", "diverged"} {
		if !strings.Contains(view, want) {
			t.Errorf("the offer does not mention %q", want)
		}
	}
}

func TestAPullThatConflictsSaysSoRatherThanReadingAsSuccess(t *testing.T) {
	m := fixture(t)

	next, cmd := m.Update(pullMsg{what: "pull", conflicts: []string{"a.txt", "b.txt"}})
	m = next.(Model)

	if cmd == nil {
		t.Error("a landed pull did not refresh the panels")
	}
	// A count sent the reader to the file list to work out which files it meant.
	if !strings.Contains(m.status, "a.txt") || !strings.Contains(m.status, "b.txt") {
		t.Errorf("status = %q, want the conflicted paths named", m.status)
	}
	if !strings.Contains(m.status, "press r") {
		t.Errorf("status = %q, want the key that settles them", m.status)
	}
}

// The footer is one line, so past a handful the names stop fitting and the
// count is the honest thing to show.
func TestManyConflictsAreCountedRatherThanListed(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(pullMsg{what: "pull", conflicts: []string{"a", "b", "c", "d"}})
	if got := next.(Model).status; !strings.Contains(got, "4 files") {
		t.Errorf("status = %q, want a count", got)
	}
}

func TestACleanPullLeavesNoticeBehind(t *testing.T) {
	m := fixture(t)
	m.status = "stale error"
	m.busy = "pulling…"

	next, cmd := m.Update(pullMsg{what: "pull"})
	m = next.(Model)

	if cmd == nil {
		t.Error("a clean pull did not refresh the panels")
	}
	if m.status != "" || m.busy != "" {
		t.Errorf("left something on the footer: status=%q busy=%q", m.status, m.busy)
	}
}

func TestAPullFailingForAnyOtherReasonStillShowsIt(t *testing.T) {
	m := fixture(t)
	next, cmd := m.Update(pullMsg{err: errors.New("git pull: could not read from remote")})
	m = next.(Model)

	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("an unrecognised failure was turned into an offer")
	}
	if !strings.Contains(m.status, "could not read from remote") {
		t.Errorf("status = %q, want git's own words", m.status)
	}
}

// git names the files it refused over, and the dialog said only that "changes"
// were in the way — leaving the reader to run git by hand to find out which.
func TestTheStashDialogNamesTheFilesInTheWay(t *testing.T) {
	m := fixture(t)
	err := errors.New("error: Your local changes to the following files would be overwritten by merge:\n" +
		"\tdocs/runbook/solana.md\n\tinternal/git/remote.go\n" +
		"Please commit your changes or stash them before you merge.\nAborting")

	next, _ := m.Update(pullMsg{what: "pull", err: err})
	m = next.(Model)

	if m.overlay.kind != overlayChoice {
		t.Fatalf("overlay = %v, want the stash question", m.overlay.kind)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"solana.md", "remote.go"} {
		if !strings.Contains(view, want) {
			t.Errorf("the dialog does not name %s", want)
		}
	}
}

// The refusal that names nothing must still ask the question rather than
// drawing an empty list where the paths would be.
func TestAnUnnamedRefusalStillAsks(t *testing.T) {
	m := fixture(t)
	err := errors.New("error: cannot pull with rebase: You have unstaged changes.")

	next, _ := m.Update(pullMsg{what: "pull", err: err})
	m = next.(Model)

	if m.overlay.kind != overlayChoice {
		t.Fatalf("overlay = %v, want the stash question", m.overlay.kind)
	}
	if len(m.overlay.extra) != 0 {
		t.Errorf("extra = %v, want nothing where git named nothing", m.overlay.extra)
	}
}

// A pull blocked by a whole directory must not push the choices off the box.
func TestABlockedPullListsAtMostAHandful(t *testing.T) {
	var paths []string
	for i := range 30 {
		paths = append(paths, fmt.Sprintf("pkg/file%d.go", i))
	}

	lines := pathLines(paths)
	if len(lines) != maxBlockingPaths+1 {
		t.Fatalf("drew %d lines for %d paths", len(lines), len(paths))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "22 more files") {
		t.Errorf("the tail reads %q, want what was cut counted", ansi.Strip(lines[len(lines)-1]))
	}
}
