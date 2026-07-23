package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findFile returns the entry for path in a fresh status read.
func findFile(t *testing.T, repo *Repo, path string) (FileChange, bool) {
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

func TestStageUnstage(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	// committed.txt starts as an unstaged edit.
	if f, _ := findFile(t, repo, "committed.txt"); f.Staged() {
		t.Fatal("fixture is wrong: committed.txt should start unstaged")
	}

	if err := repo.Stage(ctx, "committed.txt"); err != nil {
		t.Fatal(err)
	}
	if f, ok := findFile(t, repo, "committed.txt"); !ok || !f.Staged() {
		t.Errorf("after Stage: %+v", f)
	}

	if err := repo.Unstage(ctx, "committed.txt"); err != nil {
		t.Fatal(err)
	}
	if f, ok := findFile(t, repo, "committed.txt"); !ok || f.Staged() {
		t.Errorf("after Unstage: %+v", f)
	}
}

func TestStageAllIncludesUntracked(t *testing.T) {
	repo, _ := newRepo(t)
	if err := repo.StageAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	f, ok := findFile(t, repo, "untracked.txt")
	if !ok || !f.Staged() {
		t.Errorf("untracked file was not staged: %+v", f)
	}
}

func TestDiscardTrackedEdit(t *testing.T) {
	repo, dir := newRepo(t)
	f, _ := findFile(t, repo, "committed.txt")

	if err := repo.Discard(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "committed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "one\n" {
		t.Errorf("file was not restored to its committed state: %q", content)
	}
	if _, ok := findFile(t, repo, "committed.txt"); ok {
		t.Error("discarded file is still listed as changed")
	}
}

func TestDiscardUntrackedRemovesFile(t *testing.T) {
	repo, dir := newRepo(t)
	f, _ := findFile(t, repo, "untracked.txt")

	if err := repo.Discard(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked file survived being discarded")
	}
}

func TestDiscardStagedNewFileRemovesIt(t *testing.T) {
	// A file added to the index but never committed has no version to restore,
	// so discarding it must delete it rather than error out.
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.Stage(ctx, "untracked.txt"); err != nil {
		t.Fatal(err)
	}
	f, ok := findFile(t, repo, "untracked.txt")
	if !ok || !f.Staged() {
		t.Fatalf("setup failed: %+v", f)
	}

	if err := repo.Discard(ctx, f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("staged new file survived being discarded")
	}
}

func TestCommitAndAmend(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.Stage(ctx, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add a staged file", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	commits, err := repo.Log(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "add a staged file" {
		t.Fatalf("commit did not land: %+v", commits)
	}

	if err := repo.Amend(ctx, "add a staged file, reworded", CommitOpts{}); err != nil {
		t.Fatal(err)
	}
	commits, _ = repo.Log(ctx, 10)
	if len(commits) != 2 {
		t.Errorf("amend added a commit instead of replacing one: %+v", commits)
	}
	if commits[0].Subject != "add a staged file, reworded" {
		t.Errorf("amend did not reword: %q", commits[0].Subject)
	}

	msg, err := repo.HeadMessage(ctx)
	if err != nil || msg != "add a staged file, reworded" {
		t.Errorf("HeadMessage = %q, %v", msg, err)
	}
}

func TestCommitWithNothingStagedFails(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	// The fixture leaves staged.txt in the index, so empty it first.
	if err := repo.Unstage(ctx, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "empty", CommitOpts{}); err == nil {
		t.Error("committing an empty index should have errored")
	}
}

func TestCreateBranchRejectsFlagLikeNames(t *testing.T) {
	// `switch --create` reads its argument as a name, so a leading dash would
	// otherwise reach git as a flag.
	repo, _ := newRepo(t)
	ctx := context.Background()

	for _, name := range []string{"", "-f", "--force", "bad name", "a..b", "tip.lock", "x~1"} {
		if err := repo.CreateBranch(ctx, name); err == nil {
			t.Errorf("CreateBranch(%q) was accepted", name)
		}
	}
	if err := repo.CreateBranch(ctx, "feature/ok-1"); err != nil {
		t.Errorf("a valid name was rejected: %v", err)
	}
}

func TestBranchCreateCheckoutDelete(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateBranch(ctx, "feature"); err != nil {
		t.Fatal(err)
	}
	if b, _ := repo.CurrentBranch(ctx); b != "feature" {
		t.Fatalf("CreateBranch did not switch: on %q", b)
	}

	branches, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var main, feature Branch
	for _, b := range branches {
		switch b.Name {
		case "main":
			main = b
		case "feature":
			feature = b
		}
	}
	if feature.Name == "" || !feature.Head {
		t.Fatalf("feature branch missing or not HEAD: %+v", branches)
	}

	if err := repo.Checkout(ctx, main); err != nil {
		t.Fatal(err)
	}
	if b, _ := repo.CurrentBranch(ctx); b != "main" {
		t.Errorf("Checkout did not switch back: on %q", b)
	}

	if err := repo.DeleteBranch(ctx, feature, false); err != nil {
		t.Fatal(err)
	}
	branches, _ = repo.Branches(ctx)
	for _, b := range branches {
		if b.Name == "feature" {
			t.Error("branch survived deletion")
		}
	}
}

func TestDeleteUnmergedBranchNeedsForce(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateBranch(ctx, "throwaway"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "only-here.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, "only-here.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "work only on throwaway", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	branches, _ := repo.Branches(ctx)
	var main, throwaway Branch
	for _, b := range branches {
		switch b.Name {
		case "main":
			main = b
		case "throwaway":
			throwaway = b
		}
	}
	if err := repo.Checkout(ctx, main); err != nil {
		t.Fatal(err)
	}

	// git must refuse: the branch holds a commit that would be lost.
	if err := repo.DeleteBranch(ctx, throwaway, false); err == nil {
		t.Error("deleting an unmerged branch without force should have errored")
	}
	if err := repo.DeleteBranch(ctx, throwaway, true); err != nil {
		t.Errorf("forced delete failed: %v", err)
	}
}

func TestMergeCherryPickRevert(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Clean the tree so branch switching is unobstructed.
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "baseline", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateBranch(ctx, "side"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "side.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, "side.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "side work", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	commits, _ := repo.Log(ctx, 10)
	sideSHA := commits[0].SHA

	branches, _ := repo.Branches(ctx)
	var main Branch
	for _, b := range branches {
		if b.Name == "main" {
			main = b
		}
	}
	if err := repo.Checkout(ctx, main); err != nil {
		t.Fatal(err)
	}

	// cherry-pick: does git accept --end-of-options before a revision here?
	if err := repo.CherryPick(ctx, sideSHA); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "side.txt")); err != nil {
		t.Errorf("cherry-picked file is missing: %v", err)
	}

	// revert undoes it with a new commit
	commits, _ = repo.Log(ctx, 10)
	if err := repo.Revert(ctx, commits[0].SHA); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "side.txt")); !os.IsNotExist(err) {
		t.Error("revert did not undo the cherry-pick")
	}

	// merge is then a no-op fast-forward-less merge; it must at least succeed
	if err := repo.Merge(ctx, "side"); err != nil {
		t.Errorf("Merge: %v", err)
	}
}

