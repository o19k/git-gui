package tui

import (
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The Stash tab lists what an entry holds, so part of a stash can come back
// without the rest. `git stash apply` is all-or-nothing; marked paths are read
// out of the stash commit instead, which overwrites rather than merges — so it
// asks first.

// stashFilesMsg carries the file list for one stash entry. ref identifies which,
// so a slow reply for an entry the cursor has left is dropped.
type stashFilesMsg struct {
	ref   string
	files []git.FileChange
	err   error
}

// loadStashFiles reads the paths in the selected entry.
func (m *Model) loadStashFiles() tea.Cmd {
	stash, ok := m.selectedStash()
	if !ok {
		m.stashRef, m.stashFiles, m.stashMarks = "", nil, nil
		return nil
	}
	if stash.Ref == m.stashRef {
		return nil // already showing this entry
	}

	// The marks belonged to the entry being left behind.
	m.stashRef, m.stashFiles, m.stashMarks = stash.Ref, nil, nil
	m.cursor[PanelStashFiles], m.offset[PanelStashFiles] = 0, 0

	repo, ctx, ref := m.repo, m.ctx, stash.Ref
	return func() tea.Msg {
		files, err := repo.StashFiles(ctx, ref)
		return stashFilesMsg{ref: ref, files: files, err: err}
	}
}

func (m Model) selectedStashFile() (git.FileChange, bool) {
	return selected(m.stashFiles, m.cursor[PanelStashFiles])
}

// markedStashPaths is what a restore would act on: the marked paths, or the one
// under the cursor when nothing is marked.
func (m Model) markedStashPaths() []string {
	var paths []string
	for path, marked := range m.stashMarks {
		if marked {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		// Map order is random; a confirm that names files must be stable.
		sort.Strings(paths)
		return paths
	}
	if file, ok := m.selectedStashFile(); ok {
		return []string{file.Path}
	}
	return nil
}

func (m Model) stashFilesKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx

	switch key {
	case "o":
		file, ok := m.selectedStashFile()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.revealInExplorer(file.Path)
		return next, cmd, true

	case " ":
		file, ok := m.selectedStashFile()
		if !ok {
			return m, nil, true
		}
		// Copy: the map is shared with the model this update came from.
		marks := make(map[string]bool, len(m.stashMarks)+1)
		for k, v := range m.stashMarks {
			marks[k] = v
		}
		marks[file.Path] = !marks[file.Path]
		m.stashMarks = marks
		return m, nil, true

	case "u":
		paths := m.markedStashPaths()
		if len(paths) == 0 || m.stashRef == "" {
			return m, nil, true
		}
		ref := m.stashRef
		m.askConfirm("Restore from stash",
			fmt.Sprintf("Overwrite %s with the version in %s? Uncommitted changes to %s are lost.",
				pathList(paths), ref, plural(len(paths), "that file", "those files")),
			true,
			func() tea.Cmd {
				return m.do("restore from stash", func() error {
					return repo.StashApplyFiles(ctx, ref, paths)
				})
			})
		return m, nil, true
	}
	return m, nil, false
}

// pathList names what an action will touch, capped to fit a confirm.
func pathList(paths []string) string {
	switch {
	case len(paths) == 1:
		return paths[0]
	case len(paths) <= 3:
		out := paths[0]
		for _, p := range paths[1:] {
			out += ", " + p
		}
		return out
	}
	return fmt.Sprintf("%s and %d more", paths[0], len(paths)-1)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// count is plural with the number in front: "1 file", "3 files".
func count(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, plural(n, one, many))
}

// stashFileLines renders the pane, marking the paths a restore would take.
func (m Model) stashFileLines(start, end, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		f := m.stashFiles[i]

		mark := " "
		if m.stashMarks[f.Path] {
			mark = "✓"
		}
		plain := fmt.Sprintf(" %s %c %s", mark, f.Index, f.Display())
		styled := " " + theme.FooterKeyStyle.Render(mark) + " " +
			lipgloss.NewStyle().Foreground(theme.StatusColor(f.Index)).Render(string(f.Index)) +
			" " + f.Display()

		lines = append(lines, row(w, i == m.cursor[PanelStashFiles], focused, plain, styled))
	}
	return lines
}

func stashFilesKeyHints() [][2]string {
	return [][2]string{{"space", "mark"}, {"u", "restore marked"}}
}
