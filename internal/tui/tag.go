package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A tag is made where the refs are listed, at whatever that list has selected.
// It is annotated when a message is given and a plain pointer when it is not,
// which is the distinction git itself draws and the one releases care about.

// tagNameMsg carries the name while the message is asked for.
type tagNameMsg struct {
	name string
	rev  string
}

// askTag names the new tag, at the selected ref's tip.
func (m Model) askTag(branch git.Branch) (tea.Model, tea.Cmd) {
	rev := branch.Ref()
	m.askInput("New tag at "+branch.Name, "", func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		return func() tea.Msg { return tagNameMsg{name: name, rev: rev} }
	})
	return m, nil
}

// handleTagName asks what the tag says. An empty answer makes the lightweight
// kind, which is the right one for a bookmark and the wrong one for a release.
func (m Model) handleTagName(msg tagNameMsg) (tea.Model, tea.Cmd) {
	repo, ctx := m.repo, m.ctx
	name, rev := msg.name, msg.rev

	m.askInput("Message for "+name+" (empty for a plain tag)", "", func(message string) tea.Cmd {
		return m.do("tag", func() error { return repo.CreateTag(ctx, name, rev, message) })
	})
	return m, nil
}

// askPushTag publishes one tag. Separate from the branch push: a tag is not
// carried by pushing the branch it sits on.
func (m Model) askPushTag(branch git.Branch) (tea.Model, tea.Cmd) {
	repo, ctx := m.repo, m.ctx
	name := branch.Name

	m.askConfirm("Push tag",
		fmt.Sprintf("Publish %s? A tag is not sent by pushing the branch it sits on.", name),
		false,
		func() tea.Cmd {
			return m.do("push tag", func() error { return repo.PushTag(ctx, "", name) })
		})
	m.overlay.busy = "pushing…"
	return m, nil
}
