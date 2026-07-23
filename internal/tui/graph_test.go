package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// linear is a straight history, newest first.
func linear() []git.Commit {
	return []git.Commit{
		{SHA: "c", Short: "ccc", Parents: []string{"b"}},
		{SHA: "b", Short: "bbb", Parents: []string{"a"}},
		{SHA: "a", Short: "aaa"},
	}
}

// A history with no branching is one column: rails that widen without a branch
// to justify them would be decoration.
func TestAStraightHistoryStaysInOneLane(t *testing.T) {
	for i, row := range graphRows(linear()) {
		if row.node != 0 {
			t.Errorf("row %d sits in lane %d, want 0", i, row.node)
		}
		if len(row.lanes) != 1 {
			t.Errorf("row %d has %d lanes, want 1", i, len(row.lanes))
		}
	}
}

// A merge's second parent opens a lane, and the branch it opens keeps that lane
// until it joins the trunk again.
func TestAMergeOpensALaneAndTheJoinClosesIt(t *testing.T) {
	commits := []git.Commit{
		{SHA: "m", Parents: []string{"t", "f"}}, // merge
		{SHA: "t", Parents: []string{"base"}},   // trunk side
		{SHA: "f", Parents: []string{"base"}},   // feature side
		{SHA: "base"},
	}
	rows := graphRows(commits)

	// The lane the merge opens is live on the merge's own row: that is where it
	// starts, and drawing it only from the row below left the branch detached
	// from the merge that brought it in.
	if rows[0].node != 0 || len(rows[0].lanes) != 2 {
		t.Fatalf("the merge should sit in lane 0 beside the lane it opens: %+v", rows[0])
	}
	if !slices.Equal(rows[0].opened, []int{1}) {
		t.Errorf("the merge opened %v, want lane 1", rows[0].opened)
	}
	if len(rows[1].lanes) != 2 {
		t.Errorf("after the merge two branches are live: %+v", rows[1])
	}
	if rows[2].node != 1 {
		t.Errorf("the feature side sits in lane %d, want 1", rows[2].node)
	}
	// Both sides reach the same commit, so the base is one lane again.
	if rows[3].node != 0 || len(rows[3].lanes) != 1 {
		t.Errorf("the join left the rails wide: %+v", rows[3])
	}
}

// A root commit ends its lane rather than leaving a rail running off the bottom.
func TestARootCommitEndsItsLane(t *testing.T) {
	rows := graphRows([]git.Commit{{SHA: "only"}})
	if len(rows) != 1 || rows[0].node != 0 {
		t.Fatalf("rows = %+v", rows)
	}
}

// Unrelated histories (an orphan branch) get their own lane rather than being
// drawn as a continuation of the one above.
func TestAnUnrelatedHistoryGetsItsOwnLane(t *testing.T) {
	rows := graphRows([]git.Commit{
		{SHA: "a", Parents: []string{"a0"}},
		{SHA: "z"}, // reachable from nothing above it
	})
	if rows[1].node == rows[0].node {
		t.Errorf("the orphan reused lane %d", rows[0].node)
	}
}

// The rails must not grow without bound: a repository with many branches in
// flight still has to leave room for the subjects.
func TestTheRailsAreBounded(t *testing.T) {
	var commits []git.Commit
	for i := range 40 {
		commits = append(commits, git.Commit{SHA: string(rune('A' + i))})
	}
	for i, row := range graphRows(commits) {
		if len(row.lanes) > maxLanes {
			t.Fatalf("row %d drew %d lanes, over the cap", i, len(row.lanes))
		}
	}
}

// Plain and styled text must describe the same row: the plain one is what the
// width is measured against.
func TestRailsMeasureAsTheyDraw(t *testing.T) {
	rows := graphRows([]git.Commit{
		{SHA: "m", Parents: []string{"t", "f"}},
		{SHA: "t"},
	})
	plain, styled := rows[1].render(false)
	if len([]rune(plain)) != 2*len(rows[1].lanes) {
		t.Errorf("plain %q does not match %d lanes", plain, len(rows[1].lanes))
	}
	if !strings.Contains(styled, "●") {
		t.Errorf("styled %q has no commit on it", styled)
	}
}

// L is a view, not an action: it works whether or not a commit is selected, and
// it changes nothing in the repository.
func TestTheGraphKeyTogglesTheView(t *testing.T) {
	m := fixture(t)
	m = onPane(t, m, PanelCommits)

	next, cmd, handled := m.commitsKey("L")
	if !handled || cmd != nil {
		t.Fatalf("L handled=%t cmd=%v", handled, cmd)
	}
	if !next.(Model).graphOn {
		t.Error("L did not turn the graph on")
	}

	empty := fixture(t)
	empty.snap.Commits = nil
	if _, _, handled := empty.commitsKey("L"); !handled {
		t.Error("L was refused with an empty list")
	}
}

