package git

import (
	"context"
	"fmt"
	"strings"
)

// Checks reads the repository's pre-commit checks from git config. A repository
// with no checks configured returns an empty slice with no error — the absence
// of the config key is not a failure.
func (r *Repo) Checks(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "config", "--get-all", "gitgui.check")
	if err != nil {
		// git config --get-all exits non-zero when the key is absent. That is
		// the normal case, not a failure.
		if strings.Contains(err.Error(), "config") && strings.Contains(err.Error(), "gitgui.check") {
			return nil, nil
		}
		return nil, fmt.Errorf("reading checks: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	// Each configured check is one line.
	return strings.Split(out, "\n"), nil
}
