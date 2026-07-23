package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/o19k/git-gui/internal/git"
)

// TestEndToEndExplorer drives the Explorer against a real repository: the
// listing comes from git, a directory carries the worst status beneath it, an
// ignored tree is reachable but marked, and stepping in and out lands where it
// should.
func TestEndToEndExplorer(t *testing.T) {
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
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gitRun("init", "--initial-branch=main")
	write(".gitignore", "build/\n")
	write("src/main.go", "package main\n")
	write("src/util.go", "package main\n")
	write("docs/readme.md", "hello\n")
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	// One tracked file modified, one path never added, one ignored tree.
	write("src/main.go", "package main\n\nfunc main() {}\n")
	write("docs/draft.md", "wip\n")
	write("build/out.bin", "\x00\x01binary\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	// Open the Explorer and let its index arrive.
	m, cmd := press(t, m, "4")
	if m.tab != TabFiles {
		t.Fatalf("4 opened tab %v, want the Explorer", m.tab)
	}
	m, _ = run(t, m, cmd)

	if len(m.index) == 0 {
		t.Fatal("the Explorer opened with no listing at all")
	}

	// --- the listing is git's, and directories carry what is beneath them ---
	byName := map[string]fsEntry{}
	for _, e := range m.entries() {
		byName[e.Name] = e
	}
	for _, want := range []string{"src", "docs", "build"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("%q is missing from the root listing: %v", want, byName)
		}
	}
	if got := byName["src"].Status; got != 'M' {
		t.Errorf("src holds a modified file but rolled up to %q, want 'M'", got)
	}
	if got := byName["docs"].Status; got != '?' {
		t.Errorf("docs holds only an untracked file but rolled up to %q, want '?'", got)
	}
	if !byName["build"].Ignored {
		t.Error("the ignored tree is not marked ignored, so it will not be dimmed")
	}

	// --- stepping in and out ---
	m = onPane(t, m, PanelEntries)
	for i, e := range m.entries() {
		if e.Name == "src" {
			m.cursor[PanelEntries] = i
		}
	}
	m, cmd = press(t, m, "l")
	if m.cwd != "src" {
		t.Fatalf("l stepped into %q, want src", m.cwd)
	}
	m, _ = run(t, m, cmd)

	var names []string
	for _, e := range m.entries() {
		names = append(names, e.Name)
	}
	if len(names) != 2 || names[0] != "main.go" || names[1] != "util.go" {
		t.Errorf("src lists %v, want main.go and util.go", names)
	}

	// The preview follows the selection, and a modified file opens on its diff.
	if m.previewFor.path != "src/main.go" {
		t.Errorf("the preview is of %q, want src/main.go", m.previewFor.path)
	}
	if m.previewFor.kind != previewDiff {
		t.Errorf("a modified file opened on kind %v, want its diff", m.previewFor.kind)
	}

	m, _ = press(t, m, "h")
	if m.cwd != "." {
		t.Fatalf("h stepped out to %q, want the root", m.cwd)
	}
	if m.entries()[m.cursor[PanelEntries]].Name != "src" {
		t.Errorf("stepping out landed on %q, want the directory just left",
			m.entries()[m.cursor[PanelEntries]].Name)
	}

	// h at the root is a no-op rather than an escape from the repository.
	m, _ = press(t, m, "h")
	if m.cwd != "." {
		t.Errorf("h at the root moved to %q", m.cwd)
	}

	// --- Local Changes is untouched by all of this ---
	m, _ = press(t, m, "1")
	if m.mainContent == "" && m.mainTitle == "Status" {
		t.Error("opening the Explorer wiped Local Changes' pane")
	}
}

