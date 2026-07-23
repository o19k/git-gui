package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// The Explorer walks the repository as git sees it: the listing comes from
// git ls-files, so a directory step is a map lookup. An ignored tree has no
// git listing, so its entries are read from disk and marked as such.

// fsEntry is one row of a listing.
type fsEntry struct {
	Name   string
	Dir    bool
	Status byte // git's status letter, 0 when clean
	// Cached is whether the path has a row in the index — what a delete
	// branches on. Being listed does not imply it: the listing holds untracked
	// paths too.
	Cached  bool
	Ignored bool
	Link    bool
	Module  bool
	FromFS  bool // listed by ReadDir, not by ls-files
}

// readDirMsg carries a listing read from disk for a directory git does not
// cover. path tags it so a reply landing after the cursor moved is dropped.
type readDirMsg struct {
	path    string
	entries []fsEntry
	err     error
}

// explorerOverrides are the keys the Explorer takes over from the global
// switch. Two rules produce the list:
//
//   - An arrow means whatever its letter means, so h and l are taken together
//     with left and right. They step out of and into a directory; tab and
//     shift+tab still move the focus.
//   - Movement acts on the middle column whichever pane holds the focus. The
//     parent column's own cursor is a position nothing reads.
//
// Every other Explorer key must be free of the globals — asserted by the
// property test in keyspace_test.go.
var explorerOverrides = []string{
	"h", "left", "l", "right",
	"j", "down", "k", "up",
	"g", "G",
	"ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgdown", "pgup",
}

// explorerPanel reports whether a pane belongs to the Explorer.
func explorerPanel(p Panel) bool {
	return p == PanelParent || p == PanelEntries || p == PanelPreview
}

// buildIndex constructs the directory→children map from IndexTree, synthesizing
// intermediate directories as needed. It returns a fresh map each time.
func (m Model) buildIndex(entries []git.TreeEntry, ignored []string) map[string][]fsEntry {
	index := make(map[string][]fsEntry)

	statusMap := make(map[string]byte)
	for _, fc := range m.snap.Files {
		statusMap[fc.Path] = fc.Code()
	}

	seen := make(map[string]bool)
	for _, te := range entries {
		parts := strings.Split(strings.TrimPrefix(te.Path, "/"), "/")

		for i := 0; i < len(parts)-1; i++ {
			parent := strings.Join(parts[:i], "/")
			if parent == "" {
				parent = "."
			}
			child := parts[i]

			key := parent + "/" + child
			if !seen[key] {
				seen[key] = true
				index[parent] = append(index[parent], fsEntry{
					Name: child,
					Dir:  true,
				})
			}
		}

		parent := ""
		if len(parts) > 1 {
			parent = strings.Join(parts[:len(parts)-1], "/")
		} else {
			parent = "."
		}

		name := parts[len(parts)-1]
		status := statusMap[te.Path]

		entry := fsEntry{
			Name:   name,
			Status: status,
			Cached: te.Cached,
		}

		if te.Cached {
			switch te.Mode {
			case "160000":
				entry.Module = true
				entry.Dir = true
			case "120000":
				entry.Link = true
			}
		}

		index[parent] = append(index[parent], entry)
	}

	for _, ignoredPath := range ignored {
		parts := strings.Split(strings.TrimSuffix(ignoredPath, "/"), "/")

		for i := 0; i < len(parts)-1; i++ {
			parent := strings.Join(parts[:i], "/")
			if parent == "" {
				parent = "."
			}
			child := parts[i]

			key := parent + "/" + child
			if !seen[key] {
				seen[key] = true
				index[parent] = append(index[parent], fsEntry{
					Name:    child,
					Dir:     true,
					Ignored: true,
				})
			}
		}

		parent := ""
		if len(parts) > 1 {
			parent = strings.Join(parts[:len(parts)-1], "/")
		} else {
			parent = "."
		}

		name := parts[len(parts)-1]
		isDir := strings.HasSuffix(ignoredPath, "/")

		index[parent] = append(index[parent], fsEntry{
			Name:    name,
			Dir:     isDir,
			Ignored: true,
		})
	}

	// The directories were synthesised without a status of their own; this is
	// where they get one.
	rollUp(index, statusMap)

	for dir := range index {
		sortListing(index[dir], dir, m.sortMode, m.sortReverse, m.stats)
	}
	return index
}

