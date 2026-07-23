package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// Hunk mode turns the diff pane into a selectable list. A mode rather than a
// panel because the hunks belong to whatever Files has selected.

// hunkRange is where one hunk sits in the rendered diff, as line offsets.
type hunkRange struct{ start, end int }

// hunkRanges locates each hunk within the diff text.
func hunkRanges(patch string) []hunkRange {
	diff := git.ParseFileDiff(patch)
	ranges := make([]hunkRange, 0, len(diff.Hunks))

	at := len(diff.Preamble)
	for _, h := range diff.Hunks {
		size := 1 + len(h.Lines) // the @@ header plus its body
		ranges = append(ranges, hunkRange{start: at, end: at + size})
		at += size
	}
	return ranges
}

// enterHunkMode focuses the diff pane's hunks for the selected file.
func (m *Model) enterHunkMode() {
	file, ok := m.selectedFile()
	if !ok {
		return
	}
	if file.Untracked() {
		m.status = "untracked file has no hunks — stage it whole with space"
		return
	}
	if len(hunkRanges(m.mainContent)) == 0 {
		m.status = "no hunks to stage in " + file.Path
		return
	}
	m.hunkMode = true
	m.hunkCursor = 0
	m.focus = PanelDiff
	// The pane can only show one of the two.
	m.blameOn, m.blamePath, m.blameLines = false, "", nil
	m.scrollToHunk()
}

func (m *Model) exitHunkMode() {
	m.hunkMode = false
	m.hunkCursor = 0
	m.mainOffset = 0
	m.focus = PanelFiles
	m.lineMode, m.lineCursor, m.lineMarks = false, 0, nil
}

// scrollToHunk puts the selected hunk at the top of the pane.
func (m *Model) scrollToHunk() {
	ranges := hunkRanges(m.mainContent)
	if len(ranges) == 0 {
		return
	}
	m.hunkCursor = clamp(m.hunkCursor, 0, len(ranges)-1)
	m.mainOffset = m.clampMainOffset(ranges[m.hunkCursor].start)
}

// handleHunkKey serves the diff pane while hunk mode is on. Movement and
// staging are rebound to the hunks; everything else leaves the mode first.
func (m Model) handleHunkKey(key string) (tea.Model, tea.Cmd, bool) {
	ranges := hunkRanges(m.mainContent)
	if len(ranges) == 0 {
		m.exitHunkMode()
		return m, nil, true
	}

	switch key {
	case "esc", "q":
		m.exitHunkMode()
		return m, nil, true

	case "enter":
		m.enterLineMode()
		return m, nil, true

	case "j", "down":
		m.hunkCursor = clamp(m.hunkCursor+1, 0, len(ranges)-1)
		m.scrollToHunk()
		return m, nil, true

	case "k", "up":
		m.hunkCursor = clamp(m.hunkCursor-1, 0, len(ranges)-1)
		m.scrollToHunk()
		return m, nil, true

	case " ":
		file, ok := m.selectedFile()
		if !ok {
			return m, nil, true
		}
		repo, ctx, n := m.repo, m.ctx, m.hunkCursor
		opts := m.diffOpts()

		// The staged diff can only be taken back out, the unstaged one put in.
		if m.previewStaged {
			return m, m.do("unstage hunk", func() error {
				return repo.UnstageHunk(ctx, file.Path, n, opts)
			}), true
		}
		return m, m.do("stage hunk", func() error {
			return repo.StageHunk(ctx, file.Path, n, opts)
		}), true
	}
	return m, nil, false
}

// hunkPaneLines renders the diff with the selected hunk marked by a bar down
// its left edge. A bar rather than a background: the diff lines' own ANSI
// resets would punch holes in a block.
func hunkPaneLines(patch string, ranges []hunkRange, selected, offset, height int) []string {
	all := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	start, end, _ := window(len(all), offset, offset, height)

	var marked hunkRange
	if selected >= 0 && selected < len(ranges) {
		marked = ranges[selected]
	}

	bar := theme.FooterKeyStyle.Render("▌")
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		prefix := " "
		if i >= marked.start && i < marked.end {
			prefix = bar
		}
		lines = append(lines, prefix+theme.DiffLineStyle(all[i]).Render(all[i]))
	}
	return lines
}

// hunkTitle names the file and the position within it.
func hunkTitle(path string, selected, total int, staged bool) string {
	verb := "stage"
	if staged {
		verb = "unstage"
	}
	return fmt.Sprintf("%s — hunk %d/%d — space to %s", path, selected+1, total, verb)
}

// hunkKeyHints is the footer while hunk mode is on.
func hunkKeyHints() [][2]string {
	return [][2]string{
		{"j/k", "hunk"}, {"space", "stage"}, {"enter", "pick lines"}, {"esc", "back to files"},
	}
}
