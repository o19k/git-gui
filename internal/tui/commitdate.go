package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A commit carries two dates: when it was written and when it was recorded.
// Rewriting them is the same rewrite as rewording — everything after the commit
// is replayed — so it is gated the same way.

// askCommitDate asks for a date and applies it to one commit. The date comes
// first and the confirm second: there is nothing to warn about until there is
// a date.
func (m Model) askCommitDate(commit git.Commit) (Model, tea.Cmd) {
	m.askInput("Date for "+commit.Short+" — YYYY-MM-DD [HH:MM:SS]", "", func(date string) tea.Cmd {
		if date == "" {
			return nil
		}
		return func() tea.Msg { return commitDateMsg{sha: commit.SHA, short: commit.Short, date: date} }
	})
	return m, nil
}

// commitDateMsg carries the answered prompt back into the update loop, the
// only place a second overlay can be opened: the prompt's callback returns a
// command and never sees the model again.
type commitDateMsg struct {
	sha   string
	short string
	date  string
}

// confirmCommitDate is the gate in front of the rewrite.
func (m Model) confirmCommitDate(msg commitDateMsg) (tea.Model, tea.Cmd) {
	repo, ctx := m.repo, m.ctx
	m.askConfirm("Change the date",
		fmt.Sprintf("Set %s to %s? Every commit after it is replayed, so their object names change.",
			msg.short, msg.date),
		true,
		func() tea.Cmd {
			return m.do("set date", func() error { return repo.SetCommitDate(ctx, msg.sha, msg.date) })
		})
	return m, nil
}
