package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A rename inside a repository has a git spelling, and the difference is
// visible in the status: git mv leaves one rename, a plain rename leaves a
// deletion and an untracked file for someone to clean up by hand.
func TestMoveTrackedLeavesASingleRename(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.c",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.c")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	repo, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Move(ctx, "old.txt", "new.txt", true); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(out))
	if got != "R  old.txt -> new.txt" {
		t.Errorf("status after the rename is %q, want a single R entry", got)
	}
}
