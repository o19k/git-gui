package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary answer the hidden subcommands git invokes as
// its editors. os.Executable() inside Rebase resolves to this binary, so the
// rebase machinery exercised below is the real one.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case TodoSubcommand:
			if err := RunTodoEditor(os.Args[2:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		case MessageSubcommand:
			if err := RunMessageEditor(os.Args[2:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	cleanup := pinGitConfig()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

const sampleTodo = `pick 1111111 first
pick 2222222 second
pick 3333333 third

# Rebase abc..def onto abc (3 commands)
#
# Commands:
# p, pick <commit> = use commit
`

func TestRewriteTodoChangesTheVerb(t *testing.T) {
	for _, action := range []RebaseAction{ActionDrop, ActionFixup, ActionSquash, ActionReword} {
		out, err := RewriteTodo(sampleTodo, action, "2222222")
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		want := string(action) + " 2222222 second"
		if !strings.Contains(out, want) {
			t.Errorf("%s: missing %q in:\n%s", action, want, out)
		}
		if !strings.Contains(out, "pick 1111111 first") || !strings.Contains(out, "pick 3333333 third") {
			t.Errorf("%s: the other commits were disturbed:\n%s", action, out)
		}
	}
}

func TestRewriteTodoKeepsGitsComments(t *testing.T) {
	out, err := RewriteTodo(sampleTodo, ActionDrop, "2222222")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"# Commands:", "# p, pick <commit> = use commit"} {
		if !strings.Contains(out, line) {
			t.Errorf("comment %q was eaten:\n%s", line, out)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("todo must end with a newline")
	}
}

func TestRewriteTodoMoves(t *testing.T) {
	up, err := RewriteTodo(sampleTodo, ActionMoveUp, "1111111")
	if err != nil {
		t.Fatal(err)
	}
	lines := commandLines(up)
	if lines[0] != "pick 2222222 second" || lines[1] != "pick 1111111 first" {
		t.Errorf("move up did not swap with the next commit: %q", lines)
	}

	down, err := RewriteTodo(sampleTodo, ActionMoveDown, "3333333")
	if err != nil {
		t.Fatal(err)
	}
	lines = commandLines(down)
	if lines[1] != "pick 3333333 third" || lines[2] != "pick 2222222 second" {
		t.Errorf("move down did not swap with the previous commit: %q", lines)
	}
}

func TestRewriteTodoRefusesImpossibleMoves(t *testing.T) {
	if _, err := RewriteTodo(sampleTodo, ActionMoveUp, "3333333"); err == nil {
		t.Error("moving the last entry further should error")
	}
	if _, err := RewriteTodo(sampleTodo, ActionMoveDown, "1111111"); err == nil {
		t.Error("moving the first entry further should error")
	}
}

func TestRewriteTodoUnknownCommitAndAction(t *testing.T) {
	if _, err := RewriteTodo(sampleTodo, ActionDrop, "9999999"); err == nil {
		t.Error("a commit outside the plan should error")
	}
	if _, err := RewriteTodo(sampleTodo, RebaseAction("teleport"), "1111111"); err == nil {
		t.Error("an unknown action should error")
	}
}

func TestRewriteTodoMatchesAbbreviatedShas(t *testing.T) {
	// The panel holds full object names; the todo file holds short ones.
	out, err := RewriteTodo(sampleTodo, ActionDrop, "2222222aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("a full sha did not match its abbreviation: %v", err)
	}
	if !strings.Contains(out, "drop 2222222 second") {
		t.Errorf("wrong line rewritten:\n%s", out)
	}
}

func commandLines(todo string) []string {
	var out []string
	for _, line := range strings.Split(todo, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			out = append(out, trimmed)
		}
	}
	return out
}

// historyRepo builds a repository with three commits touching separate files.
func historyRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "--initial-branch=main")
	for _, name := range []string{"first", "second", "third"} {
		commitIn(t, dir, name+".txt", name+"\n", name)
	}
	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func subjects(t *testing.T, repo *Repo) []string {
	t.Helper()
	commits, err := repo.Log(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range commits {
		out = append(out, c.Subject)
	}
	return out
}

func TestRebaseDropRemovesACommit(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	commits, _ := repo.Log(ctx, 20)
	middle := commits[1] // "second"

	if err := repo.Rebase(ctx, ActionDrop, middle.SHA, ""); err != nil {
		t.Fatalf("drop: %v", err)
	}

	got := subjects(t, repo)
	if len(got) != 2 || got[0] != "third" || got[1] != "first" {
		t.Errorf("history after drop = %v", got)
	}
	if _, err := os.Stat(filepath.Join(repo.Path, "second.txt")); !os.IsNotExist(err) {
		t.Error("the dropped commit's file is still there")
	}
}

func TestRebaseFixupFoldsIntoParent(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	commits, _ := repo.Log(ctx, 20)
	newest := commits[0] // "third" folds into "second"

	if err := repo.Rebase(ctx, ActionFixup, newest.SHA, ""); err != nil {
		t.Fatalf("fixup: %v", err)
	}

	got := subjects(t, repo)
	if len(got) != 2 {
		t.Fatalf("history after fixup = %v, want 2 commits", got)
	}
	if got[0] != "second" {
		t.Errorf("fixup should keep the parent's message, got %q", got[0])
	}
	// Both files survive: fixup discards the message, not the change.
	for _, name := range []string{"second.txt", "third.txt"} {
		if _, err := os.Stat(filepath.Join(repo.Path, name)); err != nil {
			t.Errorf("%s was lost: %v", name, err)
		}
	}
}

func TestRebaseRewordReplacesTheMessage(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	commits, _ := repo.Log(ctx, 20)
	middle := commits[1]

	if err := repo.Rebase(ctx, ActionReword, middle.SHA, "second, reworded"); err != nil {
		t.Fatalf("reword: %v", err)
	}

	got := subjects(t, repo)
	if len(got) != 3 {
		t.Fatalf("reword changed the shape of history: %v", got)
	}
	if got[1] != "second, reworded" {
		t.Errorf("message = %q", got[1])
	}
}

func TestRebaseMoveReordersHistory(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	commits, _ := repo.Log(ctx, 20)
	middle := commits[1] // "second"

	// Move it one step later in history: it should end up above "third".
	if err := repo.Rebase(ctx, ActionMoveUp, middle.SHA, ""); err != nil {
		t.Fatalf("move up: %v", err)
	}

	got := subjects(t, repo)
	if len(got) != 3 {
		t.Fatalf("move changed the number of commits: %v", got)
	}
	if got[0] != "second" || got[1] != "third" {
		t.Errorf("history after move = %v, want second above third", got)
	}
}

func TestRebaseRefusesWithADirtyTree(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo.Path, "first.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commits, _ := repo.Log(ctx, 20)

	err := repo.Rebase(ctx, ActionDrop, commits[1].SHA, "")
	if err == nil {
		t.Fatal("rewriting history with uncommitted changes should be refused")
	}
	if !strings.Contains(err.Error(), "stash") {
		t.Errorf("the refusal does not say what to do: %v", err)
	}
	if inProgress, _ := repo.RebaseInProgress(ctx); inProgress {
		t.Error("a refused rebase left the repository mid-rebase")
	}
}

func TestRebaseConflictIsReportedAndCanBeAborted(t *testing.T) {
	// Two commits on the same line: dropping the first stops the rebase.
	dir := t.TempDir()
	gitIn(t, dir, "init", "--initial-branch=main")
	commitIn(t, dir, "f.txt", "base\n", "base")
	commitIn(t, dir, "f.txt", "middle\n", "middle")
	commitIn(t, dir, "f.txt", "top\n", "top")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	commits, _ := repo.Log(ctx, 20)
	before := len(commits)

	err = repo.Rebase(ctx, ActionDrop, commits[1].SHA, "")
	if err == nil {
		t.Fatal("dropping a commit the next one depends on should have conflicted")
	}
	if !strings.Contains(err.Error(), "abort") {
		t.Errorf("the error does not tell the user the way out: %v", err)
	}

	inProgress, _ := repo.RebaseInProgress(ctx)
	if !inProgress {
		t.Fatal("the stopped rebase was not detected")
	}

	if err := repo.RebaseAbort(ctx); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if inProgress, _ := repo.RebaseInProgress(ctx); inProgress {
		t.Error("abort left the rebase in progress")
	}
	if got := subjects(t, repo); len(got) != before {
		t.Errorf("abort did not restore history: %v", got)
	}
}

func TestRebaseInProgressIsFalseOnACleanRepo(t *testing.T) {
	repo := historyRepo(t)
	inProgress, err := repo.RebaseInProgress(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inProgress {
		t.Error("a clean repository reported a rebase in progress")
	}
}

func TestMessageEditorLeavesTheFileAloneWithoutAMessage(t *testing.T) {
	// Reword with an empty message must keep the original, not blank it.
	path := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(TodoEnvMessage, "")

	if err := RunMessageEditor([]string{path}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "original\n" {
		t.Errorf("the message was overwritten: %q", content)
	}
}

func TestShellQuoteSurvivesTheShellGitUses(t *testing.T) {
	// Windows cases run on Unix too: it is git for Windows that interprets the
	// result, not the machine running the test.
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "a windows path keeps its separators",
			path: `C:\Users\me\git-gui.exe`,
			want: `'C:\Users\me\git-gui.exe'`,
		},
		{
			name: "spaces do not split the argument",
			path: `C:\Program Files\git-gui\git-gui.exe`,
			want: `'C:\Program Files\git-gui\git-gui.exe'`,
		},
		{
			name: "a plain unix path",
			path: "/usr/local/bin/git-gui",
			want: "'/usr/local/bin/git-gui'",
		},
		{
			name: "an apostrophe is closed, escaped and reopened",
			path: "/home/o'brien/git-gui",
			want: `'/home/o'\''brien/git-gui'`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shellQuote(c.path); got != c.want {
				t.Errorf("shellQuote(%q) = %s, want %s", c.path, got, c.want)
			}
		})
	}
}

func TestShellQuoteRoundTripsThroughAShell(t *testing.T) {
	// A real shell must hand the string back unchanged.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	for _, path := range []string{
		`C:\Users\me\git-gui.exe`,
		`C:\Program Files\git-gui\git-gui.exe`,
		"/home/o'brien/git gui",
		"/usr/local/bin/git-gui",
	} {
		out, err := exec.Command(sh, "-c", "printf %s "+shellQuote(path)).Output()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if string(out) != path {
			t.Errorf("sh turned %q into %q", path, out)
		}
	}
}
