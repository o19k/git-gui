package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The graph is drawn from the parents git already reported, so it costs no
// extra call. A branch alive on a row gets a vertical line in its column, the
// commit gets the dot, and a merge is joined by a horizontal run to the lanes
// it opens.

// maxLanes bounds the width the rails may take. Lanes past the limit share
// the last column rather than pushing the subjects off the panel.
const maxLanes = 8

// graphRow is one commit's rails: which columns carry a line, which column
// holds the dot, and which columns this commit opened — a merge's other
// parents, whose lanes begin here. closed are the columns whose lane ends
// here, two lanes having turned out to be waiting for this commit. link is a
// lane this commit's own line continues into, or -1, which is the same meeting
// seen from the other side.
type graphRow struct {
	lanes  []bool
	node   int
	opened []int
	closed []int
	link   int
}

// graphRows assigns every commit a column. It walks the whole list: a lane's
// column is decided by the commits above it.
func graphRows(commits []git.Commit) []graphRow {
	// expected[i] is the sha the lane in column i is waiting for; empty is a
	// free column.
	var expected []string
	rows := make([]graphRow, 0, len(commits))

	for _, c := range commits {
		col := indexOfLane(expected, c.SHA)
		if col < 0 {
			col = freeLane(&expected)
		}

		row := graphRow{lanes: make([]bool, len(expected)), node: col, link: -1}
		for i, sha := range expected {
			row.lanes[i] = sha != ""
		}
		row.lanes[col] = true

		// This lane now waits for the first parent; a root commit ends it.
		expected[col] = ""
		if len(c.Parents) > 0 {
			switch other := indexOfLane(expected, c.Parents[0]); {
			case other < 0:
				expected[col] = c.Parents[0]
			case col < other:
				// Two lanes waiting on one commit would draw a rail that never
				// joins, so the left one keeps it — the trunk is leftmost, and
				// must not step sideways where a branch meets it.
				expected[col], expected[other] = c.Parents[0], ""
				row.closed = append(row.closed, other)
			default:
				// The left lane already holds the parent, so this one ends on
				// its dot and the line carries on over there.
				row.link = other
			}
		}

		// A merge's other parents open lanes of their own, unless something is
		// already waiting for them — two branches meeting is one lane, not two.
		if len(c.Parents) > 1 {
			for _, parent := range c.Parents[1:] {
				if indexOfLane(expected, parent) < 0 {
					lane := freeLane(&expected)
					expected[lane] = parent
					row.opened = append(row.opened, lane)
				}
			}
		}

		// A lane this commit opened is live on this row: it starts here.
		for _, lane := range row.opened {
			for len(row.lanes) <= lane {
				row.lanes = append(row.lanes, false)
			}
			row.lanes[lane] = true
		}
		rows = append(rows, row)

		trimLanes(&expected)
	}
	return rows
}

func indexOfLane(expected []string, sha string) int {
	if sha == "" {
		return -1
	}
	for i, want := range expected {
		if want == sha {
			return i
		}
	}
	return -1
}

// freeLane claims a column, reusing a spent one before widening the rails.
func freeLane(expected *[]string) int {
	for i, sha := range *expected {
		if sha == "" {
			return i
		}
	}
	if len(*expected) >= maxLanes {
		// Out of room: the last column carries them all.
		return maxLanes - 1
	}
	*expected = append(*expected, "")
	return len(*expected) - 1
}

// trimLanes drops spent columns off the right, so a branch that ended does not
// leave a permanent gap in the rails.
func trimLanes(expected *[]string) {
	lanes := *expected
	for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
		lanes = lanes[:len(lanes)-1]
	}
	*expected = lanes
}

// render draws one row's rails, the dot included. A merge reaches across to
// the lanes it opened.
func (g graphRow) render(merge bool) (plain, styled string) {
	glyph := "●"
	if merge {
		glyph = "◆"
	}

	// The run spans the node and every lane it starts, ends or continues into.
	// freeLane reuses spent columns, so a lane can open to the left as well.
	lo, hi := g.node, g.node
	for _, lane := range append(append([]int{}, g.opened...), g.closed...) {
		lo, hi = min(lo, lane), max(hi, lane)
	}
	if g.link >= 0 {
		lo, hi = min(lo, g.link), max(hi, g.link)
	}

	var p, s strings.Builder
	write := func(mark string, lane int) {
		p.WriteString(mark)
		if mark == " " {
			s.WriteString(mark)
			return
		}
		s.WriteString(lipgloss.NewStyle().Foreground(theme.GraphLane(lane)).Render(mark))
	}

	for i, live := range g.lanes {
		switch {
		case i == g.node:
			write(glyph, i)
		case slices.Contains(g.opened, i):
			corner := "╮"
			if i < g.node {
				corner = "╭"
			}
			write(corner, i)
		case slices.Contains(g.closed, i):
			// The lane comes from above and bends towards the node it met.
			corner := "╯"
			if i < g.node {
				corner = "╰"
			}
			write(corner, i)
		case live:
			write("│", i)
		default:
			write(" ", i)
		}

		// The gap between two columns carries the run, so the corner it ends
		// on is reached.
		if i >= lo && i < hi {
			write("─", hi)
			continue
		}
		write(" ", i)
	}
	return p.String(), s.String()
}
