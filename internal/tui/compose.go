package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A one-line prompt cannot hold a commit body, a template or a trailer, and
// amending through one would throw away whatever body the commit already had.
// So the message can also be written in $EDITOR, the same way git does it: a
// file under .git, comments stripped on the way back, an empty message meaning
// the commit is off.

// composeFile is the name the message is written to. Same shape as git's own
// COMMIT_EDITMSG, and a name of its own so a half-written message here cannot
// be picked up by a `git commit` running elsewhere.
const composeFile = "GITGUI_EDITMSG"

// composeDoneMsg reports the editor having exited over a commit message.
type composeDoneMsg struct {
	path  string
	amend bool
	err   error
}

// startCompose writes the starting message and hands the terminal to $EDITOR.
func (m Model) startCompose(amend bool) (tea.Model, tea.Cmd) {
	argv := editorCommand()
	if len(argv) == 0 {
		m.status = "set $EDITOR or $VISUAL to write a message here"
		return m, nil
	}
	if m.repo == nil {
		return m, nil
	}

	path, err := m.writeComposeFile(amend)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}

	cmd := exec.Command(argv[0], append(argv[1:], path)...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return composeDoneMsg{path: path, amend: amend, err: err}
	})
}

// writeComposeFile seeds the message: what is being amended, or the
// repository's template, and then the comments explaining what the editor is
// for. Comments are git's own '#' lines, which --cleanup=strip removes.
func (m Model) writeComposeFile(amend bool) (string, error) {
	repo, ctx := m.repo, m.ctx

	dir, err := repo.GitDir(ctx)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, composeFile)

	var body string
	switch {
	case amend:
		body, err = repo.HeadMessage(ctx)
		if err != nil {
			return "", err
		}
	default:
		body = strings.TrimRight(repo.CommitTemplate(ctx), "\n")
	}

	var b strings.Builder
	if body != "" {
		b.WriteString(body + "\n")
	}
	b.WriteString("\n")
	b.WriteString("# Write the message above. Lines starting with # are dropped,\n")
	b.WriteString("# and an empty message cancels the commit.\n")
	if amend {
		b.WriteString("# This replaces the last commit rather than adding one.\n")
	}
	for _, file := range m.snap.Files {
		if file.Staged() {
			b.WriteString("#\tstaged: " + file.Display() + "\n")
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// handleComposeDone commits what was written. The message goes to git as a
// file rather than as an argument: that is what keeps the body, and it is the
// one path a commit with more than a subject can take.
func (m Model) handleComposeDone(msg composeDoneMsg) (tea.Model, tea.Cmd) {
	// Whatever the editor did to the terminal, the repository has to be read
	// again: hooks, formatters and the editor itself all touch the tree.
	next, reload := m.reload()
	m = next

	if msg.err != nil {
		m.status = "editor: " + msg.err.Error()
		return m, reload
	}

	message, err := readComposed(msg.path)
	if err != nil {
		m.status = err.Error()
		return m, reload
	}
	if message == "" {
		m.status = "empty message: nothing committed"
		return m, reload
	}

	repo, ctx := m.repo, m.ctx
	path, amend, opts := msg.path, msg.amend, m.commitOpts()

	if amend {
		return m, tea.Batch(reload, m.do("amend", func() error {
			opts.Amend = true
			return repo.CommitFile(ctx, path, opts)
		}))
	}

	// A new commit goes through the repository's checks, the same as one made
	// from the prompt.
	m.pendingCommit = message
	committed, cmd := m.startComposeCommit(path)
	return committed, tea.Batch(reload, cmd)
}

// readComposed reads back what the editor left, without the comment lines.
func readComposed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), nil
}

// commitOpts is what every commit made here carries.
func (m Model) commitOpts() git.CommitOpts {
	return git.CommitOpts{Signoff: m.settings.Signoff}
}
