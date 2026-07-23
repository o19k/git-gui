package tui

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

// Copying goes through OSC 52, the terminal's own clipboard escape, rather
// than pbcopy, xclip or wl-copy: it needs no binary and no display server, and
// it works over ssh.
//
// The terminal has to allow it — Terminal.app does not, and tmux wants
// `set -g set-clipboard on`. Nothing reports back, so neither can this.

// copyPath copies a path to the clipboard. Repo-relative by default, which is
// what a git command wants pasted back; absolute has its own key.
func (m Model) copyPath(absolute bool) (tea.Model, tea.Cmd) {
	path, ok := m.selectedPath()
	if !ok {
		m.status = "nothing here has a path"
		return m, nil
	}
	if absolute && m.repo != nil {
		path = filepath.Join(m.repo.Path, path)
	}

	m.status = "copied " + path
	return m, copyToClipboard(path)
}

// copyToClipboard writes the escape sequence. It runs as a command rather than
// inline, Update being on the path of every keystroke.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		termenv.NewOutput(os.Stdout).Copy(text)
		return nil
	}
}
