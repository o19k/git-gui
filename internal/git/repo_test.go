package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newRepo builds a throwaway repository with one commit, one staged change,
// one unstaged change, one untracked file and one stash.
func newRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
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

	run("init", "--initial-branch=main")
	write("committed.txt", "one\n")
	run("add", "committed.txt")
	run("commit", "-m", "initial commit")

	write("stashed.txt", "to stash\n")
	run("add", "stashed.txt")
	run("stash", "push", "-m", "stashed work")

	write("staged.txt", "staged\n")
	run("add", "staged.txt")

	write("committed.txt", "one\ntwo\n") // unstaged edit
	write("untracked.txt", "new\n")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo, dir
}

func TestOpenResolvesRoot(t *testing.T) {
	_, dir := newRepo(t)

	sub := filepath.Join(dir, "nested", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	// macOS hands out /var symlinks for temp dirs; compare resolved paths.
	want, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(repo.Path)
	if got != want {
		t.Errorf("opened from a subdirectory gave %q, want the root %q", got, want)
	}
}

func TestOpenRejectsNonRepo(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); err == nil {
		t.Error("a plain directory should not open as a repository")
	}
}

func TestLoadReadsRealRepo(t *testing.T) {
	repo, _ := newRepo(t)
	snap := repo.Load(context.Background(), 50, "")

	if len(snap.Errs) != 0 {
		t.Fatalf("Load reported errors: %v", snap.Errs)
	}
	if snap.Branch != "main" {
		t.Errorf("Branch = %q, want main", snap.Branch)
	}

	byPath := map[string]FileChange{}
	for _, f := range snap.Files {
		byPath[f.Path] = f
	}
	if f, ok := byPath["staged.txt"]; !ok || !f.Staged() {
		t.Errorf("staged.txt missing or not staged: %+v", f)
	}
	if f, ok := byPath["committed.txt"]; !ok || f.Staged() {
		t.Errorf("unstaged edit missing or wrongly staged: %+v", f)
	}
	if f, ok := byPath["untracked.txt"]; !ok || !f.Untracked() {
		t.Errorf("untracked.txt missing or not untracked: %+v", f)
	}

	if len(snap.Commits) != 1 || snap.Commits[0].Subject != "initial commit" {
		t.Errorf("Commits = %+v", snap.Commits)
	}
	if len(snap.Stashes) != 1 || !strings.Contains(snap.Stashes[0].Subject, "stashed work") {
		t.Errorf("Stashes = %+v", snap.Stashes)
	}

	var head *Branch
	for i := range snap.Branches {
		if snap.Branches[i].Head {
			head = &snap.Branches[i]
		}
	}
	if head == nil || head.Name != "main" {
		t.Errorf("no HEAD branch in %+v", snap.Branches)
	}
}

func TestDiffAndPreview(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	unstaged, err := repo.Diff(ctx, "committed.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unstaged, "+two") {
		t.Errorf("unstaged diff missing the edit:\n%s", unstaged)
	}

	staged, err := repo.Diff(ctx, "staged.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "+staged") {
		t.Errorf("staged diff missing the addition:\n%s", staged)
	}

	preview, err := repo.UntrackedPreview(ctx, "untracked.txt")
	if err != nil {
		t.Fatal(err)
	}
	if preview != "new\n" {
		t.Errorf("untracked preview = %q", preview)
	}
}

func TestLogOnRepoWithoutCommits(t *testing.T) {
	// A freshly-initialised repository must load as empty, not as an error.
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "--initial-branch=main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	commits, err := repo.Log(context.Background(), 10)
	if err != nil {
		t.Errorf("Log on an empty repo errored: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("Log on an empty repo returned %+v", commits)
	}
	if branch, _ := repo.CurrentBranch(context.Background()); branch != "main" {
		t.Errorf("CurrentBranch on an unborn branch = %q, want main", branch)
	}
}

func TestACommandThatOutlivesItsDeadlineSaysSo(t *testing.T) {
	repo, _ := newRepo(t)

	// A killed command's stderr says nothing, so the error must name the cause.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	_, err := repo.run(ctx, "status")
	if err == nil {
		t.Fatal("an expired deadline produced no error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %q, want it to name the timeout", err)
	}
}

func TestAnExistingDeadlineIsNotShortened(t *testing.T) {
	repo, _ := newRepo(t)

	// Network operations set a longer deadline; exec must not clamp it.
	want := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), want)
	defer cancel()

	if _, err := repo.run(ctx, "status"); err != nil {
		t.Fatal(err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("the deadline vanished")
	}
	if remaining := time.Until(deadline); remaining < want-time.Minute {
		t.Errorf("deadline shortened to %s, want about %s", remaining, want)
	}
}
