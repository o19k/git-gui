package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One key used to stash everything, which is the wrong answer whenever the
// point was to get one change out of the way.

func TestStashTrackedLeavesUntrackedFilesAlone(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.StashPush(ctx, StashOpts{Message: "tracked only"}); err != nil {
		t.Fatalf("StashPush: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); err != nil {
		t.Errorf("an untracked file was stashed with the rest: %v", err)
	}
}

func TestStashEverythingTakesUntrackedFilesToo(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.StashPush(ctx, StashOpts{Message: "everything", Untracked: true}); err != nil {
		t.Fatalf("StashPush: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("the untracked file was left behind")
	}
}

func TestStashPathsTakeOnlyWhatWasNamed(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Two files with unstaged edits; only one of them is named.
	if err := os.WriteFile(filepath.Join(dir, "committed.txt"), []byte("one\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := repo.StashPush(ctx, StashOpts{Message: "one path", Paths: []string{"committed.txt"}}); err != nil {
		t.Fatalf("StashPush: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "committed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "changed") {
		t.Error("the named path was not stashed")
	}
	// The staged file was never named, so it is still staged.
	files, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staged := false
	for _, file := range files {
		if file.Path == "staged.txt" && file.Staged() {
			staged = true
		}
	}
	if !staged {
		t.Errorf("a path that was not named went into the stash anyway: %+v", files)
	}
}

func TestStashStagedOnlyLeavesTheWorkingTree(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// git 2.35 and newer; older ones refuse the flag, and that is worth
	// skipping over rather than failing on.
	if err := repo.StashPush(ctx, StashOpts{Message: "index", StagedOnly: true}); err != nil {
		if strings.Contains(err.Error(), "unknown option") || strings.Contains(err.Error(), "usage:") {
			t.Skip("this git has no stash --staged")
		}
		t.Fatalf("StashPush: %v", err)
	}

	files, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path == "staged.txt" && file.Staged() {
			t.Error("the index was not stashed")
		}
	}

	// The unstaged edit is still in the working tree.
	data, err := os.ReadFile(filepath.Join(dir, "committed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "two") {
		t.Error("the working tree was stashed along with the index")
	}
}
