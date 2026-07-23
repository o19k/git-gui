package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A commit carries two dates: the author date, which the log shows, and the
// committer date, when the object was written. Both are set together — setting
// only the author date leaves --pretty=fuller reading as a years-late record.

// SetCommitDate rewrites one commit's dates.
//
// The tip commit is a plain amend. Anything further back is a rewrite of every
// commit above it, and goes through the same interactive rebase reword and drop
// use: stop at the commit, amend, continue.
func (r *Repo) SetCommitDate(ctx context.Context, sha, date string) error {
	date, err := normalizeDate(date)
	if err != nil {
		return err
	}

	tip, err := r.isTipCommit(ctx, sha)
	if err != nil {
		return err
	}
	if tip {
		return r.amendDate(ctx, date)
	}

	if err := r.Rebase(ctx, ActionEdit, sha, ""); err != nil {
		return err
	}
	if err := r.amendDate(ctx, date); err != nil {
		// Leaving the rebase open would strand the repository part-way through
		// the rewrite.
		_ = r.RebaseAbort(ctx)
		return err
	}
	return r.RebaseContinue(ctx)
}

// amendDate rewrites the commit at the tip, which is where a stopped rebase
// also leaves the commit being edited.
func (r *Repo) amendDate(ctx context.Context, date string) error {
	_, err := r.runEnv(ctx, map[string]string{
		"GIT_COMMITTER_DATE": date,
		"GIT_EDITOR":         "true",
	}, "commit", "--amend", "--no-edit", "--allow-empty", "--date="+date)
	return err
}

// isTipCommit reports whether sha is the commit HEAD points at. The two are
// compared as whole object names rather than by prefix, since the caller's sha
// may be abbreviated and a prefix test would accept the wrong commit.
func (r *Repo) isTipCommit(ctx context.Context, sha string) (bool, error) {
	head, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return false, errors.New("this repository has no commits yet")
	}
	full, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", sha+"^{commit}")
	if err != nil {
		return false, fmt.Errorf("no such commit: %s", short(sha))
	}
	return strings.TrimSpace(head) == strings.TrimSpace(full), nil
}

// normalizeDate accepts what a person would type and returns the form git
// reads back unambiguously. Checked here rather than left to git: an
// unvalidated value on the command line can be read as a flag.
func normalizeDate(date string) (string, error) {
	const form = "expected YYYY-MM-DD or YYYY-MM-DD HH:MM:SS"

	date = strings.TrimSpace(date)
	if date == "" {
		return "", errors.New("the date is empty — " + form)
	}

	day, clock, hasClock := strings.Cut(date, " ")
	if err := checkFields(day, "-", []int{4, 2, 2}); err != nil {
		return "", errors.New(err.Error() + " — " + form)
	}
	if !hasClock {
		// Midnight, so a date with no time still names one instant.
		return day + " 00:00:00", nil
	}
	if err := checkFields(clock, ":", []int{2, 2, 2}); err != nil {
		return "", errors.New(err.Error() + " — " + form)
	}
	return day + " " + clock, nil
}

// checkFields verifies that s is digit groups of the given widths joined by sep.
func checkFields(s, sep string, widths []int) error {
	parts := strings.Split(s, sep)
	if len(parts) != len(widths) {
		return fmt.Errorf("%q is not a date this understands", s)
	}
	for i, part := range parts {
		if len(part) != widths[i] {
			return fmt.Errorf("%q is not a date this understands", s)
		}
		if _, err := strconv.Atoi(part); err != nil {
			return fmt.Errorf("%q is not a date this understands", s)
		}
	}
	return nil
}
