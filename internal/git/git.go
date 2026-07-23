// Package git shells out to git and hands back parsed structs.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Repo is a git repository rooted at Path.
type Repo struct {
	Path string
}

// commandTimeout bounds a local git invocation, so a git that never returns
// cannot freeze the loop waiting on it. Deadlines already set are left alone.
const commandTimeout = 60 * time.Second

// Open resolves path to its repository root.
func Open(ctx context.Context, path string) (*Repo, error) {
	probe := &Repo{Path: path}
	root, err := probe.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %s", path)
	}
	return &Repo{Path: strings.TrimSpace(root)}, nil
}

// run executes a git subcommand and returns stdout.
func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	return r.runEnv(ctx, nil, args...)
}

// runEnv is run with extra environment variables layered on top.
func (r *Repo) runEnv(ctx context.Context, extra map[string]string, args ...string) (string, error) {
	return r.exec(ctx, extra, "", args...)
}

// runStdin is run with input fed to the command's stdin.
func (r *Repo) runStdin(ctx context.Context, stdin string, args ...string) (string, error) {
	return r.exec(ctx, nil, stdin, args...)
}

func (r *Repo) exec(ctx context.Context, extra map[string]string, stdin string, args ...string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, commandTimeout)
		defer cancel()
	}

	full := append([]string{"-C", r.Path}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// GIT_OPTIONAL_LOCKS=0: the refresh loop polls, and taking index.lock every
	// tick would race whatever git is running in another terminal.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	for key, value := range extra {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Never inherit stdin: a subcommand would steal keystrokes from the TUI.
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	if err := cmd.Run(); err != nil {
		// A killed command's stderr would only say "signal: killed".
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git %s: timed out", strings.Join(args, " "))
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "),
			failureMessage(stderr.String(), stdout.String(), err))
	}
	return stdout.String(), nil
}

// failureMessage picks the most informative explanation git gave, flattened to
// one footer line. stdout is checked too: `git commit` with an empty index
// prints "nothing to commit" there, not to stderr.
func failureMessage(stderr, stdout string, err error) string {
	for _, candidate := range []string{stderr, stdout} {
		if flat := strings.Join(strings.Fields(candidate), " "); flat != "" {
			return flat
		}
	}
	return err.Error()
}

// Snapshot is everything the panels need for one repaint.
type Snapshot struct {
	Branch   string
	Files    []FileChange
	Branches []Branch
	Commits  []Commit
	Unpushed map[string]bool
	Stashes  []Stash
	Rebasing bool
	Errs     []error
}

// Load fetches every panel's data concurrently, so a refresh costs the slowest
// call rather than their sum. logRef is the ref the commit list covers; empty
// means the checked-out one.
func (r *Repo) Load(ctx context.Context, logLimit int, logRef string) Snapshot {
	var (
		snap Snapshot
		mu   sync.Mutex
		wg   sync.WaitGroup
	)

	fail := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		snap.Errs = append(snap.Errs, err)
		mu.Unlock()
	}

	wg.Add(7)
	go func() { defer wg.Done(); v, err := r.Unpushed(ctx, logRef, logLimit); snap.Unpushed = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.CurrentBranch(ctx); snap.Branch = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.Status(ctx); snap.Files = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.Branches(ctx); snap.Branches = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.LogRef(ctx, logRef, logLimit); snap.Commits = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.Stashes(ctx); snap.Stashes = v; fail(err) }()
	go func() { defer wg.Done(); v, err := r.RebaseInProgress(ctx); snap.Rebasing = v; fail(err) }()
	wg.Wait()

	return snap
}

// CurrentBranch is the checked-out branch, or "<sha> (detached)".
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	name, err := r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// unborn branch: rev-parse can't resolve HEAD yet, but status names it
		out, serr := r.run(ctx, "status", "--porcelain=v2", "--branch", "-z")
		if serr != nil {
			return "", err
		}
		for _, rec := range strings.Split(out, "\x00") {
			if after, ok := strings.CutPrefix(rec, "# branch.head "); ok {
				return after, nil
			}
		}
		return "", nil
	}
	name = strings.TrimSpace(name)
	if name != "HEAD" {
		return name, nil
	}
	sha, err := r.run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "HEAD (detached)", nil
	}
	return strings.TrimSpace(sha) + " (detached)", nil
}

