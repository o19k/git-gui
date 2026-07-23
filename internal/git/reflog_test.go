package git

import (
	"context"
	"strings"
	"testing"
)

func TestParseReflog(t *testing.T) {
	out := strings.Join([]string{
		strings.Join([]string{"a1b2", "a1b2c3d", "HEAD@{0}", "commit: add a thing", "2 minutes ago"}, us),
		strings.Join([]string{"c3d4", "c3d4e5f", "HEAD@{1}", "reset: moving to HEAD~1", "5 minutes ago"}, us),
		"",
	}, "\n")

	entries := parseReflog(out)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Selector != "HEAD@{0}" || entries[0].Action != "commit: add a thing" {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[1].SHA != "c3d4" || entries[1].When != "5 minutes ago" {
		t.Errorf("second entry = %+v", entries[1])
	}
}

// The reason the reflog is here at all: a commit no branch points at any more
// is still in it, and can be put back within reach.
func TestReflogFindsACommitAfterAHardReset(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.Stage(ctx, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "the one that gets lost", CommitOpts{}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetHard(ctx, "HEAD~1"); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	if commits, _ := repo.Log(ctx, 50); len(commits) != 1 {
		t.Fatalf("the reset did not remove the commit from the branch: %v", logSubjects(commits))
	}

	entries, err := repo.Reflog(ctx, 50)
	if err != nil {
		t.Fatalf("Reflog: %v", err)
	}

	var lost string
	for _, entry := range entries {
		if strings.Contains(entry.Action, "the one that gets lost") {
			lost = entry.SHA
		}
	}
	if lost == "" {
		t.Fatalf("the commit is not in the reflog: %+v", entries)
	}

	if err := repo.CreateBranchAt(ctx, "rescued", lost); err != nil {
		t.Fatalf("CreateBranchAt: %v", err)
	}
	commits, err := repo.Log(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Errorf("the rescued branch does not hold the commit: %v", logSubjects(commits))
	}
}
