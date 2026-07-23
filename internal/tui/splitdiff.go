package tui

import (
	"strings"

	"github.com/o19k/git-gui/internal/theme"
)

// The side-by-side view is derived from the unified text rather than asked of
// git separately, so both views are always of the same patch.

const (
	splitDivider = "│"

	// minSplitSide is the narrowest column worth showing; below it the two
	// halves hold a few characters each and the unified view is used instead.
	minSplitSide = 12
)

// splitRow is one rendered line of the two-column view. A row with only one
// side filled is a pure removal or addition; a header spans both.
type splitRow struct {
	left, right string
	header      string // set instead of left/right for @@ and file headers
}

// splitRows pairs a unified patch's removals with the additions that replaced
// them. git emits them in runs — every `-` of a change, then every `+` — so a
// run is buffered and the two are zipped, putting a rewritten line opposite
// its old self.
func splitRows(patch string) []splitRow {
	if strings.TrimRight(patch, "\n") == "" {
		// Splitting "" yields one empty field, which would render as a row.
		return nil
	}

	var rows []splitRow
	var removed, added []string

	flush := func() {
		for i := 0; i < len(removed) || i < len(added); i++ {
			var row splitRow
			if i < len(removed) {
				row.left = removed[i]
			}
			if i < len(added) {
				row.right = added[i]
			}
			rows = append(rows, row)
		}
		removed, added = nil, nil
	}

	for _, line := range strings.Split(strings.TrimRight(patch, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			// File headers, not content: they only look like +/- lines.
			flush()
			rows = append(rows, splitRow{header: line})
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line)
		case strings.HasPrefix(line, "+"):
			added = append(added, line)
		case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "diff "),
			strings.HasPrefix(line, "index "), strings.HasPrefix(line, "new "),
			strings.HasPrefix(line, "deleted "), strings.HasPrefix(line, "similarity "),
			strings.HasPrefix(line, "rename "):
			flush()
			rows = append(rows, splitRow{header: line})
		default:
			// Context, and anything else, sits unchanged on both sides.
			flush()
			rows = append(rows, splitRow{left: line, right: line})
		}
	}
	flush()
	return rows
}

// splitDiffLines renders the window of the side-by-side view that fits in a
// pane of height rows and width columns.
func splitDiffLines(patch string, offset, height, width int) []string {
	rows := splitRows(patch)
	start, end, _ := window(len(rows), offset, offset, height)

	leftW := (width - 1) / 2
	rightW := width - 1 - leftW
	if leftW < minSplitSide || rightW < minSplitSide {
		// Two columns this narrow show nothing.
		return diffLines(patch, offset, height)
	}

	divider := theme.DimStyle.Render(splitDivider)
	out := make([]string, 0, end-start)
	for _, r := range rows[start:end] {
		if r.header != "" {
			out = append(out, theme.DiffLineStyle(r.header).Render(r.header))
			continue
		}
		left := fitLine(theme.DiffLineStyle(r.left).Render(r.left), leftW)
		right := fitLine(theme.DiffLineStyle(r.right).Render(r.right), rightW)
		out = append(out, left+divider+right)
	}
	return out
}

// splitLineCount is how many rows the side-by-side view has, which is what
// bounds its scrolling.
func splitLineCount(patch string) int { return len(splitRows(patch)) }
