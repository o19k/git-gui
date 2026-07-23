package git

import (
	"context"
	"errors"
	"strings"
)

// Hunk is one @@ block of a unified diff.
type Hunk struct {
	Header string   // the @@ -a,b +c,d @@ line
	Lines  []string // body lines, each still carrying its +/-/space marker
}

// Added and Removed count the lines this hunk introduces and takes away.
func (h Hunk) Added() int   { return countPrefix(h.Lines, '+') }
func (h Hunk) Removed() int { return countPrefix(h.Lines, '-') }

func countPrefix(lines []string, prefix byte) int {
	n := 0
	for _, line := range lines {
		if line != "" && line[0] == prefix {
			n++
		}
	}
	return n
}

// FileDiff is a parsed single-file patch: the preamble git needs to identify
// the file, plus the hunks that can be applied independently.
type FileDiff struct {
	Preamble []string // diff --git, index, ---, +++ …
	Hunks    []Hunk
}

// ParseFileDiff splits the output of `git diff -- <path>` into hunks.
func ParseFileDiff(patch string) FileDiff {
	var diff FileDiff
	var current *Hunk

	for _, line := range strings.Split(strings.TrimRight(patch, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			diff.Hunks = append(diff.Hunks, Hunk{Header: line})
			current = &diff.Hunks[len(diff.Hunks)-1]
		case current == nil:
			diff.Preamble = append(diff.Preamble, line)
		default:
			current.Lines = append(current.Lines, line)
		}
	}
	return diff
}

// Patch rebuilds a patch containing only hunk n, ready for `git apply`. One at
// a time, so each patch carries the line numbers it was generated with and no
// offset arithmetic is needed.
func (f FileDiff) Patch(n int) (string, error) {
	if n < 0 || n >= len(f.Hunks) {
		return "", errors.New("no such hunk")
	}
	var b strings.Builder
	for _, line := range f.Preamble {
		b.WriteString(line + "\n")
	}
	b.WriteString(f.Hunks[n].Header + "\n")
	for _, line := range f.Hunks[n].Lines {
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

// StageHunk applies hunk n of a path's unstaged diff to the index.
func (r *Repo) StageHunk(ctx context.Context, path string, n int) error {
	raw, err := r.Diff(ctx, path, false)
	if err != nil {
		return err
	}
	patch, err := ParseFileDiff(raw).Patch(n)
	if err != nil {
		return err
	}
	return r.applyCached(ctx, patch, false)
}

// UnstageHunk reverses hunk n of a path's staged diff out of the index.
func (r *Repo) UnstageHunk(ctx context.Context, path string, n int) error {
	raw, err := r.Diff(ctx, path, true)
	if err != nil {
		return err
	}
	patch, err := ParseFileDiff(raw).Patch(n)
	if err != nil {
		return err
	}
	return r.applyCached(ctx, patch, true)
}

// applyCached feeds a patch to `git apply --cached` on stdin. --cached, not
// --index: only the index moves, never the file being edited. Context lines are
// kept, so git verifies the hunk still fits before changing anything.
func (r *Repo) applyCached(ctx context.Context, patch string, reverse bool) error {
	args := []string{"apply", "--cached"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")
	_, err := r.runStdin(ctx, patch, args...)
	return err
}
