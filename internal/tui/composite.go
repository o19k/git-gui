package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A modal is drawn over the frame, not instead of it, so the list a confirm
// names a file from stays on screen.
//
// The splice has to be ANSI-aware: both strings are already styled, so cutting
// by byte or by rune lands inside an escape sequence. Against x/ansi v0.10.1,
// three things decide whether the result is right:
//
//   - Both cuts leave SGR state open across the boundary, so a reset goes on
//     each side of the float. Otherwise the base's colour paints the float's
//     border and the float's paints the rest of the row.
//   - TruncateLeft keeps the colour of the text it keeps — it over-preserves
//     rather than dropping the opening escape, so nothing has to be rebuilt.
//   - Neither cut will split a double-width cell, so a cut lands short on the
//     left and long on the right. The deficit is padded to the exact column;
//     the float's x is a request and the padding is what makes it true.

// reset closes whatever styling was open at a seam, so neither side paints the
// other.
const reset = "\x1b[0m"

// composite draws float over base at (x, y), in cells. Every line keeps the
// width it had, or the layout below would tear.
func composite(base, float string, x, y int) string {
	if float == "" {
		return base
	}
	baseLines := strings.Split(base, "\n")
	floatLines := strings.Split(strings.TrimRight(float, "\n"), "\n")

	x = max(x, 0)
	y = clamp(y, 0, max(len(baseLines)-1, 0))

	for i, line := range floatLines {
		row := y + i
		if row >= len(baseLines) {
			// Taller than what is left of the frame: the rest is off-screen.
			break
		}
		baseLines[row] = spliceLine(baseLines[row], line, x)
	}
	return strings.Join(baseLines, "\n")
}

// spliceLine draws overlay into base starting at column x, returning a line of
// exactly base's width.
func spliceLine(base, overlay string, x int) string {
	baseW := ansi.StringWidth(base)
	if overlay == "" || x >= baseW {
		return base
	}

	// The left segment comes up a cell short when x falls inside a double-width
	// rune, the cut refusing to split it. Padding realises the requested
	// column.
	left := padTo(ansi.Truncate(base, x, ""), x)

	// What is left of the row is all the overlay can have.
	room := baseW - x
	if ansi.StringWidth(overlay) >= room {
		return left + reset + padTo(overlay, room)
	}

	overlayW := ansi.StringWidth(overlay)
	// The right segment comes up a cell long for the same reason. Skipping one
	// more cell and padding the gap keeps the total exact.
	right := ansi.TruncateLeft(base, x+overlayW, "")
	if ansi.StringWidth(right) > room-overlayW {
		right = " " + ansi.TruncateLeft(base, x+overlayW+1, "")
	}

	return left + reset + overlay + reset + padTo(right, room-overlayW)
}

// padTo returns s at exactly w cells, padding with spaces or cutting to fit.
func padTo(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width := ansi.StringWidth(s); width > w {
		s = ansi.Truncate(s, w, "")
		width = ansi.StringWidth(s)
		return s + strings.Repeat(" ", w-width)
	} else if width < w {
		return s + strings.Repeat(" ", w-width)
	}
	return s
}