func TestStashLifecycle(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.StashPush(ctx, StashOpts{Message: "everything", Untracked: true}); err != nil {
		t.Fatal(err)
	}
	files, _ := repo.Status(ctx)
	if len(files) != 0 {
		t.Errorf("working tree not clean after stashing: %+v", files)
	}
	// --include-untracked must have taken the untracked file too.
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked file was left behind by the stash")
	}

	stashes, _ := repo.Stashes(ctx)
	if len(stashes) != 2 { // the fixture already had one
		t.Fatalf("stash list = %+v", stashes)
	}
	newest := stashes[0]
	if !strings.Contains(newest.Subject, "everything") {
		t.Errorf("stash message missing: %+v", newest)
	}

	if err := repo.StashApply(ctx, newest.Ref); err != nil {
		t.Fatal(err)
	}
	if files, _ = repo.Status(ctx); len(files) == 0 {
		t.Error("apply restored nothing")
	}
	if stashes, _ = repo.Stashes(ctx); len(stashes) != 2 {
		t.Error("apply should have kept the entry on the stack")
	}

	if err := repo.StashDrop(ctx, newest.Ref); err != nil {
		t.Fatal(err)
	}
	if stashes, _ = repo.Stashes(ctx); len(stashes) != 1 {
		t.Errorf("drop left %+v", stashes)
	}
}

func TestStashPop(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.StashPush(ctx, StashOpts{Message: "popme", Untracked: true}); err != nil {
		t.Fatal(err)
	}
	stashes, _ := repo.Stashes(ctx)
	before := len(stashes)

	if err := repo.StashPop(ctx, stashes[0].Ref); err != nil {
		t.Fatal(err)
	}
	if stashes, _ = repo.Stashes(ctx); len(stashes) != before-1 {
		t.Errorf("pop did not drop the entry: %+v", stashes)
	}
	if files, _ := repo.Status(ctx); len(files) == 0 {
		t.Error("pop restored nothing")
	}
}

// A directory git is following has to leave through git rm -r: unlinking it
// leaves the index holding files that are no longer on disk, and os.Remove on
// the directory itself fails outright once it holds anything.
func TestDeletingATrackedDirectoryClearsItFromTheIndexToo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	runGit(t, dir, "init", "--initial-branch=main")
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "pkg/a.go", "package pkg\n")
	writeFile(t, dir, "pkg/b.go", "package pkg\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add a package")

	repo, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteFile(ctx, FileChange{Index: '.', Work: '.', Path: "pkg"}); err != nil {
		t.Fatalf("deleting a tracked directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "pkg")); !os.IsNotExist(err) {
		t.Error("the directory is still on disk")
	}
	// git rm stages what it removed, which is the point: the deletion is in
	// the index, ready to commit, rather than a surprise for whoever runs
	// status next.
	status, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 {
		t.Fatalf("git status holds %d entries, want the two files staged as deleted: %v", len(status), status)
	}
	for _, file := range status {
		if file.Index != 'D' || file.Work != '.' {
			t.Errorf("%s is %c%c, want a staged deletion", file.Path, file.Index, file.Work)
		}
	}
}
