package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// overlayList is the picker shape: a query buffer, a cursor, and a window over
// the matches. overlayChoice sizes its box to every row, and overlayText has no
// cursor, so neither serves for thousands of paths.

// listItem is one row of a picker. label is what is shown and filtered on;
// value is what the action receives, which for a grep hit is not the same
// thing.
type listItem struct {
	label string
	value string
}

// askList puts a filterable, scrolling picker in front of the panels. The
// action receives the chosen item's value, or is never called if cancelled.
func (m *Model) askList(title string, items []listItem, action func(string) tea.Cmd) {
	m.overlay = overlay{kind: overlayList, title: title, items: items, action: action}
}

// listMatches is the items the query selects, in order.
func (m Model) listMatches() []listItem {
	if m.overlay.query == "" {
		return m.overlay.items
	}
	var filtered []listItem
	for _, item := range m.overlay.items {
		if matches(m.overlay.query, item.label) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// listHeight is the rows a picker can use, leaving the frame, the query line
// and the key line room.
func (m Model) listHeight() int { return max(m.height-8, 3) }

// handleListKey drives the picker: typing filters, ctrl+n and ctrl+p move, and
// enter takes the selection.
func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matched := m.listMatches()
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.overlay = overlay{}
		return m, nil
	case tea.KeyEnter:
		if m.overlay.cursor >= 0 && m.overlay.cursor < len(matched) {
			action := m.overlay.action
			value := matched[m.overlay.cursor].value
			m.overlay = overlay{}
			return m, action(value)
		}
		return m, nil
	case tea.KeyBackspace:
		if s := m.overlay.query; s != "" {
			m.overlay.query = s[:len(s)-1]
		}
		m.overlay.cursor = clamp(m.overlay.cursor, 0, max(len(matched)-1, 0))
		return m, nil
	case tea.KeyCtrlU:
		m.overlay.query = ""
		m.overlay.cursor = clamp(m.overlay.cursor, 0, max(len(matched)-1, 0))
		return m, nil
	case tea.KeySpace:
		m.overlay.query += " "
		m.overlay.cursor = clamp(m.overlay.cursor, 0, max(len(matched)-1, 0))
		return m, nil
	case tea.KeyRunes:
		m.overlay.query += string(msg.Runes)
		m.overlay.cursor = clamp(m.overlay.cursor, 0, max(len(matched)-1, 0))
		return m, nil
	}

	// Cursor movement. Not j and k: every printable key belongs to the query.
	switch msg.String() {
	case "ctrl+n", "down":
		m.overlay.cursor = clamp(m.overlay.cursor+1, 0, max(len(matched)-1, 0))
		return m, nil
	case "ctrl+p", "up":
		m.overlay.cursor = clamp(m.overlay.cursor-1, 0, max(len(matched)-1, 0))
		return m, nil
	}

	return m, nil
}

// listLines renders the picker's body, showing only the visible window.
func (m Model) listLines(width int) []string {
	matched := m.listMatches()
	height := m.listHeight()

	if len(matched) == 0 {
		return []string{fitLine("no matches", width)}
	}

	start, end, _ := window(len(matched), m.overlay.cursor, m.overlay.offset, height)

	var lines []string
	for i := start; i < end; i++ {
		item := matched[i]
		marker := "  "
		if i == m.overlay.cursor {
			marker = " ▶"
		}
		line := marker + " " + item.label
		lines = append(lines, fitLine(line, width))
	}
	return lines
}
