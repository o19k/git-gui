package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Revisions are guarded with --end-of-options and paths with "--". The two are
// mutually exclusive: after --end-of-options git reads "--" as the path itself.

// Stage adds a path to the index.
func (r *Repo) Stage(ctx context.Context, path string) error {
	_, err := r.run(ctx, "add", "--", path)
	return err
}

// StageAll adds every change, including untracked files.
func (r *Repo) StageAll(ctx context.Context) error {
	_, err := r.run(ctx, "add", "--all")
	return err
}

// Unstage removes a path from the index, keeping the working tree as it is.
func (r *Repo) Unstage(ctx context.Context, path string) error {
	// reset, not restore --staged: also works for a path absent from HEAD.
	_, err := r.run(ctx, "reset", "--quiet", "HEAD", "--", path)
	return err
}

// UnstageAll empties the index back to HEAD, leaving the working tree alone.
func (r *Repo) UnstageAll(ctx context.Context) error {
	_, err := r.run(ctx, "reset", "--quiet", "HEAD")
	return err
}

// Untrack stops git following a path without deleting it, so it goes back to
// being an ordinary file on disk.
func (r *Repo) Untrack(ctx context.Context, path string) error {
	_, err := r.run(ctx, "rm", "--cached", "--quiet", "-r", "--", path)
	return err
}

// Ignore appends a path to .gitignore, creating the file if needed. Untracking
// is the caller's: a path already in the index goes on being followed whatever
// .gitignore says.
func (r *Repo) Ignore(ctx context.Context, path string) error {
	name := filepath.Join(r.Path, ".gitignore")

	existing, err := os.ReadFile(name)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// Nothing about the file's status changes, so a listed pattern would be
	// added again on every press.
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == path {
			return nil
		}
	}

	entry := path + "\n"
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		entry = "\n" + entry
	}

	file, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(entry)
	return err
}

// DeleteFile removes a path from the working tree, and from the index too when
// git was following it.
func (r *Repo) DeleteFile(ctx context.Context, file FileChange) error {
	if file.Untracked() {
		return os.Remove(filepath.Join(r.Path, file.Path))
	}
	_, err := r.run(ctx, "rm", "--force", "--quiet", "-r", "--", file.Path)
	return err
}

// RenameBranch renames a branch, keeping the upstream it tracks.
func (r *Repo) RenameBranch(ctx context.Context, old, name string) error {
	if err := checkNewBranchName(name); err != nil {
		return err
	}
	_, err := r.run(ctx, "branch", "--move", "--end-of-options", old, name)
	return err
}

// CreateBranchAt makes a branch at a commit and switches to it.
func (r *Repo) CreateBranchAt(ctx context.Context, name, sha string) error {
	if err := checkNewBranchName(name); err != nil {
		return err
	}
	_, err := r.run(ctx, "switch", "--create", name, "--end-of-options", sha)
	return err
}

// UndoCommit takes the last commit apart, leaving what it held staged. The
// commit's content is kept, so this undoes recording it rather than making it.
func (r *Repo) UndoCommit(ctx context.Context) error {
	_, err := r.run(ctx, "reset", "--soft", "HEAD~1")
	return err
}

// RebaseOnto replays the current branch's own commits on top of another ref.
func (r *Repo) RebaseOnto(ctx context.Context, upstream string) error {
	// An editor behind the alternate screen has nowhere to draw, so a rebase
	// stopping to write a message would hang instead of asking.
	_, err := r.runEnv(ctx, map[string]string{"GIT_EDITOR": "true"},
		"rebase", "--end-of-options", upstream)
	return err
}

// Discard throws away every change to a path — staged and unstaged both. For a
// file git has never seen, that means deleting it.
func (r *Repo) Discard(ctx context.Context, file FileChange) error {
	if file.Untracked() {
		return os.Remove(filepath.Join(r.Path, file.Path))
	}
	if err := r.Unstage(ctx, file.Path); err != nil {
		return err
	}
	if _, err := r.run(ctx, "checkout", "--quiet", "--", file.Path); err != nil {
		// Nothing committed to restore, so discarding means deleting.
		if strings.Contains(err.Error(), "did not match") || strings.Contains(err.Error(), "pathspec") {
			return os.Remove(filepath.Join(r.Path, file.Path))
		}
		return err
	}
	return nil
}

