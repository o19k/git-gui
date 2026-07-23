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

// TestEndToEndRealRepo drives the whole stack: open a real repository, run the
// concurrent load, feed the snapshot and preview through Update, and assert the
// frame shows real git data.
func TestEndToEndRealRepo(t *testing.T) {
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
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "--initial-branch=main")
	write("hello.txt", "line one\n")
	run("add", "hello.txt")
	run("commit", "-m", "add a greeting")
	write("hello.txt", "line one\nline two\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// The real concurrent load, not a fixture.
	snap := repo.Load(ctx, git.LoadOpts{Limit: 50})
	if len(snap.Errs) != 0 {
		t.Fatalf("Load errors: %v", snap.Errs)
	}
	next, cmd := m.Update(snapshotMsg(snap))
	m = next.(Model)

	view := m.View()
	if !strings.Contains(view, "hello.txt") {
		t.Errorf("the changed file is missing from the frame:\n%s", view)
	}
	if !strings.Contains(view, "main") {
		t.Errorf("the branch is missing from the frame:\n%s", view)
	}

	// The commits live in their own tab.
	logged, _ := m.openTab(TabLog)
	if logView := logged.(Model).View(); !strings.Contains(logView, "add a greeting") {
		t.Errorf("the commit subject is missing from the Log tab:\n%s", logView)
	}

	// The snapshot must have queued the diff for the selected file.
	if cmd == nil {
		t.Fatal("loading a snapshot queued no preview command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if !strings.Contains(m.mainContent, "+line two") {
		t.Errorf("the diff pane did not receive the real patch:\n%s", m.mainContent)
	}
	if !strings.Contains(m.View(), "line two") {
		t.Error("the patch reached the model but never made it into the frame")
	}
}

// run executes a command the update loop returned and feeds its message back,
// the way the Bubble Tea runtime would. It returns the follow-up command.
func run(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	next, follow := m.Update(cmd())
	return next.(Model), follow
}

// confirmPush takes a push from the keystroke to the command that publishes:
// P reads what would go, shows it, and waits for a yes.
func confirmPush(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	m, _ = run(t, m, cmd)
	if m.overlay.kind != overlayConfirm {
		t.Fatalf("push did not ask before publishing: status=%q", m.status)
	}
	return press(t, m, "y")
}

// TestEndToEndMutations drives stage, commit and branch creation through
// keystrokes, each verified against git itself.
func TestEndToEndMutations(t *testing.T) {
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

	gitRun("init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "a.txt")
	gitRun("commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Boot the model with a real snapshot.
	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
	m = next.(Model)

	reload := func(m Model) Model {
		t.Helper()
		next, _ := m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
		return next.(Model)
	}

	// --- space stages the selected file ---
	m.focus = PanelFiles
	m, cmd := press(t, m, "space")
	m, _ = run(t, m, cmd) // mutationMsg -> reload command
	m = reload(m)

	if f, ok := m.selectedFile(); !ok || !f.Staged() {
		t.Fatalf("space did not stage the file: %+v", f)
	}

	// --- c commits it ---
	m, cmd = press(t, m, "c")
	if cmd != nil {
		t.Fatal("commit ran before a message was typed")
	}
	for _, r := range "second commit" {
		m, _ = press(t, m, string(r))
	}
	m, cmd = press(t, m, "enter")
	// The commit goes through the repository's checks first, so the run of them
	// comes back before the commit itself does.
	m, cmd = run(t, m, cmd)
	m, _ = run(t, m, cmd)
	m = reload(m)

	commits, err := repo.Log(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "second commit" {
		t.Fatalf("commit did not land: %+v", commits)
	}
	if files, _ := repo.Status(ctx); len(files) != 0 {
		t.Errorf("working tree should be clean after committing: %+v", files)
	}

	// --- n creates and switches to a branch ---
	m = onPane(t, m, PanelBranches)
	m, cmd = press(t, m, "n")
	if cmd != nil {
		t.Fatal("branch creation ran before a name was typed")
	}
	for _, r := range "feature" {
		m, _ = press(t, m, string(r))
	}
	m, cmd = press(t, m, "enter")
	m, _ = run(t, m, cmd)
	m = reload(m)

	if branch, _ := repo.CurrentBranch(ctx); branch != "feature" {
		t.Errorf("branch creation did not switch: on %q", branch)
	}
	if !strings.Contains(m.View(), "feature") {
		t.Error("the new branch never appeared in the frame")
	}

	// --- a failed mutation reaches the footer instead of being swallowed ---
	m.focus = PanelFiles
	m, cmd = press(t, m, "c") // nothing staged now
	for _, r := range "empty" {
		m, _ = press(t, m, string(r))
	}
	m, cmd = press(t, m, "enter")
	m, cmd = run(t, m, cmd) // the checks, which pass because there are none
	m, follow := run(t, m, cmd)
	if follow != nil {
		t.Error("a failed commit should not have triggered a reload")
	}
	if m.status == "" || !strings.Contains(m.View(), "nothing") {
		t.Errorf("git's refusal never surfaced: status=%q", m.status)
	}
}

// TestEndToEndPushPull drives fetch, push and pull through keystrokes against a
// real origin on the filesystem.
func TestEndToEndPushPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	root := t.TempDir()

	gitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit := func(dir, name, content, message string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(dir, "add", name)
		gitIn(dir, "commit", "-m", message)
	}

	origin := filepath.Join(root, "origin.git")
	mine := filepath.Join(root, "mine")
	theirs := filepath.Join(root, "theirs")
	gitIn(root, "init", "--bare", "--initial-branch=main", origin)
	gitIn(root, "clone", origin, mine)
	commit(mine, "a.txt", "one\n", "initial")
	gitIn(mine, "push", "--set-upstream", "origin", "main")
	gitIn(root, "clone", origin, theirs)

	repo, err := git.Open(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	reload := func(m Model) Model {
		t.Helper()
		next, _ := m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
		return next.(Model)
	}
	m = reload(m)

	// --- P pushes a local commit to origin ---
	commit(mine, "b.txt", "two\n", "my work")
	m = reload(m)

	m, cmd := press(t, m, "P")
	if m.busy == "" {
		t.Error("push did not tell the user it was running")
	}
	m, cmd = confirmPush(t, m, cmd)
	m, _ = run(t, m, cmd)
	if m.busy != "" {
		t.Error("the in-flight note outlived the push")
	}
	if m.status != "" {
		t.Fatalf("push failed: %s", m.status)
	}

	out, err := exec.Command("git", "-C", origin, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "my work") {
		t.Errorf("the commit never reached origin:\n%s", out)
	}

	// --- f then p bring someone else's commit down ---
	// The other clone catches up first, or the fixture races itself.
	gitIn(theirs, "pull")
	commit(theirs, "c.txt", "three\n", "their work")
	gitIn(theirs, "push")

	m, cmd = press(t, m, "f")
	m, _ = run(t, m, cmd)
	m = reload(m)
	if m.status != "" {
		t.Fatalf("fetch failed: %s", m.status)
	}
	if !strings.Contains(m.View(), "↓1") {
		t.Error("the frame does not show the branch is behind after fetching")
	}

	m, cmd = press(t, m, "p")
	m, _ = run(t, m, cmd)
	if m.status != "" {
		t.Fatalf("pull failed: %s", m.status)
	}
	if _, err := os.Stat(filepath.Join(mine, "c.txt")); err != nil {
		t.Errorf("pull did not bring their commit down: %v", err)
	}

	// --- a rejected push surfaces git's reason instead of failing silently ---
	commit(theirs, "d.txt", "four\n", "their second")
	gitIn(theirs, "push")
	commit(mine, "e.txt", "five\n", "my conflicting work")
	m = reload(m)

	m, cmd = press(t, m, "P")
	m, cmd = confirmPush(t, m, cmd)
	m, follow := run(t, m, cmd)
	if follow != nil {
		t.Error("a rejected push should not have triggered a reload")
	}
	if !strings.Contains(m.status, "reject") && !strings.Contains(m.status, "fetch first") {
		t.Errorf("git's rejection never surfaced: %q", m.status)
	}
	if m.busy != "" {
		t.Error("the in-flight note survived a failed push")
	}
}

// TestEndToEndHunkStaging stages one hunk of a two-hunk file through the
// interface and checks that only that hunk reached the index.
func TestEndToEndHunkStaging(t *testing.T) {
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
	write := func(lines ...string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gitRun("init", "--initial-branch=main")
	write("one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten")
	gitRun("add", "f.txt")
	gitRun("commit", "-m", "initial")
	write("ONE", "two", "three", "four", "five", "six", "seven", "eight", "nine", "TEN")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	load := func(m Model) Model {
		t.Helper()
		next, cmd := m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
		m = next.(Model)
		if cmd != nil { // the queued diff request
			next, _ = m.Update(cmd())
			m = next.(Model)
		}
		return m
	}
	m = load(m)

	m.focus = PanelFiles
	m, _ = press(t, m, "enter")
	if !m.hunkMode {
		t.Fatalf("hunk mode did not open; diff was:\n%s", m.mainContent)
	}
	if got := len(hunkRanges(m.mainContent)); got != 2 {
		t.Fatalf("expected 2 hunks, got %d:\n%s", got, m.mainContent)
	}
	if !strings.Contains(m.View(), "hunk 1/2") {
		t.Error("the pane does not say which hunk is selected")
	}

	// Stage the first hunk only.
	m, cmd := press(t, m, " ")
	m, _ = run(t, m, cmd)
	if m.status != "" {
		t.Fatalf("staging the hunk failed: %s", m.status)
	}
	m = load(m)

	staged, err := repo.Diff(ctx, "f.txt", true, git.DiffOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "+ONE") {
		t.Errorf("the chosen hunk is not in the index:\n%s", staged)
	}
	if strings.Contains(staged, "+TEN") {
		t.Errorf("the other hunk was staged as well:\n%s", staged)
	}
}

// TestEndToEndDropCommit rewrites history through the Commits panel.
func TestEndToEndDropCommit(t *testing.T) {
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
	for _, name := range []string{"first", "second", "third"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if name == "first" {
			gitRun("init", "--initial-branch=main")
		}
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun("add", name+".txt")
		gitRun("commit", "-m", name)
	}

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
	m = next.(Model)

	m = onPane(t, m, PanelCommits)
	m, _ = press(t, m, "j") // the middle commit, "second"
	if c, _ := m.selectedCommit(); c.Subject != "second" {
		t.Fatalf("selected %q, expected second", c.Subject)
	}

	m, cmd := press(t, m, "d")
	if cmd != nil {
		t.Fatal("drop ran without asking")
	}
	m, cmd = press(t, m, "y")
	m, _ = run(t, m, cmd)
	if m.status != "" {
		t.Fatalf("drop failed: %s", m.status)
	}

	commits, err := repo.Log(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "third" || commits[1].Subject != "first" {
		t.Errorf("history after drop = %+v", commits)
	}
	if _, err := os.Stat(filepath.Join(dir, "second.txt")); !os.IsNotExist(err) {
		t.Error("the dropped commit's file is still on disk")
	}
}

// A new file previews as itself, not as a patch — and it arrived as plain text
// while the same file read in the Explorer came back coloured. Nothing about
// the tab it is read in changes what the file is.
func TestANewFileIsColouredInLocalChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	withColour(t)

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

	if err := os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("init", "--initial-branch=main")
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	const source = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
	m = next.(Model)
	m.cursor[PanelFiles] = indexOfPath(m.files(), "new.go")
	if m.cursor[PanelFiles] < 0 {
		t.Fatalf("new.go is not in %v", m.files())
	}
	m = drain(t, m, m.refreshPreview())

	if !strings.Contains(m.mainTitle, "New file") {
		t.Fatalf("the pane is titled %q", m.mainTitle)
	}
	if len(m.mainStyled) == 0 {
		t.Fatal("the new file arrived with no colouring")
	}
	if !strings.Contains(m.View(), "\x1b[") {
		t.Error("nothing on screen is coloured")
	}

	// A patch is coloured by what its lines mean, not by what they say, so it
	// must not pick up the source colouring on the way past.
	if err := os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, git.LoadOpts{Limit: 50})))
	m = next.(Model)
	m.cursor[PanelFiles] = indexOfPath(m.files(), "kept.txt")
	m = drain(t, m, m.refreshPreview())

	if len(m.mainStyled) != 0 {
		t.Errorf("a patch came back with %d source-coloured lines", len(m.mainStyled))
	}
}
