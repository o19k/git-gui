package git

import (
	"context"
	"strings"
)

// The Explorer's listing comes from the index rather than the filesystem, so
// .gitignore, the index and submodule boundaries are git's answer.

// TreeEntry is one path git knows about, with enough of its mode to tell a
// file from a submodule or a symlink.
type TreeEntry struct {
	Path   string
	Mode   string // "100644", "120000" symlink, "160000" gitlink; empty when untracked
	Cached bool   // in the index — the discriminator a delete branches on
}

// IndexTree lists every tracked and untracked path in one read.
//
// --stage rather than --cached, for the mode: without it a gitlink is
// indistinguishable from a regular file. The output has two shapes in one
// stream — "<mode> <oid> <stage>\t<path>" for indexed entries, a bare path for
// untracked ones — which also decides index membership without a second call.
func (r *Repo) IndexTree(ctx context.Context) ([]TreeEntry, error) {
	out, err := r.run(ctx, "ls-files", "--stage", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}

	records := strings.Split(out, "\x00")
	var entries []TreeEntry

	for _, rec := range records {
		if rec == "" {
			continue
		}

		if parts := strings.SplitN(rec, "\t", 2); len(parts) == 2 {
			modeOidStage := parts[0]
			path := parts[1]

			fields := strings.Fields(modeOidStage)
			if len(fields) >= 1 {
				entries = append(entries, TreeEntry{
					Path:   path,
					Mode:   fields[0],
					Cached: true,
				})
			}
		} else {
			entries = append(entries, TreeEntry{
				Path:   rec,
				Mode:   "",
				Cached: false,
			})
		}
	}

	return entries, nil
}

// IgnoredPrefixes lists what .gitignore excludes, with directories collapsed
// so a large node_modules costs one entry rather than thousands.
//
// The entries are not all directories: ignored files come through too, told
// apart only by the trailing slash git puts on a directory.
func (r *Repo) IgnoredPrefixes(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return nil, err
	}

	records := strings.Split(out, "\x00")
	var entries []string

	for _, rec := range records {
		if rec != "" {
			entries = append(entries, rec)
		}
	}

	return entries, nil
}

// Grep searches the working tree's contents. -I skips binaries, --untracked
// covers files not yet added.
//
// git grep exits 1 when nothing matched, which exec reports as an error; that
// is an empty result, not a failure.
func (r *Repo) Grep(ctx context.Context, query string) ([]string, error) {
	out, err := r.run(ctx, "grep", "-n", "-I", "--untracked", "-e", query)

	// git grep exits with code 1 when nothing matched; that's an empty result.
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return []string{}, nil
		}
		return nil, err
	}

	records := strings.Split(strings.TrimSpace(out), "\n")
	var entries []string

	for _, rec := range records {
		if rec != "" {
			entries = append(entries, rec)
		}
	}

	return entries, nil
}
