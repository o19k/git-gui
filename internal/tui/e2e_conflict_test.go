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

// TestEndToEndConflict drives a real merge conflict through the interface: the
// banner says so, r offers both sides, and picking one settles it in git.
func TestEndToEndConflict(t *testing.T) {
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
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gitRun("init", "--initial-branch=main")
	write("base\n")
	gitRun("add", "f.txt")
	gitRun("commit", "-m", "base")

	gitRun("switch", "--create", "side")
	write("theirs\n")
	gitRun("commit", "-am", "their edit")

	gitRun("switch", "main")
	write("ours\n")
	gitRun("commit", "-am", "our edit")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// The merge is expected to stop: that is what makes the conflict.
	_ = repo.Merge(ctx, "side")

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	reload := func(m Model) Model {
		t.Helper()
		next, _ := m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
		return next.(Model)
	}
	m = reload(m)

	// --- the banner announces it without being asked ---
	if !strings.Contains(m.View(), "conflicted") {
		t.Fatalf("the conflict is not on screen:\n%s", m.View())
	}

	// --- r offers a way out, and keeping theirs settles it ---
	m = onPane(t, m, PanelFiles)
	for i, f := range m.files() {
		if f.Path == "f.txt" {
			m.cursor[PanelFiles] = i
		}
	}

	m, cmd := press(t, m, "r")
	if cmd != nil {
		t.Fatal("r resolved the conflict without asking which way")
	}
	if m.overlay.kind != overlayChoice {
		t.Fatalf("r offered nothing: status=%q", m.status)
	}

	m, cmd = press(t, m, "2") // keep theirs
	m, _ = run(t, m, cmd)
	if m.status != "" {
		t.Fatalf("resolving failed: %s", m.status)
	}

	data, err := os.ReadFile(filepath.Join(dir, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "theirs\n" {
		t.Errorf("f.txt = %q, want the incoming version", data)
	}

	left, err := repo.Unmerged(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("git still calls the path conflicted: %v", left)
	}

	m = reload(m)
	if strings.Contains(m.View(), "conflicted") {
		t.Error("the banner outlived the conflict")
	}
}
