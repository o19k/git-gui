package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// An annotation answers "who wrote this line" and immediately raises "why", and
// the only place the why is written down is the commit. So the gutter is
// walkable: one key opens the commit, another blames the file as its parent
// held it, which is how a line survives the reformat that hid it.

// handleBlameKey serves the keys that mean something only while annotations are
// on. Movement stays with the ordinary scroll: the cursor follows it.
func (m Model) handleBlameKey(key string) (tea.Model, tea.Cmd, bool) {
	// Only while the pane holding the annotations has the focus. With the file
	// list focused these keys are its own: j and k move between files, and the
	// annotations follow that selection.
	//
	// Not conditional on there being any lines yet: a walk to a parent clears
	// them while the next read is in flight, and the way back has to work in
	// that gap.
	if !m.focus.content() {
		return m, nil, false
	}

	switch key {
	case "j", "down":
		m.moveBlame(1)
		return m, nil, true

	case "k", "up":
		m.moveBlame(-1)
		return m, nil, true

	case "enter":
		return m.openBlameCommit()

	case "<":
		return m.blameParent()

	case ">":
		if m.blameRev == "" {
			m.status = "already annotating the working copy"
			return m, nil, true
		}
		m.blameRev = ""
		m.blameLines, m.blameStyled, m.blameCursor = nil, nil, 0
		m.mainOffset = 0
		return m, readBlame(m.ctx, m.repo, m.blamePath, "", currentSyntax()), true
	}
	return m, nil, false
}

// moveBlame walks the annotated file, scrolling only as far as it has to.
func (m *Model) moveBlame(delta int) {
	m.blameCursor = clamp(m.blameCursor+delta, 0, max(len(m.blameLines)-1, 0))

	height := m.mainHeight()
	switch {
	case m.blameCursor < m.mainOffset:
		m.mainOffset = m.clampMainOffset(m.blameCursor)
	case m.blameCursor >= m.mainOffset+height:
		m.mainOffset = m.clampMainOffset(m.blameCursor - height + 1)
	}
}

// selectedBlame is the line the annotation keys act on.
func (m Model) selectedBlame() (string, bool) {
	if m.blameCursor < 0 || m.blameCursor >= len(m.blameLines) {
		return "", false
	}
	return m.blameLines[m.blameCursor].Short, true
}

// openBlameCommit shows the commit that last touched the line, in the tab that
// can read it. A commit older than the list the Log holds cannot be selected
// there, and saying so beats jumping somewhere arbitrary.
func (m Model) openBlameCommit() (tea.Model, tea.Cmd, bool) {
	short, ok := m.selectedBlame()
	if !ok {
		return m, nil, true
	}

	at := -1
	for i, commit := range m.snap.Commits {
		if strings.HasPrefix(commit.SHA, short) || commit.Short == short {
			at = i
			break
		}
	}
	if at < 0 {
		m.status = short + " is not in the commits this tab holds — raise gitgui.loglimit or search for it with S"
		return m, nil, true
	}

	// Annotations and a commit's patch are both the content pane.
	m.blameOn, m.blamePath, m.blameLines, m.blameStyled = false, "", nil, nil
	m.blameRev, m.blameCursor = "", 0

	next, cmd := m.openTab(TabLog)
	model := next.(Model)
	model.focus = PanelCommits
	model.cursor[PanelCommits] = at
	model.syncOffset(PanelCommits)

	preview := model.refreshPreview()
	return model, tea.Batch(cmd, preview), true
}

// blameParent annotates the file as the parent of this line's commit held it.
func (m Model) blameParent() (tea.Model, tea.Cmd, bool) {
	short, ok := m.selectedBlame()
	if !ok {
		return m, nil, true
	}

	m.blameRev = short + "^"
	m.blameLines, m.blameStyled, m.blameCursor = nil, nil, 0
	m.mainOffset = 0
	m.status = "annotating " + m.blamePath + " as of " + m.blameRev

	return m, readBlame(m.ctx, m.repo, m.blamePath, m.blameRev, currentSyntax()), true
}

func blameKeyHints() [][2]string {
	return [][2]string{
		{"j/k", "line"}, {"enter", "the commit"}, {"<", "before it"},
		{">", "back to now"}, {"b", "back to the diff"},
	}
}
