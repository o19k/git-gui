package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func choiceFixture(t *testing.T, picked *string) Model {
	t.Helper()
	m := fixture(t)
	mark := func(name string) func() tea.Cmd {
		return func() tea.Cmd {
			*picked = name
			return nil
		}
	}
	m.askChoice("Pick one", "Two ways forward.", []choice{
		{label: "First", hint: "the one on top", action: mark("first")},
		{label: "Second", hint: "the one below", busy: "working…", action: mark("second")},
	})
	return m
}

func TestAChoiceIsDrawnWithItsOptionsNumbered(t *testing.T) {
	var picked string
	view := choiceFixture(t, &picked).View()

	for _, want := range []string{"Pick one", "Two ways forward.", "1 First", "2 Second", "the one on top"} {
		if !strings.Contains(view, want) {
			t.Errorf("the offer is missing %q", want)
		}
	}
}

func TestANumberKeyPicksItsOptionOutright(t *testing.T) {
	var picked string
	m := choiceFixture(t, &picked)

	m, _ = press(t, m, "2")

	if picked != "second" {
		t.Errorf("picked %q, want the second option", picked)
	}
	if m.overlay.kind != overlayNone {
		t.Error("the offer stayed on screen after being answered")
	}
	if m.busy != "working…" {
		t.Errorf("busy = %q, want the option's own note", m.busy)
	}
}

func TestMovingAndAcceptingPicksTheHighlightedOption(t *testing.T) {
	var picked string
	m := choiceFixture(t, &picked)

	m = key(t, m, "j")
	m, _ = press(t, m, "enter")

	if picked != "second" {
		t.Errorf("picked %q, want the option the cursor was on", picked)
	}
}

func TestTheCursorStopsAtTheEndsOfAChoice(t *testing.T) {
	var picked string
	m := choiceFixture(t, &picked)

	for range 5 {
		m = key(t, m, "j")
	}
	if m.overlay.cursor != 1 {
		t.Errorf("cursor = %d, want it held at the last option", m.overlay.cursor)
	}
	for range 5 {
		m = key(t, m, "k")
	}
	if m.overlay.cursor != 0 {
		t.Errorf("cursor = %d, want it held at the first option", m.overlay.cursor)
	}
}

func TestEscapeLeavesAChoiceUnanswered(t *testing.T) {
	var picked string
	m := choiceFixture(t, &picked)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)

	if picked != "" {
		t.Errorf("escape ran %q", picked)
	}
	if m.overlay.kind != overlayNone {
		t.Error("escape left the offer on screen")
	}
}

func textFixture(t *testing.T) Model {
	t.Helper()
	m := fixture(t)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line " + string(rune('a'+i%26))
	}
	m.showText("Long", lines)
	return m
}

func TestTextScrollsAndStopsAtBothEnds(t *testing.T) {
	m := textFixture(t)

	m = key(t, m, "G")
	last := m.overlay.offset
	if last == 0 {
		t.Fatal("G did not scroll to the end")
	}
	m = key(t, m, "j")
	if m.overlay.offset != last {
		t.Errorf("offset = %d, want it held at the end", m.overlay.offset)
	}

	m = key(t, m, "g")
	if m.overlay.offset != 0 {
		t.Errorf("offset = %d, want the top", m.overlay.offset)
	}
	m = key(t, m, "k")
	if m.overlay.offset != 0 {
		t.Errorf("offset = %d, want it held at the top", m.overlay.offset)
	}
}

func TestTextClosesOnEscape(t *testing.T) {
	m := textFixture(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if after := next.(Model); after.overlay.kind != overlayNone {
		t.Error("escape did not close the window")
	}
}

func TestAConfirmCanListWhatItIsAbout(t *testing.T) {
	m := fixture(t)
	m.askConfirm("Push", "Publish 2 commits?", false, func() tea.Cmd { return nil })
	m.overlay.extra = []string{"aaa1111 first", "bbb2222 second"}

	view := m.View()
	for _, want := range []string{"Publish 2 commits?", "aaa1111 first", "bbb2222 second"} {
		if !strings.Contains(view, want) {
			t.Errorf("the confirm is missing %q", want)
		}
	}
}
