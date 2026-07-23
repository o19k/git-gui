package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Interactive rebase without an interactive editor: GIT_SEQUENCE_EDITOR points
// at this binary running a hidden subcommand, which rewrites one todo line and
// exits. git carries out the plan as if a person had edited it.

// RebaseAction is an edit to a single commit in the todo list.
type RebaseAction string

const (
	// ActionDrop removes the commit from history.
	ActionDrop RebaseAction = "drop"
	// ActionFixup folds the commit into its parent, discarding its message.
	ActionFixup RebaseAction = "fixup"
	// ActionSquash folds the commit into its parent, keeping both messages.
	ActionSquash RebaseAction = "squash"
	// ActionReword replaces the commit's message.
	ActionReword RebaseAction = "reword"
	// ActionEdit stops the rebase at the commit so it can be amended. Unlike
	// the others it does not finish on its own: git exits successfully with the
	// rebase held open, and the caller amends and continues.
	ActionEdit RebaseAction = "edit"
	// ActionMoveUp moves the commit one step later in history.
	ActionMoveUp RebaseAction = "moveup"
	// ActionMoveDown moves the commit one step earlier in history.
	ActionMoveDown RebaseAction = "movedown"
)

// Hidden subcommands this program runs as, standing in for git's editors.
const (
	TodoSubcommand    = "__rebase-todo"
	MessageSubcommand = "__rebase-message"
)

// TodoEnvMessage carries a reword's new message to the hidden editor
// subcommand, through the environment so nothing needs escaping.
const TodoEnvMessage = "GIT_GUI_REBASE_MESSAGE"

// RewriteTodo applies action to the todo entry for sha and returns the new todo
// file. Comments and blank lines are left untouched — git writes its own help
// into the file.
func RewriteTodo(todo string, action RebaseAction, sha string) (string, error) {
	lines := strings.Split(strings.TrimRight(todo, "\n"), "\n")

	var commands []int
	target := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		commands = append(commands, i)
		if fields := strings.Fields(trimmed); len(fields) >= 2 && shaMatches(fields[1], sha) {
			target = len(commands) - 1
		}
	}
	if target < 0 {
		return "", fmt.Errorf("commit %s is not in the rebase plan", short(sha))
	}

	at := commands[target]
	switch action {
	case ActionDrop, ActionFixup, ActionSquash, ActionReword, ActionEdit:
		lines[at] = replaceVerb(lines[at], string(action))

	case ActionMoveUp, ActionMoveDown:
		// The todo runs oldest-first, so later in history is further down.
		neighbour := target + 1
		if action == ActionMoveDown {
			neighbour = target - 1
		}
		if neighbour < 0 || neighbour >= len(commands) {
			return "", errors.New("the commit is already at the end of the rebase plan")
		}
		a, b := commands[target], commands[neighbour]
		lines[a], lines[b] = lines[b], lines[a]

	default:
		return "", fmt.Errorf("unknown rebase action %q", action)
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// replaceVerb swaps the command word at the start of a todo line, keeping the
// original indentation and the rest of the line intact.
func replaceVerb(line, verb string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line
	}
	return indent + verb + " " + strings.Join(fields[1:], " ")
}

// shaMatches reports whether two object names refer to the same commit, given
// that the todo file abbreviates them.
func shaMatches(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= 4 && strings.HasPrefix(b, a)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// Rebase performs action on sha via a non-interactive interactive rebase.
//
// message is used only by ActionReword.
func (r *Repo) Rebase(ctx context.Context, action RebaseAction, sha, message string) error {
	if dirty, err := r.hasChanges(ctx); err != nil {
		return err
	} else if dirty {
		return errors.New("commit or stash your changes before rewriting history")
	}

	base, err := r.rebaseBase(ctx, action, sha)
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate this program to drive the rebase: %w", err)
	}

	// git runs both through a shell, so the path is quoted. GIT_EDITOR is set
	// too: an editor opening behind the TUI would hang.
	env := map[string]string{
		"GIT_SEQUENCE_EDITOR": fmt.Sprintf("%s %s %s %s",
			shellQuote(self), TodoSubcommand, action, sha),
		"GIT_EDITOR":   fmt.Sprintf("%s %s", shellQuote(self), MessageSubcommand),
		TodoEnvMessage: message,
	}

	args := []string{"rebase", "--interactive", "--no-autosquash", "--quiet"}
	args = append(args, base...)

	if _, err := r.runEnv(ctx, env, args...); err != nil {
		if inProgress, _ := r.RebaseInProgress(ctx); inProgress {
			return fmt.Errorf("%w — the rebase stopped; resolve and continue, or abort", err)
		}
		return err
	}
	return nil
}

// rebaseBase works out where the rebase has to start. Rewriting one commit
// needs its parent; folding into the parent or swapping with a neighbour needs
// the grandparent. Falls back to --root when that ancestor does not exist.
func (r *Repo) rebaseBase(ctx context.Context, action RebaseAction, sha string) ([]string, error) {
	depth := "^"
	switch action {
	case ActionFixup, ActionSquash, ActionMoveUp, ActionMoveDown:
		depth = "^^"
	}
	if _, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", sha+depth); err == nil {
		return []string{"--end-of-options", sha + depth}, nil
	}
	if depth == "^^" {
		if _, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", sha+"^"); err != nil {
			return nil, errors.New("the root commit has no parent to fold into")
		}
	}
	return []string{"--root"}, nil
}

// hasChanges reports whether the working tree or index differs from HEAD.
func (r *Repo) hasChanges(ctx context.Context) (bool, error) {
	out, err := r.run(ctx, "status", "--porcelain=v2", "-z")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// RebaseInProgress reports whether a rebase is stopped part-way through.
func (r *Repo) RebaseInProgress(ctx context.Context) (bool, error) {
	out, err := r.run(ctx, "rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(r.Path, gitDir)
	}
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// RebaseAbort throws away a stopped rebase and restores the original branch.
func (r *Repo) RebaseAbort(ctx context.Context) error {
	_, err := r.run(ctx, "rebase", "--abort")
	return err
}

// RebaseContinue resumes a stopped rebase once conflicts are resolved.
func (r *Repo) RebaseContinue(ctx context.Context) error {
	_, err := r.runEnv(ctx, map[string]string{"GIT_EDITOR": "true"}, "rebase", "--continue")
	return err
}

// shellQuote wraps s for the shell git runs its editors through. Single quotes
// so backslashes stay literal — git for Windows passes the editor string to sh,
// and `C:\Users\me\git-gui.exe` has to survive it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RunTodoEditor is this program acting as git's sequence editor: it rewrites
// one entry in the todo file it is handed. args is [action, sha, todo-path].
func RunTodoEditor(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("%s expects an action, a commit and a file", TodoSubcommand)
	}
	action, sha, path := RebaseAction(args[0]), args[1], args[2]

	todo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rewritten, err := RewriteTodo(string(todo), action, sha)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rewritten), 0o644)
}

// RunMessageEditor is this program acting as git's commit-message editor. An
// empty message leaves the file as git wrote it, keeping the original.
func RunMessageEditor(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%s expects a file", MessageSubcommand)
	}
	message := os.Getenv(TodoEnvMessage)
	if strings.TrimSpace(message) == "" {
		return nil
	}
	return os.WriteFile(args[0], []byte(strings.TrimRight(message, "\n")+"\n"), 0o644)
}
