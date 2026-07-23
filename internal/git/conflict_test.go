package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// conflicted builds a repository whose merge of "side" into main leaves f.txt
// unmerged, with "ours" holding the main line and "theirs" the side one.
func conflictedRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	repo, dir := newRepo(t)
	ctx := context.Background()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A shared starting point, then one edit on each side of the same line.
	git("stash", "push", "--include-untracked", "-m", "clear the tree")
	write("base\n")
	git("add", "f.txt")
	git("commit", "-m", "base")

	git("switch", "--create", "side")
	write("theirs\n")
	git("commit", "-am", "their edit")

	git("switch", "main")
	write("ours\n")
	git("commit", "-am", "our edit")

	// The merge is expected to fail: that is what leaves the conflict.
	_ = repo.Merge(ctx, "side")

	paths, err := repo.Unmerged(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "f.txt" {
		t.Fatalf("the fixture did not conflict: %v", paths)
	}
	return repo, dir
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUnmergedListsOnlyTheConflictedPaths(t *testing.T) {
	repo, _ := conflictedRepo(t)

	paths, err := repo.Unmerged(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "f.txt" {
		t.Errorf("Unmerged = %v, want just the conflicted file", paths)
	}
}

func TestResolvingOursKeepsTheBranchesOwnVersion(t *testing.T) {
	repo, dir := conflictedRepo(t)
	ctx := context.Background()

	if err := repo.ResolveOurs(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, dir, "f.txt"); got != "ours\n" {
		t.Errorf("f.txt = %q, want the version main already had", got)
	}
	if paths, _ := repo.Unmerged(ctx); len(paths) != 0 {
		t.Errorf("still conflicted after resolving: %v", paths)
	}
}

func TestResolvingTheirsKeepsTheIncomingVersion(t *testing.T) {
	repo, dir := conflictedRepo(t)
	ctx := context.Background()

	if err := repo.ResolveTheirs(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, dir, "f.txt"); got != "theirs\n" {
		t.Errorf("f.txt = %q, want the version being merged in", got)
	}
	if paths, _ := repo.Unmerged(ctx); len(paths) != 0 {
		t.Errorf("still conflicted after resolving: %v", paths)
	}
}

func TestMarkResolvedTakesTheFileAsItStands(t *testing.T) {
	repo, dir := conflictedRepo(t)
	ctx := context.Background()

	// What resolving by hand leaves behind: neither side, but no markers.
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("ours\ntheirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkResolved(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}

	if paths, _ := repo.Unmerged(ctx); len(paths) != 0 {
		t.Errorf("still conflicted after marking resolved: %v", paths)
	}
	if got := readFile(t, dir, "f.txt"); got != "ours\ntheirs\n" {
		t.Errorf("f.txt = %q, want the hand-edited version untouched", got)
	}
}

func TestStatusReportsAConflictedPathAsUnmerged(t *testing.T) {
	repo, _ := conflictedRepo(t)

	files, err := repo.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "f.txt" {
			if f.Code() != 'U' {
				t.Errorf("f.txt status = %q, want U so the interface can spot it", f.Code())
			}
			return
		}
	}
	t.Error("the conflicted file is missing from the status")
}

func TestMergeInProgressAndAbort(t *testing.T) {
	repo, dir := conflictedRepo(t)
	ctx := context.Background()

	if merging, _ := repo.MergeInProgress(ctx); !merging {
		t.Error("a stopped merge did not report itself")
	}
	if err := repo.MergeAbort(ctx); err != nil {
		t.Fatal(err)
	}
	if merging, _ := repo.MergeInProgress(ctx); merging {
		t.Error("the merge survived being aborted")
	}
	if got := readFile(t, dir, "f.txt"); got != "ours\n" {
		t.Errorf("f.txt = %q, want the branch back as it was", got)
	}
}

func TestMergeContinueRecordsTheMergeCommit(t *testing.T) {
	repo, _ := conflictedRepo(t)
	ctx := context.Background()

	if err := repo.ResolveTheirs(ctx, "f.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.MergeContinue(ctx); err != nil {
		t.Fatal(err)
	}

	if merging, _ := repo.MergeInProgress(ctx); merging {
		t.Error("the merge was never finished")
	}
	commits, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 || !strings.Contains(commits[0].Subject, "Merge") {
		t.Errorf("no merge commit at the tip: %+v", commits)
	}
}
