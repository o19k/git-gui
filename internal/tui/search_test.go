package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

func TestSearchAsksWhatToLookFor(t *testing.T) {
	m, _ := press(t, fixture(t), "S")

	if m.overlay.kind != overlayChoice {
		t.Fatalf("S did not ask: overlay kind = %d", m.overlay.kind)
	}
	// Three things git can answer, and no Clear until there is something to
	// clear.
	if len(m.overlay.choices) != 3 {
		t.Errorf("choices = %d, want message, author and content", len(m.overlay.choices))
	}
}

func TestSearchSetsTheQueryAndSaysSoInTheTitle(t *testing.T) {
	m := fixture(t)

	next, _ := m.Update(searchSetMsg{field: searchMessage, value: "needle"})
	m = next.(Model)

	if m.logQuery.Message != "needle" {
		t.Fatalf("query = %+v", m.logQuery)
	}
	if m.tab != TabLog {
		t.Error("the answer is in the Log tab, so that is where the search should land")
	}
	if m.cursor[PanelCommits] != 0 {
		t.Error("the cursor should start at the top of a list of another shape")
	}

	// A short list must never be mistaken for a short history.
	if title := m.panelTitle(PanelCommits); !strings.Contains(title, "needle") {
		t.Errorf("the panel title does not name the query: %q", title)
	}
}

func TestSearchOffersToClearOnlyWhenSomethingIsSet(t *testing.T) {
	m := fixture(t)
	m.logQuery = git.LogQuery{Author: "ada"}

	m, _ = press(t, m, "S")
	if len(m.overlay.choices) != 4 {
		t.Fatalf("choices = %d, want a Clear alongside the three", len(m.overlay.choices))
	}

	next, _ := m.Update(searchFieldMsg{field: searchClear})
	m = next.(Model)

	if !m.logQuery.Empty() {
		t.Errorf("clearing left %+v", m.logQuery)
	}
}

func TestSearchPromptIsPrefilledWithTheQueryItReplaces(t *testing.T) {
	m := fixture(t)
	m.logQuery = git.LogQuery{Message: "half typed"}

	next, _ := m.Update(searchFieldMsg{field: searchMessage})
	m = next.(Model)

	if m.overlay.kind != overlayInput || m.overlay.value != "half typed" {
		t.Errorf("prompt = %q kind = %d", m.overlay.value, m.overlay.kind)
	}
}
