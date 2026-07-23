package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The listing is ordered by name until asked otherwise. Size and modified
// time need something git does not carry, so they stat the filesystem once
// per directory rather than once per frame.

type sortMode int

const (
	sortName sortMode = iota
	sortStatus
	sortExtension
	sortSize
	sortTime
)

// sortNames name the orders in the panel title and in the picker.
var sortNames = map[sortMode]string{
	sortName:      "name",
	sortStatus:    "git status",
	sortExtension: "extension",
	sortSize:      "size",
	sortTime:      "modified",
}

// needsStat reports whether an order asks the filesystem something the index
// cannot answer.
func (s sortMode) needsStat() bool { return s == sortSize || s == sortTime }

// fileMeta is what a stat is kept for. Nothing else is read: the two orders
// are the only reason the numbers exist.
type fileMeta struct {
	size  int64
	mtime time.Time
}

// statsMsg carries a directory's stats. dir tags it so a reply landing after
// the listing was rebuilt is merged rather than mistaken for the current one.
type statsMsg struct {
	dir  string
	meta map[string]fileMeta
}

// sortListing orders one directory's children in place. Directories stay
// above files in every order, including reversed.
func sortListing(entries []fsEntry, dir string, mode sortMode, reverse bool, stats map[string]fileMeta) {
	sign := 1
	if reverse {
		sign = -1
	}

	slices.SortStableFunc(entries, func(a, b fsEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		if by := sign * compareBy(a, b, dir, mode, stats); by != 0 {
			return by
		}
		// Name is the tie-break under every order, so the listing is stable
		// across rebuilds.
		return sign * strings.Compare(a.Name, b.Name)
	})
}

// compareBy is the ordering the mode names, zero where it cannot tell the two
// apart. A stat that has not arrived compares equal, which leaves the entries
// in name order until it does.
func compareBy(a, b fsEntry, dir string, mode sortMode, stats map[string]fileMeta) int {
	switch mode {
	case sortStatus:
		// The same ranking the listing colours by. Highest first: the ranking
		// runs from clean upwards.
		return statusRank(b.Status) - statusRank(a.Status)

	case sortExtension:
		return strings.Compare(extensionOf(a.Name), extensionOf(b.Name))

	case sortSize:
		am, bm := stats[childPath(dir, a.Name)], stats[childPath(dir, b.Name)]
		// Largest first.
		return int(min(max(bm.size-am.size, -1), 1))

	case sortTime:
		am, bm := stats[childPath(dir, a.Name)], stats[childPath(dir, b.Name)]
		return bm.mtime.Compare(am.mtime) // newest first
	}
	return 0
}

// extensionOf is the suffix an entry sorts under. A dotfile has none: ".gitignore"
// is a name that begins with a dot, not a file of type "gitignore".
func extensionOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[i+1:]
	}
	return ""
}

// askSort offers the orders as a menu; there are no two-key bindings here to
// spell them as a sequence.
func (m *Model) askSort() {
	// Read here rather than inside the action: the action runs against whatever
	// copy the closure caught.
	current := m.sortMode

	var choices []choice
	for _, mode := range []sortMode{sortName, sortStatus, sortExtension, sortSize, sortTime} {
		label := sortNames[mode]
		if mode == current {
			label += " ✓"
		}
		choices = append(choices, choice{
			label:  label,
			action: func() tea.Cmd { return func() tea.Msg { return sortMsg{mode: mode} } },
		})
	}
	choices = append(choices, choice{
		label:  "reverse",
		hint:   "flip whichever order is set",
		action: func() tea.Cmd { return func() tea.Msg { return sortMsg{mode: current, flip: true} } },
	})

	m.askChoice("Sort by", "Directories stay above files in every order.", choices)
}

// sortMsg carries a pick out of the menu. The choice travels as a message: an
// action runs after Update has returned, so it cannot write to the live model.
type sortMsg struct {
	mode sortMode
	flip bool
}

