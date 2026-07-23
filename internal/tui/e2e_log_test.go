package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A unit test can prove the ref is remembered and the marks are drawn. Only a
// real repository proves the reading is actually triggered by the cursor and
// that git answers what the panel then shows.
func TestEndToEndTheCommitListFollowsTheSelectedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit := func(name, subject string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(subject+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", name)
		run("commit", "-m", subject)
	}

	run("init", "--initial-branch=main")
	commit("a.txt", "on main")
	run("branch", "sidebranch")
	run("checkout", "sidebranch")
	commit("b.txt", "only on sidebranch")
	run("checkout", "main")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	m = deliver(t, m, snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})), 0)

	// Standing on main, the side branch's commit is not in the list.
	if strings.Contains(m.View(), "only on sidebranch") {
		t.Fatal("main's log already holds the side branch's commit")
	}

	opened, _ := m.openTab(TabLog)
	m = opened.(Model)
	m.focus = PanelBranches
	m.cursor[PanelBranches] = indexOfBranch(m.branches(), "sidebranch")
	if m.cursor[PanelBranches] < 0 {
		t.Fatalf("sidebranch is not in %v", m.branches())
	}

	cmd := m.refreshPreview()
	m = drain(t, m, cmd)

	if !strings.Contains(m.View(), "only on sidebranch") {
		t.Error("the commit list did not follow the selected branch")
	}
	if got := m.panelTitle(PanelCommits); !strings.Contains(got, "sidebranch") {
		t.Errorf("title = %q, want the branch named", got)
	}
}

// Nothing here has ever been pushed, but there is also no remote to be ahead
// of — the marks are for commits other people cannot see, which is a
// distinction a repository with no remote cannot make.
func TestEndToEndMarksNeedARemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	origin := t.TempDir()

	run := func(where string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", where}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit := func(name, subject string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(subject+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(dir, "add", name)
		run(dir, "commit", "-m", subject)
	}

	run(origin, "init", "--bare", "--initial-branch=main")
	run(dir, "init", "--initial-branch=main")
	commit("a.txt", "published work")
	run(dir, "remote", "add", "origin", origin)
	run(dir, "push", "-u", "origin", "main")
	commit("b.txt", "kept back")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	unpushed, err := repo.Unpushed(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := repo.LogRef(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %v", commits)
	}
	if !unpushed[commits[0].SHA] {
		t.Error("the commit held back is not marked")
	}
	if unpushed[commits[1].SHA] {
		t.Error("a commit the remote holds is marked")
	}
}

func indexOfBranch(branches []git.Branch, name string) int {
	for i, b := range branches {
		if b.Name == name {
			return i
		}
	}
	return -1
}
