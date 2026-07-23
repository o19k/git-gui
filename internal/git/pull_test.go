package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullRefusesRatherThanOverwriteLocalChanges(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	// The same file edited on both sides, one committed and one not.
	commitIn(t, theirs, "a.txt", "one\ntheirs\n", "their work")
	gitIn(t, theirs, "push")
	if err := os.WriteFile(filepath.Join(repo.Path, "a.txt"), []byte("one\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := repo.Pull(ctx)
	if err == nil {
		t.Fatal("a pull over uncommitted changes should have failed")
	}
	if !IsDirtyTree(err) {
		t.Errorf("the refusal was not recognised as a dirty tree: %v", err)
	}
	if IsNotFastForward(err) {
		t.Errorf("a dirty tree was mistaken for a diverged branch: %v", err)
	}
}

func TestPullAutostashGetsThroughWhereAPlainPullWillNot(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	// Their commit touches a different file, so putting the stash back is clean.
	commitIn(t, theirs, "b.txt", "theirs\n", "their work")
	gitIn(t, theirs, "push")
	if err := os.WriteFile(filepath.Join(repo.Path, "a.txt"), []byte("one\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo.Path, "add", "a.txt")

	if err := repo.PullAutostash(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(repo.Path, "b.txt")); err != nil {
		t.Errorf("the pull never landed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo.Path, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nmine\n" {
		t.Errorf("a.txt = %q, want the local edit put back", data)
	}
	if left, _ := repo.Unmerged(ctx); len(left) != 0 {
		t.Errorf("a clean autostash left conflicts: %v", left)
	}
}

// A conflicting autostash is the case that reads as success and is not: git
// pulls, fails to put the changes back, and still exits zero.
func TestPullAutostashReportsSuccessWhilePuttingBackConflicts(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, theirs, "a.txt", "one\ntheirs\n", "their work")
	gitIn(t, theirs, "push")
	if err := os.WriteFile(filepath.Join(repo.Path, "a.txt"), []byte("one\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := repo.PullAutostash(ctx); err != nil {
		t.Fatalf("git reported a failure it does not actually report: %v", err)
	}

	left, err := repo.Unmerged(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0] != "a.txt" {
		t.Errorf("Unmerged = %v, want the conflict the error did not mention", left)
	}
}

func TestPullRebaseAndMergeBothReconcileADivergedBranch(t *testing.T) {
	cases := map[string]func(*Repo, context.Context) error{
		"rebase": (*Repo).PullRebase,
		"merge":  (*Repo).PullMerge,
	}
	for name, pull := range cases {
		t.Run(name, func(t *testing.T) {
			repo, theirs := remoteFixture(t)
			ctx := context.Background()

			commitIn(t, theirs, "c.txt", "three\n", "their work")
			gitIn(t, theirs, "push")
			commitIn(t, repo.Path, "d.txt", "four\n", "my work")

			if err := repo.Pull(ctx); !IsNotFastForward(err) {
				t.Fatalf("the fixture is not diverged: %v", err)
			}
			if err := pull(repo, ctx); err != nil {
				t.Fatal(err)
			}

			for _, name := range []string{"c.txt", "d.txt"} {
				if _, err := os.Stat(filepath.Join(repo.Path, name)); err != nil {
					t.Errorf("%s did not survive: %v", name, err)
				}
			}
		})
	}
}

func TestOutgoingListsOnlyWhatTheUpstreamHasNotSeen(t *testing.T) {
	repo, _ := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, repo.Path, "b.txt", "two\n", "mine one")
	commitIn(t, repo.Path, "c.txt", "three\n", "mine two")

	commits, err := repo.Outgoing(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("Outgoing returned %d commits, want the 2 unpushed ones", len(commits))
	}
	if commits[0].Subject != "mine two" {
		t.Errorf("Outgoing[0] = %q, want the newest first", commits[0].Subject)
	}
}

func TestOutgoingIsEmptyWhenTheBranchMatchesItsUpstream(t *testing.T) {
	repo, _ := remoteFixture(t)

	commits, err := repo.Outgoing(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 0 {
		t.Errorf("Outgoing = %d commits, want none", len(commits))
	}
}

func TestPushUpToLeavesTheLaterCommitsBehind(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, repo.Path, "b.txt", "two\n", "publish me")
	commitIn(t, repo.Path, "c.txt", "three\n", "keep me local")

	commits, err := repo.Outgoing(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Outgoing is newest first, so the one to publish up to is the older.
	if err := repo.PushUpTo(ctx, "", commits[1].SHA, "main"); err != nil {
		t.Fatal(err)
	}

	gitIn(t, theirs, "fetch")
	out := gitIn(t, theirs, "log", "--oneline", "origin/main")
	if !strings.Contains(out, "publish me") {
		t.Errorf("the commit to publish never arrived:\n%s", out)
	}
	if strings.Contains(out, "keep me local") {
		t.Errorf("a commit past the chosen one was published too:\n%s", out)
	}
}