func (m Model) handleSort(msg sortMsg) (tea.Model, tea.Cmd) {
	m.sortMode = msg.mode
	if msg.flip {
		m.sortReverse = !m.sortReverse
	}
	m.resortAll()

	// The order may want numbers no listing has yet.
	cmd := m.explorerMoved()
	return m, cmd
}

// resortAll re-orders every listing held. The maps and their slices are shared
// with earlier copies of the model, so both are replaced rather than written
// through.
func (m *Model) resortAll() {
	m.index = resorted(m.index, m.sortMode, m.sortReverse, m.stats)
	m.fsIndex = resorted(m.fsIndex, m.sortMode, m.sortReverse, m.stats)
}

func resorted(in map[string][]fsEntry, mode sortMode, reverse bool, stats map[string]fileMeta) map[string][]fsEntry {
	out := make(map[string][]fsEntry, len(in))
	for dir, entries := range in {
		sorted := slices.Clone(entries)
		sortListing(sorted, dir, mode, reverse, stats)
		out[dir] = sorted
	}
	return out
}

// statDir reads the sizes and times one directory's listing sorts by, and nil
// unless the order needs them and something is missing.
func (m Model) statDir(dir string) tea.Cmd {
	if !m.sortMode.needsStat() || m.repo == nil {
		return nil
	}
	entries := m.listingOf(dir)
	if len(entries) == 0 {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if _, known := m.stats[childPath(dir, e.Name)]; !known {
			names = append(names, e.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}

	root := m.repo.Path
	return func() tea.Msg {
		meta := make(map[string]fileMeta, len(names))
		for _, name := range names {
			path := childPath(dir, name)
			// Lstat, not Stat: the row stands for the symlink itself, not for
			// whatever it points at.
			info, err := os.Lstat(filepath.Join(root, path))
			if err != nil {
				continue
			}
			meta[path] = fileMeta{size: info.Size(), mtime: info.ModTime()}
		}
		return statsMsg{dir: dir, meta: meta}
	}
}

func (m Model) handleStats(msg statsMsg) (tea.Model, tea.Cmd) {
	if len(msg.meta) == 0 {
		return m, nil
	}

	stats := make(map[string]fileMeta, len(m.stats)+len(msg.meta))
	maps.Copy(stats, m.stats)
	maps.Copy(stats, msg.meta)
	m.stats = stats

	// Only the directory the numbers are about needs re-ordering.
	if entries := m.listingOf(msg.dir); len(entries) > 0 {
		sorted := slices.Clone(entries)
		sortListing(sorted, msg.dir, m.sortMode, m.sortReverse, m.stats)
		m.putListing(msg.dir, sorted)
	}
	m.clampCursors()
	return m, nil
}

// putListing installs one directory's rows without writing through a map an
// earlier copy of the model is still holding.
func (m *Model) putListing(dir string, entries []fsEntry) {
	if _, ok := m.index[dir]; ok {
		next := make(map[string][]fsEntry, len(m.index))
		maps.Copy(next, m.index)
		next[dir] = entries
		m.index = next
		return
	}
	if _, ok := m.fsIndex[dir]; ok {
		next := make(map[string][]fsEntry, len(m.fsIndex))
		maps.Copy(next, m.fsIndex)
		next[dir] = entries
		m.fsIndex = next
	}
}

// explorerMoved is what follows a step to another directory or another row:
// the preview the selection asks for, and the stats the order asks for. Both
// columns are stated, since they share one order.
func (m *Model) explorerMoved() tea.Cmd {
	cmds := []tea.Cmd{m.refreshPreview(), m.statDir(m.cwd)}
	if m.cwd != "" && m.cwd != "." {
		cmds = append(cmds, m.statDir(parentOf(m.cwd)))
	}
	return tea.Batch(cmds...)
}

// sortLabel names the order in the listing's title. Name order says nothing,
// being what a listing looks like anyway.
func (m Model) sortLabel() string {
	if m.sortMode == sortName && !m.sortReverse {
		return ""
	}
	label := fmt.Sprintf(" · by %s", sortNames[m.sortMode])
	if m.sortReverse {
		label += " ↑"
	}
	return label
}
