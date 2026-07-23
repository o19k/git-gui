package git

import (
	"context"
	"errors"
	"strings"
)

// A worktree is a second checkout of the same repository: another branch, in
// another directory, sharing one object store. The tool can open one, so the
// list doubles as a way of moving between them.

// Worktree is one checkout of the repository.
type Worktree struct {
	Path   string
	Branch string // short name, empty when HEAD is detached
	Head   string // the commit it sits on

	Bare     bool
	Detached bool
	Locked   bool
}

// Name is the branch where there is one, and the commit where there is not.
func (w Worktree) Name() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Branch != "":
		return w.Branch
	case len(w.Head) >= 7:
		return w.Head[:7] + " (detached)"
	}
	return "(detached)"
}

// Worktrees lists every checkout of this repository, the main one first.
func (r *Repo) Worktrees(ctx context.Context) ([]Worktree, error) {
	out, err := r.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out), nil
}

// parseWorktrees reads `worktree list --porcelain`: records separated by a
// blank line, each opening with the path.
func parseWorktrees(out string) []Worktree {
	var (
		trees   []Worktree
		current *Worktree
	)
	flush := func() {
		if current != nil {
			trees = append(trees, *current)
			current = nil
		}
	}

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			current = &Worktree{Path: value}
		case "":
			flush()
		default:
			if current == nil {
				continue
			}
			switch key {
			case "HEAD":
				current.Head = value
			case "branch":
				current.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "bare":
				current.Bare = true
			case "detached":
				current.Detached = true
			case "locked":
				current.Locked = true
			}
		}
	}
	flush()
	return trees
}

// AddWorktree checks a branch out into another directory. With create the
// branch is made at HEAD first; without it the branch has to exist already, and
// git refuses one that is checked out somewhere else.
//
// A relative path is taken from the repository root, which is where every
// command here runs.
func (r *Repo) AddWorktree(ctx context.Context, path, branch string, create bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is empty")
	}
	if err := checkRefName("branch", branch); err != nil {
		return err
	}

	args := []string{"worktree", "add"}
	if create {
		args = append(args, "-b", branch, "--end-of-options", path)
	} else {
		args = append(args, "--end-of-options", path, branch)
	}
	_, err := r.run(ctx, args...)
	return err
}

// RemoveWorktree deletes a checkout's directory and the administrative files
// that pointed at it. Without force git refuses one holding changes.
func (r *Repo) RemoveWorktree(ctx context.Context, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := r.run(ctx, append(args, "--end-of-options", path)...)
	return err
}
