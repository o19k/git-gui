package tui

import (
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Search goes through git: ls-files and grep already respect .gitignore, the
// index and submodule boundaries.

// grepHit is one line git grep matched.
type grepHit struct {
	Path string
	Line int
	Text string
}

// grepMsg carries a finished search. query tags it.
type grepMsg struct {
	query string
	hits  []grepHit
	err   error
}

// navigateMsg asks the Explorer to go to a path. Both pickers hand their
// selection over this way: an action runs after Update has returned, so it
// cannot write to the live model.
type navigateMsg struct {
	path string

	// line is 1-based, and zero means the path was chosen rather than a line
	// inside it.
	line int
}

// openPicker lists every indexed path and navigates to the chosen one. It
// reads nothing: the index is already in memory.
func (m Model) openPicker() (Model, tea.Cmd) {
	var items []listItem
	for dir, entries := range m.index {
		for _, e := range entries {
			// An ignored tree is reachable by walking into it, but it is not
			// one of the repository's paths.
			if e.Ignored {
				continue
			}
			path := childPath(dir, e.Name)
			items = append(items, listItem{label: path, value: path})
		}
	}

	// A map has no order, and the picker's must not change between openings.
	slices.SortFunc(items, func(a, b listItem) int {
		return strings.Compare(a.label, b.label)
	})

	m.askList("Open", items, func(path string) tea.Cmd {
		return func() tea.Msg { return navigateMsg{path: path} }
	})

	return m, nil
}

// handleNavigate moves to a path chosen in a picker. The path is absolute
// within the repository, so it may name a directory never visited.
func (m Model) handleNavigate(msg navigateMsg) (tea.Model, tea.Cmd) {
	if msg.path == "" {
		return m, nil
	}

	m.dirCursor[m.cwd] = m.cursor[PanelEntries]

	// A directory is somewhere to stand; a file is something to select in the
	// directory holding it.
	if e, ok := m.entryAt(msg.path); ok && e.Dir {
		m.cwd = msg.path
		m.cursor[PanelEntries] = 0
		m.offset[PanelEntries] = 0
		if _, inIndex := m.index[m.cwd]; !inIndex {
			if _, onDisk := m.fsIndex[m.cwd]; !onDisk {
				return m, m.readDir(m.cwd)
			}
		}
		cmd := m.explorerMoved()
		return m, cmd
	}

	name := msg.path
	if i := strings.LastIndex(msg.path, "/"); i >= 0 {
		name = msg.path[i+1:]
	}

	// The choice reveals a hidden file rather than landing with the cursor
	// somewhere else.
	if strings.HasPrefix(name, ".") {
		m.showHidden = true
	}

	m.cwd = parentOf(msg.path)
	m.cursor[PanelEntries] = 0
	for i, e := range m.entries() {
		if e.Name == name {
			m.cursor[PanelEntries] = i
			break
		}
	}
	m.syncOffset(PanelEntries)

	cmd := m.explorerMoved()

	// A line was asked for, and a diff has no line to scroll to.
	if msg.line > 0 {
		m.pendingLine = msg.line
		if m.previewFor.kind != previewContent {
			m.previewFor.kind = previewContent
			cmd = m.previewCommand()
		}
	}
	return m, cmd
}

// startGrep searches file contents. git grep exits 1 when nothing matched,
// which r.exec surfaces as an error; that is an empty result, not a failure.
func (m Model) startGrep(query string) tea.Cmd {
	repo, ctx := m.repo, m.ctx
	return func() tea.Msg {
		lines, err := repo.Grep(ctx, query)

		var hits []grepHit
		for _, line := range lines {
			// Format: path:line:text
			if parts := strings.SplitN(line, ":", 3); len(parts) >= 2 {
				path := parts[0]
				lineNum, _ := strconv.Atoi(parts[1])
				text := ""
				if len(parts) > 2 {
					text = parts[2]
				}
				hits = append(hits, grepHit{
					Path: path,
					Line: lineNum,
					Text: text,
				})
			}
		}

		return grepMsg{
			query: query,
			hits:  hits,
			err:   err,
		}
	}
}

func (m Model) handleGrep(msg grepMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	var items []listItem
	for _, hit := range msg.hits {
		label := hit.Path + ":" + strconv.Itoa(hit.Line) + " " + hit.Text
		items = append(items, listItem{
			label: label,
			value: hit.Path + ":" + strconv.Itoa(hit.Line),
		})
	}

	if len(items) == 0 {
		m.askList("Search", []listItem{{label: "no matches", value: ""}}, nil)
		return m, nil
	}

	m.askList("Search results", items, func(value string) tea.Cmd {
		if value == "" {
			return nil
		}

		if parts := strings.SplitN(value, ":", 2); len(parts) == 2 {
			path := parts[0]
			lineNum, _ := strconv.Atoi(parts[1])

			return func() tea.Msg {
				return navigateMsg{path: path, line: lineNum}
			}
		}

		return nil
	})

	return m, nil
}

// explorerSearchKey handles the search keys.
func (m Model) explorerSearchKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "o":
		nextM, cmd := m.openPicker()
		m = nextM
		return m, cmd, true

	case "s":
		m.askInput("Search files for", "", func(query string) tea.Cmd {
			if query == "" {
				return nil
			}
			return m.startGrep(query)
		})
		return m, nil, true
	}

	return m, nil, false
}
