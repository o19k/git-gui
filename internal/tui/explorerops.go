package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// The Explorer's operations use each path's git spelling whenever it has one:
// renaming a tracked file is git mv, deleting one is git rm. changeFor bridges
// the Explorer's paths to the FileChange the git mutators take.

// changeFor resolves a path to the FileChange the mutators expect. Three
// outcomes: a path with a working-tree change comes from the snapshot
// verbatim; a path in the index but clean gets Index '.', so a delete goes
// through git rm; a path in neither gets '?', so a delete unlinks.
//
// Directories must not be resolved here — they are never in ls-files
// --cached, so they would come back '?' and route a delete to os.Remove,
// which fails on anything non-empty. The call site handles them.
func (m Model) changeFor(path string) (git.FileChange, bool) {
	for _, file := range m.snap.Files {
		if file.Path == path {
			return file, true
		}
	}

	// Nothing in the working tree, so the only question is whether git holds a
	// copy. The listing carries untracked paths too, so it cannot answer.
	if e, ok := m.entryAt(path); ok && e.Cached {
		return git.FileChange{Index: '.', Work: '.', Path: path}, true
	}
	return git.FileChange{Index: '?', Work: '?', Path: path}, true
}

// entryAt finds a path's listing entry, wherever it was listed from.
func (m Model) entryAt(path string) (fsEntry, bool) {
	dir, name := parentOf(path), filepath.Base(path)
	if dir == "" {
		dir = "."
	}
	for _, from := range []map[string][]fsEntry{m.index, m.fsIndex} {
		for _, e := range from[dir] {
			if e.Name == name {
				return e, true
			}
		}
	}
	return fsEntry{}, false
}

// markedPaths is what the next operation reaches: the marked set if there is
// one, otherwise the selection. Paths rather than FileChanges, because the
// Explorer lists clean files too.
func (m Model) markedPaths() []string {
	if len(m.explorerMarks) > 0 {
		var paths []string
		for path := range m.explorerMarks {
			paths = append(paths, path)
		}
		return paths
	}

	entries := m.entries()
	if len(entries) == 0 {
		return nil
	}
	if m.cursor[PanelEntries] >= len(entries) {
		return nil
	}

	selected := entries[m.cursor[PanelEntries]]
	if m.cwd == "" {
		return []string{selected.Name}
	}
	return []string{filepath.Join(m.cwd, selected.Name)}
}

func (m *Model) toggleExplorerMark() {
	entries := m.entries()
	if len(entries) == 0 || m.cursor[PanelEntries] >= len(entries) {
		return
	}

	selected := entries[m.cursor[PanelEntries]]
	path := selected.Name
	if m.cwd != "" {
		path = filepath.Join(m.cwd, selected.Name)
	}

	if m.explorerMarks == nil {
		m.explorerMarks = make(map[string]bool)
	}

	if m.explorerMarks[path] {
		delete(m.explorerMarks, path)
	} else {
		m.explorerMarks[path] = true
	}
}