// statusRank orders the status letters by how much they want attention. A
// directory shows the worst thing under it, so this ranking is what decides
// which of its children speaks for it.
func statusRank(status byte) int {
	switch status {
	case 'U':
		return 5
	case 'D':
		return 4
	case 'M':
		return 3
	case 'A':
		return 2
	case '?':
		return 1
	}
	return 0
}

// rollUp gives every directory the worst status found anywhere beneath it, at
// any depth. Walking up from each changed path costs one pass over the
// changes rather than a search per directory.
func rollUp(index map[string][]fsEntry, statusMap map[string]byte) {
	worst := make(map[string]byte, len(statusMap))
	for path, status := range statusMap {
		for dir := parentOf(path); dir != ""; dir = parentOf(dir) {
			if statusRank(status) > statusRank(worst[dir]) {
				worst[dir] = status
			}
			if dir == "." {
				break
			}
		}
	}

	for dir, children := range index {
		for i, e := range children {
			if !e.Dir {
				continue
			}
			if s, ok := worst[childPath(dir, e.Name)]; ok {
				index[dir][i].Status = s
			}
		}
	}
}

// parentOf is the directory holding path, with the repository root spelled "."
// and nothing above it.
func parentOf(path string) string {
	if path == "." || path == "" {
		return ""
	}
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return "."
}