// The fallback path and the previews that need a real file behind them: an
// ignored tree git says nothing about, a binary, and a symlink.
func TestEndToEndExplorerDiskFallbackAndPreviews(t *testing.T) {
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
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	selectEntry := func(m Model, name string) Model {
		t.Helper()
		for i, e := range m.entries() {
			if e.Name == name {
				m.cursor[PanelEntries] = i
				// A preview git has to be asked for arrives as a command; a
				// symlink's target and a directory's listing are already in
				// hand and set no command at all.
				if cmd := m.refreshPreview(); cmd != nil {
					next, _ := run(t, m, cmd)
					return next
				}
				return m
			}
		}
		t.Fatalf("%q is not in the listing of %q", name, m.cwd)
		return m
	}

	gitRun("init", "--initial-branch=main")
	write(".gitignore", "build/\n")
	write("src/main.go", "package main\n")
	if err := os.Symlink("src/main.go", filepath.Join(dir, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	// The ignored tree exists only on disk as far as git is concerned.
	write("build/out.bin", "\x00\x01\x02binary payload\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	m, cmd := press(t, m, "4")
	m, _ = run(t, m, cmd)

	// --- a symlink previews its target rather than the file it points at ---
	m = selectEntry(m, "link.go")
	if !strings.Contains(m.previewTitle, "Link") {
		t.Errorf("the preview of a symlink is titled %q, want it named as a link", m.previewTitle)
	}
	if !strings.Contains(m.previewContent, "src/main.go") {
		t.Errorf("the preview of a symlink is %q, want its target", m.previewContent)
	}

	// --- an ignored directory is entered by reading the disk ---
	m = selectEntry(m, "build")
	m, cmd = press(t, m, "l")
	if m.cwd != "build" {
		t.Fatalf("l stepped into %q, want build", m.cwd)
	}
	if cmd == nil {
		t.Fatal("stepping into a directory git does not cover issued no disk read")
	}
	m, _ = run(t, m, cmd)

	if _, ok := m.fsIndex["build"]; !ok {
		t.Fatal("the disk listing was not installed")
	}
	if title := m.explorerTitle(PanelEntries); !strings.Contains(title, "(from disk)") {
		t.Errorf("the listing is titled %q, want it to say where it came from", title)
	}

	// --- a binary is described, never dumped ---
	m = selectEntry(m, "out.bin")
	if !strings.Contains(m.previewContent, "binary") {
		t.Errorf("the preview of a binary is %q, want it described", m.previewContent)
	}
	if strings.ContainsRune(m.previewContent, '\x00') {
		t.Error("the preview of a binary contains its bytes")
	}

	// --- a tick does not undo the disk listing or read it again (R13) ---
	before := m.fsIndex["build"]
	next, _ = m.Update(tickMsg{})
	m = next.(Model)
	if got := m.fsIndex["build"]; len(got) != len(before) {
		t.Errorf("a tick left %d entries for build, want the %d already read", len(got), len(before))
	}
}

// drain runs a command and everything it batches, feeding each message back in.
// The tick issues a batch, and the parts of it that matter are not the first
// one. The tick it schedules to follow itself is dropped, or this never ends.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	return deliver(t, m, cmd(), 0)
}

func deliver(t *testing.T, m Model, msg tea.Msg, depth int) Model {
	t.Helper()
	if depth > 8 {
		return m
	}
	if _, isTick := msg.(tickMsg); isTick && depth > 0 {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m = deliver(t, m, c(), depth+1)
			}
		}
		return m
	}

	next, follow := m.Update(msg)
	m = next.(Model)
	if follow != nil {
		m = deliver(t, m, follow(), depth+1)
	}
	return m
}

