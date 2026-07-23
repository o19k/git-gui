package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// Local Changes knows what git has recorded against a path; the Explorer knows
// where it sits in the tree. These two keys carry a selection between them.

// selectedPath is the path the focused pane's selection stands for, if it
// stands for one. Branches and commits do not.
func (m Model) selectedPath() (string, bool) {
	switch m.focus {
	case PanelFiles:
		if f, ok := m.selectedFile(); ok {
			return f.Path, true
		}
	case PanelCommitFiles:
		if f, ok := m.selectedCommitFile(); ok {
			return f.Path, true
		}
	case PanelStashFiles:
		if f, ok := m.selectedStashFile(); ok {
			return f.Path, true
		}
	case PanelParent, PanelEntries, PanelPreview:
		return m.explorerPath()
	}
	return "", false
}

// explorerPath is the selected entry's path, or the directory being listed
// when the listing is empty — an empty directory is still somewhere.
func (m Model) explorerPath() (string, bool) {
	entries := m.entries()
	if len(entries) == 0 {
		if m.cwd == "" {
			return ".", true
		}
		return m.cwd, true
	}
	i := clamp(m.cursor[PanelEntries], 0, len(entries)-1)
	return childPath(m.cwd, entries[i].Name), true
}

// revealInExplorer opens the Explorer standing on a path.
//
// The listing it needs may not be in memory: opening the tab starts a read,
// and pendingPath holds the destination until it lands.
func (m Model) revealInExplorer(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		return m, nil
	}
	opening := m.tab != TabFiles
	next, cmd := m.openTab(TabFiles)
	moved := next.(Model)

	// Only an opening tab has a listing on the way. Set while the tab is open,
	// this would be found by the poll and drag the cursor back.
	if opening {
		moved.pendingPath = path
	}

	return moved, tea.Batch(cmd, func() tea.Msg { return navigateMsg{path: path} })
}

// showInChanges puts the cursor on a path in Local Changes. Only a path git
// has something to say about is listed there at all.
func (m Model) showInChanges(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		return m, nil
	}

	// A filter narrows what the cursor can address, so it is cleared rather
	// than the path reported missing.
	if i := indexOfPath(m.files(), path); i < 0 && m.filter[PanelFiles] != "" {
		m.filter[PanelFiles] = ""
	}

	i := indexOfPath(m.files(), path)
	if i < 0 {
		m.status = fmt.Sprintf("%s has no uncommitted change", path)
		return m, nil
	}

	next, cmd := m.openTab(TabChanges)
	moved := next.(Model)
	moved.focus = PanelFiles
	moved.cursor[PanelFiles] = i
	moved.syncOffset(PanelFiles)

	return moved, tea.Batch(cmd, moved.refreshPreview())
}

// indexOfPath finds a path in a file list, or -1.
func indexOfPath(files []git.FileChange, path string) int {
	for i, f := range files {
		if f.Path == path {
			return i
		}
	}
	return -1
}
