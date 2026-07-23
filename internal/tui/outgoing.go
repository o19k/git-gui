package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The branch counter only says how many commits would go, so a push asks
// first and lists them.
//
// Where they go is asked too, but only when there is more than one answer: a
// fork has an origin it can push to and an upstream it cannot, and picking the
// wrong one is a pull request opened against the wrong repository.

// outgoingMsg carries what a push would publish, and where it could go.
type outgoingMsg struct {
	// remote is where this push has been settled on going, empty until the
	// question has been asked. remotes is what there is to choose from, read
	// alongside the commits so the choice costs no extra round trip.
	remote  string
	remotes []string

	branch      string
	hasUpstream bool
	commits     []git.Commit
	err         error
}

// startPush reads what the push would publish, and where it could go, before
// asking about either.
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
		// A failure to list the remotes is not one worth stopping the push
		// for: it only costs the question about which one.
		remotes, _ := repo.Remotes(ctx)
		return outgoingMsg{
			remotes: remotes, branch: name,
			hasUpstream: hasUpstream, commits: commits, err: err,
		}
	}
}

// askPush turns that reading into the confirm, or into the reason there is
// nothing to confirm. Where the push goes is asked first, and only where there
// is more than one answer: a fork has an origin it can push to and an upstream
// it cannot, and the wrong one is a pull request against the wrong repository.
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

	if msg.remote == "" && len(msg.remotes) > 1 {
		choices := make([]choice, 0, len(msg.remotes))
		for _, remote := range msg.remotes {
			// The same reading comes back with the destination filled in, so
			// nothing is read twice.
			answered := msg
			answered.remote = remote
			choices = append(choices, choice{
				label:  remote,
				action: func() tea.Cmd { return func() tea.Msg { return answered } },
			})
		}
		m.askChoice("Push to",
			fmt.Sprintf("%s to publish, and more than one remote to publish them to.",
				count(len(msg.commits), "commit", "commits")),
			choices)
		return m, nil
	}

	repo, ctx := m.repo, m.ctx
	remote, branch, hasUpstream := msg.remote, msg.branch, msg.hasUpstream

	where := displayRemote(remote) + "/" + branch
	if !hasUpstream || remote != "" {
		where = "a new " + where
	}
	m.askConfirm("Push",
		fmt.Sprintf("Publish %s to %s?", count(len(msg.commits), "commit", "commits"), where),
		false,
		func() tea.Cmd {
			return m.do("push", func() error { return repo.Push(ctx, remote, branch, hasUpstream) })
		})
	m.overlay.extra = outgoingLines(msg.commits)
	m.overlay.busy = "pushing…"
	return m, nil
}

// displayRemote names a remote in a sentence. An empty one is wherever the
// branch already goes, which for a branch with an upstream is what git would
// have chosen anyway.
func displayRemote(remote string) string {
	if remote == "" {
		return "origin"
	}
	return remote
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
