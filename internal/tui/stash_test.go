package tui

import (
	"strings"
	"testing"
)

func TestStashAsksWhatGoesIn(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)

	m, _ = press(t, m, "s")
	if m.overlay.kind != overlayChoice {
		t.Fatalf("s did not ask: overlay kind = %d", m.overlay.kind)
	}
	if len(m.overlay.choices) != 3 {
		t.Errorf("choices = %d, want everything, tracked and staged", len(m.overlay.choices))
	}
}

// Marking files is how every other action in this panel is aimed, so the stash
// answers to it too.
func TestStashOffersTheMarkedFilesWhenThereAreSome(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m, _ = press(t, m, "m") // mark the selected file

	m, _ = press(t, m, "s")
	if len(m.overlay.choices) != 4 {
		t.Fatalf("choices = %d, want the marked files alongside the three", len(m.overlay.choices))
	}
	if !strings.Contains(m.overlay.choices[3].label, "marked") {
		t.Errorf("the fourth choice is %q", m.overlay.choices[3].label)
	}
}

func TestStashKindThenAsksForTheMessage(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(stashKindMsg{kind: stashStaged})
	m = next.(Model)

	if m.overlay.kind != overlayInput {
		t.Fatalf("the message was not asked for: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.title, "Stash message") {
		t.Errorf("prompt title = %q", m.overlay.title)
	}
}
