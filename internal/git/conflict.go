package git

import (
	"context"
	"strings"
)

// A conflicted path sits in the index three times over: the common ancestor at
// stage 1, the branch's own version at stage 2 ("ours"), and the version being
// applied at stage 3 ("theirs").

// Unmerged lists the paths that still hold a conflict.
func (r *Repo) Unmerged(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// ResolveOurs settles a conflict on the version the branch already had.
func (r *Repo) ResolveOurs(ctx context.Context, path string) error {
	return r.resolve(ctx, "--ours", path)
}

// ResolveTheirs settles a conflict on the version being applied.
func (r *Repo) ResolveTheirs(ctx context.Context, path string) error {
	return r.resolve(ctx, "--theirs", path)
}

func (r *Repo) resolve(ctx context.Context, side, path string) error {
	if _, err := r.run(ctx, "checkout", side, "--", path); err != nil {
		return err
	}
	return r.MarkResolved(ctx, path)
}

// MarkResolved stages a path as it now stands, which is how git is told a
// conflict edited by hand is finished with.
func (r *Repo) MarkResolved(ctx context.Context, path string) error {
	_, err := r.run(ctx, "add", "--", path)
	return err
}

// MergeInProgress reports a merge stopped part-way, which needs its own commit
// to finish rather than a rebase --continue.
func (r *Repo) MergeInProgress(ctx context.Context) (bool, error) {
	_, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	return err == nil, nil
}

// MergeContinue records the merge commit once every conflict is resolved.
func (r *Repo) MergeContinue(ctx context.Context) error {
	_, err := r.run(ctx, "commit", "--no-edit")
	return err
}

// MergeAbort throws away a stopped merge and puts the branch back.
func (r *Repo) MergeAbort(ctx context.Context) error {
	_, err := r.run(ctx, "merge", "--abort")
	return err
}

func splitLines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