// CommitOpts are the parts of a commit that are not its message.
type CommitOpts struct {
	// Signoff adds the Signed-off-by trailer. Signing with a key is not here:
	// git decides that from commit.gpgsign, which this goes through.
	Signoff bool

	// Amend replaces the last commit instead of adding one.
	Amend bool
}

// args are the flags these options add to a commit command.
func (o CommitOpts) args() []string {
	var args []string
	if o.Signoff {
		args = append(args, "--signoff")
	}
	if o.Amend {
		args = append(args, "--amend")
	}
	return args
}

// Commit records the index under message.
func (r *Repo) Commit(ctx context.Context, message string, opts CommitOpts) error {
	args := append([]string{"commit"}, opts.args()...)
	_, err := r.run(ctx, append(args, "--message", message)...)
	return err
}

// CommitFile records the index under a message read from a file, which is how a
// message with a body gets in: a command line carries one, but the prompt that
// fills it in does not.
//
// --cleanup=strip drops the comment lines the template puts there, the same
// treatment the message would get had git opened the editor itself.
func (r *Repo) CommitFile(ctx context.Context, path string, opts CommitOpts) error {
	args := append([]string{"commit"}, opts.args()...)
	_, err := r.run(ctx, append(args, "--cleanup=strip", "--file", path)...)
	return err
}

// Amend replaces the last commit, keeping its parent.
func (r *Repo) Amend(ctx context.Context, message string, opts CommitOpts) error {
	opts.Amend = true
	return r.Commit(ctx, message, opts)
}

// Checkout switches to a branch.
func (r *Repo) Checkout(ctx context.Context, branch Branch) error {
	// A remote branch goes by its short name so git creates the tracking branch.
	name := branch.Name
	if branch.Kind == RefRemote {
		_, err := r.run(ctx, "switch", "--guess", "--end-of-options",
			strings.TrimPrefix(name, "origin/"))
		return err
	}
	_, err := r.run(ctx, "checkout", "--end-of-options", name)
	return err
}

// CreateBranch makes a branch at HEAD and switches to it.
func (r *Repo) CreateBranch(ctx context.Context, name string) error {
	// After --end-of-options, `switch --create` reads the argument as the start
	// point, not the new name — so the name is validated instead of separated.
	if err := checkNewBranchName(name); err != nil {
		return err
	}
	_, err := r.run(ctx, "switch", "--create", name)
	return err
}

// checkNewBranchName rejects names git would misread or refuse.
func checkNewBranchName(name string) error { return checkRefName("branch", name) }

// checkRefName rejects names git would misread or refuse. Stricter than git, so
// nothing reaching the command line can be taken for a flag. kind names what is
// being made, since branches and tags share the rules but not the message.
func checkRefName(kind, name string) error {
	switch {
	case name == "":
		return errors.New(kind + " name is empty")
	case strings.HasPrefix(name, "-"):
		return errors.New(kind + " name may not begin with '-'")
	case strings.ContainsAny(name, " \t\n~^:?*[\\"):
		return errors.New(kind + " name contains a character git forbids")
	case strings.Contains(name, ".."), strings.HasSuffix(name, ".lock"),
		strings.HasPrefix(name, "/"), strings.HasSuffix(name, "/"):
		return errors.New(kind + " name is not a valid ref")
	}
	return nil
}

// DeleteBranch removes a local branch or a tag. With force, an unmerged branch
// goes too.
func (r *Repo) DeleteBranch(ctx context.Context, branch Branch, force bool) error {
	if branch.Kind == RefTag {
		_, err := r.run(ctx, "tag", "--delete", "--end-of-options", branch.Name)
		return err
	}
	flag := "--delete"
	if force {
		flag = "-D"
	}
	_, err := r.run(ctx, "branch", flag, "--end-of-options", branch.Name)
	return err
}

