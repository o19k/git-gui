package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func statusOf(t *testing.T, repo *Repo, path string) (FileChange, bool) {
	t.Helper()
	files, err := repo.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == path {
			return f, true
		}
	}
	return FileChange{}, false
}

func TestUnstageAllEmptiesTheIndexWithoutTouchingTheTree(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnstageAll(ctx); err != nil {
		t.Fatal(err)
	}

	files, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Staged() {
			t.Errorf("%s is still staged", f.Path)
		}
	}
	// The edits themselves survive; only the index was rolled back.
	if got := readFile(t, dir, "committed.txt"); got != "one\ntwo\n" {
		t.Errorf("committed.txt = %q, want the working-tree edit kept", got)
	}
}

func TestUntrackLeavesTheFileOnDisk(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.Untrack(ctx, "committed.txt"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "committed.txt")); err != nil {
		t.Errorf("untracking deleted the file: %v", err)
	}
	f, ok := statusOf(t, repo, "committed.txt")
	if !ok {
		t.Fatal("the untracked file vanished from the status")
	}
	if f.Index != 'D' {
		t.Errorf("index status = %q, want D — git no longer follows it", f.Index)
	}
}

func TestIgnoreAppendsOnceAndCreatesTheFile(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.Ignore(ctx, "untracked.txt"); err != nil {
		t.Fatal(err)
	}
	// Pressing it twice must not list the pattern twice.
	if err := repo.Ignore(ctx, "untracked.txt"); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(readFile(t, dir, ".gitignore"), "\n"), "\n")
	if n := slices.Index(lines, "untracked.txt"); n < 0 {
		t.Fatalf(".gitignore does not hold the pattern: %v", lines)
	}
	if len(lines) != 1 {
		t.Errorf(".gitignore = %v, want the pattern listed once", lines)
	}
	if _, ok := statusOf(t, repo, "untracked.txt"); ok {
		t.Error("the ignored file still shows as a change")
	}
}

func TestIgnoreKeepsAFileWithoutATrailingNewlineReadable(t *testing.T) {
	repo, dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := repo.Ignore(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, dir, ".gitignore"); got != "first\nsecond\n" {
		t.Errorf(".gitignore = %q, want the new pattern on its own line", got)
	}
}

func TestDeleteFileRemovesItTrackedOrNot(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	for _, name := range []string{"committed.txt", "untracked.txt"} {
		file, ok := statusOf(t, repo, name)
		if !ok {
			file = FileChange{Path: name, Index: '.', Work: '.'}
		}
		if err := repo.DeleteFile(ctx, file); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s is still on disk", name)
		}
	}
}

func TestRenameBranchKeepsTheBranchAndItsCommits(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.RenameBranch(ctx, "main", "trunk"); err != nil {
		t.Fatal(err)
	}

	branches, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range branches {
		if b.Kind == RefLocal {
			names = append(names, b.Name)
		}
	}
	if !slices.Contains(names, "trunk") || slices.Contains(names, "main") {
		t.Errorf("local branches = %v, want main renamed to trunk", names)
	}
}

func TestRenameBranchRefusesANameGitWouldMisread(t *testing.T) {
	repo, _ := newRepo(t)
	if err := repo.RenameBranch(context.Background(), "main", "--force"); err == nil {
		t.Error("a name that reads as a flag was accepted")
	}
}

func TestUndoCommitKeepsWhatTheCommitHeld(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "work"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UndoCommit(ctx); err != nil {
		t.Fatal(err)
	}

	commits, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Errorf("log holds %d commits, want the undone one gone", len(commits))
	}
	f, ok := statusOf(t, repo, "staged.txt")
	if !ok || !f.Staged() {
		t.Error("what the commit held did not come back staged")
	}
}

func TestCreateBranchAtStartsFromTheNamedCommit(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	commits, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	first := commits[len(commits)-1]

	if err := repo.CreateBranchAt(ctx, "from-first", first.SHA); err != nil {
		t.Fatal(err)
	}

	after, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].SHA != first.SHA {
		t.Errorf("the new branch is not at the named commit: %+v", after)
	}
}

func TestFileLogListsOnlyTheCommitsThatTouchedThePath(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, "other.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "unrelated"); err != nil {
		t.Fatal(err)
	}

	out, err := repo.FileLog(ctx, "committed.txt", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Errorf("the file's own history is missing:\n%s", out)
	}
	if strings.Contains(out, "unrelated") {
		t.Errorf("a commit that never touched the file is listed:\n%s", out)
	}
}
