package git

import (
	"context"
	"fmt"
)

// CompareCommits lists the commits that target holds and base does not, in newest-first
// order. This answers the question "what would I gain if I took target".
func (r *Repo) CompareCommits(ctx context.Context, base, target string) ([]Commit, error) {
	out, err := r.run(ctx, "log", "--format="+logFormat,
		"--end-of-options", base+".."+target)
	if err != nil {
		return nil, err
	}
	return parseLog(out), nil
}

// CompareFiles lists the files that differ between base and target. With mergeBase,
// it uses a three-dot range to show files that differ since the branches parted
// (excluding changes on the base branch after the merge base). Without mergeBase,
// it uses a two-dot range for a direct comparison of the two tips.
func (r *Repo) CompareFiles(ctx context.Context, base, target string, mergeBase bool) ([]FileChange, error) {
	rangeSpec := base + ".." + target
	if mergeBase {
		rangeSpec = base + "..." + target
	}

	out, err := r.run(ctx, "diff", "--name-status", "-z", "-M",
		"--end-of-options", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("diff %s: %w", rangeSpec, err)
	}
	return parseNameStatus(out), nil
}
