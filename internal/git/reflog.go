package git

import (
	"context"
	"fmt"
	"strings"
)

// The reflog is the only record of where a branch was before a rebase, a reset
// or a checkout moved it, so it is what an undone-too-much is recovered from.

// ReflogEntry is one movement of HEAD.
type ReflogEntry struct {
	SHA      string
	Short    string
	Selector string // HEAD@{2}
	Action   string // what moved it: "commit: …", "rebase (finish): …"
	When     string
}

const reflogFormat = "%H" + us + "%h" + us + "%gd" + us + "%gs" + us + "%ar"

// Reflog reads the most recent movements of HEAD.
func (r *Repo) Reflog(ctx context.Context, limit int) ([]ReflogEntry, error) {
	out, err := r.run(ctx, "reflog", "--format="+reflogFormat,
		fmt.Sprintf("--max-count=%d", limit))
	if err != nil {
		// A repository with no commits has no reflog, which is not a failure.
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "unknown revision") {
			return nil, nil
		}
		return nil, err
	}
	return parseReflog(out), nil
}

func parseReflog(out string) []ReflogEntry {
	var entries []ReflogEntry
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, us)
		if len(fields) < 5 {
			continue
		}
		entries = append(entries, ReflogEntry{
			SHA: fields[0], Short: fields[1], Selector: fields[2],
			Action: fields[3], When: fields[4],
		})
	}
	return entries
}

// ResetHard moves the current branch to a revision and makes the working tree
// match it. What the tree held is gone; what was committed is still in the
// reflog.
func (r *Repo) ResetHard(ctx context.Context, rev string) error {
	_, err := r.run(ctx, "reset", "--hard", "--end-of-options", rev)
	return err
}

// CheckoutRev detaches HEAD at a revision, for looking at a commit without
// moving any branch onto it.
func (r *Repo) CheckoutRev(ctx context.Context, rev string) error {
	_, err := r.run(ctx, "switch", "--detach", "--end-of-options", rev)
	return err
}