// Status lists working-tree and index changes.
func (r *Repo) Status(ctx context.Context) ([]FileChange, error) {
	out, err := r.run(ctx, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// Branches lists local branches, remotes and tags with divergence counts.
func (r *Repo) Branches(ctx context.Context) ([]Branch, error) {
	out, err := r.run(ctx, "for-each-ref",
		"--format="+refFormat,
		"--sort=-committerdate",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	return parseRefs(out), nil
}

// Log reads the most recent commits reachable from HEAD.
func (r *Repo) Log(ctx context.Context, limit int) ([]Commit, error) {
	return r.LogRef(ctx, "", limit)
}

// LogRef reads the most recent commits reachable from ref, HEAD when it is
// empty. Topological order rather than date order, so a branch's commits
// arrive together instead of interleaved with another's by timestamp.
func (r *Repo) LogRef(ctx context.Context, ref string, limit int) ([]Commit, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := r.run(ctx, "log",
		"--format="+logFormat,
		fmt.Sprintf("--max-count=%d", limit),
		"--topo-order",
		"--end-of-options", ref)
	if err != nil {
		// An empty repository is not an error worth showing.
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "unknown revision") {
			return nil, nil
		}
		return nil, err
	}
	return parseLog(out), nil
}

// Unpushed is the set of commits on ref that no remote holds. Reachability
// from the remote-tracking refs rather than the upstream counter: a branch
// with no upstream is not thereby unpublished.
func (r *Repo) Unpushed(ctx context.Context, ref string, limit int) (map[string]bool, error) {
	if ref == "" {
		ref = "HEAD"
	}
	// Both --not switches come before the ref: the first excludes everything the
	// remotes hold, the second flips back so the ref itself is included. git
	// rejects a --not after a rev, so --end-of-options cannot guard the ref.
	out, err := r.run(ctx, "rev-list",
		fmt.Sprintf("--max-count=%d", limit),
		"--not", "--remotes", "--not", ref)
	if err != nil {
		// No commits yet, or a ref that has just gone: the marks do not
		// appear.
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "unknown revision") {
			return nil, nil
		}
		return nil, err
	}

	unpushed := make(map[string]bool)
	for _, sha := range strings.Fields(out) {
		unpushed[sha] = true
	}
	return unpushed, nil
}

// Stashes lists the stash stack.
func (r *Repo) Stashes(ctx context.Context) ([]Stash, error) {
	out, err := r.run(ctx, "stash", "list", "--format=%gd%x1f%s")
	if err != nil {
		return nil, err
	}
	return parseStashes(out), nil
}

// Diff returns the unified diff for one path. With staged, it is the index
// against HEAD; otherwise the working tree against the index.
func (r *Repo) Diff(ctx context.Context, path string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "-M"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--end-of-options", "--", path)
	return r.run(ctx, args...)
}

// UntrackedPreview is the contents of a file git has no diff for yet.
func (r *Repo) UntrackedPreview(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(r.Path + string(os.PathSeparator) + path)
	if err != nil {
		return "", err
	}
	const maxBytes = 256 * 1024
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return string(data), nil
}

// CommitDiff is the patch a commit introduced, against its first parent.
func (r *Repo) CommitDiff(ctx context.Context, sha string) (string, error) {
	return r.run(ctx, "show", "--no-color", "-M", "--stat", "--patch",
		"--format=%C(auto)commit %H%n%anAuthor: %an <%ae>%nDate:   %ad%n%n    %s%n%n%b",
		"--end-of-options", sha)
}

// CommitFiles lists the paths a commit touched, against its first parent.
func (r *Repo) CommitFiles(ctx context.Context, sha string) ([]FileChange, error) {
	out, err := r.run(ctx, "show", "--no-color", "-M", "--name-status", "-z",
		"--format=", "--end-of-options", sha)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// CommitFileDiff is the part of a commit that touches one path.
func (r *Repo) CommitFileDiff(ctx context.Context, sha, path string) (string, error) {
	return r.run(ctx, "show", "--no-color", "-M", "--format=",
		"--end-of-options", sha, "--", path)
}

// StashDiff is the patch a stash entry holds.
func (r *Repo) StashDiff(ctx context.Context, ref string) (string, error) {
	return r.run(ctx, "stash", "show", "--no-color", "--patch", "--end-of-options", ref)
}

// StashFileDiff is the part of a stash entry that touches one path. It goes
// through `diff` against the stash's first parent rather than `stash show`,
// which rejects a pathspec outright ("Too many revisions specified").
func (r *Repo) StashFileDiff(ctx context.Context, ref, path string) (string, error) {
	return r.run(ctx, "diff", "--no-color", "-M",
		"--end-of-options", ref+"^", ref, "--", path)
}

// StashFiles lists the paths a stash entry changed, each with the status letter
// it carries against the commit the stash was taken from.
func (r *Repo) StashFiles(ctx context.Context, ref string) ([]FileChange, error) {
	out, err := r.run(ctx, "stash", "show", "--no-color", "--name-status", "-z",
		"--end-of-options", ref)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// FileLog is the recent history of one path. --follow so a rename does not cut
// the history short at the point the name changed.
func (r *Repo) FileLog(ctx context.Context, path string, limit int) (string, error) {
	return r.run(ctx, "log", "--no-color", "--follow",
		"--format="+prettyLogFormat,
		fmt.Sprintf("--max-count=%d", limit), "--", path)
}

// BranchLog is the recent history of a ref.
func (r *Repo) BranchLog(ctx context.Context, ref string, limit int) (string, error) {
	return r.run(ctx, "log", "--no-color", "--format="+prettyLogFormat,
		fmt.Sprintf("--max-count=%d", limit), "--end-of-options", ref)
}