// explorerOpKey handles the operation keys. Reported unhandled when the key is
// not one of them.
func (m Model) explorerOpKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx

	switch key {
	case "n":
		m.askInput("New file", "", func(name string) tea.Cmd {
			if name == "" {
				return nil
			}
			path := filepath.Join(m.cwd, name)
			return m.do("create file", func() error {
				return os.WriteFile(filepath.Join(repo.Path, path), []byte{}, 0o644)
			})
		})
		return m, nil, true

	case "N":
		m.askInput("New directory", "", func(name string) tea.Cmd {
			if name == "" {
				return nil
			}
			path := filepath.Join(m.cwd, name)
			return m.do("create directory", func() error {
				return os.MkdirAll(filepath.Join(repo.Path, path), 0o755)
			})
		})
		return m, nil, true

	case "m":
		paths := m.markedPaths()
		if len(paths) == 0 || len(paths) > 1 {
			return m, nil, true
		}
		oldPath := paths[0]
		oldName := filepath.Base(oldPath)
		m.askInput("Rename to", oldName, func(newName string) tea.Cmd {
			if newName == "" || newName == oldName {
				return nil
			}
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			return m.do("rename", func() error {
				tracked := m.isPathTracked(oldPath)
				return repo.Move(ctx, oldPath, newPath, tracked)
			})
		})
		return m, nil, true

	case "x":
		paths := m.markedPaths()
		if len(paths) == 0 {
			return m, nil, true
		}

		// A tracked file is recoverable from the index or from history; an
		// untracked one is gone for good. The question says which it is.
		var confirmMsg string
		if len(paths) == 1 {
			p := paths[0]
			fullPath := filepath.Join(repo.Path, p)
			info, err := os.Stat(fullPath)
			isDir := err == nil && info.IsDir()

			confirmMsg = fmt.Sprintf("Delete %q?", p)
			if isDir {
				count, _ := countFilesInDir(fullPath)
				confirmMsg = fmt.Sprintf("Delete %q and everything in it (%d files)?", p, count)
			}
			if !m.isPathTracked(p) {
				confirmMsg += " git holds no copy of it."
			}
		} else {
			confirmMsg = fmt.Sprintf("Delete %d items?", len(paths))
			var untracked int
			for _, p := range paths {
				if !m.isPathTracked(p) {
					untracked++
				}
			}
			if untracked > 0 {
				confirmMsg += fmt.Sprintf(" git holds no copy of %d of them.", untracked)
			}
		}

		danger := true
		m.askConfirm("Delete", confirmMsg, danger, func() tea.Cmd {
			return m.do("delete", func() error {
				for _, p := range paths {
					fullPath := filepath.Join(repo.Path, p)
					info, err := os.Stat(fullPath)
					isDir := err == nil && info.IsDir()

					if isDir {
						// A directory has no index row of its own, so whether
						// git holds a copy is a question about its children.
						// changeFor would answer "untracked" for every one.
						if !m.isPathTracked(p) {
							if err := os.RemoveAll(fullPath); err != nil {
								return err
							}
						} else {
							change := git.FileChange{Index: '.', Work: '.', Path: p}
							if err := repo.DeleteFile(ctx, change); err != nil {
								return err
							}
						}
					} else {
						change, _ := m.changeFor(p)
						if err := repo.DeleteFile(ctx, change); err != nil {
							return err
						}
					}
				}
				return nil
			})
		})
		return m, nil, true

	case "d":
		paths := m.markedPaths()
		if len(paths) == 0 {
			return m, nil, true
		}

		if len(paths) > 1 {
			m.status = "discard works on one file at a time"
			return m, nil, true
		}

		path := paths[0]
		confirmMsg := fmt.Sprintf("Throw away changes to %q?", path)

		m.askConfirm("Discard", confirmMsg, true, func() tea.Cmd {
			return m.do("discard", func() error {
				change, _ := m.changeFor(path)
				return repo.Discard(ctx, change)
			})
		})
		return m, nil, true

	case " ":
		paths := m.markedPaths()
		if len(paths) == 0 {
			return m, nil, true
		}

		allStaged := true
		for _, p := range paths {
			if isStaged, ok := m.isPathStaged(p); !ok || !isStaged {
				allStaged = false
				break
			}
		}

		if allStaged {
			return m, m.do("unstage", func() error {
				for _, p := range paths {
					if err := repo.Unstage(ctx, p); err != nil {
						return err
					}
				}
				return nil
			}), true
		}

		return m, m.do("stage", func() error {
			for _, p := range paths {
				if err := repo.Stage(ctx, p); err != nil {
					return err
				}
			}
			return nil
		}), true

	case "i":
		paths := m.markedPaths()
		if len(paths) == 0 {
			return m, nil, true
		}

		return m, m.do("ignore", func() error {
			for _, p := range paths {
				fullPath := filepath.Join(repo.Path, p)
				info, err := os.Stat(fullPath)
				isDir := err == nil && info.IsDir()

				pattern := p
				if isDir {
					pattern = p + "/"
				}

				if err := repo.Ignore(ctx, pattern); err != nil {
					return err
				}
			}
			return nil
		}), true

	case "M":
		m.toggleExplorerMark()
		return m, nil, true
	}

	return m, nil, false
}

// isPathTracked reports whether git holds a copy of path, or of anything
// beneath it. It decides between git mv and a plain rename, and between
// git rm -r and unlinking a tree.
func (m Model) isPathTracked(path string) bool {
	if e, ok := m.entryAt(path); ok && e.Cached {
		return true
	}
	for dir, entries := range m.index {
		if dir != path && !strings.HasPrefix(dir, path+"/") {
			continue
		}
		for _, e := range entries {
			if e.Cached {
				return true
			}
		}
	}
	return false
}

// isPathStaged checks if a path is staged in the git index.
func (m Model) isPathStaged(path string) (bool, bool) {
	for _, file := range m.snap.Files {
		if file.Path == path {
			return file.Staged(), true
		}
	}
	return false, false
}

// countFilesInDir recursively counts files in a directory.
func countFilesInDir(dir string) (int, error) {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			c, _ := countFilesInDir(filepath.Join(dir, e.Name()))
			count += c
		} else {
			count++
		}
	}
	return count, nil
}
