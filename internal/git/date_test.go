package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeDateAcceptsWhatAPersonWouldType(t *testing.T) {
	cases := map[string]string{
		"2020-05-05":          "2020-05-05 00:00:00",
		"2020-05-05 10:11:12": "2020-05-05 10:11:12",
		"  2020-05-05  ":      "2020-05-05 00:00:00",
	}
	for in, want := range cases {
		got, err := normalizeDate(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeDateRefusesWhatGitWouldMisread(t *testing.T) {
	for _, in := range []string{
		"", "   ",
		"yesterday",        // git would take it, but not as anything checkable
		"2020-5-5",         // unpadded
		"05-05-2020",       // the other way round
		"2020-05-05 10",    // half a time
		"2020-05-05T10:00", // ISO's separator, which git reads differently
		"--date=now",       // reads as a flag
	} {
		if _, err := normalizeDate(in); err == nil {
			t.Errorf("normalizeDate(%q) was accepted", in)
		}
	}
}

func TestTheDateRefusalSaysWhatIsAccepted(t *testing.T) {
	_, err := normalizeDate("nonsense")
	if err == nil {
		t.Fatal("nonsense was accepted")
	}
	if !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error = %q, want it to name the form", err)
	}
}

// datesOf reads back both of a commit's dates, which is the point: setting one
// and not the other leaves the log reading as though the work were recorded
// years after it was written.
func datesOf(t *testing.T, repo *Repo, ref string) (author, committer string) {
	t.Helper()
	out, err := repo.run(context.Background(), "log", "-1", "--date=short",
		"--format=%ad%x1f%cd", "--end-of-options", ref)
	if err != nil {
		t.Fatal(err)
	}
	a, c, _ := strings.Cut(strings.TrimSpace(out), "\x1f")
	return a, c
}

// datedRepo is a clean repository with three commits, which rewriting history
// requires — the shared fixture deliberately leaves changes lying around.
func datedRepo(t *testing.T) *Repo {
	t.Helper()
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "third.txt", "three\n")
	if err := repo.Stage(ctx, "third.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "third"); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestSettingTheTipCommitsDateMovesBothOfThem(t *testing.T) {
	repo := datedRepo(t)
	ctx := context.Background()

	commits, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.SetCommitDate(ctx, commits[0].SHA, "2020-05-05"); err != nil {
		t.Fatal(err)
	}

	author, committer := datesOf(t, repo, "HEAD")
	if author != "2020-05-05" || committer != "2020-05-05" {
		t.Errorf("dates = author %s, committer %s, want both 2020-05-05", author, committer)
	}
}

func TestSettingAnEarlierCommitsDateReplaysWhatFollows(t *testing.T) {
	repo := datedRepo(t)
	ctx := context.Background()

	before, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	target := before[1] // not the tip, so the rebase path is the one exercised

	if err := repo.SetCommitDate(ctx, target.SHA, "2019-01-02 03:04:05"); err != nil {
		t.Fatal(err)
	}

	if stopped, _ := repo.RebaseInProgress(ctx); stopped {
		t.Fatal("the rebase was left open")
	}

	after, err := repo.Log(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("history is %d commits, want the same %d", len(after), len(before))
	}
	for i := range after {
		if after[i].Subject != before[i].Subject {
			t.Errorf("commit %d is now %q, want %q", i, after[i].Subject, before[i].Subject)
		}
	}

	author, committer := datesOf(t, repo, after[1].SHA)
	if author != "2019-01-02" || committer != "2019-01-02" {
		t.Errorf("dates = author %s, committer %s, want both 2019-01-02", author, committer)
	}
	// Replaying rewrites the commits above it, which is the cost being warned about.
	if after[0].SHA == before[0].SHA {
		t.Error("the commit after the edited one kept its object name")
	}
}

func TestSettingADateRefusesACommitThatIsNotThere(t *testing.T) {
	repo := datedRepo(t)
	err := repo.SetCommitDate(context.Background(),
		"0000000000000000000000000000000000000000", "2020-05-05")
	if err == nil {
		t.Fatal("a commit that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "no such commit") {
		t.Errorf("error = %q, want it to say the commit is missing", err)
	}
}

func TestSettingADateNeedsACleanTree(t *testing.T) {
	// The shared fixture has uncommitted changes, which a rebase refuses.
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
	writeFile(t, repo.Path, "dirty.txt", "uncommitted\n")

	err = repo.SetCommitDate(ctx, commits[len(commits)-1].SHA, "2020-05-05")
	if err == nil {
		t.Fatal("history was rewritten over uncommitted changes")
	}
	if !strings.Contains(err.Error(), "stash") {
		t.Errorf("error = %q, want it to say what to do first", err)
	}
}
