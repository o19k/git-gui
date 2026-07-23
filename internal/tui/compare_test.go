package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

func TestStartCompareWithHeadBranchShows(t *testing.T) {
	// startCompare should reject if the branch is already checked out (Head: true).
	m := fixture(t)

	m, cmd := m.startCompare(git.Branch{Name: "main", Head: true})

	if cmd != nil {
		t.Error("comparing the branch you are on ran a git command")
	}
	if !strings.Contains(m.status, "already checked out") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

func TestStartCompareNonHeadBranchInitiates(t *testing.T) {
	// startCompare should set compareRef and busy when given a non-HEAD branch.
	m := fixture(t)

	branch := git.Branch{Name: "feature", Kind: git.RefLocal, Head: false}
	m, _ = m.startCompare(branch)

	if m.compareRef != "feature" {
		t.Errorf("compareRef = %q, want 'feature'", m.compareRef)
	}
	if m.compareAll {
		t.Errorf("compareAll = %v, want false at start", m.compareAll)
	}
	if !strings.Contains(m.busy, "comparing") {
		t.Errorf("busy = %q, want 'comparing' message", m.busy)
	}
}

func TestShowCompareDropsStaleReplies(t *testing.T) {
	// showCompare should ignore messages for branches the cursor has left.
	m := fixture(t)
	m.compareRef = "stale-branch"

	msg := compareMsg{ref: "other-branch"}
	next, _ := m.showCompare(msg)
	m = next.(Model)

	if m.compareRef != "stale-branch" {
		t.Errorf("compareRef changed to %q, should have been dropped", m.compareRef)
	}
}

func TestShowComparePresentsFindingsAsText(t *testing.T) {
	// showCompare should open a text overlay with commits and files formatted.
	m := fixture(t)
	m.compareRef = "feature"

	msg := compareMsg{
		ref: "feature",
		commits: []git.Commit{
			{SHA: "aaa", Short: "aaa1111", Subject: "add feature file"},
		},
		files: []git.FileChange{
			{Index: 'A', Work: '.', Path: "feature.txt"},
			{Index: 'M', Work: '.', Path: "shared.txt"},
		},
	}

	next, _ := m.showCompare(msg)
	m = next.(Model)

	if m.overlay.kind != overlayText {
		t.Errorf("overlay.kind = %d, want overlayText (%d)", m.overlay.kind, overlayText)
	}
	if len(m.overlay.lines) == 0 {
		t.Fatal("overlay.lines is empty")
	}

	allText := strings.Join(m.overlay.lines, "\n")
	if !strings.Contains(allText, "add feature file") {
		t.Errorf("overlay does not contain the commit subject")
	}
	if !strings.Contains(allText, "feature.txt") {
		t.Errorf("overlay does not contain the file path")
	}
}

func TestCompareLinesTouches(t *testing.T) {
	// compareLines should produce output with both commits and files sections.
	commits := []git.Commit{
		{Short: "abc1234", Subject: "test commit"},
	}
	files := []git.FileChange{
		{Index: 'M', Work: '.', Path: "test.go"},
	}

	text := strings.Join(compareLines(commits, files), "\n")

	for _, want := range []string{"1 commit", "1 file", "abc1234", "test commit", "test.go"} {
		if !strings.Contains(text, want) {
			t.Errorf("the comparison is missing %q", want)
		}
	}
}

func TestCompareLinesTouchesEmptyLists(t *testing.T) {
	// Nothing to show is a finding of its own — an empty window would read as a
	// comparison that failed rather than one that found no difference.
	text := strings.Join(compareLines(nil, nil), "\n")

	if !strings.Contains(text, "HEAD already has everything") {
		t.Error("an empty commit list does not say what that means")
	}
	if !strings.Contains(text, "identical") {
		t.Error("an empty file list does not say what that means")
	}
}

func TestToggleScopeWhenNotComparingReturnsNil(t *testing.T) {
	// toggleCompareScope should do nothing if no comparison is active.
	m := fixture(t)
	m.compareRef = ""

	cmd := m.toggleCompareScope()

	if cmd != nil {
		t.Errorf("toggleCompareScope returned a command when compareRef was empty")
	}
}

func TestToggleScopeTogglesFlag(t *testing.T) {
	// toggleCompareScope should flip the compareAll flag.
	m := fixture(t)
	m.compareRef = "feature"
	m.compareAll = false
	m.snap = git.Snapshot{
		Branches: []git.Branch{
			{Name: "feature", Kind: git.RefLocal, Head: false},
		},
	}

	_ = m.toggleCompareScope()

	if !m.compareAll {
		t.Errorf("compareAll = %v, want true after toggle", m.compareAll)
	}
}

func TestOverlayTextFKeyTogglesCompare(t *testing.T) {
	// Pressing 'f' while viewing a comparison should toggle the scope.
	m := fixture(t)
	m.compareRef = "feature"
	m.compareAll = false
	m.snap = git.Snapshot{
		Branches: []git.Branch{
			{Name: "feature", Kind: git.RefLocal, Head: false},
		},
	}
	m.overlay = overlay{kind: overlayText, lines: []string{"test"}, compare: true}

	next, cmd := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = next.(Model)

	if cmd == nil {
		t.Error("f did not reload the comparison")
	}
	if !m.compareAll {
		t.Error("f did not change the file scope")
	}
	if !strings.Contains(m.View(), "file scope") {
		t.Error("the window does not say f is available")
	}
}

func TestOverlayTextFKeyIgnoredWithoutCompare(t *testing.T) {
	// A comparison left behind must not leave f live in the next text window —
	// file history opens the same overlay and f means nothing there.
	m := fixture(t)
	m.compareRef = "feature"
	m.overlay = overlay{kind: overlayText, lines: []string{"test"}}

	next, cmd := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = next.(Model)

	if cmd != nil {
		t.Errorf("handleOverlayKey returned command for 'f' when no comparison was active")
	}
	if m.overlay.kind != overlayText {
		t.Errorf("overlay was closed by 'f' without an active comparison")
	}
}
