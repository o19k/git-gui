package git

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

const networkTimeout = 2 * time.Minute

// A credential prompt behind the alternate screen has nowhere to draw and
// nowhere to read from, so it would hang forever. Disabled, git fails saying it
// could not authenticate. ssh-agent keys are unaffected.
var noPromptEnv = map[string]string{
	"GIT_TERMINAL_PROMPT": "0",
}

// runRemote executes a network command with prompting disabled and a deadline.
func (r *Repo) runRemote(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()

	_, err := r.runEnv(ctx, noPromptEnv, args...)
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		return errors.New("git " + args[0] + ": timed out after " + networkTimeout.String())
	}
	return err
}

// Fetch updates remote-tracking refs and prunes ones that are gone.
func (r *Repo) Fetch(ctx context.Context) error {
	return r.runRemote(ctx, "fetch", "--prune")
}

// Pull fast-forwards the current branch to its upstream. --ff-only so a single
// keystroke cannot make a merge commit or a conflict.
func (r *Repo) Pull(ctx context.Context) error {
	return r.runRemote(ctx, "pull", "--ff-only")
}

// PullAutostash pulls with local changes set aside and put back afterwards.
//
// git exits 0 even when putting them back conflicts, reporting it on stderr
// only, so a caller cannot tell the two apart from the error alone — check
// Unmerged afterwards.
func (r *Repo) PullAutostash(ctx context.Context) error {
	return r.runRemote(ctx, "pull", "--ff-only", "--autostash")
}

// PullRebase replays the local commits on top of the upstream, for a branch
// that has diverged from it.
func (r *Repo) PullRebase(ctx context.Context) error {
	return r.runRemote(ctx, "pull", "--rebase", "--autostash")
}

// PullMerge joins the upstream into the local branch with a merge commit, the
// other way out of a diverged branch.
func (r *Repo) PullMerge(ctx context.Context) error {
	return r.runRemote(ctx, "pull", "--no-rebase", "--no-edit", "--autostash")
}

// IsNotFastForward reports the refusal that means the branch and its upstream
// have each moved on, so neither can be reached from the other.
func IsNotFastForward(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Not possible to fast-forward")
}

// IsDirtyTree reports the refusal that means pulling would write over changes
// that are not committed yet.
func IsDirtyTree(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "would be overwritten") ||
		strings.Contains(err.Error(), "unstaged changes") ||
		strings.Contains(err.Error(), "You have unstaged")
}

// BlockingPaths are the paths git named as standing in the way of an operation
// it refused. It indents them under an introducing line and reports them
// nowhere else — there is no porcelain form of this refusal — so the message is
// the only source. Empty means git refused without naming anything.
func BlockingPaths(err error) []string {
	if err == nil {
		return nil
	}

	var paths []string
	listing := false
	for _, line := range strings.Split(err.Error(), "\n") {
		indented := line != strings.TrimLeft(line, " \t")
		switch {
		case strings.Contains(line, "following files") ||
			strings.Contains(line, "following untracked working tree files"):
			listing = true
		case listing && indented:
			paths = append(paths, strings.TrimSpace(line))
		case listing:
			// Anything back at the margin ends the list — git closes it with a
			// sentence telling you what to do about it.
			listing = false
		}
	}
	return paths
}

// Outgoing lists the commits the current branch holds that its upstream does
// not, newest first, so a push can say what it is about to publish.
func (r *Repo) Outgoing(ctx context.Context, branch string) ([]Commit, error) {
	out, err := r.run(ctx, "log", "--format="+logFormat,
		"--end-of-options", "@{upstream}.."+branch)
	if err != nil {
		// A branch with no upstream publishes all of itself.
		if strings.Contains(err.Error(), "no upstream") ||
			strings.Contains(err.Error(), "unknown revision") {
			return r.Log(ctx, 50)
		}
		return nil, err
	}
	return parseLog(out), nil
}

// PushUpTo publishes the branch only as far as one commit, leaving whatever
// comes after it local.
func (r *Repo) PushUpTo(ctx context.Context, sha, branch string) error {
	if err := checkNewBranchName(branch); err != nil {
		return err
	}
	return r.runRemote(ctx, "push", "origin", sha+":refs/heads/"+branch)
}

// Push publishes the current branch, setting an upstream if it has none.
func (r *Repo) Push(ctx context.Context, branch string, hasUpstream bool) error {
	if hasUpstream {
		return r.runRemote(ctx, "push")
	}
	if err := checkNewBranchName(branch); err != nil {
		return err
	}
	return r.runRemote(ctx, "push", "--set-upstream", "origin", branch)
}

// ForcePush overwrites the upstream with the local branch. --force-with-lease,
// never a bare --force: it refuses when the remote holds unseen commits.
func (r *Repo) ForcePush(ctx context.Context) error {
	return r.runRemote(ctx, "push", "--force-with-lease")
}

// Remotes lists the configured remote names.
func (r *Repo) Remotes(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// splitRemote cuts a remote-tracking name such as origin/feature/x into the
// remote and the branch on it.
//
// The remote is matched against the configured names rather than cut at the
// first slash: a branch name may hold slashes too, and only the configuration
// says where one ends and the other begins.
func (r *Repo) splitRemote(ctx context.Context, ref string) (remote, branch string, err error) {
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return "", "", err
	}
	// Longest first, so a remote called "origin" cannot claim a ref that
	// belongs to one called "origin/mirror".
	slices.SortFunc(remotes, func(a, b string) int { return len(b) - len(a) })

	for _, name := range remotes {
		if rest, ok := strings.CutPrefix(ref, name+"/"); ok && rest != "" {
			return name, rest, nil
		}
	}
	return "", "", errors.New(ref + ": no configured remote holds this branch")
}

// DeleteRemoteBranch removes a branch on the remote itself, and with it the
// tracking ref that followed it. There is no force and no lesser form.
func (r *Repo) DeleteRemoteBranch(ctx context.Context, branch Branch) error {
	if branch.Kind != RefRemote {
		return errors.New(branch.Name + " is not a remote branch")
	}
	remote, name, err := r.splitRemote(ctx, branch.Name)
	if err != nil {
		return err
	}
	// origin/HEAD is a pointer at the remote's default branch, not a branch,
	// and pushing a deletion of it asks the remote to unset its default.
	if name == "HEAD" {
		return errors.New(branch.Name + " points at the remote's default branch rather than being one")
	}
	if err := checkNewBranchName(name); err != nil {
		return err
	}
	return r.runRemote(ctx, "push", remote, "--delete", name)
}
