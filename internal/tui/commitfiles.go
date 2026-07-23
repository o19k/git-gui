package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The Log tab's third column stacks the files a commit touched over the patch
// itself, so a large commit can be read one path at a time.

// commitFilesMsg carries the file list for one commit. sha says which, so a
// slow reply for a commit the cursor has left is dropped.
type commitFilesMsg struct {
	sha   string
	files []git.FileChange
	err   error
}

// loadCommitFiles reads the paths in the selected commit.
func (m *Model) loadCommitFiles() tea.Cmd {
	commit, ok := m.selectedCommit()
	if !ok {
		m.commitSHA, m.commitFiles = "", nil
		return nil
	}
	if commit.SHA == m.commitSHA {
		return nil // already showing this commit
	}

	m.commitSHA, m.commitFiles = commit.SHA, nil
	m.cursor[PanelCommitFiles], m.offset[PanelCommitFiles] = 0, 0

	repo, ctx, sha := m.repo, m.ctx, commit.SHA
	return func() tea.Msg {
		files, err := repo.CommitFiles(ctx, sha)
		return commitFilesMsg{sha: sha, files: files, err: err}
	}
}

func (m Model) selectedCommitFile() (git.FileChange, bool) {
	return selected(m.commitFiles, m.cursor[PanelCommitFiles])
}

// commitFileLines renders the pane, one row per path the commit touched.
func (m Model) commitFileLines(start, end, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		f := m.commitFiles[i]

		plain := fmt.Sprintf(" %c %s", f.Index, f.Display())
		styled := " " + lipgloss.NewStyle().Foreground(theme.StatusColor(f.Index)).
			Render(string(f.Index)) + " " + f.Display()

		lines = append(lines, row(w, i == m.cursor[PanelCommitFiles], focused, plain, styled))
	}
	return lines
}

// The pane has no mutations of its own: the commit keys live on the Commits
// list, and this one only chooses which patch the pane below shows.
func commitFilesKeyHints() [][2]string {
	return [][2]string{{"j/k", "file"}, {"shift+tab", "whole commit"}}
}
