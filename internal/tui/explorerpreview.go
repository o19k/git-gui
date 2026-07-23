package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// previewKind is what the Explorer's right column shows about the selected
// path. The kind on landing follows what the file is: a modified file opens on
// its diff, a clean one on its content.
type previewKind int

const (
	previewContent previewKind = iota
	previewDiff
	previewBlame
	previewHistory
)

// previewID tags a preview read with what it is a preview of. A reply whose id
// no longer matches the selection is dropped on arrival.
type previewID struct {
	path string
	kind previewKind
}

// explorerPreviewMsg is separate from previewMsg, whose handler writes Local
// Changes' fields.
type explorerPreviewMsg struct {
	id      previewID
	title   string
	content string
	lines   []git.BlameLine

	// styled is the content coloured as source, and empty when it is not: a
	// symlink's target and a directory's listing are content too.
	styled []string
	err    error
}

// refreshExplorerPreview issues the read for the current selection.
func (m *Model) refreshExplorerPreview() tea.Cmd {
	repo := m.repo

	var entry fsEntry
	var path string
	var found bool

	entries := m.entries()
	if len(entries) > 0 && m.cursor[PanelEntries] < len(entries) {
		entry = entries[m.cursor[PanelEntries]]
		found = true
		// childPath rather than a join: at the root cwd is ".", and nothing
		// spells the path "./a.txt".
		path = childPath(m.cwd, entry.Name)
	}

	if !found {
		m.previewFor = previewID{}
		m.previewContent = ""
		m.previewTitle = ""
		m.previewOffset = 0
		m.previewStyled = nil
		return nil
	}

	kind := previewContent
	if !entry.Dir && !entry.Link && !entry.Module {
		for _, fc := range m.snap.Files {
			if fc.Path == path {
				kind = previewDiff
				break
			}
		}
	}

	// What the file is decides the kind on landing, and only on landing: the
	// poll runs this every few seconds, and recomputing would drop a blame or
	// a history back to the diff.
	landed := m.previewFor.path != path
	if !landed {
		kind = m.previewFor.kind
	}

	m.previewFor = previewID{path: path, kind: kind}
	if landed {
		m.previewOffset = 0
		// The colouring belongs to the content it was made from.
		m.previewStyled = nil
	}

	// A directory previews the listing the middle column would draw, which is
	// already in memory.
	if entry.Dir {
		m.previewTitle = "Contents — " + entry.Name
		m.previewContent = listingText(m.listingOf(path))
		if m.previewContent == "" {
			// git listing nothing under it is not the same as it holding
			// nothing — an ignored tree is full and unlisted.
			m.previewContent = "nothing git tracks here — press l to read it from disk"
			if !entry.Ignored {
				m.previewContent = "empty"
			}
		}
		return nil
	}

	// A symlink previews where it points and is never followed: the target is
	// not in the listing, and following would have to answer for cycles.
	if entry.Link {
		m.previewTitle = "Link — " + entry.Name
		id, title := m.previewFor, m.previewTitle
		return func() tea.Msg {
			target, err := os.Readlink(repo.Path + "/" + path)
			// Every path in the listing is written with slashes; a link's
			// target comes back in the host's separator.
			return explorerPreviewMsg{id: id, title: title, content: filepath.ToSlash(target), err: err}
		}
	}

	// Submodules are marked and do not descend, so no preview.
	if entry.Module {
		m.previewTitle = "Submodule — " + entry.Name
		m.previewContent = "(submodule)"
		return nil
	}

	m.previewTitle = previewHeading(kind, entry.Name)
	return m.previewCommand()
}

// previewHeading names the pane for what it is showing. Shared by the two
// places that set the title: landing on a path, and cycling the view of one.
func previewHeading(kind previewKind, name string) string {
	switch kind {
	case previewDiff:
		return "Diff — " + name
	case previewBlame:
		return "Blame — " + name
	case previewHistory:
		return "History — " + name
	}
	return "Content — " + name
}

// formatSize formats a byte count as a human-readable size.
func formatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(bytes)
	for _, unit := range units {
		if size < 1024 {
			if unit == "B" {
				return fmt.Sprintf("%d %s", int(size), unit)
			}
			return formatFloatSize(size, 1) + " " + unit
		}
		size /= 1024
	}
	return formatFloatSize(size, 1) + " TB"
}

// formatFloatSize formats a float size with the given decimal places, trimming trailing zeros.
func formatFloatSize(size float64, decimals int) string {
	format := fmt.Sprintf("%%.%df", decimals)
	result := strings.TrimRight(strings.TrimRight(fmt.Sprintf(format, size), "0"), ".")
	return result
}

