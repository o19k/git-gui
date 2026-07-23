package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// One key used to mean "set everything aside", which is the wrong answer often
// enough: a stash is usually taken to get one change out of the way, and
// stashing the rest with it is how work goes missing for an afternoon.

type stashKind int

const (
	stashEverything stashKind = iota
	stashTracked
	stashStaged
	stashMarked
)

// stashKindMsg carries the chosen shape while the message is asked for.
type stashKindMsg struct {
	kind  stashKind
	paths []string
}

// askStash offers what the stash should take.
func (m Model) askStash() (tea.Model, tea.Cmd) {
	// Explicitly marked paths only, not the "or the selection" markedFiles
	// falls back to: a stash of the one file under the cursor is not something
	// to offer as though it had been asked for.
	marked := m.explicitMarks()
	pick := func(kind stashKind, paths []string) func() tea.Cmd {
		return func() tea.Cmd {
			return func() tea.Msg { return stashKindMsg{kind: kind, paths: paths} }
		}
	}

	choices := []choice{
		{label: "Everything", hint: "tracked changes and untracked files", action: pick(stashEverything, nil)},
		{label: "Tracked only", hint: "leaves files git has never seen where they are", action: pick(stashTracked, nil)},
		{label: "Staged only", hint: "what the index holds; the working tree is left alone", action: pick(stashStaged, nil)},
	}
	if len(marked) > 0 {
		choices = append(choices, choice{
			label:  fmt.Sprintf("Just %s", count(len(marked), "the marked file", "the marked files")),
			hint:   pathList(marked),
			action: pick(stashMarked, marked),
		})
	}

	m.askChoice("Stash", "What goes into the entry?", choices)
	return m, nil
}

// handleStashKind asks for the message, then takes the stash.
func (m Model) handleStashKind(msg stashKindMsg) (tea.Model, tea.Cmd) {
	repo, ctx := m.repo, m.ctx
	opts := git.StashOpts{Paths: msg.paths}
	switch msg.kind {
	case stashEverything:
		opts.Untracked = true
	case stashStaged:
		opts.StagedOnly = true
	}

	m.askInput("Stash message", "", func(message string) tea.Cmd {
		opts.Message = message
		return m.do("stash", func() error { return repo.StashPush(ctx, opts) })
	})
	return m, nil
}
