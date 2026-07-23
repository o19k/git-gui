package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// The bug this fixes: the Commits panel listed HEAD whatever the Branches
// cursor stood on, so selecting a branch and stepping right showed the same
// history it showed before.
func TestSelectingABranchAimsTheCommitList(t *testing.T) {
	m := fixture(t)
	m = onPane(t, m, PanelBranches)
	m.cursor[PanelBranches] = 1 // origin/main

	cmd := m.refreshPreview()
	if cmd == nil {
		t.Fatal("selecting a branch read nothing")
	}
	if got := m.logRef; got != "refs/remotes/origin/main" {
		t.Errorf("logRef = %q, want the selected branch", got)
	}
}

// Every snapshot runs the same code. Re-reading whenever it does would leave a
// read permanently in flight and the panel permanently reloading.
func TestTheSameBranchIsNotReReadOnEverySnapshot(t *testing.T) {
	m := fixture(t)
	m.repo = &git.Repo{Path: t.TempDir()}

	if cmd := m.aimLog("refs/heads/feature"); cmd == nil {
		t.Error("a new selection was not read")
	}
	if cmd := m.aimLog("refs/heads/feature"); cmd != nil {
		t.Error("an unchanged selection was read again")
	}
}

func TestALandedLogReplacesTheCommitsAndItsMarks(t *testing.T) {
	m := fixture(t)
	m.logRef = "refs/heads/feature"
	m.cursor[PanelCommits] = 1

	next, _ := m.handleLog(logMsg{
		ref:      "refs/heads/feature",
		commits:  []git.Commit{{SHA: "zzz", Short: "zzz9999", Subject: "only one"}},
		unpushed: map[string]bool{"zzz": true},
	})
	after := next.(Model)

	if len(after.snap.Commits) != 1 || after.snap.Commits[0].SHA != "zzz" {
		t.Errorf("commits = %v", after.snap.Commits)
	}
	if !after.snap.Unpushed["zzz"] {
		t.Error("the marks did not come with the commits")
	}
	// A position in the old history means nothing in this one.
	if after.cursor[PanelCommits] != 0 {
		t.Errorf("cursor = %d, want the top of the new list", after.cursor[PanelCommits])
	}
}

// A read for the branch the cursor has already left would otherwise overwrite
// the list with a history nobody is looking at.
func TestALogForAnotherBranchIsDropped(t *testing.T) {
	m := fixture(t)
	m.logRef = "refs/heads/main"

	next, _ := m.handleLog(logMsg{
		ref:     "refs/heads/stale",
		commits: []git.Commit{{SHA: "zzz", Short: "zzz9999"}},
	})
	if got := len(next.(Model).snap.Commits); got != 2 {
		t.Errorf("the stale reply landed: %d commits", got)
	}
}

// The title is the only place a list of someone else's history says so.
func TestTheTitleNamesARefThatIsNotTheCheckedOutOne(t *testing.T) {
	m := fixture(t)

	if got := m.panelTitle(PanelCommits); strings.Contains(got, "·") {
		t.Errorf("title = %q, want no ref named while on the checked-out branch", got)
	}

	m.logRef = "refs/heads/main" // the checked-out one, spelled out
	if got := m.panelTitle(PanelCommits); strings.Contains(got, "·") {
		t.Errorf("title = %q, want the checked-out branch to stay unnamed", got)
	}

	m.logRef = "refs/remotes/origin/main"
	if got := m.panelTitle(PanelCommits); !strings.Contains(got, "origin/main") {
		t.Errorf("title = %q, want the ref named", got)
	}
}

// Rebase, undo and date edits act on the checked-out branch. Run from another
// branch's list they would rewrite history the reader cannot see.
func TestRewritingIsRefusedWhileReadingAnotherBranch(t *testing.T) {
	m := fixture(t)
	m = onPane(t, m, PanelCommits)
	m.logRef = "refs/remotes/origin/main"

	for _, k := range []string{"s", "r", "d", "K", "J", "z", "t"} {
		next, cmd, handled := m.commitsKey(k)
		if !handled {
			t.Errorf("%q was not handled", k)
			continue
		}
		if cmd != nil {
			t.Errorf("%q ran something against another branch's history", k)
		}
		status := next.(Model).status
		if !strings.Contains(status, "main") {
			t.Errorf("%q: status = %q, want it to name where rewriting works", k, status)
		}
	}
}

// Reading is not rewriting: cherry-pick and branch-at take a commit from
// wherever it is and apply it here.
func TestReadingKeysStillWorkOnAnotherBranch(t *testing.T) {
	m := fixture(t)
	m = onPane(t, m, PanelCommits)
	m.logRef = "refs/remotes/origin/main"

	next, _, handled := m.commitsKey("c")
	if !handled || next.(Model).overlay.kind == overlayNone {
		t.Error("cherry-pick was refused on another branch's commit")
	}
}
