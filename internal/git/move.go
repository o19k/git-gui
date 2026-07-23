package git

import (
	"context"
	"os"
)

// Move renames a path. git mv keeps the index in step, where a plain rename
// would leave the old path staged as a deletion and the new one untracked. An
// untracked path has no index entry, so it moves on disk.
func (r *Repo) Move(ctx context.Context, from, to string, tracked bool) error {
	if tracked {
		_, err := r.run(ctx, "mv", from, to)
		return err
	}
	return os.Rename(from, to)
}