// The listing is a cache with a repository under it: what git sees changes
// without this program doing anything, and a submodule is another repository
// that must not be walked into as though it were a directory.
func TestEndToEndExplorerStaysCurrentAndStopsAtSubmodules(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	sub := t.TempDir()

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
	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run(sub, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(sub, "lib.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(sub, "add", ".")
	run(sub, "commit", "-m", "lib")

	run(dir, "init", "--initial-branch=main")
	write(".gitignore", "build/\n")
	write("src/main.go", "package main\n")
	run(dir, "add", ".")
	run(dir, "commit", "-m", "initial")
	run(dir, "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor")
	run(dir, "commit", "-m", "add the submodule")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)

	// --- a gitlink is named as one and refuses to open ---
	var vendor fsEntry
	for i, e := range m.entries() {
		if e.Name == "vendor" {
			vendor, m.cursor[PanelEntries] = e, i
		}
	}
	if !vendor.Module {
		t.Fatalf("vendor is %+v, want it marked as a submodule", vendor)
	}
	before := m.cwd
	stepped, _ := press(t, m, "l")
	if stepped.cwd != before {
		t.Errorf("l descended into the submodule at %q", stepped.cwd)
	}
	if stepped.status == "" {
		t.Error("l on a submodule did nothing and said nothing")
	}

	// --- a file appearing outside this program shows up on the tick ---
	write("src/late.go", "package main\n")
	write("build/out.txt", "one\n")
	m = drain(t, m, func() tea.Msg { return tickMsg{} })

	var found bool
	for _, e := range m.listingOf("src") {
		found = found || e.Name == "late.go"
	}
	if !found {
		t.Error("a file written outside the program never appeared, so the listing is stale")
	}

	// --- R drops a disk listing so it can be read again ---
	for i, e := range m.entries() {
		if e.Name == "build" {
			m.cursor[PanelEntries] = i
		}
	}
	m, cmd = press(t, m, "l")
	m = drain(t, m, cmd)
	if got := len(m.fsIndex["build"]); got != 1 {
		var names []string
		for _, e := range m.entries() {
			names = append(names, e.Name)
		}
		t.Fatalf("the disk listing holds %d entries, want the one file written (cwd=%q listing=%v)",
			got, m.cwd, names)
	}

	write("build/second.txt", "two\n")
	m = drain(t, m, func() tea.Msg { return tickMsg{} })
	if got := len(m.fsIndex["build"]); got != 1 {
		t.Errorf("the tick re-read the disk listing: %d entries", got)
	}

	m, cmd = press(t, m, "R")
	m = drain(t, m, cmd)
	if got := len(m.fsIndex["build"]); got != 2 {
		t.Errorf("R left %d entries for build, want both files re-read", got)
	}
}

// The Explorer's preview owns its own fields. Sharing them with Local Changes
// would put the two tabs in the same three variables, and opening one would
// empty the other.
func TestTheExplorerLeavesLocalChangesAlone(t *testing.T) {
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
	write("a.txt", "one\n")
	write("b.txt", "one\n")
	run("add", ".")
	run("commit", "-m", "initial")
	write("a.txt", "two\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	// Local Changes, showing a diff and annotating the file.
	m = drain(t, m, m.refreshPreview())
	m, cmd := press(t, m, "b")
	m = drain(t, m, cmd)
	diff, title := m.mainContent, m.mainTitle
	if diff == "" || !m.blameOn {
		t.Fatalf("the fixture did not reach a blamed diff: blame=%v content=%d bytes", m.blameOn, len(diff))
	}

	m, cmd = press(t, m, "4")
	m = drain(t, m, cmd)
	m = drain(t, m, m.refreshPreview())
	if m.previewContent == "" {
		t.Fatal("the Explorer previewed nothing, so this proves nothing about the fields")
	}

	m, _ = press(t, m, "1")
	if m.mainContent != diff {
		t.Error("the Explorer replaced Local Changes' diff")
	}
	if m.mainTitle != title {
		t.Errorf("the Explorer replaced Local Changes' title with %q", m.mainTitle)
	}
	if !m.blameOn || m.blamePath == "" {
		t.Error("the Explorer turned Local Changes' annotations off")
	}
}

// What deleting costs depends on whether git holds a copy, so the question has
// to differ: a tracked file is recoverable and an untracked one is not, and
// asking the same thing about both makes one read as final and the other as
// routine.
func TestTheDeleteQuestionSaysWhenGitHoldsNoCopy(t *testing.T) {
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
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "--initial-branch=main")
	write("tracked.txt", "kept\n")
	run("add", ".")
	run("commit", "-m", "initial")
	write("fresh.txt", "new\n")
	write("junk/one.txt", "a\n")
	write("junk/two.txt", "b\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)
	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)

	ask := func(name string) string {
		t.Helper()
		at := m
		var found bool
		for i, e := range at.entries() {
			if e.Name == name {
				at.cursor[PanelEntries], found = i, true
			}
		}
		if !found {
			t.Fatalf("%q is not in the listing", name)
		}
		next, _, handled := at.explorerOpKey("x")
		if !handled {
			t.Fatal("x is not handled")
		}
		after := next.(Model)
		if after.overlay.kind != overlayConfirm {
			t.Fatalf("deleting %q did not ask first", name)
		}
		return after.overlay.body
	}

	if got := ask("tracked.txt"); strings.Contains(got, "no copy") {
		t.Errorf("deleting a tracked file says %q, and git does hold a copy of it", got)
	}
	if got := ask("fresh.txt"); !strings.Contains(got, "no copy") {
		t.Errorf("deleting an untracked file says %q, without saying it is unrecoverable", got)
	}

	junk := ask("junk")
	if !strings.Contains(junk, "no copy") {
		t.Errorf("deleting an untracked directory says %q", junk)
	}
	if !strings.Contains(junk, "2 files") {
		t.Errorf("deleting a directory says %q, without saying how much is in it", junk)
	}
}

// A file long enough to be slow to draw is cut on the way in, so the pane's
// cost is bounded by the cap rather than by the file.
func TestALongFileIsPreviewedUpToItsCap(t *testing.T) {
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

	const lines = 3000
	if err := os.WriteFile(filepath.Join(dir, "long.txt"),
		[]byte(strings.Repeat("a line of text\n", lines)), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init", "--initial-branch=main")
	run("add", ".")
	run("commit", "-m", "initial")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)
	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)
	m = drain(t, m, m.refreshPreview())

	if m.previewFor.path != "long.txt" {
		t.Fatalf("the preview is of %q", m.previewFor.path)
	}
	if got := m.previewLen(); got != 2000 {
		t.Errorf("a %d-line file previews %d lines, want it capped at 2000", lines, got)
	}
}