// The marks say which commits nobody else has. Without a remote every commit
// qualifies, and a mark on every row says nothing.
func TestMarksNeedARemoteToMeanAnything(t *testing.T) {
	m := fixture(t)
	m.snap.Unpushed = map[string]bool{"aaa": true}

	if m.unpushed() == nil {
		t.Error("a repository with a remote lost its marks")
	}

	m.snap.Branches = []git.Branch{{Name: "main", Kind: git.RefLocal, Head: true}}
	if m.unpushed() != nil {
		t.Error("a repository with no remote still marked rows")
	}
}

func TestAnUnpushedCommitIsMarked(t *testing.T) {
	commits := []git.Commit{{SHA: "a", Short: "aaa", Subject: "here"}}

	marked := commitLines(commits, nil, map[string]bool{"a": true}, 0, 1, -1, 60, false)
	plain := commitLines(commits, nil, nil, 0, 1, -1, 60, false)

	if !strings.Contains(marked[0], "↑") {
		t.Errorf("unmarked: %q", marked[0])
	}
	if strings.Contains(plain[0], "↑") {
		t.Errorf("marked without cause: %q", plain[0])
	}
}

// Where a branch's lane and the trunk's meet, only one may carry the shared
// parent onward. Handing it to the branch's lane left the mainline stepping
// sideways at the join and staying there — the root of a linear history drawn
// one column right of every commit above it.
func TestTheTrunkKeepsTheLeftmostLane(t *testing.T) {
	// merge ─┬─ trunk ─┬─ base
	//        └─ feature ┘
	commits := []git.Commit{
		{SHA: "merge", Parents: []string{"trunk", "feature"}},
		{SHA: "feature", Parents: []string{"base"}},
		{SHA: "trunk", Parents: []string{"base"}},
		{SHA: "base"},
	}

	rows := graphRows(commits)
	if got := rows[3].node; got != 0 {
		t.Errorf("the base sits in lane %d, want the trunk's own lane 0", got)
	}
	if got := len(rows[3].lanes); got != 1 {
		t.Errorf("the join left %d lanes open below it, want 1", got)
	}
}

// A lane opens at the merge that brings it in, so it is drawn on the merge's
// own row rather than a row below it.
func TestAMergeIsJoinedToTheLaneItOpens(t *testing.T) {
	rows := graphRows([]git.Commit{
		{SHA: "m", Parents: []string{"t", "f"}},
		{SHA: "t", Parents: []string{"base"}},
		{SHA: "f", Parents: []string{"base"}},
		{SHA: "base"},
	})

	plain, _ := rows[0].render(true)
	if !strings.Contains(plain, "─") {
		t.Errorf("the merge row is %q, want a run reaching the lane it opened", plain)
	}
	if !strings.Contains(plain, "╮") {
		t.Errorf("the merge row is %q, want the opened lane's corner", plain)
	}
}

// Where two lanes turn out to await one commit, one of them ends on that row.
// That is known as the row is built — unlike where a lane will end, which is
// not — so the rail bends into the join instead of stopping mid-air.
func TestAClosingLaneBendsIntoTheJoin(t *testing.T) {
	rows := graphRows([]git.Commit{
		{SHA: "m", Parents: []string{"t", "f"}},
		{SHA: "f", Parents: []string{"base"}},
		{SHA: "t", Parents: []string{"base"}},
		{SHA: "base"},
	})

	// The trunk reaches the shared parent while the feature lane is still
	// waiting for it, so the feature lane closes on the trunk's row.
	joined := rows[2]
	if !slices.Equal(joined.closed, []int{1}) {
		t.Fatalf("closed = %v, want the feature lane: %+v", joined.closed, joined)
	}
	plain, _ := joined.render(false)
	if !strings.Contains(plain, "╯") {
		t.Errorf("the join row is %q, want the closing corner", plain)
	}
}

// A commit whose parent a lane to its left already holds ends its own lane, and
// the line carries on over there rather than simply stopping.
func TestALaneEndingIntoTheLeftIsLinked(t *testing.T) {
	rows := graphRows([]git.Commit{
		{SHA: "m", Parents: []string{"t", "f"}},
		{SHA: "t", Parents: []string{"base"}},
		{SHA: "f", Parents: []string{"base"}},
		{SHA: "base"},
	})

	if rows[2].link != 0 {
		t.Fatalf("link = %d, want the trunk's lane: %+v", rows[2].link, rows[2])
	}
	plain, _ := rows[2].render(false)
	if !strings.Contains(plain, "─") {
		t.Errorf("the row is %q, want a run reaching the lane it continues into", plain)
	}
}
