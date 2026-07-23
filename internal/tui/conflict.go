package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// A conflicted path holds two versions at once. "ours" is what the branch
// already had, "theirs" what is coming in — the same for a merge, a rebase, a
// cherry-pick and an autostash going back.

// conflicted reports whether a path is one git could not merge on its own.
func conflicted(f git.FileChange) bool { return f.Code() == 'U' }

// conflictCount is how many paths are waiting to be resolved.
func (m Model) conflictCount() (n int) {
	for _, f := range m.snap.Files {
		if conflicted(f) {
			n++
		}
	}
	return n
}

// askResolve offers the three ways to settle one conflicted path. Editing by
// hand is the fourth, and ends in marking it resolved.
func (m *Model) askResolve(file git.FileChange) {
	if !conflicted(file) {
		m.status = file.Path + " is not conflicted"
		return
	}
	self := *m
	repo, ctx, path := m.repo, m.ctx, file.Path

	m.askChoice("Resolve "+path,
		"Both sides changed this file and git could not decide. Picking a side throws the other away for this path.",
		[]choice{
			{
				label:  "Keep ours",
				hint:   "the version your branch already had",
				action: func() tea.Cmd { return self.do("resolve", func() error { return repo.ResolveOurs(ctx, path) }) },
			},
			{
				label:  "Keep theirs",
				hint:   "the version being applied",
				action: func() tea.Cmd { return self.do("resolve", func() error { return repo.ResolveTheirs(ctx, path) }) },
			},
			{
				label:  "Mark resolved",
				hint:   "the file as it now stands, markers and all",
				action: func() tea.Cmd { return self.do("resolve", func() error { return repo.MarkResolved(ctx, path) }) },
			},
		})
}

// conflictBanner is the line the Changes tab shows instead of its counts while
// anything is unmerged, since nothing else about the repository can proceed.
func (m Model) conflictBanner() string {
	n := m.conflictCount()
	return fmt.Sprintf("%s conflicted — r resolve · space mark resolved",
		count(n, "file", "files"))
}

// fileHistoryMsg carries one path's log. path says whose, so a slow reply for a
// file the cursor has left is dropped.
type fileHistoryMsg struct {
	path string
	log  string
	err  error
}

// loadFileHistory reads the commits that touched one path.
func (m Model) loadFileHistory(path string) tea.Cmd {
	repo, ctx := m.repo, m.ctx
	return func() tea.Msg {
		log, err := repo.FileLog(ctx, path, 200)
		return fileHistoryMsg{path: path, log: log, err: err}
	}
}

// showFileHistory puts one path's log in front of the panels.
func (m Model) showFileHistory(msg fileHistoryMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}
	pretty := strings.TrimRight(renderPrettyLog(msg.log), "\n")
	m.showText("History — "+msg.path, strings.Split(pretty, "\n"))
	return m, nil
}