// Merge merges a ref into the current branch, without opening an editor.
func (r *Repo) Merge(ctx context.Context, name string) error {
	_, err := r.run(ctx, "merge", "--no-edit", "--end-of-options", name)
	return err
}

// CherryPick applies a commit onto the current branch.
func (r *Repo) CherryPick(ctx context.Context, sha string) error {
	_, err := r.run(ctx, "cherry-pick", "--end-of-options", sha)
	return err
}

// Revert records a new commit undoing sha.
func (r *Repo) Revert(ctx context.Context, sha string) error {
	_, err := r.run(ctx, "revert", "--no-edit", "--end-of-options", sha)
	return err
}

// StashOpts chooses what a stash takes. The zero value is every tracked
// change, which is git's own default.
type StashOpts struct {
	Message string

	// Untracked also sets aside files git has never seen.
	Untracked bool

	// StagedOnly stashes what the index holds and leaves the working tree
	// alone. Needs git 2.35 or newer.
	StagedOnly bool

	// KeepIndex stashes the changes but leaves the index as it was, so a
	// staged-up commit can still be made afterwards.
	KeepIndex bool

	// Paths narrows the stash to these paths. Untracked files among them are
	// included whatever Untracked says: git takes the pathspec as the answer.
	Paths []string
}

// StashPush sets changes aside.
func (r *Repo) StashPush(ctx context.Context, opts StashOpts) error {
	args := []string{"stash", "push"}
	switch {
	case opts.StagedOnly:
		args = append(args, "--staged")
	case opts.Untracked:
		args = append(args, "--include-untracked")
	}
	if opts.KeepIndex && !opts.StagedOnly {
		args = append(args, "--keep-index")
	}
	if opts.Message != "" {
		args = append(args, "--message", opts.Message)
	}
	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}
	_, err := r.run(ctx, args...)
	return err
}

// StashApply restores a stash entry and keeps it on the stack.
func (r *Repo) StashApply(ctx context.Context, ref string) error {
	_, err := r.run(ctx, "stash", "apply", "--end-of-options", ref)
	return err
}

// StashPop restores a stash entry and drops it.
func (r *Repo) StashPop(ctx context.Context, ref string) error {
	_, err := r.run(ctx, "stash", "pop", "--end-of-options", ref)
	return err
}

// StashApplyFiles restores only the named paths from a stash entry, leaving the
// entry on the stack. `stash apply` is all-or-nothing, so the paths are read
// out of the stash commit's tree instead — which overwrites them rather than
// merging into them.
func (r *Repo) StashApplyFiles(ctx context.Context, ref string, paths []string) error {
	if len(paths) == 0 {
		return errors.New("no files selected")
	}
	args := append([]string{"checkout", "--end-of-options", ref, "--"}, paths...)
	_, err := r.run(ctx, args...)
	return err
}

// StashDrop deletes a stash entry.
func (r *Repo) StashDrop(ctx context.Context, ref string) error {
	_, err := r.run(ctx, "stash", "drop", "--end-of-options", ref)
	return err
}

// HeadMessage is the last commit's full message, used to pre-fill an amend.
func (r *Repo) HeadMessage(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "log", "-1", "--format=%B")
	return strings.TrimSpace(out), err
}

// GitDir is the repository's administrative directory, as an absolute path. It
// is asked for rather than assumed to be .git: in a worktree that name is a
// file pointing elsewhere.
func (r *Repo) GitDir(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CommitTemplate is the contents of the file commit.template names, empty when
// none is configured or it cannot be read. A repository that ships one expects
// every commit to start from it.
func (r *Repo) CommitTemplate(ctx context.Context) string {
	path, err := r.run(ctx, "config", "--get", "commit.template")
	if err != nil {
		return ""
	}
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Path, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// expandHome resolves the leading ~ git allows in a configured path.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
