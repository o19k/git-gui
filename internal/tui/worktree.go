package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// A worktree is another branch checked out in another directory, sharing one
// object store. Opening one here swaps the repository the whole tool is looking
// at, which is cheaper than leaving and starting again somewhere else.

// worktreesMsg carries the read of the checkouts.
type worktreesMsg struct {
	trees []git.Worktree
	err   error
}

// worktreePickMsg is the chosen checkout; newWorktreeMsg is the row that makes
// one instead of opening one.
type worktreePickMsg struct{ tree git.Worktree }
type newWorktreeMsg struct{}

// openedMsg carries a repository opened somewhere else, or the reason it could
// not be.
type openedMsg struct {
	repo *git.Repo
	err  error
}

func (m Model) loadWorktrees() tea.Cmd {
	repo, ctx := m.repo, m.ctx
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		trees, err := repo.Worktrees(ctx)
		return worktreesMsg{trees: trees, err: err}
	}
}

// showWorktrees lists the checkouts, marking the one being looked at.
func (m Model) showWorktrees(msg worktreesMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	here := m.repoPath()
	byPath := make(map[string]git.Worktree, len(msg.trees))
	items := make([]listItem, 0, len(msg.trees)+1)
	for _, tree := range msg.trees {
		byPath[tree.Path] = tree
		marker := "  "
		if samePath(tree.Path, here) {
			marker = "▸ "
		}
		items = append(items, listItem{
			label: fmt.Sprintf("%s%-24s %s", marker, tree.Name(), tree.Path),
			value: tree.Path,
		})
	}
	items = append(items, listItem{label: "  + new worktree…", value: ""})

	m.askList("Worktrees", items, func(path string) tea.Cmd {
		if path == "" {
			return func() tea.Msg { return newWorktreeMsg{} }
		}
		tree := byPath[path]
		return func() tea.Msg { return worktreePickMsg{tree: tree} }
	})
	return m, nil
}

// handleWorktreePick offers what can be done with the chosen checkout. The one
// already open offers nothing but removal, which git refuses for it anyway.
func (m Model) handleWorktreePick(msg worktreePickMsg) (tea.Model, tea.Cmd) {
	tree := msg.tree
	repo, ctx := m.repo, m.ctx

	if samePath(tree.Path, m.repoPath()) {
		m.status = "already open here"
		return m, nil
	}

	self := m
	m.askChoice("Worktree "+tree.Name(), tree.Path, []choice{
		{
			label:  "Open it",
			hint:   "look at this checkout instead, without leaving the tool",
			action: func() tea.Cmd { return openRepo(ctx, tree.Path) },
		},
		{
			label:  "Remove it",
			hint:   "deletes the directory; git refuses while it holds changes",
			danger: true,
			action: func() tea.Cmd {
				return self.do("remove worktree", func() error {
					return repo.RemoveWorktree(ctx, tree.Path, false)
				})
			},
		},
	})
	return m, nil
}

// handleNewWorktree asks where the checkout goes and which branch it holds. The
// branch is made at HEAD: a branch that already exists is checked out somewhere
// else more often than not, and git refuses that.
func (m Model) handleNewWorktree(newWorktreeMsg) (tea.Model, tea.Cmd) {
	m.askInput("New worktree directory", "", func(path string) tea.Cmd {
		if path == "" {
			return nil
		}
		return func() tea.Msg { return worktreeBranchMsg{path: path} }
	})
	return m, nil
}

// worktreeBranchMsg carries the directory while the branch is asked for.
type worktreeBranchMsg struct{ path string }

func (m Model) handleWorktreeBranch(msg worktreeBranchMsg) (tea.Model, tea.Cmd) {
	repo, ctx, path := m.repo, m.ctx, msg.path

	// The directory's own name is the obvious branch name, and usually the
	// right one.
	prefill := filepath.Base(path)
	m.askInput("New branch for "+path, prefill, func(branch string) tea.Cmd {
		if branch == "" {
			return nil
		}
		return m.do("add worktree", func() error {
			return repo.AddWorktree(ctx, path, branch, true)
		})
	})
	return m, nil
}

// handleOpened points the whole model at another checkout. Everything read
// from the old one is dropped rather than carried across: the paths, the
// commits and the index all belong to the repository that is being left.
func (m Model) handleOpened(msg openedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	next := New(m.ctx, msg.repo, WithSettings(m.settings))
	next.width, next.height = m.width, m.height
	next.splitDiff, next.graphOn = m.splitDiff, m.graphOn
	next.status = "opened " + msg.repo.Path

	return next, tea.Batch(next.load(), next.loadIndex())
}

// --- the Worktrees panel, in the Log tab ---
//
// The list is what W opens in a modal, given a pane of its own: with AI agents
// each branch tends to live in its own checkout, so seeing where each one sits
// beside the branch list is worth a column.

// worktreeRef is what the Commits panel should list for a checkout: its branch
// where it has one, and the commit it sits on when it is detached.
func worktreeRef(w git.Worktree) string {
	if w.Branch != "" {
		return "refs/heads/" + w.Branch
	}
	return w.Head
}

func (m Model) selectedWorktree() (git.Worktree, bool) {
	return selected(m.worktrees(), m.cursor[PanelWorktrees])
}

// worktreeLines renders the Worktrees panel, marking the one open now.
func (m Model) worktreeLines(start, end, cursor, w int, focused bool) []string {
	here := m.repoPath()
	trees := m.worktrees()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := trees[i]

		mark := " "
		if samePath(t.Path, here) {
			mark = "▸"
		}

		name := t.Name()
		tail := "  " + t.Path
		if t.Locked {
			tail += " (locked)"
		}

		plain := " " + mark + " " + name + tail
		styled := " " + theme.FooterKeyStyle.Render(mark) + " " + name + theme.DimStyle.Render(tail)

		lines = append(lines, row(w, i == cursor, focused, plain, styled))
	}
	return lines
}

// worktreesKey serves the Worktrees panel: open a checkout, add one, or remove
// one. It reuses the same messages the modal picker sends.
func (m Model) worktreesKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx
	tree, has := m.selectedWorktree()

	switch key {
	case "enter":
		if !has {
			return m, nil, true
		}
		if samePath(tree.Path, m.repoPath()) {
			m.status = "already open here"
			return m, nil, true
		}
		return m, openRepo(ctx, tree.Path), true

	case "n":
		return m, func() tea.Msg { return newWorktreeMsg{} }, true

	case "d":
		if !has {
			return m, nil, true
		}
		if samePath(tree.Path, m.repoPath()) {
			m.status = "cannot remove the worktree you are in"
			return m, nil, true
		}
		m.askConfirm("Remove worktree",
			fmt.Sprintf("Delete %s? git refuses while it holds changes.", tree.Path), true,
			func() tea.Cmd {
				return m.do("remove worktree", func() error {
					return repo.RemoveWorktree(ctx, tree.Path, false)
				})
			})
		return m, nil, true
	}
	return m, nil, false
}

// repoPath is the root of the repository being looked at, empty when there is
// none — which only a test builds.
func (m Model) repoPath() string {
	if m.repo == nil {
		return ""
	}
	return m.repo.Path
}

// samePath compares two paths git printed. They come from git and are absolute,
// so this is about symlinks and trailing separators rather than about
// resolution.
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// openRepo is the command that opens another checkout, kept apart so a test can
// drive it without a picker.
func openRepo(ctx context.Context, path string) tea.Cmd {
	return func() tea.Msg {
		repo, err := git.Open(ctx, path)
		return openedMsg{repo: repo, err: err}
	}
}