// handleExplorerPreview installs a preview that is still current.
func (m Model) handleExplorerPreview(msg explorerPreviewMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.previewFor {
		return m, nil
	}
	if msg.err != nil {
		m.status = msg.err.Error()
		m.previewContent = ""
		return m, nil
	}
	m.previewTitle = msg.title
	m.previewContent = msg.content

	m.previewStyled = msg.styled
	if len(msg.lines) > 0 {
		m.previewLines = msg.lines
	}

	// Clamped rather than zeroed: this read is as often the poll's as a new
	// selection's. Landing on a new path resets it where the read is issued.
	m.previewOffset = m.clampPreviewOffset(m.previewOffset)

	// A search asked for a line, and this is the first content to measure it
	// against.
	if m.pendingLine > 0 {
		m.previewOffset = m.clampPreviewOffset(m.pendingLine - 1)
		m.pendingLine = 0
	}
	return m, nil
}

// cyclePreview moves to the next preview kind.
func (m *Model) cyclePreview() tea.Cmd {
	if m.previewFor.path == "" {
		return nil
	}

	m.previewFor.kind = (m.previewFor.kind + 1) % 4
	m.previewOffset = 0
	m.previewStyled = nil

	// The heading moves with the view rather than arriving with the read,
	// which carries the kind it was issued under.
	name := m.previewFor.path
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	m.previewTitle = previewHeading(m.previewFor.kind, name)

	return m.previewCommand()
}

// previewCommand issues a read command for the current preview selection.
func (m *Model) previewCommand() tea.Cmd {
	repo, ctx := m.repo, m.ctx

	// The model is built before a repository is opened.
	if repo == nil {
		return nil
	}

	// Copied out rather than read off the model inside the closure: the command
	// runs after Update has returned.
	id := m.previewFor
	path, title := id.path, m.previewTitle
	palette := currentSyntax()

	switch m.previewFor.kind {
	case previewContent:
		return func() tea.Msg {
			data, err := os.ReadFile(repo.Path + "/" + path)
			if err != nil {
				return explorerPreviewMsg{id: id, title: title, err: err}
			}

			// Check for binary by looking for NUL byte in first 8 KiB.
			checkLen := len(data)
			if checkLen > 8192 {
				checkLen = 8192
			}
			isBinary := false
			for i := 0; i < checkLen; i++ {
				if data[i] == 0 {
					isBinary = true
					break
				}
			}

			if isBinary {
				sizeStr := formatSize(int64(len(data)))
				content := "binary, " + sizeStr
				return explorerPreviewMsg{id: id, title: title, content: content}
			}

			// Cap at 256 KiB and 2000 lines.
			const maxBytes = 256 * 1024
			if len(data) > maxBytes {
				data = data[:maxBytes]
			}

			content := string(data)
			lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
			if len(lines) > 2000 {
				lines = lines[:2000]
			}
			content = strings.Join(lines, "\n")

			return explorerPreviewMsg{
				id: id, title: title, content: content,
				styled: highlight(path, content, palette),
			}
		}

	case previewDiff:
		return func() tea.Msg {
			content, err := repo.Diff(ctx, path, false)
			return explorerPreviewMsg{id: id, title: title, content: content, err: err}
		}

	case previewBlame:
		return func() tea.Msg {
			lines, err := repo.Blame(ctx, path)
			return explorerPreviewMsg{
				id: id, title: title, lines: lines,
				styled: highlightBlame(path, lines, palette), err: err,
			}
		}

	case previewHistory:
		return func() tea.Msg {
			content, err := repo.FileLog(ctx, path, 200)
			if err == nil {
				content = renderPrettyLog(content)
			}
			return explorerPreviewMsg{id: id, title: title, content: content, err: err}
		}
	}

	return nil
}

// previewPaneLines renders the right column.
func (m Model) previewPaneLines(innerH, innerW int) []string {
	// What is drawn follows previewFor, not the cursor: the content belongs to
	// the path the read was tagged with.
	if m.previewFor.path == "" {
		return emptyLines("nothing selected")
	}

	if m.previewFor.kind == previewBlame {
		if len(m.previewLines) == 0 {
			return emptyLines("annotating…")
		}
		return renderBlame(m.previewLines, m.previewStyled, m.previewOffset, innerH, innerW)
	}

	if m.previewContent == "" {
		return emptyLines("loading…")
	}

	switch m.previewFor.kind {
	case previewDiff:
		// One setting for the whole program, so a patch lays out here the way
		// it does in Local Changes. A history is a list of commits, not a
		// patch, and has no second side.
		if m.splitDiff {
			return splitDiffLines(m.previewContent, m.previewOffset, innerH, innerW)
		}
		return diffLines(m.previewContent, m.previewOffset, innerH)
	case previewHistory:
		return diffLines(m.previewContent, m.previewOffset, innerH)
	}

	// Plain text scrolls by the offset alone: a preview has no selection for
	// window's cursor to track.
	all := m.previewStyled
	if len(all) == 0 {
		all = strings.Split(strings.TrimRight(m.previewContent, "\n"), "\n")
	}
	start := clamp(m.previewOffset, 0, max(len(all)-1, 0))
	return all[start:min(start+innerH, len(all))]
}
