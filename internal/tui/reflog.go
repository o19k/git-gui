package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// The reflog is the way back from a rebase that went wrong, a reset that took
// too much, or a branch deleted with commits only it held. Nothing else in the
// tool can reach a commit no ref points at any more.

const reflogLimit = 200

// reflogMsg carries the read of HEAD's movements.
type reflogMsg struct {
	entries []git.ReflogEntry
	err     error
}

// reflogPick says which entry was chosen, kept as a value rather than a cursor
// so the list can close before the question about it is asked.
type reflogPickMsg struct{ entry git.ReflogEntry }

func (m Model) loadReflog() tea.Cmd {
	repo, ctx := m.repo, m.ctx
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		entries, err := repo.Reflog(ctx, reflogLimit)
		return reflogMsg{entries: entries, err: err}
	}
}

// showReflog puts the movements in a picker. The label carries everything the
// choice is made on; the value is the position, which is what git resolves.
func (m Model) showReflog(msg reflogMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}
	if len(msg.entries) == 0 {
		m.status = "no reflog: nothing has moved HEAD yet"
		return m, nil
	}

	byPosition := make(map[string]git.ReflogEntry, len(msg.entries))
	items := make([]listItem, 0, len(msg.entries))
	for _, entry := range msg.entries {
		byPosition[entry.Selector] = entry
		items = append(items, listItem{
			label: fmt.Sprintf("%-14s %s  %s  (%s)", entry.Selector, entry.Short, entry.Action, entry.When),
			value: entry.Selector,
		})
	}

	m.askList("Reflog — where HEAD has been", items, func(position string) tea.Cmd {
		entry, ok := byPosition[position]
		if !ok {
			return nil
		}
		return func() tea.Msg { return reflogPickMsg{entry: entry} }
	})
	return m, nil
}

// handleReflogPick offers what can be done with a commit the reflog found. A
// branch is the safe one and comes first: it puts the commit back within reach
// without moving anything that is already there.
func (m Model) handleReflogPick(msg reflogPickMsg) (tea.Model, tea.Cmd) {
	entry := msg.entry
	repo, ctx := m.repo, m.ctx
	self := m

	m.askChoice("Reflog "+entry.Selector,
		fmt.Sprintf("%s — %s", entry.Short, entry.Action),
		[]choice{
			{
				label: "New branch here",
				hint:  "makes the commit reachable again, moving nothing else",
				action: func() tea.Cmd {
					return func() tea.Msg { return reflogBranchMsg{entry: entry} }
				},
			},
			{
				label: "Check it out",
				hint:  "detaches HEAD at this commit, leaving every branch where it is",
				action: func() tea.Cmd {
					return self.do("checkout", func() error { return repo.CheckoutRev(ctx, entry.SHA) })
				},
			},
			{
				label:  "Reset this branch to it",
				hint:   "moves the branch and overwrites the working tree — uncommitted changes go",
				danger: true,
				action: func() tea.Cmd {
					return func() tea.Msg { return reflogResetMsg{entry: entry} }
				},
			},
		})
	return m, nil
}

// reflogBranchMsg and reflogResetMsg carry the decision back so the prompt and
// the confirmation are asked from the update loop rather than from inside a
// choice's action, which has no model to put an overlay on.
type reflogBranchMsg struct{ entry git.ReflogEntry }
type reflogResetMsg struct{ entry git.ReflogEntry }

func (m Model) handleReflogBranch(msg reflogBranchMsg) (tea.Model, tea.Cmd) {
	repo, ctx, sha := m.repo, m.ctx, msg.entry.SHA
	m.askInput("New branch at "+msg.entry.Short, "", func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		return m.do("create branch", func() error { return repo.CreateBranchAt(ctx, name, sha) })
	})
	return m, nil
}

func (m Model) handleReflogReset(msg reflogResetMsg) (tea.Model, tea.Cmd) {
	repo, ctx, entry := m.repo, m.ctx, msg.entry
	m.askConfirm("Reset to "+entry.Short,
		fmt.Sprintf("Move %s to %s and make the working tree match it? Uncommitted changes are lost; commits are still in this reflog.",
			m.snap.Branch, entry.Short), true,
		func() tea.Cmd {
			return m.do("reset", func() error { return repo.ResetHard(ctx, entry.SHA) })
		})
	return m, nil
}