// The colouring has to survive the whole path: the read runs on its own
// goroutine, and what it produces has to reach the pane.
func TestASourceFilePreviewsColoured(t *testing.T) {
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

	const source = "package main\n\n// the entry point\nfunc main() {\n\tprintln(42)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("init", "--initial-branch=main")
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)
	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)
	m = drain(t, m, m.refreshPreview())

	if m.previewFor.path != "main.go" || m.previewFor.kind != previewContent {
		t.Fatalf("the preview is %+v, want main.go as content", m.previewFor)
	}
	if len(m.previewStyled) == 0 {
		t.Fatal("a Go file arrived with no colouring")
	}

	pane := strings.Join(m.previewPaneLines(20, 60), "\n")
	if !strings.Contains(pane, "\x1b[") {
		t.Error("the pane drew the file with no colour in it")
	}
	if !strings.Contains(ansi.Strip(pane), "the entry point") {
		t.Error("the colouring changed what the pane says")
	}

	// A clean file with no lexer for it is drawn as it stands.
	plain := m
	plain.previewStyled = nil
	if got := strings.Join(plain.previewPaneLines(20, 60), "\n"); strings.Contains(got, "\x1b[") {
		t.Error("a file with no colouring was drawn with colour anyway")
	}
}

// TestEndToEndSortBySize drives the size order against a real repository. The
// numbers it sorts by are read per directory, in a command, and installed by a
// message: a unit test can assert the comparator, but only this can show that
// the read is actually triggered and that what comes back reaches the listing.
func TestEndToEndSortBySize(t *testing.T) {
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

	for name, size := range map[string]int{"small.txt": 10, "middle.txt": 500, "big.txt": 5000} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("x", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun("init", "--initial-branch=main")
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = drain(t, m, m.loadIndex())
	m.tab, m.focus = TabFiles, PanelEntries

	// In name order the biggest file is in the middle.
	if got := names(m.entries()); got[0] != "big.txt" {
		t.Fatalf("name order = %v", got)
	}

	m, cmd := press(t, m, ",")
	if m.overlay.kind != overlayChoice {
		t.Fatalf("no order was offered: overlay kind %d", m.overlay.kind)
	}
	// The size row is fourth: name, git status, extension, size, modified.
	next, cmd = m.takeChoice(3)
	m = drain(t, next.(Model), cmd)

	want := []string{"big.txt", "middle.txt", "small.txt"}
	if got := names(m.entries()); !slices.Equal(got, want) {
		t.Errorf("size order = %v, want %v", got, want)
	}
	if !strings.Contains(m.explorerTitle(PanelEntries), "size") {
		t.Errorf("the title does not say what the order is: %q", m.explorerTitle(PanelEntries))
	}
}

// e cycles the preview to blame or history, and the snapshot poll — which runs
// every few seconds whether or not anything moved — recomputed the kind from
// the file and put the diff straight back. The chosen view lasted about a
// second, which read as it flashing up and vanishing.
func TestTheChosenPreviewSurvivesTheSnapshotPoll(t *testing.T) {
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

	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("package main\n\nfunc main() {}\n")
	gitRun("init", "--initial-branch=main")
	gitRun("add", ".")
	gitRun("commit", "-m", "first")
	write("package main\n\nfunc main() { println(1) }\n")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)
	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)
	m = drain(t, m, m.refreshPreview())

	// A modified file lands on its diff, so two steps reach the history.
	for range 2 {
		m = drain(t, m, m.cyclePreview())
	}
	if m.previewFor.kind != previewHistory {
		t.Fatalf("e twice reached %v, want the file's history", m.previewFor.kind)
	}
	if !strings.Contains(m.previewTitle, "History") {
		t.Errorf("the pane is titled %q", m.previewTitle)
	}
	if !strings.Contains(m.previewContent, "first") {
		t.Errorf("the history does not hold the commit that touched the file: %q", m.previewContent)
	}

	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	if m.previewFor.kind != previewHistory {
		t.Errorf("the poll put the pane back to %v", m.previewFor.kind)
	}
}

