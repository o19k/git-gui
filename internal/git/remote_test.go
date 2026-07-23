package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// gitIn runs a git command in dir with a deterministic identity.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// commitIn writes a file and commits it.
func commitIn(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-m", message)
}

// remoteFixture builds a bare origin plus two clones, all on the filesystem —
// the real push/pull machinery with no network and no credentials.
func remoteFixture(t *testing.T) (repo *Repo, other string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	mine := filepath.Join(root, "mine")
	theirs := filepath.Join(root, "theirs")

	gitIn(t, root, "init", "--bare", "--initial-branch=main", origin)
	gitIn(t, root, "clone", origin, mine)
	commitIn(t, mine, "a.txt", "one\n", "initial")
	gitIn(t, mine, "push", "--set-upstream", "origin", "main")
	gitIn(t, root, "clone", origin, theirs)

	r, err := Open(context.Background(), mine)
	if err != nil {
		t.Fatal(err)
	}
	return r, theirs
}

func TestPushPublishesAndSetsUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	mine := filepath.Join(root, "mine")
	gitIn(t, root, "init", "--bare", "--initial-branch=main", origin)
	gitIn(t, root, "clone", origin, mine)
	commitIn(t, mine, "a.txt", "one\n", "initial")

	repo, err := Open(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}

	// main has no upstream yet, so this must publish it.
	if err := repo.Push(ctx, "", "main", false); err != nil {
		t.Fatalf("Push: %v", err)
	}

	branches, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var head Branch
	for _, b := range branches {
		if b.Head {
			head = b
		}
	}
	if head.Upstream != "origin/main" {
		t.Errorf("upstream = %q, want origin/main", head.Upstream)
	}
	if !strings.Contains(gitIn(t, origin, "log", "--oneline"), "initial") {
		t.Error("the commit never reached origin")
	}
}

func TestPushWithUpstreamSendsCommits(t *testing.T) {
	repo, _ := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, repo.Path, "b.txt", "two\n", "second")
	if err := repo.Push(ctx, "", "main", true); err != nil {
		t.Fatalf("Push: %v", err)
	}

	branches, _ := repo.Branches(ctx)
	for _, b := range branches {
		if b.Head && b.Ahead != 0 {
			t.Errorf("still %d ahead after pushing", b.Ahead)
		}
	}
}

func TestFetchThenPullFastForwards(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	// Someone else pushes.
	commitIn(t, theirs, "c.txt", "three\n", "their work")
	gitIn(t, theirs, "push")

	if err := repo.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	branches, _ := repo.Branches(ctx)
	behind := 0
	for _, b := range branches {
		if b.Head {
			behind = b.Behind
		}
	}
	if behind != 1 {
		t.Errorf("after fetch, behind = %d, want 1", behind)
	}

	if err := repo.Pull(ctx); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo.Path, "c.txt")); err != nil {
		t.Errorf("pull did not bring their commit: %v", err)
	}
}

func TestPullRefusesWhenItCannotFastForward(t *testing.T) {
	// Diverged histories: --ff-only must fail rather than merge.
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, theirs, "c.txt", "three\n", "their work")
	gitIn(t, theirs, "push")
	commitIn(t, repo.Path, "d.txt", "four\n", "my work")

	err := repo.Pull(ctx)
	if err == nil {
		t.Fatal("a diverged pull should have failed")
	}
	if !strings.Contains(err.Error(), "diverg") && !strings.Contains(err.Error(), "fast-forward") {
		t.Errorf("the reason git refused was lost: %v", err)
	}
}

func TestPushRejectedWhenBehind(t *testing.T) {
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, theirs, "c.txt", "three\n", "their work")
	gitIn(t, theirs, "push")
	commitIn(t, repo.Path, "d.txt", "four\n", "my work")

	err := repo.Push(ctx, "", "main", true)
	if err == nil {
		t.Fatal("pushing over someone else's commit should have been rejected")
	}
	if !strings.Contains(err.Error(), "reject") && !strings.Contains(err.Error(), "fetch first") {
		t.Errorf("git's rejection was lost: %v", err)
	}
}

func TestForcePushLeaseProtectsUnseenCommits(t *testing.T) {
	// --force-with-lease must refuse while the remote holds unfetched commits.
	repo, theirs := remoteFixture(t)
	ctx := context.Background()

	commitIn(t, theirs, "c.txt", "three\n", "their work")
	gitIn(t, theirs, "push")

	// Rewrite local history without ever fetching theirs.
	commitIn(t, repo.Path, "d.txt", "four\n", "my rewrite")

	if err := repo.ForcePush(ctx, "", "main"); err == nil {
		t.Fatal("force-with-lease should have refused: the remote moved unseen")
	}

	// After fetching, the lease is current and the force is allowed.
	if err := repo.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo.Path, "reset", "--hard", "HEAD~1")
	commitIn(t, repo.Path, "e.txt", "five\n", "my rewrite, take two")

	if err := repo.ForcePush(ctx, "", "main"); err != nil {
		t.Errorf("force-with-lease refused a current lease: %v", err)
	}
	if !strings.Contains(gitIn(t, repo.Path, "log", "--oneline", "origin/main"), "take two") {
		t.Error("the force push did not land")
	}
}

func TestPushValidatesBranchNameWhenPublishing(t *testing.T) {
	repo, _ := remoteFixture(t)
	// Publishing passes the name positionally, so it needs the same guard.
	if err := repo.Push(context.Background(), "", "--delete", false); err == nil {
		t.Error("a flag-like branch name was accepted for publishing")
	}
}

func TestRemotesListsConfiguredRemotes(t *testing.T) {
	repo, _ := remoteFixture(t)
	remotes, err := repo.Remotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 || remotes[0] != "origin" {
		t.Errorf("Remotes = %v", remotes)
	}
}

