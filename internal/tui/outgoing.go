package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The branch counter only says how many commits would go, so a push asks
// first and lists them.

// outgoingMsg carries what a push would publish.
type outgoingMsg struct {
	branch      string
	hasUpstream bool
	commits     []git.Commit
	err         error
}

// startPush reads what the push would publish before asking about it.
func (m Model) startPush() (Model, tea.Cmd) {
	head, ok := m.headBranch()
	if !ok {
		m.status = "nothing to push: no branch checked out"
		return m, nil
	}

	repo, ctx := m.repo, m.ctx
	name, hasUpstream := head.Name, head.Upstream != ""
	m.busy = "reading what would be published…"

	return m, func() tea.Msg {
		commits, err := repo.Outgoing(ctx, name)
		return outgoingMsg{branch: name, hasUpstream: hasUpstream, commits: commits, err: err}
	}
}

// askPush turns that reading into the confirm, or into the reason there is
// nothing to confirm.
func (m Model) askPush(msg outgoingMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}
	if len(msg.commits) == 0 {
		m.status = "nothing to push: " + msg.branch + " matches its upstream"
		return m, nil
	}

	repo, ctx := m.repo, m.ctx
	branch, hasUpstream := msg.branch, msg.hasUpstream

	where := "origin/" + branch
	if !hasUpstream {
		where = "a new " + where
	}
	m.askConfirm("Push",
		fmt.Sprintf("Publish %s to %s?", count(len(msg.commits), "commit", "commits"), where),
		false,
		func() tea.Cmd {
			return m.do("push", func() error { return repo.Push(ctx, branch, hasUpstream) })
		})
	m.overlay.extra = outgoingLines(msg.commits)
	m.overlay.busy = "pushing…"
	return m, nil
}

// outgoingLines lists the commits a push would send, newest first, capped so
// the confirm stays on screen.
func outgoingLines(commits []git.Commit) []string {
	const most = 8

	shown := commits
	if len(shown) > most {
		shown = shown[:most]
	}

	lines := make([]string, 0, len(shown)+1)
	for _, c := range shown {
		lines = append(lines, theme.DimStyle.Render(c.Short)+" "+theme.NormalStyle.Render(c.Subject))
	}
	if rest := len(commits) - len(shown); rest > 0 {
		lines = append(lines, theme.DimStyle.Render(fmt.Sprintf("… and %d more", rest)))
	}
	return lines
}
