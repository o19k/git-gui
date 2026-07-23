package tui

import (
	"strings"
	"testing"
)

func TestStagingEverythingAndTakingItAllBackAreBothBound(t *testing.T) {
	cases := map[string]string{"a": "stage all", "u": "unstage all"}
	for k, want := range cases {
		m := onPane(t, fixture(t), PanelFiles)
		if _, cmd := press(t, m, k); cmd == nil {
			t.Errorf("%q ran nothing, want %s", k, want)
		}
	}
}

func TestTheFooterNamesBothWholeIndexKeys(t *testing.T) {
	view := onPane(t, fixture(t), PanelFiles).View()

	for _, want := range []string{"stage all", "unstage all"} {
		if !strings.Contains(view, want) {
			t.Errorf("the footer does not offer %q", want)
		}
	}
}

func TestUntrackAsksAndIsRefusedForAFileGitNeverSaw(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 0 // staged.go, tracked

	m, cmd := press(t, m, "t")
	if cmd != nil {
		t.Error("t untracked without asking")
	}
	if m.overlay.kind != overlayConfirm || !strings.Contains(m.overlay.body, "staged.go") {
		t.Errorf("no confirm naming the file: kind=%d body=%q", m.overlay.kind, m.overlay.body)
	}

	m = onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 2 // new.go, untracked
	m, cmd = press(t, m, "t")
	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("t acted on a file git does not follow")
	}
	if !strings.Contains(m.status, "untracked") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

func TestIgnoringATrackedFileSaysItWillBeUntrackedToo(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 0 // staged.go, tracked

	m, _ = press(t, m, "i")

	if m.overlay.kind != overlayConfirm {
		t.Fatalf("i did not ask: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.body, "untracked") {
		t.Errorf("the confirm hides the second half of the action: %q", m.overlay.body)
	}
}

func TestIgnoringAnUntrackedFileIsJustThePattern(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 2 // new.go

	m, _ = press(t, m, "i")

	if !strings.Contains(m.overlay.body, ".gitignore") {
		t.Errorf("the confirm does not say what it edits: %q", m.overlay.body)
	}
	if strings.Contains(m.overlay.body, "untracked too") {
		t.Errorf("the confirm promises to untrack a file git does not follow: %q", m.overlay.body)
	}
}

func TestDeletingAFileIsGatedAsDestructive(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)

	m, cmd := press(t, m, "x")

	if cmd != nil {
		t.Error("x deleted without asking")
	}
	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Errorf("delete is not gated as destructive: kind=%d danger=%v", m.overlay.kind, m.overlay.danger)
	}
	if !strings.Contains(m.overlay.body, "cannot be undone") {
		t.Errorf("the confirm does not say it is final: %q", m.overlay.body)
	}
}

func TestRenamingABranchIsOfferedWithTheOldNameFilledIn(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches)
	m.cursor[PanelBranches] = 0 // main

	m, cmd := press(t, m, "m")

	if cmd != nil {
		t.Error("m renamed without asking")
	}
	if m.overlay.kind != overlayInput || m.overlay.value != "main" {
		t.Errorf("no prompt pre-filled with the old name: kind=%d value=%q", m.overlay.kind, m.overlay.value)
	}
}

func TestARemoteBranchCannotBeRenamedHere(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches)
	m.cursor[PanelBranches] = 1 // origin/main

	m, cmd := press(t, m, "m")

	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("m acted on a remote branch")
	}
	if !strings.Contains(m.status, "local branch") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

func TestMergeMovedToShiftMSoRenameCanHaveM(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches)
	m.cursor[PanelBranches] = 1 // not the checked-out branch

	m, _ = press(t, m, "M")

	if m.overlay.kind != overlayConfirm || !strings.Contains(m.overlay.body, "Merge") {
		t.Errorf("shift+m does not merge: kind=%d body=%q", m.overlay.kind, m.overlay.body)
	}
}

func TestRebasingOntoAnotherBranchIsGatedAsARewrite(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches)
	m.cursor[PanelBranches] = 1

	m, cmd := press(t, m, "r")

	if cmd != nil {
		t.Error("r rebased without asking")
	}
	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Errorf("rebase is not gated as destructive: kind=%d danger=%v", m.overlay.kind, m.overlay.danger)
	}
	if !strings.Contains(m.overlay.body, "rewritten") {
		t.Errorf("the confirm does not say history changes: %q", m.overlay.body)
	}
}

func TestUndoIsOfferedOnlyForTheLastCommit(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits)
	m.cursor[PanelCommits] = 0

	m, cmd := press(t, m, "z")
	if cmd != nil {
		t.Error("z undid without asking")
	}
	if m.overlay.kind != overlayConfirm {
		t.Fatalf("z did not ask: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.body, "nothing is lost") {
		t.Errorf("the confirm does not say the work survives: %q", m.overlay.body)
	}

	m = onPane(t, fixture(t), PanelCommits)
	m.cursor[PanelCommits] = 1
	m, cmd = press(t, m, "z")
	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("z acted on a commit further back")
	}
	if !strings.Contains(m.status, "only the last commit") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

func TestABranchCanBeStartedFromAnyCommit(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits)
	m.cursor[PanelCommits] = 1

	m, cmd := press(t, m, "n")

	if cmd != nil {
		t.Error("n created a branch without asking for a name")
	}
	if m.overlay.kind != overlayInput || !strings.Contains(m.overlay.title, "bbb2222") {
		t.Errorf("the prompt does not name the commit: kind=%d title=%q", m.overlay.kind, m.overlay.title)
	}
}

func TestPushUpToHereNamesTheCommitItStopsAt(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits)
	m.cursor[PanelCommits] = 1

	m, cmd := press(t, m, "P")

	if cmd != nil {
		t.Error("shift+p pushed without asking")
	}
	if !strings.Contains(m.overlay.body, "bbb2222") {
		t.Errorf("the confirm does not name where it stops: %q", m.overlay.body)
	}
}

func TestHistoryIsReachableFromACommitsFileList(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommitFiles)
	m.commitSHA = "aaa"
	m.commitFiles = m.snap.Files[:1]

	if _, cmd := press(t, m, "h"); cmd == nil {
		t.Error("h did not ask for the file's history")
	}
}