func TestRemoteOperationsDoNotPromptForCredentials(t *testing.T) {
	// An https URL that would demand a username must fail fast, not hang.
	repo, _ := remoteFixture(t)
	ctx := context.Background()

	gitIn(t, repo.Path, "remote", "set-url", "origin", "https://127.0.0.1:1/nope.git")

	done := make(chan error, 1)
	go func() { done <- repo.Fetch(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("fetching an unreachable remote should have failed")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("fetch hung — git is waiting on a prompt it cannot draw")
	}
}

// branchesOn lists the branch names a repository holds, remote-tracking refs
// included, so a deletion can be seen from both sides.
func branchesOn(t *testing.T, repo *Repo) []string {
	t.Helper()
	refs, err := repo.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return names
}

func TestDeleteRemoteBranchRemovesItOnBothSides(t *testing.T) {
	repo, _ := remoteFixture(t)
	ctx := context.Background()

	gitIn(t, repo.Path, "switch", "--create", "spike")
	gitIn(t, repo.Path, "push", "--set-upstream", "origin", "spike")
	gitIn(t, repo.Path, "switch", "main")
	gitIn(t, repo.Path, "fetch", "--prune")

	if err := repo.DeleteRemoteBranch(ctx, Branch{Name: "origin/spike", Kind: RefRemote}); err != nil {
		t.Fatal(err)
	}

	// On the remote itself, which is what the operation is about.
	if out := gitIn(t, repo.Path, "ls-remote", "--heads", "origin", "spike"); strings.TrimSpace(out) != "" {
		t.Errorf("the branch is still on the remote: %q", out)
	}
	// And the tracking ref that followed it, so the panel does not keep
	// offering a branch that is gone.
	if names := branchesOn(t, repo); slices.Contains(names, "origin/spike") {
		t.Errorf("origin/spike is still listed: %v", names)
	}
	// The local branch is the user's own and is not part of the bargain.
	if names := branchesOn(t, repo); !slices.Contains(names, "spike") {
		t.Errorf("the local branch went too: %v", names)
	}
}

// A branch name may hold slashes, so the split cannot simply cut at the first.
func TestDeleteRemoteBranchTakesTheWholeBranchName(t *testing.T) {
	repo, _ := remoteFixture(t)

	gitIn(t, repo.Path, "switch", "--create", "feature/deep/name")
	gitIn(t, repo.Path, "push", "origin", "feature/deep/name")
	gitIn(t, repo.Path, "switch", "main")
	gitIn(t, repo.Path, "fetch", "--prune")

	err := repo.DeleteRemoteBranch(context.Background(),
		Branch{Name: "origin/feature/deep/name", Kind: RefRemote})
	if err != nil {
		t.Fatal(err)
	}
	if out := gitIn(t, repo.Path, "ls-remote", "--heads", "origin"); strings.Contains(out, "deep") {
		t.Errorf("the branch is still on the remote: %q", out)
	}
}

// origin/HEAD is a pointer at the remote's default branch. Pushing a deletion
// of it asks the remote to unset that default, which is never what the key on
// a branch list meant.
func TestDeleteRemoteBranchRefusesTheDefaultPointer(t *testing.T) {
	repo, _ := remoteFixture(t)

	err := repo.DeleteRemoteBranch(context.Background(),
		Branch{Name: "origin/HEAD", Kind: RefRemote})
	if err == nil {
		t.Fatal("origin/HEAD was pushed as a deletion")
	}
	if !strings.Contains(err.Error(), "default branch") {
		t.Errorf("err = %v, want it to explain what origin/HEAD is", err)
	}
}

func TestDeleteRemoteBranchRefusesARefNoRemoteOwns(t *testing.T) {
	repo, _ := remoteFixture(t)

	err := repo.DeleteRemoteBranch(context.Background(),
		Branch{Name: "upstream/main", Kind: RefRemote})
	if err == nil {
		t.Fatal("a ref belonging to no configured remote was pushed somewhere")
	}
	if !strings.Contains(err.Error(), "no configured remote") {
		t.Errorf("err = %v", err)
	}
}

// The three kinds share one panel and one key, and pushing a deletion of a
// local branch's name would aim at whatever the remote happens to have there.
func TestDeleteRemoteBranchRefusesTheOtherKinds(t *testing.T) {
	repo, _ := remoteFixture(t)

	for _, branch := range []Branch{
		{Name: "main", Kind: RefLocal},
		{Name: "v1.0", Kind: RefTag},
	} {
		if err := repo.DeleteRemoteBranch(context.Background(), branch); err == nil {
			t.Errorf("%q was treated as a remote branch", branch.Name)
		}
	}
}

// git reports the paths in the way only in the refusal's prose, so the parse
// has to survive both wordings and stop where the list does.
func TestBlockingPathsAreReadOutOfGitsRefusal(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "tracked",
			text: "error: Your local changes to the following files would be overwritten by merge:\n" +
				"\tdocs/runbook/solana.md\n\tsrc/main.go\n" +
				"Please commit your changes or stash them before you merge.\nAborting",
			want: []string{"docs/runbook/solana.md", "src/main.go"},
		},
		{
			name: "untracked",
			text: "error: The following untracked working tree files would be overwritten by merge:\n" +
				"\tnew.txt\nPlease move or remove them before you merge.",
			want: []string{"new.txt"},
		},
		{
			name: "named nothing",
			text: "error: cannot pull with rebase: You have unstaged changes.",
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BlockingPaths(errors.New(c.text))
			if !slices.Equal(got, c.want) {
				t.Errorf("BlockingPaths = %v, want %v", got, c.want)
			}
		})
	}

	if got := BlockingPaths(nil); got != nil {
		t.Errorf("BlockingPaths(nil) = %v", got)
	}
}
