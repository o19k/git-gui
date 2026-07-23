package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Opening a file hands the terminal over to $EDITOR. tea.ExecProcess leaves
// the alternate screen, stops reading input, and restores both when the editor
// exits; anything else leaves two programs drawing to one terminal.

// editorDoneMsg reports the editor having exited.
type editorDoneMsg struct{ err error }

// editorCommand is the editor and its arguments. $VISUAL wins over $EDITOR by
// convention.
//
// The value is split on spaces so "code -w" and "emacsclient -nw" work; a path
// with spaces in it would be split wrongly, which is the price of not shelling
// out.
func editorCommand() []string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return strings.Fields(value)
		}
	}
	return nil
}

// openInEditor edits a path in the repository.
func (m Model) openInEditor(path string) (tea.Model, tea.Cmd) {
	argv := editorCommand()
	if len(argv) == 0 {
		m.status = "set $EDITOR or $VISUAL to open files here"
		return m, nil
	}
	if m.repo == nil {
		return m, nil
	}

	cmd := exec.Command(argv[0], append(argv[1:], filepath.Join(m.repo.Path, path))...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err: err}
	})
}

// handleEditorDone picks the repository back up. The file may have changed, so
// the snapshot, the listing and the preview are all taken again and the sort's
// stats are dropped.
func (m Model) handleEditorDone(msg editorDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = "editor: " + msg.err.Error()
	}

	m.stats = make(map[string]fileMeta)
	m.previewStyled = nil

	next, cmd := m.reload()
	return next, tea.Batch(cmd, next.loadIndex(), next.previewCommand())
}
