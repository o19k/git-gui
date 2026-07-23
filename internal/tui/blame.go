package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// Blame replaces the patch in the content pane rather than sitting beside it:
// a gutter wide enough for a commit and an author leaves too little room for
// the code otherwise.

// blameMsg carries a file's blame. path says whose, so a slow reply for a file
// the cursor has left is dropped. styled is the code column coloured as source,
// empty for a file of no language the highlighter knows.
type blameMsg struct {
	path   string
	lines  []git.BlameLine
	styled []string
	err    error
}

// readBlame annotates a file and colours the code beside the gutter. The
// colouring is a pass over the whole file, so it happens once per read rather
// than once per frame.
func readBlame(ctx context.Context, repo *git.Repo, path string, palette syntax) tea.Cmd {
	return func() tea.Msg {
		lines, err := repo.Blame(ctx, path)
		return blameMsg{path: path, lines: lines, styled: highlightBlame(path, lines, palette), err: err}
	}
}

// highlightBlame colours a blame's code column. The lexer takes a file rather
// than a list of lines, so they are rejoined; a lexer that folds or splits
// lines gives an unusable result, which is dropped.
func highlightBlame(path string, lines []git.BlameLine, palette syntax) []string {
	if len(lines) == 0 {
		return nil
	}
	text := make([]string, len(lines))
	for i, l := range lines {
		text[i] = l.Text
	}
	styled := highlight(path, strings.Join(text, "\n"), palette)
	if len(styled) != len(lines) {
		return nil
	}
	return styled
}

// toggleBlame turns annotations on for the selected file, or off again.
func (m Model) toggleBlame() (tea.Model, tea.Cmd) {
	if m.blameOn {
		m.blameOn, m.blamePath, m.blameLines, m.blameStyled = false, "", nil, nil
		m.mainOffset = 0
		return m, m.refreshPreview()
	}

	file, ok := m.selectedFile()
	if !ok || m.tab != TabChanges {
		m.status = "annotations need a file selected in Local Changes"
		return m, nil
	}
	// git blame reads the file's history, which these two do not have.
	if file.Untracked() {
		m.status = "an untracked file has no history to annotate"
		return m, nil
	}
	if file.Code() == 'D' {
		m.status = "a deleted file has no working copy to annotate"
		return m, nil
	}

	m.blameOn, m.blamePath, m.blameLines, m.blameStyled = true, file.Path, nil, nil
	m.mainOffset = 0

	return m, readBlame(m.ctx, m.repo, file.Path, currentSyntax())
}

// followBlame re-annotates when the selection moves to another file. A file
// blame cannot read turns annotations off rather than leaving them stale.
func (m *Model) followBlame() tea.Cmd {
	if !m.blameOn {
		return nil
	}
	file, ok := m.selectedFile()
	if !ok || m.tab != TabChanges || file.Untracked() || file.Code() == 'D' {
		m.blameOn, m.blamePath, m.blameLines, m.blameStyled = false, "", nil, nil
		return nil
	}
	if file.Path == m.blamePath {
		return nil
	}

	m.blamePath, m.blameLines, m.blameStyled = file.Path, nil, nil
	m.mainOffset = 0

	return readBlame(m.ctx, m.repo, file.Path, currentSyntax())
}

// renderBlame formats a window of blame lines at a given width, for both the
// Local Changes pane and the Explorer's. styled is the code column already
// coloured, one entry per line, and may be empty — an unknown language loses
// the colours rather than the annotations.
func renderBlame(lines []git.BlameLine, styled []string, offset, height, width int) []string {
	if len(lines) == 0 {
		return emptyLines("annotating…")
	}
	start, end, _ := window(len(lines), offset, offset, height)

	// One gutter width for the whole pane, so the code starts in one column.
	authorW := 0
	for _, l := range lines {
		authorW = max(authorW, len(l.Author))
	}
	authorW = min(authorW, maxBlameAuthor)

	out := make([]string, 0, end-start)
	for i, l := range lines[start:end] {
		code := l.Text
		if row := start + i; row < len(styled) {
			code = styled[row]
		}
		gutter := fmt.Sprintf("%-7s %-*s %-10s", l.Short, authorW, truncate(l.Author, authorW), l.When)
		out = append(out, theme.DimStyle.Render(gutter)+" "+code)
	}
	return out
}

// blamePaneLines renders the window of the annotated file that fits in height rows.
func (m Model) blamePaneLines(height, width int) []string {
	return renderBlame(m.blameLines, m.blameStyled, m.mainOffset, height, width)
}

const maxBlameAuthor = 16

func truncate(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return s[:w-1] + "…"
}

// blameTitle names what is annotated.
func (m Model) blameTitle() string {
	return "Blame — " + m.blamePath
}

func (m Model) blameLineCount() int { return len(m.blameLines) }

func blameKeyHints() [][2]string {
	return [][2]string{{"j/k", "line"}, {"b", "back to the diff"}}
}
