package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mouse capture is opt-in (see cmd/git-gui): it costs the terminal's own text
// selection.

const wheelStep = 3

// handleMouse routes an event to the pane under the pointer, not the focused one.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	// Modal: clicking behind the overlay would act on an unseen pane.
	if m.overlay.kind != overlayNone {
		return m, nil
	}

	p, row, ok := m.panelAt(msg.X, msg.Y)
	if !ok {
		// The tab bar, the banner and the footer are not scrollable surfaces.
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		delta := wheelStep
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -wheelStep
		}
		if p.content() {
			m.mainOffset = m.clampMainOffset(m.mainOffset + delta)
			return m, nil
		}
		// Focus follows, since the content pane tracks the selection.
		m.focus = p
		return m.moveCursor(delta)

	case tea.MouseButtonLeft:
		m.focus = p
		if p.content() {
			return m, nil
		}
		if n := m.panelLen(p); n > 0 && row >= 0 {
			// Past the last entry, keep the selection instead of snapping.
			if i := m.offset[p] + row; i < n {
				m.cursor[p] = i
			}
		}
		m.syncOffset(p)
		return m, m.refreshPreview()
	}
	return m, nil
}

// panelAt maps a screen cell to the pane drawn there and the row's index within
// that pane's viewport, walking the same widths the renderer does. Cells on a
// frame report row -1; cells outside the body report ok=false.
func (m Model) panelAt(x, y int) (panel Panel, row int, ok bool) {
	top := 1 + m.bannerHeight()
	h := m.bodyHeight()
	if y < top || y >= top+h {
		return 0, 0, false
	}

	widths := m.paneWidths()
	left := 0
	for i, column := range tabColumns[m.tab] {
		if x < left || x >= left+widths[i] {
			left += widths[i]
			continue
		}
		// Inside the column, walk the stack to find which pane owns this row.
		heights := m.paneHeights(column)
		paneTop := top
		for j, p := range column {
			ph := heights[j]
			if y >= paneTop && y < paneTop+ph {
				row := y - paneTop - 1
				if row >= ph-2 {
					row = -1
				}
				return p, row, true
			}
			paneTop += ph
		}
		return 0, 0, false
	}
	return 0, 0, false
}
