package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// A pull has three ways of not simply working, each with its own way out:
// local changes stand in the way, the branch has diverged from its upstream,
// or putting the set-aside changes back conflicts. Each is caught and offered
// the move that answers it.

// pullMsg is the outcome of a pull. conflicts is the paths left unmerged; git
// reports those on stderr while still exiting successfully, so they are asked
// for afterwards rather than read out of the error.
type pullMsg struct {
	what      string
	err       error
	conflicts []string
}

// pull runs one attempt and reports what it left behind.
func (m Model) pull(what string, run func(*git.Repo, context.Context) error) tea.Cmd {
	repo, ctx := m.repo, m.ctx
	return func() tea.Msg {
		err := run(repo, ctx)
		var conflicts []string
		if err == nil {
			conflicts, _ = repo.Unmerged(ctx)
		}
		return pullMsg{what: what, err: err, conflicts: conflicts}
	}
}

// startPull is the plain fast-forward attempt every pull begins with.
func (m Model) startPull() (Model, tea.Cmd) {
	m.busy = "pulling…"
	return m, m.pull("pull", (*git.Repo).Pull)
}

// handlePull turns a finished pull into either a result or the next question.
func (m Model) handlePull(msg pullMsg) (tea.Model, tea.Cmd) {
	m.busy = ""

	switch {
	case msg.err == nil:
		if len(msg.conflicts) > 0 {
			// Named while they fit: the footer is one line.
			m.status = fmt.Sprintf("%s conflicted in %s — press r on a file to resolve",
				msg.what, listOrCount(msg.conflicts))
			return m.reload()
		}
		m.status = ""
		return m.reload()

	case git.IsDirtyTree(msg.err):
		m.askStashPull(git.BlockingPaths(msg.err))
		return m, nil

	case git.IsNotFastForward(msg.err):
		m.askDiverged()
		return m, nil
	}

	m.status = msg.err.Error()
	return m, nil
}

// askStashPull offers the way past local changes that would be written over.
// The paths are the ones git named, since the reader may want to keep exactly
// one of them.
func (m *Model) askStashPull(paths []string) {
	// A copy: the choice runs long after this model value has been replaced.
	self := *m
	m.askChoice("Local changes are in the way",
		"Pulling would write over changes you have not committed. They can be set aside for the pull and put back straight after.",
		[]choice{
			{
				label:  "Stash, pull, and put them back",
				hint:   "your changes return on top of the new commits",
				busy:   "pulling…",
				action: func() tea.Cmd { return self.pull("pull", (*git.Repo).PullAutostash) },
			},
			{
				label:  "Leave them alone",
				hint:   "commit or stash by hand first",
				action: func() tea.Cmd { return nil },
			},
		})
	m.overlay.extra = pathLines(paths)
}

// listOrCount names the paths where a footer can hold them and counts them
// where it cannot.
func listOrCount(paths []string) string {
	if len(paths) == 0 || len(paths) > 3 {
		return count(len(paths), "file", "files")
	}
	return strings.Join(paths, ", ")
}

// maxBlockingPaths bounds the list so the choices under it stay on screen.
// What is cut is counted rather than dropped silently.
const maxBlockingPaths = 8

// pathLines is the blocking paths as the overlay draws them, the directory
// dim and the filename plain.
func pathLines(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	shown := paths
	var rest int
	if len(shown) > maxBlockingPaths {
		shown, rest = shown[:maxBlockingPaths], len(shown)-maxBlockingPaths
	}

	lines := make([]string, 0, len(shown)+1)
	for _, path := range shown {
		dir, name := "", path
		if i := strings.LastIndexByte(path, '/'); i >= 0 {
			dir, name = path[:i+1], path[i+1:]
		}
		lines = append(lines, theme.DimStyle.Render(dir)+theme.NormalStyle.Render(name))
	}
	if rest > 0 {
		lines = append(lines, theme.DimStyle.Render(fmt.Sprintf("and %d more %s", rest, plural(rest, "file", "files"))))
	}
	return lines
}

// askDiverged offers the two ways to reconcile a branch and an upstream that
// have each moved on. Neither is a safe default.
func (m *Model) askDiverged() {
	self := *m
	m.askChoice("The branch has diverged",
		"Your branch and its upstream each hold commits the other does not, so there is no fast-forward. Both ways below keep every commit.",
		[]choice{
			{
				label:  "Rebase",
				hint:   "replay your commits on top of the upstream",
				busy:   "rebasing onto the upstream…",
				action: func() tea.Cmd { return self.pull("rebase", (*git.Repo).PullRebase) },
			},
			{
				label:  "Merge",
				hint:   "join the two with a merge commit",
				busy:   "merging the upstream…",
				action: func() tea.Cmd { return self.pull("merge", (*git.Repo).PullMerge) },
			},
		})
}
