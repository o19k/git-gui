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

// TestEndToEndCommitChecks drives a commit past a configured check, both when
// it passes and when it fails, against a real repository.
func TestEndToEndCommitChecks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()

	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
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

	gitRun("init", "--initial-branch=main")
	write("a.txt", "one\n")
	gitRun("add", "a.txt")
	gitRun("commit", "-m", "initial")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	newModel := func() Model {
		t.Helper()
		m := New(ctx, repo)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = next.(Model)
		next, _ = m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
		return onPane(t, next.(Model), PanelFiles)
	}
	// commit types a message and carries it as far as the checks' verdict.
	commit := func(m Model, message string) (Model, tea.Cmd) {
		t.Helper()
		m, cmd := press(t, m, "c")
		if cmd != nil {
			t.Fatal("commit ran before a message was typed")
		}
		for _, r := range message {
			m, _ = press(t, m, string(r))
		}
		m, cmd = press(t, m, "enter")
		return run(t, m, cmd)
	}

	// --- a check that passes lets the commit through ---
	gitRun("config", "--add", "gitgui.check", "true")
	write("b.txt", "two\n")
	m := newModel()
	m, _ = press(t, m, "a") // stage everything
	m, _ = run(t, m, m.do("stage all", func() error { return repo.StageAll(ctx) }))

	m = newModel()
	m, cmd := commit(m, "passes")
	if m.overlay.kind != overlayNone {
		t.Fatalf("a passing check still asked something: %q", m.overlay.title)
	}
	m, _ = run(t, m, cmd)

	commits, err := repo.Log(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "passes" {
		t.Fatalf("the commit did not land: %+v", commits)
	}

	// --- a check that fails holds the commit back and says why ---
	gitRun("config", "--unset-all", "gitgui.check")
	gitRun("config", "--add", "gitgui.check", "echo the tests are unhappy >&2; false")
	write("c.txt", "three\n")
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}

	m = newModel()
	m, _ = commit(m, "fails")

	if m.overlay.kind != overlayChoice {
		t.Fatalf("a failing check did not stop the commit: overlay kind = %d", m.overlay.kind)
	}
	view := m.View()
	for _, want := range []string{"Check failed", "the tests are unhappy", "Commit anyway"} {
		if !strings.Contains(view, want) {
			t.Errorf("the report is missing %q", want)
		}
	}

	if after, _ := repo.Log(ctx, 10); len(after) != 2 {
		t.Errorf("the commit was recorded despite the failed check: %+v", after)
	}

	// --- commit anyway is a real way past it ---
	m, cmd = press(t, m, "1")
	m, _ = run(t, m, cmd)
	if m.status != "" {
		t.Fatalf("committing anyway failed: %s", m.status)
	}
	after, err := repo.Log(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 || after[0].Subject != "fails" {
		t.Errorf("commit anyway did not record the commit: %+v", after)
	}
}
