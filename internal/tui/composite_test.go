package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/o19k/git-gui/internal/git"
)

func TestCompositeWithModalOpenShowsBothTitles(t *testing.T) {
	m := fixture(t)
	// Open a confirm modal over the Local Changes tab.
	m.askConfirm("Delete file?", "This cannot be undone.", true, nil)

	view := m.View()

	// Should contain both the modal title and a pane title from behind it.
	if !strings.Contains(view, "Delete file?") {
		t.Error("modal title not visible")
	}
	if !strings.Contains(view, "Changes") {
		t.Error("pane title from frame behind modal not visible")
	}
}

func TestCompositeFrameExactWidth(t *testing.T) {
	m := fixture(t)
	// Add styled content with CJK characters.
	m.snap.Files = []git.FileChange{
		{Index: 'M', Work: '.', Path: "你好世界.go"},
		{Index: '.', Work: 'M', Path: "test_combining_é.go"},
	}

	next, _ := m.Update(snapshotMsg(m.snap))
	m = next.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	if len(lines) != m.height {
		t.Errorf("view has %d lines, want %d", len(lines), m.height)
	}

	// Every line must be exactly m.width columns.
	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w != m.width {
			t.Errorf("line %d: width %d, want %d", i, w, m.width)
		}
		// Check for lone escape sequences or unterminated CSI.
		if strings.Contains(line, "\x1b[") && !strings.Contains(line, "m") {
			t.Errorf("line %d may have unterminated CSI", i)
		}
	}
}

func TestCompositeBaseBehindFloatUnchanged(t *testing.T) {
	// The float covers the first five cells; everything past them is the base's
	// own bytes, styling included. A splice that rebuilt the right-hand segment
	// would show up here as a colour that no longer matches.
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("200")).Render("RED")
	base := "Line 1 " + styled + " end"
	const float = "FLOAT"

	result := composite(base, float, 0, 0)

	// The seam carries a reset of its own, so what has to survive unchanged is
	// the visible text, not the exact bytes.
	tail := ansi.Strip(ansi.TruncateLeft(base, len(float), ""))
	if got := ansi.Strip(ansi.TruncateLeft(result, len(float), "")); got != tail {
		t.Errorf("right of the float reads %q, want the base's own %q", got, tail)
	}
	if !strings.Contains(result, reset+float+reset) {
		t.Error("the float is not fenced by resets, so styling will bleed across the seam")
	}
	if got, want := ansi.StringWidth(result), ansi.StringWidth(base); got != want {
		t.Errorf("the splice changed the line width to %d, want %d", got, want)
	}
}

func TestCompositeClampsFloatToTerminal(t *testing.T) {
	m := fixture(t)
	m.width, m.height = 60, 10

	// Create a float larger than terminal.
	largeFloat := strings.Repeat("X", 100) + "\n" + strings.Repeat("Y", 100)

	frame := m.frameOnly()
	result := composite(frame, largeFloat, 0, 0)

	lines := strings.Split(result, "\n")
	if len(lines) != m.height {
		t.Errorf("result has %d lines, want %d (terminal height)", len(lines), m.height)
	}

	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w != m.width {
			t.Errorf("line %d has width %d, want %d (terminal width)", i, w, m.width)
		}
	}
}

func TestCompositeSmallTerminalStillRendersFrame(t *testing.T) {
	m := fixture(t)
	m.width, m.height = 4, 2

	frame := m.frameOnly()
	result := composite(frame, "X", 0, 0)

	// Should not be empty.
	if result == "" {
		t.Error("result is empty for small terminal")
	}

	lines := strings.Split(result, "\n")
	if len(lines) == 0 {
		t.Error("result has no lines")
	}
}

func TestOverlayListFiltersAsTyped(t *testing.T) {
	m := fixture(t)
	items := []listItem{
		{label: "apple", value: "apple_val"},
		{label: "application", value: "app_val"},
		{label: "banana", value: "banana_val"},
		{label: "band", value: "band_val"},
	}

	m.askList("Pick one", items, func(string) tea.Cmd { return nil })

	// Filter for "app".
	m.overlay.query = "app"
	matched := m.listMatches()

	if len(matched) != 2 {
		t.Errorf("filtered to %d items, want 2", len(matched))
	}
	for _, item := range matched {
		if !strings.Contains(item.label, "app") {
			t.Errorf("matched item %q does not contain filter", item.label)
		}
	}
}

