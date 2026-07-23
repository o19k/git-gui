package git

import (
	"context"
	"strings"
)

// CreateTag names a revision. With a message the tag is annotated, which is an
// object of its own carrying who made it and when; without one it is a plain
// pointer.
func (r *Repo) CreateTag(ctx context.Context, name, rev, message string) error {
	if err := checkRefName("tag", name); err != nil {
		return err
	}

	args := []string{"tag"}
	if message != "" {
		args = append(args, "--annotate", "--message", message)
	}
	args = append(args, "--end-of-options", name)
	if rev != "" {
		args = append(args, rev)
	}
	_, err := r.run(ctx, args...)
	return err
}

// PushTag publishes one tag. The full refs/tags/ form so a tag and a branch of
// the same name cannot be confused for one another.
func (r *Repo) PushTag(ctx context.Context, remote, name string) error {
	if err := checkRefName("tag", name); err != nil {
		return err
	}
	if remote == "" {
		remote = "origin"
	}
	return r.runRemote(ctx, "push", remote, "refs/tags/"+name)
}

// DeleteRemoteTag removes a tag from the remote. The local tag is left alone:
// deleting it is a separate, undoable step.
func (r *Repo) DeleteRemoteTag(ctx context.Context, remote, name string) error {
	if err := checkRefName("tag", name); err != nil {
		return err
	}
	if remote == "" {
		remote = "origin"
	}
	return r.runRemote(ctx, "push", remote, "--delete", "refs/tags/"+name)
}

// HasTag reports whether a tag of this name already exists locally.
func (r *Repo) HasTag(ctx context.Context, name string) bool {
	out, err := r.run(ctx, "tag", "--list", "--end-of-options", name)
	return err == nil && strings.TrimSpace(out) != ""
}
