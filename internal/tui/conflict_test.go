package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// conflictFixture is the standard model with one path left unmerged.
func conflictFixture(t *testing.T) Model {
	t.Helper()
	m := fixture(t)
	m.snap.Files = append(m.snap.Files, git.FileChange{Index: 'U', Work: 'U', Path: "merged.go"})
	return m
}

func TestConflictsTakeOverTheBanner(t *testing.T) {
	m := conflictFixture(t)
	m.snap.Rebasing = true

	view := m.View()
	if !strings.Contains(view, "1 file conflicted") {
		t.Error("the banner does not say the repository is conflicted")
	}
	if strings.Contains(view, "rebase stopped") {
		t.Error("the rebase line hid the conflict, which has to be settled first")
	}
	if strings.Contains(view, "staged ·") {
		t.Error("the counts hid the conflict")
	}
}

func TestTheFooterOffersTheKeysThatSettleAConflict(t *testing.T) {
	m := onPane(t, conflictFixture(t), PanelFiles)

	view := m.View()
	for _, want := range []string{"resolve", "mark resolved"} {
		if !strings.Contains(view, want) {
			t.Errorf("the footer does not offer %q", want)
		}
	}
}

func TestResolveOffersBothSidesAndTheHandEditedFile(t *testing.T) {
	m := onPane(t, conflictFixture(t), PanelFiles)
	m.cursor[PanelFiles] = 3 // merged.go

	m, cmd := press(t, m, "r")

	if cmd != nil {
		t.Error("r settled the conflict without asking which way")
	}
	if m.overlay.kind != overlayChoice {
		t.Fatalf("r did not offer anything: overlay kind = %d", m.overlay.kind)
	}
	if len(m.overlay.choices) != 3 {
		t.Fatalf("offered %d ways out, want ours, theirs and mark resolved", len(m.overlay.choices))
	}
	view := m.View()
	for _, want := range []string{"merged.go", "Keep ours", "Keep theirs", "Mark resolved"} {
		if !strings.Contains(view, want) {
			t.Errorf("the offer does not mention %q", want)
		}
	}
}

func TestResolveOnAFileThatIsNotConflictedSaysSo(t *testing.T) {
	m := onPane(t, conflictFixture(t), PanelFiles)
	m.cursor[PanelFiles] = 0 // staged.go

	m, cmd := press(t, m, "r")

	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("r acted on a file with no conflict")
	}
	if !strings.Contains(m.status, "not conflicted") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

// A conflicted path reads as staged, so space would otherwise take it back out
// of the index rather than settling it.
func TestSpaceMarksAConflictResolvedRatherThanUnstagingIt(t *testing.T) {
	m := onPane(t, conflictFixture(t), PanelFiles)
	m.cursor[PanelFiles] = 3

	file, ok := m.selectedFile()
	if !ok || !conflicted(file) {
		t.Fatal("the fixture does not have a conflict selected")
	}
	// A conflicted path passes Staged, which is the trap this guards against.
	if !file.Staged() {
		t.Fatal("this test no longer exercises what it was written for")
	}
	if _, cmd := press(t, m, " "); cmd == nil {
		t.Error("space did nothing on a conflicted file")
	}
	// What it actually runs is checked end to end in TestEndToEndConflict.
}

func TestTheBannerGoesBackToCountsOnceNothingIsConflicted(t *testing.T) {
	view := onPane(t, fixture(t), PanelFiles).View()

	if strings.Contains(view, "conflicted") {
		t.Error("a clean tree still claims to be conflicted")
	}
	if !strings.Contains(view, "staged ·") {
		t.Error("the counts never came back")
	}
}

func TestFileHistoryOpensAsAReadableList(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(fileHistoryMsg{
		path: "dirty.go",
		log:  "aaa1111  first\x1fAda, 2 days ago\nbbb2222  second\x1fGrace, 3 days ago\n",
	})
	m = next.(Model)

	if m.overlay.kind != overlayText {
		t.Fatalf("the history did not open: overlay kind = %d", m.overlay.kind)
	}
	view := m.View()
	for _, want := range []string{"History — dirty.go", "first", "second", "Ada"} {
		if !strings.Contains(view, want) {
			t.Errorf("the history is missing %q", want)
		}
	}
}

func TestFileHistoryFailureSurfacesInsteadOfOpeningEmpty(t *testing.T) {
	m := fixture(t)
	next, _ := m.Update(fileHistoryMsg{path: "dirty.go", err: errFake})
	m = next.(Model)

	if m.overlay.kind != overlayNone {
		t.Error("a failed history still opened a window")
	}
	if !strings.Contains(m.status, "boom") {
		t.Errorf("status = %q, want git's message", m.status)
	}
}

func TestHistoryIsRefusedForAFileGitHasNeverSeen(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 2 // new.go, untracked

	m, cmd := press(t, m, "H")

	if cmd != nil {
		t.Error("H asked git for the history of a file it does not know")
	}
	if !strings.Contains(m.status, "no history") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}