func TestOverlayListCursorMovement(t *testing.T) {
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
		t.Errorf("after ctrl+n, cursor is %d, want 1", m.overlay.cursor)
	}

	next, _ = m.handleListKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)
	if m.overlay.cursor != 0 {
		t.Errorf("after ctrl+p, cursor is %d, want 0", m.overlay.cursor)
	}
}

// A printable key narrows the list; it does not move through it. A picker that
// moved the cursor when a name was typed would be unusable for its one job.
func TestOverlayListTypingDoesNotMoveTheCursor(t *testing.T) {
	m := fixture(t)
	m.askList("Pick", []listItem{
		{label: "jam", value: "jam"},
		{label: "jar", value: "jar"},
	}, func(string) tea.Cmd { return nil })

	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)

	if m.overlay.cursor != 0 {
		t.Errorf("typing moved the cursor to %d", m.overlay.cursor)
	}
	if m.overlay.query != "j" {
		t.Errorf("query is %q, want it to have taken the keystroke", m.overlay.query)
	}
}

func TestOverlayListEnterReturnsSelection(t *testing.T) {
	m := fixture(t)
	selected := ""
	items := []listItem{
		{label: "a", value: "value_a"},
		{label: "b", value: "value_b"},
	}

	m.askList("Pick", items, func(s string) tea.Cmd {
		selected = s
		return func() tea.Msg { return nil }
	})

	m.overlay.cursor = 1
	next, cmd := m.handleListKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	// The overlay should be closed.
	if m.overlay.kind != overlayNone {
		t.Errorf("overlay still open after enter")
	}

	// The action carries the item's value, not its label, and the only way to
	// see which it got is to run the command it returned.
	if cmd == nil {
		t.Fatal("no command returned")
	}
	cmd()
	if selected != "value_b" {
		t.Errorf("the action received %q, want the cursor's value %q", selected, "value_b")
	}
}

func TestOverlayListEscCancels(t *testing.T) {
	m := fixture(t)
	items := []listItem{{label: "a", value: "a"}}

	m.askList("Pick", items, func(string) tea.Cmd { return nil })

	next, _ := m.handleListKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)

	if m.overlay.kind != overlayNone {
		t.Error("overlay still open after esc")
	}
}

func TestOverlayListRenders10kItemsInBoundedBox(t *testing.T) {
	m := fixture(t)
	m.width, m.height = 100, 40

	// Create 10,000 items.
	items := make([]listItem, 10000)
	for i := 0; i < 10000; i++ {
		items[i] = listItem{label: "item_" + string(rune('0'+i%10)), value: "val"}
	}

	m.askList("Search", items, func(string) tea.Cmd { return nil })

	// The overlay should render without creating a box taller than the terminal.
	listLines := m.listLines(m.width - 8)

	// Should be constrained by listHeight.
	maxHeight := m.listHeight()
	if len(listLines) > maxHeight {
		t.Errorf("list has %d lines, max height is %d", len(listLines), maxHeight)
	}
}

func TestOverlayListGoldenString(t *testing.T) {
	m := fixture(t)
	m.width, m.height = 80, 30

	items := []listItem{
		{label: "file1.go", value: "file1.go"},
		{label: "file2.go", value: "file2.go"},
		{label: "test.rs", value: "test.rs"},
	}

	m.askList("Open file", items, func(string) tea.Cmd { return nil })

	// Manually trigger a view render.
	view := m.View()

	// Should contain the modal title and the frame behind it.
	if !strings.Contains(view, "Open file") {
		t.Error("modal title missing")
	}
	if !strings.Contains(view, "Changes") {
		t.Error("frame title missing")
	}

	// Check basic structure.
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Errorf("view has %d lines, want %d", len(lines), m.height)
	}

	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w != m.width {
			t.Errorf("line %d has width %d, want %d", i, w, m.width)
		}
	}
}
