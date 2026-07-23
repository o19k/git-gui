package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A list pane can be narrowed by a substring typed after `/`. The filter is
// state of the pane rather than a modal, and every read of a list goes through
// the filtered accessors below.

// startFilter opens the filter on the focused list pane for typing.
func (m *Model) startFilter() {
	if m.focus.content() {
		return
	}
	m.filtering = true
}

// endFilter stops typing. Cancelling also clears what was typed.
func (m *Model) endFilter(cancel bool) {
	m.filtering = false
	if cancel {
		m.filter[m.focus] = ""
	}
}

// handleFilterKey serves the focused pane while its filter is being typed.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.endFilter(true)
	case tea.KeyEnter:
		m.endFilter(false)
	case tea.KeyBackspace:
		if s := m.filter[m.focus]; s != "" {
			m.filter[m.focus] = s[:len(s)-1]
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter[m.focus] += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter[m.focus] += " "
		}
	default:
		return m, nil
	}

	// The list under the cursor just changed shape, so the selection and the
	// preview have to follow it.
	m.clampCursors()
	return m, m.refreshPreview()
}

// matches reports whether an entry's text satisfies a filter. An empty filter
// matches everything, and matching is case-insensitive.
func matches(filter, text string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(filter))
}

// --- filtered views of the snapshot ---
//
// Every list read goes through these: a cursor over a filtered list would
// otherwise address the wrong entry of the snapshot.

func (m Model) files() []git.FileChange {
	f := m.filter[PanelFiles]
	if f == "" {
		return m.snap.Files
	}
	out := make([]git.FileChange, 0, len(m.snap.Files))
	for _, e := range m.snap.Files {
		if matches(f, e.Display()) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) branches() []git.Branch {
	f := m.filter[PanelBranches]
	if f == "" {
		return m.snap.Branches
	}
	out := make([]git.Branch, 0, len(m.snap.Branches))
	for _, e := range m.snap.Branches {
		if matches(f, e.Name) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) commits() []git.Commit {
	f := m.filter[PanelCommits]
	if f == "" {
		return m.snap.Commits
	}
	out := make([]git.Commit, 0, len(m.snap.Commits))
	for _, e := range m.snap.Commits {
		if matches(f, e.Subject) || matches(f, e.Short) || matches(f, e.Author) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) stashes() []git.Stash {
	f := m.filter[PanelStash]
	if f == "" {
		return m.snap.Stashes
	}
	out := make([]git.Stash, 0, len(m.snap.Stashes))
	for _, e := range m.snap.Stashes {
		if matches(f, e.Subject) || matches(f, e.Ref) {
			out = append(out, e)
		}
	}
	return out
}

// filterSuffix is what the pane's title shows about its filter: the term while
// it is being typed, with a cursor, or a quiet note that one is still applied.
func (m Model) filterSuffix(p Panel) string {
	if m.filtering && m.focus == p {
		return "  /" + m.filter[p] + "▏"
	}
	if m.filter[p] != "" {
		return "  /" + m.filter[p]
	}
	return ""
}

func filterKeyHints() [][2]string {
	return [][2]string{{"type", "narrow the list"}, {"enter", "keep"}, {"esc", "clear"}}
}
