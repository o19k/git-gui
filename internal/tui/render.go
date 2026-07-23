package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/theme"
)

// Rounded box-drawing runes.
const (
	topLeft     = "╭"
	topRight    = "╮"
	bottomLeft  = "╰"
	bottomRight = "╯"
	horizontal  = "─"
	vertical    = "│"
)

// renderBox draws one panel: a rounded frame with the title inlaid in the top
// border. Lipgloss has no border-title, so the frame is assembled by hand. w
// and h are outer dimensions; content is fitted to the interior exactly.
func renderBox(title string, lines []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		// Too small to frame, but the space is still the pane's: fill it with
		// exactly h blank rows of w columns so the panes around it keep their
		// places instead of collapsing and tearing the frame.
		return blankBox(w, h)
	}
	inner := w - 2
	innerH := h - 2

	borderColor := lipgloss.Color(theme.Border)
	titleStyle := theme.TitleStyle
	if focused {
		borderColor = lipgloss.Color(theme.BorderFocus)
		titleStyle = theme.TitleFocusStyle
	}
	border := lipgloss.NewStyle().Foreground(borderColor)

	// ╭─ Title ─────╮
	renderedTitle := titleStyle.Render(title)
	fill := inner - lipgloss.Width(renderedTitle) - 1
	if fill < 0 {
		// Too narrow for the title: a plain rule beats overflowing the column.
		renderedTitle, fill = "", inner-1
	}
	top := border.Render(topLeft+horizontal) + renderedTitle +
		border.Render(strings.Repeat(horizontal, max(fill, 0))+topRight)

	side := border.Render(vertical)
	body := make([]string, 0, innerH)
	for i := range innerH {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		body = append(body, side+fitLine(line, inner)+side)
	}

	bottom := border.Render(bottomLeft + strings.Repeat(horizontal, inner) + bottomRight)

	return strings.Join(append(append([]string{top}, body...), bottom), "\n")
}

// blankBox is h rows of w spaces: the placeholder for a pane too small to
// frame, holding the exact rectangle so neighbours do not shift.
func blankBox(w, h int) string {
	if h <= 0 {
		return ""
	}
	row := strings.Repeat(" ", max(w, 0))
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

// fitLine pads or truncates a possibly-styled line to exactly w columns.
// Truncation comes first: Style.Width also enables word wrapping, and a folded
// line would tear the frame below it.
func fitLine(line string, w int) string {
	if w <= 0 {
		return ""
	}
	fitted := lipgloss.NewStyle().MaxWidth(w).Render(line)
	if pad := w - lipgloss.Width(fitted); pad > 0 {
		fitted += strings.Repeat(" ", pad)
	}
	return fitted
}

// window returns the slice of items visible for a viewport of height h, and
// keeps the cursor inside it by adjusting offset. Only the visible slice is
// ever styled.
func window(total, cursor, offset, h int) (start, end, newOffset int) {
	if h <= 0 || total == 0 {
		return 0, 0, 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+h {
		offset = cursor - h + 1
	}
	offset = clamp(offset, 0, max(total-h, 0))
	return offset, min(offset+h, total), offset
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
