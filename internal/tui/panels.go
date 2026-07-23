package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// row picks between a per-segment coloured line and a flat selection bar. A
// selected row is drawn from plain text, styled segments carrying ANSI resets
// that would punch holes in the background.
func row(w int, selected, focused bool, plain, styled string) string {
	if !selected {
		return styled
	}
	if focused {
		return theme.SelectedStyle.Render(fitLine(plain, w))
	}
	return theme.SelectedBlurStyle.Render(fitLine(plain, w))
}

// statusLines is the summary panel: branch, divergence and counts.
func statusLines(snap git.Snapshot) []string {
	branch := snap.Branch
	if branch == "" {
		branch = "(no commits yet)"
	}

	var ahead, behind int
	for _, b := range snap.Branches {
		if b.Head {
			ahead, behind = b.Ahead, b.Behind
			break
		}
	}
	divergence := ""
	if ahead > 0 {
		divergence += theme.FooterKeyStyle.Render(fmt.Sprintf(" ↑%d", ahead))
	}
	if behind > 0 {
		divergence += lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Blue)).
			Render(fmt.Sprintf(" ↓%d", behind))
	}

	var staged, unstaged int
	for _, f := range snap.Files {
		if f.Staged() {
			staged++
		}
		if f.Work != '.' && f.Work != 0 {
			unstaged++
		}
	}

	second := theme.MutedStyle.Render(fmt.Sprintf(" %d staged · %d unstaged · %d stashed",
		staged, unstaged, len(snap.Stashes)))
	if snap.Rebasing {
		// Nothing else behaves normally until a stopped rebase is resolved.
		second = theme.ErrorStyle.Render(" rebase stopped — c continue · a abort")
	}

	return []string{
		theme.NormalStyle.Render(" "+branch) + divergence,
		second,
	}
}

// fileLines renders the Files panel, one row per changed path, including marks.
func (m Model) fileLines(files []git.FileChange, start, end, cursor, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		f := files[i]
		code := f.Code()

		// A leading dot marks a path with something in the index.
		mark := " "
		if f.Staged() {
			mark = "●"
		}

		checkmark := m.markPrefix(f)
		plain := fmt.Sprintf(" %s %s %c %s", checkmark, mark, code, f.Display())
		styled := " " + theme.FooterKeyStyle.Render(checkmark) + " " +
			theme.FooterKeyStyle.Render(mark) + " " +
			lipgloss.NewStyle().Foreground(theme.StatusColor(code)).Render(string(code)) + " " +
			f.Display()

		lines = append(lines, row(w, i == cursor, focused, plain, styled))
	}
	return lines
}

// branchLines renders the Branches panel with divergence counts.
func branchLines(branches []git.Branch, start, end, cursor, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		b := branches[i]

		mark := " "
		if b.Head {
			mark = "*"
		}

		name := b.Name
		styledName := b.Name
		switch b.Kind {
		case git.RefRemote:
			styledName = theme.MutedStyle.Render(b.Name)
		case git.RefTag:
			name = "⚑ " + b.Name
			styledName = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Yellow)).Render(name)
		}

		track, styledTrack := "", ""
		if b.Ahead > 0 {
			t := fmt.Sprintf(" ↑%d", b.Ahead)
			track, styledTrack = track+t, styledTrack+theme.FooterKeyStyle.Render(t)
		}
		if b.Behind > 0 {
			t := fmt.Sprintf(" ↓%d", b.Behind)
			track += t
			styledTrack += lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Blue)).Render(t)
		}

		plain := " " + mark + " " + name + track
		styled := " " + theme.FooterKeyStyle.Render(mark) + " " + styledName + styledTrack

		lines = append(lines, row(w, i == cursor, focused, plain, styled))
	}
	return lines
}

// commitLines renders the Commits panel. Without the graph a row carries one
// glyph, coloured per row and shaped by whether the commit is a merge. With it,
// rows holds the rails the parents imply and the glyph sits in its lane.
func commitLines(commits []git.Commit, rows []graphRow, unpushed map[string]bool, start, end, cursor, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		c := commits[i]

		// The column is there whether or not the row is marked, so the
		// subjects stay aligned.
		plainMove, styledMove := " ", " "
		if unpushed[c.SHA] {
			plainMove = "↑"
			styledMove = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary)).Render("↑")
		}

		glyph := "●"
		if c.Merge() {
			glyph = "◆"
		}

		plainMark := glyph + " "
		styledMark := lipgloss.NewStyle().Foreground(theme.GraphLane(i)).Render(glyph) + " "
		if i < len(rows) {
			plainMark, styledMark = rows[i].render(c.Merge())
		}

		plain := fmt.Sprintf(" %s%s%s %s", plainMove, plainMark, c.Short, c.Subject)
		styled := " " + styledMove + styledMark + theme.DimStyle.Render(c.Short) + " " + c.Subject

		lines = append(lines, row(w, i == cursor, focused, plain, styled))
	}
	return lines
}

// stashLines renders the Stash panel.
func stashLines(stashes []git.Stash, start, end, cursor, w int, focused bool) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		s := stashes[i]
		plain := " " + s.Ref + " " + s.Subject
		styled := " " + theme.DimStyle.Render(s.Ref) + " " + s.Subject

		lines = append(lines, row(w, i == cursor, focused, plain, styled))
	}
	return lines
}

// emptyLines is the placeholder a panel shows when it has nothing to list.
func emptyLines(text string) []string {
	return []string{theme.DimStyle.Render(" " + text)}
}

// diffLines styles the main pane's content by diff marker. Only the visible
// window is styled.
func diffLines(content string, offset, height int) []string {
	all := strings.Split(strings.TrimRight(content, "\n"), "\n")
	start, end, _ := window(len(all), offset, offset, height)
	lines := make([]string, 0, end-start)
	for _, line := range all[start:end] {
		lines = append(lines, theme.DiffLineStyle(line).Render(line))
	}
	return lines
}
