package tui

import (
	"sort"

	"github.com/o19k/git-gui/internal/git"
)

// Marking turns the file list into a set: what is marked is what the next
// action reaches. With nothing marked, an action means the file under the
// cursor.

// toggleFileMark adds the selected path to the set, or takes it back out.
func (m *Model) toggleFileMark() {
	file, ok := m.selectedFile()
	if !ok {
		return
	}
	// Copy: the map is shared with the model this update came from.
	marks := make(map[string]bool, len(m.fileMarks)+1)
	for k, v := range m.fileMarks {
		marks[k] = v
	}
	marks[file.Path] = !marks[file.Path]
	m.fileMarks = marks
}

// markedFiles is what an action should act on: everything marked, or the
// selection when nothing is.
func (m Model) markedFiles() []git.FileChange {
	var paths []string
	for path, marked := range m.fileMarks {
		if marked {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		// Map order is random; a confirm that names files must be stable.
		sort.Strings(paths)
		var files []git.FileChange
		for _, path := range paths {
			for _, file := range m.files() {
				if file.Path == path {
					files = append(files, file)
					break
				}
			}
		}
		return files
	}
	if file, ok := m.selectedFile(); ok {
		return []git.FileChange{file}
	}
	return nil
}

// explicitMarks is what was marked by hand, in a stable order, and nothing
// when nothing was.
func (m Model) explicitMarks() []string {
	var marked []string
	for path, on := range m.fileMarks {
		if on {
			marked = append(marked, path)
		}
	}
	sort.Strings(marked)
	return marked
}

// paths is the marked set as a confirm can name it.
func paths(files []git.FileChange) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// clearFileMarks empties the set, which every action does once it has run.
func (m *Model) clearFileMarks() {
	m.fileMarks = nil
}

// markPrefix is the column drawn ahead of a file, showing whether it is marked.
func (m Model) markPrefix(f git.FileChange) string {
	if m.fileMarks[f.Path] {
		return "✓"
	}
	return " "
}
