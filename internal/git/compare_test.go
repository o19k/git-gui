package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// twoWayRepo builds a repository with a main branch and a feature branch that
// diverged from a common ancestor, so tests can verify both commit and file diffs.
func twoWayRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	repo, dir := newRepo(t)

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
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Clear the fixture's working tree.
	git("stash", "push", "--include-untracked", "-m", "clear")

	// Create a shared ancestor.
	write("shared.txt", "shared\n")
	git("add", "shared.txt")
	git("commit", "-m", "shared ancestor")

	// Create a feature branch with its own commits and file changes.
	git("switch", "--create", "feature")
	write("feature.txt", "feature\n")
	git("add", "feature.txt")
	git("commit", "-m", "add feature")
	write("shared.txt", "shared from feature\n")
	git("add", "shared.txt")
	git("commit", "-m", "edit shared in feature")

	// Back to main with its own diverging changes.
	git("switch", "main")
	write("main.txt", "main\n")
	git("add", "main.txt")
	git("commit", "-m", "add main")

	return repo, dir
}

func TestCompareListsOnlyWhatTheOtherBranchHolds(t *testing.T) {
	repo, _ := twoWayRepo(t)

	commits, err := repo.CompareCommits(context.Background(), "main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	if len(commits) != 2 {
		t.Errorf("got %d commits, want 2", len(commits))
	}
	// Should be newest first, so "edit shared in feature" comes before "add feature".
	if len(commits) > 0 && !strings.Contains(commits[0].Subject, "edit") {
		t.Errorf("newest commit has subject %q, want 'edit shared'", commits[0].Subject)
	}
	if len(commits) > 1 && !strings.Contains(commits[1].Subject, "add feature") {
		t.Errorf("second commit has subject %q, want 'add feature'", commits[1].Subject)
	}
}

func TestCompareFilesDirectRange(t *testing.T) {
	repo, _ := twoWayRepo(t)

	files, err := repo.CompareFiles(context.Background(), "main", "feature", false)
	if err != nil {
		t.Fatal(err)
	}

	// Should include both the new feature.txt and the edited shared.txt.
	byPath := map[string]FileChange{}
	for _, f := range files {
		byPath[f.Path] = f
	}

	if _, ok := byPath["feature.txt"]; !ok {
		t.Errorf("feature.txt missing from direct comparison")
	}
	if _, ok := byPath["shared.txt"]; !ok {
		t.Errorf("shared.txt missing from direct comparison")
	}
}

func TestCompareFilesMergeBaseRange(t *testing.T) {
	repo, _ := twoWayRepo(t)

	files, err := repo.CompareFiles(context.Background(), "main", "feature", true)
	if err != nil {
		t.Fatal(err)
	}

	// Merge-base range excludes changes on the base branch after the split,
	// so it should still have both files since we're comparing against the ancestor.
	byPath := map[string]FileChange{}
	for _, f := range files {
		byPath[f.Path] = f
	}

	if _, ok := byPath["feature.txt"]; !ok {
		t.Errorf("feature.txt missing from merge-base comparison")
	}
	if _, ok := byPath["shared.txt"]; !ok {
		t.Errorf("shared.txt missing from merge-base comparison")
	}
}
