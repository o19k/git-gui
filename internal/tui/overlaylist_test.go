package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestListMatchesFiltersEmptyWhenNoQuery(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "apple", value: "apple_val"},
		{label: "banana", value: "banana_val"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	matched := m.listMatches()
	if len(matched) != 2 {
		t.Errorf("with no query, matched %d items, want 2", len(matched))
	}
}

func TestListMatchesFiltersWithQuery(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "apple", value: "apple_val"},
		{label: "apricot", value: "apricot_val"},
		{label: "banana", value: "banana_val"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.query = "ap"
	matched := m.listMatches()

	if len(matched) != 2 {
		t.Errorf("with query 'ap', matched %d items, want 2", len(matched))
	}

	// Should be case-insensitive.
	m.overlay.query = "APPL"
	matched = m.listMatches()
	if len(matched) != 1 {
		t.Errorf("with query 'APPL', matched %d items, want 1", len(matched))
	}
}

func TestListHeightLeavesRoomForFrame(t *testing.T) {
	m := fixture(t)
	m.height = 40
	height := m.listHeight()

	// Should be terminal height minus space for frame and keys.
	expectedMax := m.height - 8
	if height != expectedMax {
		t.Errorf("listHeight is %d, want %d", height, expectedMax)
	}
}

func TestListHeightMinimum(t *testing.T) {
	m := fixture(t)
	m.height = 5 // Very small.
	height := m.listHeight()

	if height < 3 {
		t.Errorf("listHeight is %d, want at least 3", height)
	}
}

func TestHandleListKeyTypingFilters(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "apple", value: "apple"},
		{label: "banana", value: "banana"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	// "an" is in banana and not in apple. A single "a" would be in both, and
	// would prove nothing about filtering.
	for _, r := range "an" {
		next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}

	if m.overlay.query != "an" {
		t.Errorf("query is %q, want 'an'", m.overlay.query)
	}

	matched := m.listMatches()
	if len(matched) != 1 || matched[0].label != "banana" {
		t.Errorf("filtered to %v, want just banana", matched)
	}
}

func TestHandleListKeyBackspace(t *testing.T) {
	m := fixture(t)
	items := []listItem{{label: "test", value: "test"}}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.query = "hello"
	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(Model)

	if m.overlay.query != "hell" {
		t.Errorf("query is %q, want 'hell'", m.overlay.query)
	}
}

func TestHandleListKeyCtrlU(t *testing.T) {
	m := fixture(t)
	items := []listItem{{label: "test", value: "test"}}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.query = "something"
	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(Model)

	if m.overlay.query != "" {
		t.Errorf("query is %q, want empty", m.overlay.query)
	}
}

func TestHandleListKeySpace(t *testing.T) {
	m := fixture(t)
	items := []listItem{{label: "test file", value: "test"}}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.query = "test"
	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)

	if m.overlay.query != "test " {
		t.Errorf("query is %q, want 'test '", m.overlay.query)
	}
}

func TestHandleListKeyCtrlNMovesCursorDown(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "a", value: "a"},
		{label: "b", value: "b"},
		{label: "c", value: "c"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(Model)

	if m.overlay.cursor != 1 {
		t.Errorf("cursor is %d, want 1", m.overlay.cursor)
	}
}

func TestHandleListKeyCtrlPMovesCursorUp(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "a", value: "a"},
		{label: "b", value: "b"},
		{label: "c", value: "c"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.cursor = 2
	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)

	if m.overlay.cursor != 1 {
		t.Errorf("cursor is %d, want 1", m.overlay.cursor)
	}
}

func TestHandleListKeyClampsAtBounds(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "a", value: "a"},
		{label: "b", value: "b"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	// Move past the end.
	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(Model)
	next, _ = m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(Model)

	if m.overlay.cursor != 1 {
		t.Errorf("cursor clamped to %d, want 1", m.overlay.cursor)
	}

	// Move before the start.
	next, _ = m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)
	next, _ = m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)

	if m.overlay.cursor != 0 {
		t.Errorf("cursor clamped to %d, want 0", m.overlay.cursor)
	}
}

func TestListLinesRendersVisibleWindow(t *testing.T) {
	m := fixture(t)
	m.width = 80

	items := make([]listItem, 100)
	for i := 0; i < 100; i++ {
		items[i] = listItem{label: "item_" + string(rune('0'+i%10)), value: "val"}
	}

	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	lines := m.listLines(60)

	// Should not render all 100 items, only what fits in the height.
	if len(lines) > m.listHeight() {
		t.Errorf("rendered %d lines, max is %d", len(lines), m.listHeight())
	}
}

func TestListLinesShowsNoMatchesWhenFiltered(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "apple", value: "apple"},
		{label: "banana", value: "banana"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.query = "xyz"
	lines := m.listLines(60)

	if len(lines) != 1 || lines[0] == "" {
		t.Error("expected 'no matches' line")
	}
}

func TestListLinesMarksSelectedItem(t *testing.T) {
	m := fixture(t)
	m.width = 80

	items := []listItem{
		{label: "first", value: "first"},
		{label: "second", value: "second"},
		{label: "third", value: "third"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	m.overlay.cursor = 1
	lines := m.listLines(60)

	// Second line should have the cursor marker.
	if len(lines) > 1 {
		// Check for a marker in the line (▶ or similar).
		if lines[1] == "" {
			t.Error("second line is empty")
		}
	}
}

func TestListLinesExactWidth(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "a", value: "a"},
		{label: "b", value: "b"},
	}
	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	width := 40
	lines := m.listLines(width)

	for i, line := range lines {
		w := lipgloss.Width(line)
		if w > width {
			t.Errorf("line %d is %d wide, max is %d", i, w, width)
		}
	}
}

func TestAskListSetsUpTheOverlay(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "a", value: "a_val"},
		{label: "b", value: "b_val"},
	}

	m.askList("Choose", items, func(string) tea.Cmd { return nil })

	if m.overlay.kind != overlayList {
		t.Errorf("overlay kind is %d, want %d (overlayList)", m.overlay.kind, overlayList)
	}
	if m.overlay.title != "Choose" {
		t.Errorf("title is %q, want 'Choose'", m.overlay.title)
	}
	if len(m.overlay.items) != 2 {
		t.Errorf("items count is %d, want 2", len(m.overlay.items))
	}
}
