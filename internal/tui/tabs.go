package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/theme"
)

// tabBar is the top row: the workspaces on the left, the repository's own state
// docked right. The number that opens a tab is drawn in the label, so the keys
// need no separate legend.
func (m Model) tabBar() string {
	var b strings.Builder
	for t := range tabCount {
		style, num := theme.TabStyle, theme.TabNumStyle
		if Tab(t) == m.tab {
			style, num = theme.TabActiveStyle, theme.TabActiveNumStyle
		}
		// The digit and the words are styled separately, then padded as a
		// pair: rendering one styled string inside another pads twice and
		// closes the outer style at the inner one's reset.
		label := num.Render(fmt.Sprint(t+1)) + style.UnsetPadding().Render(" "+tabTitles[t])
		b.WriteString(lipgloss.NewStyle().Padding(0, 2).Render(label))
	}

	tabs := b.String()
	status := m.repoStatus()

	gap := m.width - lipgloss.Width(tabs) - lipgloss.Width(status)
	if gap < 1 {
		// Too narrow for both: the tabs say where you are, so they win.
		return fitLine(tabs, m.width)
	}
	return tabs + strings.Repeat(" ", gap) + status
}

// repoStatus is the docked right corner: which repository, on which branch, and
// how far it has drifted from its upstream.
func (m Model) repoStatus() string {
	if m.loading && m.snap.Branch == "" {
		return theme.DimStyle.Render("loading… ")
	}

	branch := m.snap.Branch
	if branch == "" {
		branch = "no commits yet"
	}

	var ahead, behind int
	for _, b := range m.snap.Branches {
		if b.Head {
			ahead, behind = b.Ahead, b.Behind
			break
		}
	}

	name := ""
	if m.repo != nil {
		name = filepath.Base(m.repo.Path) + " "
	}

	out := theme.NormalStyle.Render(name) + theme.MutedStyle.Render(branch)
	if ahead > 0 {
		out += theme.FooterKeyStyle.Render(fmt.Sprintf(" ↑%d", ahead))
	}
	if behind > 0 {
		out += lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Blue)).
			Render(fmt.Sprintf(" ↓%d", behind))
	}
	return out + theme.DimStyle.Render("  ✕ q ")
}

// banner is the row above the panes announcing a stopped operation. It exists
// only while one is stopped, because until then nothing else in the tab works.
func (m Model) banner() string {
	// A conflict outranks a stopped rebase: the rebase line offers continuing,
	// which git refuses until the conflicts are gone.
	if n := m.conflictCount(); n > 0 {
		return fitLine(theme.ErrorStyle.Render(" "+m.conflictBanner()), m.width)
	}
	if m.snap.Rebasing {
		return fitLine(theme.ErrorStyle.Render(" rebase stopped — ")+
			theme.FooterKeyStyle.Render("c")+theme.FooterDescStyle.Render(" continue · ")+
			theme.FooterKeyStyle.Render("a")+theme.FooterDescStyle.Render(" abort"), m.width)
	}
	counts := fmt.Sprintf("%d staged · %d unstaged", m.stagedCount(), m.unstagedCount())
	return fitLine(theme.MutedStyle.Render(" "+counts), m.width)
}

func (m Model) stagedCount() (n int) {
	for _, f := range m.snap.Files {
		if f.Staged() {
			n++
		}
	}
	return n
}

func (m Model) unstagedCount() (n int) {
	for _, f := range m.snap.Files {
		if f.Work != '.' && f.Work != 0 {
			n++
		}
	}
	return n
}