// The same reset walked a long file back to its first line every few seconds
// while it was being read.
func TestThePreviewScrollSurvivesTheSnapshotPoll(t *testing.T) {
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

	var body strings.Builder
	for i := range 300 {
		fmt.Fprintf(&body, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("init", "--initial-branch=main")
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(ctx, repo)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)
	m, cmd := press(t, m, "4")
	m = drain(t, m, cmd)
	m = drain(t, m, m.refreshPreview())

	m.focus = PanelPreview
	m = drain(t, m, nil)
	moved, _ := m.explorerScroll(120)
	m = moved.(Model)
	scrolled := m.previewOffset
	if scrolled == 0 {
		t.Fatal("the preview did not scroll")
	}

	next, _ = m.Update(snapshotMsg(repo.Load(ctx, 50, "")))
	m = next.(Model)

	if m.previewOffset != scrolled {
		t.Errorf("the poll moved the preview from line %d to %d", scrolled, m.previewOffset)
	}
}

// Landing on another file is a new subject, so it starts at its top and on the
// kind that file calls for rather than inheriting the last one's.
func TestAnotherFileStartsFreshRatherThanInheriting(t *testing.T) {
	m := Model{
		previewFor:    previewID{path: "old.txt", kind: previewHistory},
		previewOffset: 42,
		cwd:           ".",
		index: map[string][]fsEntry{".": {
			{Name: "new.txt", Cached: true},
		}},
		dirCursor: map[string]int{},
	}

	m.refreshExplorerPreview()

	if m.previewFor.path != "new.txt" {
		t.Fatalf("the preview is of %q", m.previewFor.path)
	}
	if m.previewFor.kind != previewContent {
		t.Errorf("kind = %v, want a clean file's content", m.previewFor.kind)
	}
	if m.previewOffset != 0 {
		t.Errorf("offset = %d, want the new file read from its top", m.previewOffset)
	}
}
