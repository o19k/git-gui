package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// Line mode sits inside hunk mode: the hunk is chosen there, and here the lines
// of it are. It is what a hunk that holds two unrelated changes needs, and the
// one thing this tool was missing that sends people back to `git add -p`.

// enterLineMode opens the selected hunk for picking lines out of.
func (m *Model) enterLineMode() {
	hunks := m.currentHunks()
	if m.hunkCursor < 0 || m.hunkCursor >= len(hunks) {
		return
	}
	if countChangedLines(hunks[m.hunkCursor]) == 0 {
		m.status = "this hunk has no added or removed lines"
		return
	}

	m.lineMode = true
	m.lineMarks = make(map[int]bool)
	m.lineCursor = firstChangedLine(hunks[m.hunkCursor])
	m.scrollToLine()
}

func (m *Model) exitLineMode() {
	m.lineMode = false
	m.lineCursor = 0
	m.lineMarks = nil
	m.scrollToHunk()
}

// currentHunks parses the diff on screen. Parsed rather than kept: the diff is
// re-read on every snapshot, and a hunk list left over from the last one would
// point into text that has moved.
func (m Model) currentHunks() []git.Hunk {
	return git.ParseFileDiff(m.mainContent).Hunks
}

// countChangedLines is how many lines of a hunk can be picked; context cannot.
func countChangedLines(h git.Hunk) int {
	n := 0
	for _, line := range h.Lines {
		if changedLine(line) {
			n++
		}
	}
	return n
}

// changedLine reports a body line that adds or removes something.
func changedLine(line string) bool {
	return line != "" && (line[0] == '+' || line[0] == '-')
}

func firstChangedLine(h git.Hunk) int {
	for i, line := range h.Lines {
		if changedLine(line) {
			return i
		}
	}
	return 0
}

// lineRow is where a line of the hunk body sits in the rendered diff, so the
// cursor can be drawn and scrolled to.
func (m Model) lineRow(index int) int {
	ranges := hunkRanges(m.mainContent)
	if m.hunkCursor < 0 || m.hunkCursor >= len(ranges) {
		return 0
	}
	// The hunk's own @@ header is the first row of its range.
	return ranges[m.hunkCursor].start + 1 + index
}

// scrollToLine keeps the picked line inside the pane.
func (m *Model) scrollToLine() {
	row := m.lineRow(m.lineCursor)
	height := m.mainHeight()
	switch {
	case row < m.mainOffset:
		m.mainOffset = m.clampMainOffset(row)
	case row >= m.mainOffset+height:
		m.mainOffset = m.clampMainOffset(row - height + 1)
	}
}

// moveLine walks to the next line that can be picked, skipping context.
func (m *Model) moveLine(delta int) {
	hunks := m.currentHunks()
	if m.hunkCursor < 0 || m.hunkCursor >= len(hunks) {
		return
	}
	lines := hunks[m.hunkCursor].Lines

	for at := m.lineCursor + delta; at >= 0 && at < len(lines); at += delta {
		if changedLine(lines[at]) {
			m.lineCursor = at
			m.scrollToLine()
			return
		}
	}
}

// pickedLines is what would be staged: the marked lines, or the one under the
// cursor when nothing is marked.
func (m Model) pickedLines() map[int]bool {
	if len(m.lineMarks) > 0 {
		return m.lineMarks
	}
	return map[int]bool{m.lineCursor: true}
}

// handleLineKey serves the diff pane while lines are being picked.
func (m Model) handleLineKey(key string) (tea.Model, tea.Cmd, bool) {
	hunks := m.currentHunks()
	if m.hunkCursor < 0 || m.hunkCursor >= len(hunks) {
		m.exitLineMode()
		return m, nil, true
	}

	switch key {
	case "esc":
		m.exitLineMode()
		return m, nil, true

	case "j", "down":
		m.moveLine(1)
		return m, nil, true

	case "k", "up":
		m.moveLine(-1)
		return m, nil, true

	case " ":
		if m.lineMarks == nil {
			m.lineMarks = make(map[int]bool)
		}
		// A fresh map: the old one is shared with every earlier copy of the
		// model, which bubbletea keeps.
		marks := make(map[int]bool, len(m.lineMarks)+1)
		for at, on := range m.lineMarks {
			marks[at] = on
		}
		if marks[m.lineCursor] {
			delete(marks, m.lineCursor)
		} else {
			marks[m.lineCursor] = true
		}
		m.lineMarks = marks
		m.moveLine(1)
		return m, nil, true

	case "a":
		// Everything in this hunk, which is what hunk mode's space does — here
		// it is the way out of a marking gone wrong.
		marks := make(map[int]bool)
		for at, line := range hunks[m.hunkCursor].Lines {
			if changedLine(line) {
				marks[at] = true
			}
		}
		m.lineMarks = marks
		return m, nil, true

	case "enter":
		file, ok := m.selectedFile()
		if !ok {
			return m, nil, true
		}
		repo, ctx, n := m.repo, m.ctx, m.hunkCursor
		chosen, opts := m.pickedLines(), m.diffOpts()

		// The staged diff can only be taken back out, the unstaged one put in.
		if m.previewStaged {
			cmd := m.do("unstage lines", func() error {
				return repo.UnstageLines(ctx, file.Path, n, chosen, opts)
			})
			m.exitLineMode()
			return m, cmd, true
		}
		cmd := m.do("stage lines", func() error {
			return repo.StageLines(ctx, file.Path, n, chosen, opts)
		})
		m.exitLineMode()
		return m, cmd, true
	}
	return m, nil, false
}

// linePaneLines renders the diff with the hunk's pickable lines marked: a bar
// for the cursor, and a tick in the gutter for every line that would be staged.
func (m Model) linePaneLines(height int) []string {
	all := strings.Split(strings.TrimRight(m.mainContent, "\n"), "\n")
	start, end, _ := window(len(all), m.mainOffset, m.mainOffset, height)

	cursorRow := m.lineRow(m.lineCursor)
	marked := make(map[int]bool, len(m.lineMarks))
	for at := range m.lineMarks {
		marked[m.lineRow(at)] = true
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		// Two gutter columns: where the cursor is, and what is picked. A bar
		// and a tick rather than a background, for the same reason hunk mode
		// uses one — the diff's own colours would punch holes in a block.
		bar, tick := " ", " "
		if i == cursorRow {
			bar = "▌"
		}
		if marked[i] {
			tick = "✓"
		}
		prefix := theme.FooterKeyStyle.Render(bar + tick)
		lines = append(lines, prefix+theme.DiffLineStyle(all[i]).Render(all[i]))
	}
	return lines
}

// lineTitle names the file, the hunk and how much of it is picked.
func (m Model) lineTitle(path string, total int) string {
	verb := "stage"
	if m.previewStaged {
		verb = "unstage"
	}
	picked := len(m.lineMarks)
	if picked == 0 {
		return fmt.Sprintf("%s — hunk %d/%d — lines — enter %ss this one",
			path, m.hunkCursor+1, total, verb)
	}
	return fmt.Sprintf("%s — hunk %d/%d — %s picked — enter %ss them",
		path, m.hunkCursor+1, total, count(picked, "line", "lines"), verb)
}

func lineKeyHints() [][2]string {
	return [][2]string{
		{"j/k", "line"}, {"space", "pick"}, {"a", "all"},
		{"enter", "stage picked"}, {"esc", "back to hunks"},
	}
}
