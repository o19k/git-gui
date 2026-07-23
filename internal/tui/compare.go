package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// Comparing a branch with the one checked out is what would come across if it
// were taken: the commits it holds and HEAD does not, and the files those
// commits leave different.

// compareMsg carries one comparison. ref says whose, so a slow reply for a
// branch the cursor has left is dropped.
type compareMsg struct {
	ref     string
	commits []git.Commit
	files   []git.FileChange
	err     error
}

// startCompare reads what branch holds that the current one does not.
func (m Model) startCompare(branch git.Branch) (Model, tea.Cmd) {
	// Comparing the branch you are already on is meaningless.
	if branch.Head {
		m.status = "this branch is already checked out"
		return m, nil
	}

	m.compareRef = branch.Name
	m.compareAll = false
	m.busy = "comparing branches…"
	return m, m.loadCompare(branch)
}

// loadCompare reads both halves of a comparison in one command, so the window
// opens once rather than redrawing when the second lands.
func (m Model) loadCompare(branch git.Branch) tea.Cmd {
	repo, ctx := m.repo, m.ctx
	target, mergeBase := branch.Ref(), m.compareAll

	return func() tea.Msg {
		commits, err := repo.CompareCommits(ctx, "HEAD", target)
		if err != nil {
			return compareMsg{ref: branch.Name, err: err}
		}
		files, err := repo.CompareFiles(ctx, "HEAD", target, mergeBase)
		if err != nil {
			return compareMsg{ref: branch.Name, err: err}
		}
		return compareMsg{ref: branch.Name, commits: commits, files: files}
	}
}

// showCompare turns a finished comparison into the window that presents it.
func (m Model) showCompare(msg compareMsg) (tea.Model, tea.Cmd) {
	m.busy = ""

	// Drop stale replies for branches the cursor has left.
	if msg.ref != m.compareRef {
		return m, nil
	}

	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	// The two file lists can differ without either looking wrong, so the title
	// says which scope this is.
	scope := "since the branches parted"
	if !m.compareAll {
		scope = "between the two tips"
	}
	m.showText(fmt.Sprintf("%s against HEAD — files %s", msg.ref, scope),
		compareLines(msg.commits, msg.files))
	m.overlay.compare = true

	return m, nil
}

// compareLines formats the comparison as a scrollable text overlay, with a
// section for the commits and one for the files.
func compareLines(commits []git.Commit, files []git.FileChange) []string {
	lines := []string{theme.TitleFocusStyle.Render(count(len(commits), "commit", "commits") + " it holds and HEAD does not")}
	if len(commits) == 0 {
		lines = append(lines, theme.DimStyle.Render("  nothing — HEAD already has everything on it"))
	}
	for _, c := range commits {
		lines = append(lines, "  "+theme.DimStyle.Render(c.Short)+" "+theme.NormalStyle.Render(c.Subject))
	}

	lines = append(lines, "", theme.TitleFocusStyle.Render(count(len(files), "file", "files")+" that differ"))
	if len(files) == 0 {
		lines = append(lines, theme.DimStyle.Render("  nothing — the two trees are identical"))
	}
	for _, f := range files {
		letter := lipgloss.NewStyle().Foreground(theme.StatusColor(f.Index)).Render(string(f.Index))
		lines = append(lines, "  "+letter+" "+theme.NormalStyle.Render(f.Display()))
	}

	return lines
}

// toggleCompareScope switches between the direct commit range and the merge-base
// range, then reloads to show which files differ under the new scope.
func (m *Model) toggleCompareScope() tea.Cmd {
	if m.compareRef == "" {
		return nil
	}

	// Looked up again rather than held: the refs reload on every tick.
	var branch git.Branch
	for _, b := range m.snap.Branches {
		if b.Name == m.compareRef {
			branch = b
			break
		}
	}
	if branch.Name == "" {
		m.status = m.compareRef + " is gone"
		return nil
	}

	m.compareAll = !m.compareAll
	m.busy = "comparing branches…"
	return m.loadCompare(branch)
}
