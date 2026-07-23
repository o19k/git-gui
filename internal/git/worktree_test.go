package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

const worktreeListing = `worktree /home/x/project
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /home/x/project-fix
HEAD 2222222222222222222222222222222222222222
branch refs/heads/fix

worktree /home/x/project-look
HEAD 3333333333333333333333333333333333333333
detached
`

func TestParseWorktrees(t *testing.T) {
	trees := parseWorktrees(worktreeListing)

	if len(trees) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(trees))
	}
	if trees[1].Path != "/home/x/project-fix" || trees[1].Branch != "fix" {
		t.Errorf("second entry = %+v", trees[1])
	}
	if !trees[2].Detached || trees[2].Branch != "" {
		t.Errorf("the detached checkout was read as a branch: %+v", trees[2])
	}
	// The name is what the picker shows, and a detached checkout has to say so
	// rather than showing an empty column.
	if got := trees[2].Name(); !strings.Contains(got, "detached") {
		t.Errorf("detached name = %q", got)
	}
	if got := trees[0].Name(); got != "main" {
		t.Errorf("branch name = %q", got)
	}
}

func TestAddAndRemoveWorktree(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	path := filepath.Join(dir, "..", "side")
	if err := repo.AddWorktree(ctx, path, "side", true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	trees, err := repo.Worktrees(ctx)
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	if len(trees) != 2 {
		t.Fatalf("got %d worktrees after adding one, want 2", len(trees))
	}

	var added Worktree
	for _, tree := range trees {
		if tree.Branch == "side" {
			added = tree
		}
	}
	if added.Path == "" {
		t.Fatalf("the new checkout is not in %+v", trees)
	}

	// It is a repository of its own as far as opening goes, which is what
	// makes it worth listing.
	opened, err := Open(ctx, added.Path)
	if err != nil {
		t.Fatalf("opening the worktree: %v", err)
	}
	if branch, _ := opened.CurrentBranch(ctx); branch != "side" {
		t.Errorf("the opened checkout is on %q, want side", branch)
	}

	if err := repo.RemoveWorktree(ctx, added.Path, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if trees, _ := repo.Worktrees(ctx); len(trees) != 1 {
		t.Errorf("the checkout survived removal: %+v", trees)
	}
}

func TestAddWorktreeRefusesNamesGitWouldMisread(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.AddWorktree(ctx, filepath.Join(dir, "..", "x"), "--force", true); err == nil {
		t.Error("a branch name that reads as a flag was accepted")
	}
	if err := repo.AddWorktree(ctx, "", "fine", true); err == nil {
		t.Error("an empty path was accepted")
	}
}