// childPath joins a listing's directory to one of its entries.
func childPath(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// listingOf is a directory's children as listed, git first and disk second,
// with no filtering.
func (m Model) listingOf(dir string) []fsEntry {
	if dir == "" {
		dir = "."
	}
	if entries, ok := m.index[dir]; ok {
		return entries
	}
	return m.fsIndex[dir]
}

// listingText is a listing as plain rows, for previewing a directory.
func listingText(entries []fsEntry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.Name)
		if e.Dir {
			b.WriteByte('/')
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// entries is the middle column's listing, filtered.
func (m Model) entries() []fsEntry {
	var all []fsEntry
	if m.cwd == "" {
		m.cwd = "."
	}

	// git first, disk second: fsIndex is never rebuilt by the tick, so a disk
	// listing must not shadow the git one.
	if entries, ok := m.index[m.cwd]; ok {
		all = entries
	} else if entries, ok := m.fsIndex[m.cwd]; ok {
		all = entries
	}

	filter := m.filter[PanelEntries]

	// Runs once per pane per frame, so the common case — nothing hidden and
	// nothing typed — hands back the listing instead of copying it.
	if m.showHidden && filter == "" {
		return all
	}

	out := make([]fsEntry, 0, len(all))
	for _, e := range all {
		if !m.showHidden && strings.HasPrefix(e.Name, ".") {
			continue
		}
		if !matches(filter, e.Name) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// parentEntries is the left column's listing.
func (m Model) parentEntries() []fsEntry {
	// Above the root is the repository itself. Listing the root's children
	// here would put the same rows in two columns.
	if m.cwd == "" || m.cwd == "." {
		return []fsEntry{{Name: ".", Dir: true}}
	}
	return m.visible(m.listingOf(parentOf(m.cwd)))
}

// visible drops what the listing is currently hiding. Shared by both columns.
func (m Model) visible(all []fsEntry) []fsEntry {
	out := make([]fsEntry, 0, len(all))
	for _, e := range all {
		if !m.showHidden && strings.HasPrefix(e.Name, ".") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// explorerLen is panelLen for the Explorer's panes.
func (m Model) explorerLen(p Panel) int {
	switch p {
	case PanelParent:
		return len(m.parentEntries())
	case PanelEntries:
		return len(m.entries())
	}
	return 0
}

// explorerTitle is panelTitle for the Explorer's panes.
func (m Model) explorerTitle(p Panel) string {
	switch p {
	case PanelParent:
		return "Parent"
	case PanelEntries:
		title := "Files"
		if m.cwd != "" && m.cwd != "." {
			title = m.cwd
		}
		if _, ok := m.fsIndex[m.cwd]; ok {
			title += " (from disk)"
		}
		// An order other than by name is not visible in the rows, so the title
		// carries it.
		return title + m.sortLabel()
	case PanelPreview:
		// Nothing is selected yet, and an untitled box reads as a fault.
		if m.previewTitle == "" {
			return "Preview"
		}
		return m.previewTitle
	}
	return ""
}

// entryStyle is the style for one entry.
func entryStyle(e fsEntry) lipgloss.Style {
	if e.Ignored {
		return theme.DimStyle
	}

	if e.Status == 0 {
		return theme.NormalStyle
	}

	return lipgloss.NewStyle().Foreground(theme.StatusColor(e.Status))
}

// explorerLines is panelLines for the Explorer's panes.
func (m Model) explorerLines(p Panel, innerH, innerW int) []string {
	if p == PanelPreview {
		return m.previewPaneLines(innerH, innerW)
	}

	var entries []fsEntry
	switch p {
	case PanelParent:
		entries = m.parentEntries()
	case PanelEntries:
		entries = m.entries()
	default:
		return nil
	}

	focused := m.focus == p
	n := len(entries)
	if n == 0 {
		if p != PanelEntries {
			return emptyLines("")
		}
		// A directory neither listing covers has a disk read still out —
		// saying it is empty would be wrong.
		_, inIndex := m.index[m.cwd]
		listing, onDisk := m.fsIndex[m.cwd]
		switch {
		case !inIndex && !onDisk:
			return emptyLines("listing…")
		case onDisk && len(listing) == 0:
			return emptyLines("empty")
		}
		return emptyLines("")
	}

	start, end, _ := window(n, m.cursor[p], m.offset[p], innerH)

	var lines []string
	for i := start; i < end; i++ {
		e := entries[i]
		style := entryStyle(e)

		name := e.Name
		if e.Dir {
			name += "/"
		}
		if e.Module {
			name = "⊞ " + name
		}
		if e.Link {
			name = "→ " + name
		}

		display := name
		if i == m.cursor[p] && focused {
			display = " " + display
		} else {
			display = "  " + display
		}

		line := style.Render(display)
		lines = append(lines, line)
	}

	return lines
}

// explorerEnd is larger than any listing or file, so clamping a move by it
// lands on the far end.
const explorerEnd = 1 << 30

// scrolled is the pane a movement key acts on: the preview when it holds the
// focus, and the middle column from either of the other two.
func (m Model) scrolled() Panel {
	if m.focus == PanelPreview {
		return PanelPreview
	}
	return PanelEntries
}

// explorerDelta is how far a movement key moves, in rows of the pane it acts on.
func (m Model) explorerDelta(key string) int {
	page := max(m.paneHeight(m.scrolled())-2, 1)

	switch key {
	case "j", "down":
		return 1
	case "k", "up":
		return -1
	case "g":
		return -explorerEnd
	case "G":
		return explorerEnd
	case "ctrl+d":
		return max(page/2, 1)
	case "ctrl+u":
		return -max(page/2, 1)
	case "ctrl+f", "pgdown":
		return page
	case "ctrl+b", "pgup":
		return -page
	}
	return 0
}

// explorerScroll moves the preview's viewport or the middle column's selection.
func (m Model) explorerScroll(delta int) (tea.Model, tea.Cmd) {
	if m.scrolled() == PanelPreview {
		m.previewOffset = m.clampPreviewOffset(m.previewOffset + delta)
		return m, nil
	}

	n := len(m.entries())
	if n == 0 {
		return m, nil
	}
	m.cursor[PanelEntries] = clamp(m.cursor[PanelEntries]+delta, 0, n-1)
	m.syncOffset(PanelEntries)
	return m, m.refreshPreview()
}

// previewLen is how many rows the preview holds. Blame arrives as lines and
// everything else as text, so the kind decides which field to measure.
func (m Model) previewLen() int {
	if m.previewFor.kind == previewBlame {
		return len(m.previewLines)
	}
	if m.previewContent == "" {
		return 0
	}
	// The side-by-side view pairs lines, so it is the shorter of the two.
	if m.previewFor.kind == previewDiff && m.splitDiff {
		return splitLineCount(m.previewContent)
	}
	return len(strings.Split(strings.TrimRight(m.previewContent, "\n"), "\n"))
}

// clampPreviewOffset keeps the last row on screen.
func (m Model) clampPreviewOffset(v int) int {
	return clamp(v, 0, max(m.previewLen()-(m.paneHeight(PanelPreview)-2), 0))
}

// explorerKey handles a key pressed with an Explorer pane focused.
func (m Model) explorerKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "h", "left":
		// The preview is the column right of the listing, so leaving it
		// leftwards lands on the listing rather than stepping out.
		if m.focus == PanelPreview {
			m.focus = PanelEntries
			return m, nil, true
		}
		if m.cwd == "" || m.cwd == "." {
			return m, nil, true
		}
		// Leaving remembers the position as much as entering does.
		m.dirCursor[m.cwd] = m.cursor[PanelEntries]
		parts := strings.Split(m.cwd, "/")
		m.cwd = "."
		if len(parts) > 1 {
			m.cwd = strings.Join(parts[:len(parts)-1], "/")
		}
		m.cursor[PanelEntries] = m.dirCursor[m.cwd]
		m.syncOffset(PanelEntries)
		// Split from the return: explorerMoved mutates m, and evaluation order
		// inside a single return statement is unspecified.
		cmd := m.explorerMoved()
		return m, cmd, true

	case "l", "right", "enter":
		entries := m.entries()
		if len(entries) == 0 {
			return m, nil, true
		}
		e := entries[m.cursor[PanelEntries]]

		// The column right of a file is its preview, which is the only way to
		// reach the pane a long file is scrolled in. enter opens the file.
		if !e.Dir {
			if key != "enter" {
				if m.previewLen() > 0 {
					m.focus = PanelPreview
				}
				return m, nil, true
			}
			next, cmd := m.openInEditor(childPath(m.cwd, e.Name))
			return next, cmd, true
		}

		if e.Module {
			m.status = "cannot descend into submodule"
			return m, nil, true
		}

		m.dirCursor[m.cwd] = m.cursor[PanelEntries]

		if m.cwd == "." {
			m.cwd = e.Name
		} else {
			m.cwd = m.cwd + "/" + e.Name
		}

		m.cursor[PanelEntries] = m.dirCursor[m.cwd]

		if _, inIndex := m.index[m.cwd]; !inIndex {
			if _, inFsIndex := m.fsIndex[m.cwd]; !inFsIndex {
				return m, m.readDir(m.cwd), true
			}
		}

		m.syncOffset(PanelEntries)
		// Split from the return: explorerMoved mutates m, and evaluation order
		// inside a single return statement is unspecified.
		cmd := m.explorerMoved()
		return m, cmd, true

	case "j", "down", "k", "up", "g", "G",
		"ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgdown", "pgup":
		next, cmd := m.explorerScroll(m.explorerDelta(key))
		return next, cmd, true

	case ".":
		// One flag serves both columns. The cursor is clamped rather than
		// kept: the entry under it may be the dotfile that just disappeared.
		m.showHidden = !m.showHidden
		m.cursor[PanelEntries] = clamp(m.cursor[PanelEntries], 0, max(len(m.entries())-1, 0))
		m.syncOffset(PanelEntries)
		return m, m.refreshPreview(), true

	case "e":
		return m, m.cyclePreview(), true

	case ",":
		m.askSort()
		return m, nil, true

	case "O":
		path, ok := m.explorerPath()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.showInChanges(path)
		return next, cmd, true

	default:
		if next, cmd, handled := m.explorerOpKey(key); handled {
			return next, cmd, true
		}
		if next, cmd, handled := m.explorerSearchKey(key); handled {
			return next, cmd, true
		}
	}

	return m, nil, false
}

// readDir issues a command to list a directory from disk.
func (m Model) readDir(dir string) tea.Cmd {
	repo := m.repo

	// Read here, on the update goroutine: reading it from the model inside the
	// command would race the key that changes it.
	mode, reverse, stats := m.sortMode, m.sortReverse, m.stats

	return func() tea.Msg {
		repoPath := repo.Path
		absDir := filepath.Join(repoPath, dir)

		entries, err := os.ReadDir(absDir)
		if err != nil {
			return readDirMsg{path: dir, err: err}
		}

		var fsEntries []fsEntry
		for _, e := range entries {
			entry := fsEntry{
				Name:   e.Name(),
				FromFS: true,
			}
			if e.IsDir() {
				entry.Dir = true
			} else {
				info, _ := e.Info()
				if info != nil && (info.Mode()&os.ModeSymlink) != 0 {
					entry.Link = true
				}
			}
			fsEntries = append(fsEntries, entry)
		}

		sortListing(fsEntries, dir, mode, reverse, stats)

		return readDirMsg{path: dir, entries: fsEntries, err: nil}
	}
}

// explorerKeyHints is the footer for the Explorer's panes.
func explorerKeyHints() [][2]string {
	return [][2]string{
		{"h/l", "out/in · l on a file reads it"}, {"enter", "edit"}, {"e", "view"}, {"o", "open"},
		{"s", "search"}, {",", "sort"}, {".", "hidden"},
	}
}

// loadIndex reads the repository's tree and the ignored prefixes. Issued on
// tab entry and on the tick, outside the tick's in-flight guard.
func (m Model) loadIndex() tea.Cmd {
	repo := m.repo
	ctx := m.ctx

	// The model is built before a repository is opened.
	if repo == nil {
		return nil
	}

	return func() tea.Msg {
		entries, err := repo.IndexTree(ctx)
		if err != nil {
			return loadIndexMsg{err: err}
		}

		ignored, err := repo.IgnoredPrefixes(ctx)
		if err != nil {
			return loadIndexMsg{err: err}
		}

		return loadIndexMsg{entries: entries, ignored: ignored}
	}
}

type loadIndexMsg struct {
	entries []git.TreeEntry
	ignored []string
	err     error
}

// handleLoadIndex updates the index and fsIndex from a load.
func (m Model) handleLoadIndex(msg loadIndexMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	// The remembered position is for landing in a directory, not for every
	// refresh of one — this also runs on the tick.
	landing := len(m.listingOf(m.cwd)) == 0

	m.index = m.buildIndex(msg.entries, msg.ignored)
	m.ignored = msg.ignored

	m.clampCursors()
	if cursor, ok := m.dirCursor[m.cwd]; ok && landing {
		m.cursor[PanelEntries] = cursor
		m.syncOffset(PanelEntries)
	}

	// Another tab asked for a path before there was a listing to find it in.
	if m.pendingPath != "" {
		path := m.pendingPath
		m.pendingPath = ""
		return m.handleNavigate(navigateMsg{path: path})
	}

	cmd := m.explorerMoved()
	return m, cmd
}

// handleReadDir installs a listing read from disk.
func (m Model) handleReadDir(msg readDirMsg) (tea.Model, tea.Cmd) {
	if msg.path != m.cwd {
		return m, nil
	}

	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	// A re-list of a directory already on screen — what R asks for — is not a
	// landing.
	landing := len(m.listingOf(msg.path)) == 0

	m.fsIndex[msg.path] = msg.entries

	if landing {
		if cursor, ok := m.dirCursor[msg.path]; ok {
			m.cursor[PanelEntries] = cursor
		} else {
			m.cursor[PanelEntries] = 0
		}
	}

	m.syncOffset(PanelEntries)
	cmd := m.explorerMoved()
	return m, cmd
}
