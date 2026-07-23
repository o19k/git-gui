package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

// PatchLines rebuilds a patch holding only the chosen lines of hunk n, with
// the counts in its header recomputed to match. chosen holds indices into the
// hunk's body.
//
// A line the chooser left out is treated by whether the file the patch will be
// applied to already holds it: one that is there stays, as context, and one
// that is not is left out of the patch entirely. Which side that is depends on
// the direction, so reverse flips the two.
func (f FileDiff) PatchLines(n int, chosen map[int]bool, reverse bool) (string, error) {
	if n < 0 || n >= len(f.Hunks) {
		return "", errors.New("no such hunk")
	}
	hunk := f.Hunks[n]

	oldStart, newStart, ok := parseHunkHeader(hunk.Header)
	if !ok {
		return "", errors.New("cannot read the hunk header: " + hunk.Header)
	}

	// inPreimage is the marker of a line the target file already holds. Staging
	// applies the patch forwards, so that is the old side; unstaging applies it
	// backwards, so it is the new side.
	inPreimage := byte('-')
	if reverse {
		inPreimage = '+'
	}

	var (
		body               []string
		oldCount, newCount int
		picked             bool
		kept               bool // whether the line before this one made it in
	)
	for i, line := range hunk.Lines {
		if line == "" {
			// A stripped trailing space: an empty context line.
			body, kept = append(body, ""), true
			oldCount, newCount = oldCount+1, newCount+1
			continue
		}

		switch marker := line[0]; {
		case marker == '\\':
			// "\ No newline at end of file" belongs to the line above it.
			if kept {
				body = append(body, line)
			}

		case marker != '+' && marker != '-':
			body, kept = append(body, line), true
			oldCount, newCount = oldCount+1, newCount+1

		case chosen[i]:
			body, kept, picked = append(body, line), true, true
			if marker == '-' {
				oldCount++
			} else {
				newCount++
			}

		case marker == inPreimage:
			// Left out, but the target holds it: it has to stay, as context.
			body, kept = append(body, " "+line[1:]), true
			oldCount, newCount = oldCount+1, newCount+1

		default:
			// Left out and the target does not hold it: it never appears.
			kept = false
		}
	}

	if !picked {
		return "", errors.New("no lines selected")
	}

	var b strings.Builder
	for _, line := range f.Preamble {
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
	for _, line := range body {
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

// parseHunkHeader reads the two starting line numbers out of a @@ line.
func parseHunkHeader(header string) (oldStart, newStart int, ok bool) {
	fields := strings.Fields(header)
	if len(fields) < 3 || fields[0] != "@@" {
		return 0, 0, false
	}
	old, oldOK := parseRangeStart(fields[1], '-')
	next, newOK := parseRangeStart(fields[2], '+')
	return old, next, oldOK && newOK
}

// parseRangeStart reads the "-12,7" half of a hunk header. The count is
// optional, and only the start is needed here: the counts are recomputed.
func parseRangeStart(field string, sign byte) (int, bool) {
	if field == "" || field[0] != sign {
		return 0, false
	}
	start, _, _ := strings.Cut(field[1:], ",")
	n, err := strconv.Atoi(start)
	if err != nil {
		return 0, false
	}
	return n, true
}

// StageHunk applies hunk n of a path's unstaged diff to the index.
func (r *Repo) StageHunk(ctx context.Context, path string, n int, opts DiffOpts) error {
	return r.applyHunk(ctx, path, n, nil, false, opts)
}

// UnstageHunk reverses hunk n of a path's staged diff out of the index.
func (r *Repo) UnstageHunk(ctx context.Context, path string, n int, opts DiffOpts) error {
	return r.applyHunk(ctx, path, n, nil, true, opts)
}

// StageLines applies only the chosen lines of hunk n to the index. chosen holds
// indices into the hunk's body.
func (r *Repo) StageLines(ctx context.Context, path string, n int, chosen map[int]bool, opts DiffOpts) error {
	return r.applyHunk(ctx, path, n, chosen, false, opts)
}

// UnstageLines takes only the chosen lines of hunk n back out of the index.
func (r *Repo) UnstageLines(ctx context.Context, path string, n int, chosen map[int]bool, opts DiffOpts) error {
	return r.applyHunk(ctx, path, n, chosen, true, opts)
}

// applyHunk builds a one-hunk patch and feeds it to the index. chosen nil means
// the whole hunk. The diff is re-read here rather than passed in, so the patch
// carries the line numbers git is about to check it against; the options must
// match what the caller saw, or the hunk numbering will not.
func (r *Repo) applyHunk(ctx context.Context, path string, n int, chosen map[int]bool, reverse bool, opts DiffOpts) error {
	raw, err := r.Diff(ctx, path, reverse, opts.Applicable())
	if err != nil {
		return err
	}

	diff := ParseFileDiff(raw)
	patch := ""
	if chosen == nil {
		patch, err = diff.Patch(n)
	} else {
		patch, err = diff.PatchLines(n, chosen, reverse)
	}
	if err != nil {
		return err
	}
	return r.applyCached(ctx, patch, reverse)
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
