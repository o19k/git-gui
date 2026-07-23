package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// mutationMsg reports the outcome of a repository change.
type mutationMsg struct {
	op  string
	err error
}

// do wraps a mutation so its result comes back through the update loop instead
// of blocking the render.
func (m Model) do(op string, fn func() error) tea.Cmd {
	return func() tea.Msg { return mutationMsg{op: op, err: fn()} }
}

// handlePanelKey routes a keystroke that only means something in the focused
// panel. handled=false falls through to the global bindings.
func (m Model) handlePanelKey(key string) (tea.Model, tea.Cmd, bool) {
	// A stopped rebase claims c and a everywhere: nothing else in the
	// repository behaves normally until it is resolved.
	if m.snap.Rebasing {
		if next, cmd, handled := m.rebaseKey(key); handled {
			return next, cmd, true
		}
	}

	// A content pane has no actions of its own; its movement keys are global.
	if m.focus.content() {
		return m, nil, false
	}

	switch m.focus {
	case PanelFiles:
		return m.filesKey(key)
	case PanelBranches:
		return m.branchesKey(key)
	case PanelWorktrees:
		return m.worktreesKey(key)
	case PanelCommits:
		return m.commitsKey(key)
	case PanelStash:
		return m.stashKey(key)
	case PanelCommitFiles:
		return m.commitFilesKey(key)
	case PanelStashFiles:
		return m.stashFilesKey(key)
	case PanelParent, PanelEntries, PanelPreview:
		return m.explorerKey(key)
	}
	return m, nil, false
}

// rebaseKey serves the keys that exist only while a rebase is stopped.
func (m Model) rebaseKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx

	switch key {
	case "c":
		return m, m.do("rebase continue", func() error { return repo.RebaseContinue(ctx) }), true

	case "a":
		m.askConfirm("Abort rebase",
			"Throw away the part-finished rebase and put the branch back where it was?", true,
			func() tea.Cmd {
				return m.do("rebase abort", func() error { return repo.RebaseAbort(ctx) })
			})
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) filesKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx
	file, hasFile := m.selectedFile()

	switch key {
	case "enter":
		if !hasFile {
			return m, nil, true
		}
		m.enterHunkMode()
		return m, nil, true

	case " ":
		if !hasFile {
			return m, nil, true
		}
		// An unmerged path counts as staged, so space would otherwise take a
		// conflict back out of the index. Conflicts resolve one at a time.
		if conflicted(file) {
			return m, m.do("mark resolved", func() error { return repo.MarkResolved(ctx, file.Path) }), true
		}
		files := m.markedFiles()
		if len(files) == 0 {
			return m, nil, true
		}
		// Marked files move as a group. Mixed marks stage rather than unstage.
		allStaged := true
		for _, f := range files {
			if !f.Staged() {
				allStaged = false
				break
			}
		}
		if allStaged {
			return m, m.do("unstage", func() error {
				for _, f := range files {
					if err := repo.Unstage(ctx, f.Path); err != nil {
						return err
					}
				}
				return nil
			}), true
		}
		return m, m.do("stage", func() error {
			for _, f := range files {
				if err := repo.Stage(ctx, f.Path); err != nil {
					return err
				}
			}
			return nil
		}), true

	case "o":
		if !hasFile {
			return m, nil, true
		}
		next, cmd := m.revealInExplorer(file.Path)
		return next, cmd, true

	case "a":
		return m, m.do("stage all", func() error { return repo.StageAll(ctx) }), true

	case "u":
		return m, m.do("unstage all", func() error { return repo.UnstageAll(ctx) }), true

	case "r":
		if !hasFile {
			return m, nil, true
		}
		m.askResolve(file)
		return m, nil, true

	case "t":
		if !hasFile || file.Untracked() {
			m.status = "only a file git is following can be untracked"
			return m, nil, true
		}
		m.askConfirm("Untrack",
			fmt.Sprintf("Stop following %s? The file stays on disk, but git records it as deleted.", file.Path),
			false,
			func() tea.Cmd {
				return m.do("untrack", func() error { return repo.Untrack(ctx, file.Path) })
			})
		return m, nil, true

	case "i":
		if !hasFile {
			return m, nil, true
		}
		// Adding a pattern does nothing for a path already in the index, so a
		// tracked file is untracked in the same step.
		tracked := !file.Untracked()
		body := fmt.Sprintf("Add %s to .gitignore?", file.Path)
		if tracked {
			body += " It is tracked, so it is untracked too — otherwise the pattern would have no effect."
		}
		m.askConfirm("Ignore", body, false, func() tea.Cmd {
			return m.do("ignore", func() error {
				if tracked {
					if err := repo.Untrack(ctx, file.Path); err != nil {
						return err
					}
				}
				return repo.Ignore(ctx, file.Path)
			})
		})
		return m, nil, true

	case "x":
		files := m.markedFiles()
		if len(files) == 0 {
			return m, nil, true
		}
		body := fmt.Sprintf("Delete %s from disk? This cannot be undone.", pathList(paths(files)))
		m.askConfirm("Delete "+count(len(files), "file", "files"), body, true,
			func() tea.Cmd {
				return m.do("delete", func() error {
					for _, f := range files {
						if err := repo.DeleteFile(ctx, f); err != nil {
							return err
						}
					}
					return nil
				})
			})
		return m, nil, true

	case "H":
		if !hasFile || file.Untracked() {
			m.status = "an untracked file has no history"
			return m, nil, true
		}
		return m, m.loadFileHistory(file.Path), true

	case "d":
		files := m.markedFiles()
		if len(files) == 0 {
			return m, nil, true
		}
		body := fmt.Sprintf("Throw away all changes to %s? This cannot be undone.", pathList(paths(files)))
		m.askConfirm("Discard changes", body, true,
			func() tea.Cmd {
				return m.do("discard", func() error {
					for _, f := range files {
						if err := repo.Discard(ctx, f); err != nil {
							return err
						}
					}
					return nil
				})
			})
		return m, nil, true

	case "m":
		m.toggleFileMark()
		return m, nil, true

	case "c":
		// The commit goes through the repository's own checks, which may hold it
		// back; with none configured that is the same as committing outright.
		self := m
		m.askInput("Commit message", "", func(message string) tea.Cmd {
			if message == "" {
				return nil
			}
			_, cmd := self.startCommit(message)
			return cmd
		})
		return m, nil, true

	case "C":
		// The editor is the only way in for a message with a body, a template
		// or a trailer.
		next, cmd := m.startCompose(false)
		return next, cmd, true

	case "A":
		// A commit with a body cannot be re-typed at a one-line prompt without
		// losing it, so that one goes to the editor instead.
		return m, m.startAmend(), true

	case "s":
		next, cmd := m.askStash()
		return next, cmd, true
	}
	return m, nil, false
}

// startAmend re-opens the last commit's message: at the prompt when it is one
// line, and in the editor when there is more of it to keep.
func (m Model) startAmend() tea.Cmd {
	repo, ctx := m.repo, m.ctx
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		message, err := repo.HeadMessage(ctx)
		return amendMsg{message: message, err: err}
	}
}

// amendMsg carries the message being replaced, so the choice between the
// prompt and the editor is made on what is actually there.
type amendMsg struct {
	message string
	err     error
}

func (m Model) handleAmend(msg amendMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}
	if strings.Contains(msg.message, "\n") {
		return m.startCompose(true)
	}

	repo, ctx, opts := m.repo, m.ctx, m.commitOpts()
	m.askInput("Amend last commit", msg.message, func(message string) tea.Cmd {
		if message == "" {
			return nil
		}
		return m.do("amend", func() error { return repo.Amend(ctx, message, opts) })
	})
	return m, nil
}

func (m Model) branchesKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx
	branch, hasBranch := m.selectedBranch()

	switch key {
	case "enter":
		if !hasBranch || branch.Head {
			return m, nil, true
		}
		return m, m.do("checkout", func() error { return repo.Checkout(ctx, branch) }), true

	case "n":
		m.askInput("New branch name", "", func(name string) tea.Cmd {
			if name == "" {
				return nil
			}
			return m.do("create branch", func() error { return repo.CreateBranch(ctx, name) })
		})
		return m, nil, true

	case "d", "D":
		if !hasBranch {
			return m, nil, true
		}
		if branch.Head {
			m.status = "cannot delete the branch you are on"
			return m, nil, true
		}
		// A remote branch has no force: the push either removes it or is
		// refused, and the far side keeps no reflog. Both keys ask the same
		// question.
		if branch.Kind == git.RefRemote {
			m.askConfirm("Delete remote branch",
				fmt.Sprintf("Delete %s on the remote? It goes for everyone who has it, and nothing on that side remembers it.", branch.Name),
				true,
				func() tea.Cmd {
					return m.do("delete remote branch", func() error {
						return repo.DeleteRemoteBranch(ctx, branch)
					})
				})
			return m, nil, true
		}
		force := key == "D"
		body := fmt.Sprintf("Delete %s?", branch.Name)
		if force {
			body = fmt.Sprintf("Force-delete %s, including commits it alone holds? This cannot be undone.", branch.Name)
		}
		m.askConfirm("Delete branch", body, force, func() tea.Cmd {
			return m.do("delete branch", func() error { return repo.DeleteBranch(ctx, branch, force) })
		})
		return m, nil, true

	case "M":
		if !hasBranch || branch.Head {
			return m, nil, true
		}
		m.askConfirm("Merge",
			fmt.Sprintf("Merge %s into %s?", branch.Name, m.snap.Branch), false,
			func() tea.Cmd {
				return m.do("merge", func() error { return repo.Merge(ctx, branch.Name) })
			})
		return m, nil, true

	case "m":
		if !hasBranch {
			return m, nil, true
		}
		if branch.Kind != git.RefLocal {
			m.status = "only a local branch can be renamed here"
			return m, nil, true
		}
		old := branch.Name
		m.askInput("Rename "+old, old, func(name string) tea.Cmd {
			if name == "" || name == old {
				return nil
			}
			return m.do("rename branch", func() error { return repo.RenameBranch(ctx, old, name) })
		})
		return m, nil, true

	case "c":
		if !hasBranch || branch.Head {
			return m, nil, true
		}
		next, cmd := m.startCompare(branch)
		return next, cmd, true

	case "t":
		if !hasBranch {
			return m, nil, true
		}
		next, cmd := m.askTag(branch)
		return next, cmd, true

	case "P":
		// On a tag, publish that tag. On anything else the key keeps its
		// ordinary meaning, which is pushing the branch that is checked out.
		if !hasBranch || branch.Kind != git.RefTag {
			return m, nil, false
		}
		next, cmd := m.askPushTag(branch)
		return next, cmd, true

	case "r":
		if !hasBranch || branch.Head {
			return m, nil, true
		}
		m.askConfirm("Rebase onto",
			fmt.Sprintf("Replay %s's own commits on top of %s? History is rewritten, and a conflict stops the rebase part-way.",
				m.snap.Branch, branch.Name), true,
			func() tea.Cmd {
				return m.do("rebase", func() error { return repo.RebaseOnto(ctx, branch.Ref()) })
			})
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) commitsKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx

	// The rails are of the list, not of a selection, so an empty panel still
	// toggles them.
	if key == "L" {
		m.graphOn = !m.graphOn
		return m, nil, true
	}

	commit, hasCommit := m.selectedCommit()
	if !hasCommit {
		return m, nil, false
	}

	// Rebase, undo and date edits act on the checked-out branch, so rewriting
	// from another branch's list would rewrite somewhere unseen.
	switch key {
	case "s", "r", "d", "K", "J", "z", "t":
		if !m.onHead() {
			m.status = fmt.Sprintf("this is %s — rewriting works on the checked-out branch, %s",
				m.logName(), m.snap.Branch)
			return m, nil, true
		}
	}

	switch key {
	case "s":
		m.askConfirm("Squash into parent",
			fmt.Sprintf("Fold %s (%s) into the commit below it? Its message is discarded and history is rewritten.",
				commit.Short, commit.Subject), true,
			func() tea.Cmd {
				return m.do("squash", func() error {
					return repo.Rebase(ctx, git.ActionFixup, commit.SHA, "")
				})
			})
		return m, nil, true

	case "r":
		m.askInput("Reword commit "+commit.Short, commit.Subject, func(message string) tea.Cmd {
			if message == "" {
				return nil
			}
			return m.do("reword", func() error {
				return repo.Rebase(ctx, git.ActionReword, commit.SHA, message)
			})
		})
		return m, nil, true

	case "d":
		m.askConfirm("Drop commit",
			fmt.Sprintf("Remove %s (%s) from history? Everything after it is rewritten.",
				commit.Short, commit.Subject), true,
			func() tea.Cmd {
				return m.do("drop", func() error {
					return repo.Rebase(ctx, git.ActionDrop, commit.SHA, "")
				})
			})
		return m, nil, true

	case "K":
		return m, m.do("move commit", func() error {
			return repo.Rebase(ctx, git.ActionMoveUp, commit.SHA, "")
		}), true

	case "J":
		return m, m.do("move commit", func() error {
			return repo.Rebase(ctx, git.ActionMoveDown, commit.SHA, "")
		}), true

	case "c":
		m.askConfirm("Cherry-pick",
			fmt.Sprintf("Apply %s (%s) onto %s?", commit.Short, commit.Subject, m.snap.Branch), false,
			func() tea.Cmd {
				return m.do("cherry-pick", func() error { return repo.CherryPick(ctx, commit.SHA) })
			})
		return m, nil, true

	case "v":
		m.askConfirm("Revert",
			fmt.Sprintf("Commit the inverse of %s (%s)?", commit.Short, commit.Subject), false,
			func() tea.Cmd {
				return m.do("revert", func() error { return repo.Revert(ctx, commit.SHA) })
			})
		return m, nil, true

	case "z":
		// Only the last commit: further back is a rewrite of everything above
		// it, which is what squash and drop are for.
		if m.cursor[PanelCommits] != 0 || len(m.commits()) == 0 {
			m.status = "only the last commit can be undone — use s, r or d further back"
			return m, nil, true
		}
		m.askConfirm("Undo commit",
			fmt.Sprintf("Take %s (%s) apart? What it held comes back as staged changes; nothing is lost.",
				commit.Short, commit.Subject), false,
			func() tea.Cmd {
				return m.do("undo commit", func() error { return repo.UndoCommit(ctx) })
			})
		return m, nil, true

	case "t":
		next, cmd := m.askCommitDate(commit)
		return next, cmd, true

	case "n":
		m.askInput("New branch at "+commit.Short, "", func(name string) tea.Cmd {
			if name == "" {
				return nil
			}
			return m.do("create branch", func() error {
				return repo.CreateBranchAt(ctx, name, commit.SHA)
			})
		})
		return m, nil, true

	case "P":
		head, ok := m.headBranch()
		if !ok {
			m.status = "nothing to push: no branch checked out"
			return m, nil, true
		}
		m.askConfirm("Push up to here",
			fmt.Sprintf("Publish %s as far as %s (%s), leaving anything after it local?",
				head.Name, commit.Short, commit.Subject), false,
			func() tea.Cmd {
				return m.do("push", func() error {
					return repo.PushUpTo(ctx, "", commit.SHA, head.Name)
				})
			})
		return m, nil, true
	}
	return m, nil, false
}

// commitFilesKey serves the list of paths a commit touched.
//
// History is on H rather than h: a panel binding is offered the key before the
// navigation is, so h here would swallow the way back to the commit list.
func (m Model) commitFilesKey(key string) (tea.Model, tea.Cmd, bool) {
	file, ok := m.selectedCommitFile()
	if !ok {
		return m, nil, false
	}
	switch key {
	case "H":
		return m, m.loadFileHistory(file.Path), true
	case "o":
		next, cmd := m.revealInExplorer(file.Path)
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) stashKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx
	stash, hasStash := m.selectedStash()
	if !hasStash {
		return m, nil, false
	}

	switch key {
	case " ":
		return m, m.do("stash apply", func() error { return repo.StashApply(ctx, stash.Ref) }), true

	case "enter":
		return m, m.do("stash pop", func() error { return repo.StashPop(ctx, stash.Ref) }), true

	case "d":
		m.askConfirm("Drop stash",
			fmt.Sprintf("Delete %s (%s)? This cannot be undone.", stash.Ref, stash.Subject), true,
			func() tea.Cmd {
				return m.do("stash drop", func() error { return repo.StashDrop(ctx, stash.Ref) })
			})
		return m, nil, true
	}
	return m, nil, false
}

// --- selection helpers ---

// The lists these read are the filtered ones, so an action can never reach an
// entry the filter is hiding. Each bounds-checks: typing into a filter shrinks
// the list under the cursor.

func selected[T any](list []T, cursor int) (T, bool) {
	if cursor < 0 || cursor >= len(list) {
		var zero T
		return zero, false
	}
	return list[cursor], true
}

func (m Model) selectedFile() (git.FileChange, bool) {
	return selected(m.files(), m.cursor[PanelFiles])
}

func (m Model) selectedBranch() (git.Branch, bool) {
	return selected(m.branches(), m.cursor[PanelBranches])
}

func (m Model) selectedCommit() (git.Commit, bool) {
	return selected(m.commits(), m.cursor[PanelCommits])
}

func (m Model) selectedStash() (git.Stash, bool) {
	return selected(m.stashes(), m.cursor[PanelStash])
}

// panelKeyHints is the footer's context section.
func (m Model) panelKeyHints(p Panel) [][2]string {
	// A conflict replaces the pane's ordinary keys rather than joining them.
	if m.conflictCount() > 0 && p == PanelFiles {
		return [][2]string{{"r", "resolve"}, {"space", "mark resolved"}, {"d", "discard"}}
	}
	if m.snap.Rebasing {
		return [][2]string{{"c", "continue rebase"}, {"a", "abort rebase"}}
	}
	if p.content() {
		return [][2]string{{"j/k", "line"}, {"^f/^b", "page"}, {"v", "split/unified"}}
	}
	switch p {
	case PanelFiles:
		return [][2]string{{"space", "stage"}, {"m", "mark"}, {"enter", "hunks"}, {"a/u", "stage/unstage all"}, {"c", "commit"}, {"d", "discard"}}
	case PanelBranches:
		return [][2]string{{"enter", "checkout"}, {"n", "new"}, {"m", "rename"}, {"M", "merge"}, {"r", "rebase onto"}, {"d/D", "delete"}}
	case PanelWorktrees:
		return [][2]string{{"enter", "open"}, {"n", "new"}, {"d", "remove"}}
	case PanelCommits:
		// The graph key leads: nothing on screen hints at a view toggle.
		return [][2]string{{"L", "graph"}, {"s", "squash"}, {"r", "reword"}, {"d", "drop"}, {"z", "undo last"}, {"c", "cherry-pick"}, {"v", "revert"}}
	case PanelCommitFiles:
		return commitFilesKeyHints()
	case PanelStash:
		return [][2]string{{"space", "apply"}, {"enter", "pop"}, {"d", "drop"}}
	case PanelStashFiles:
		return stashFilesKeyHints()
	case PanelPreview:
		// The listing's keys do not apply to a pane being read.
		hints := [][2]string{{"j/k", "scroll"}, {"h", "back to the files"}, {"e", "view"}, {"enter", "edit"}}
		// Only a patch has two sides.
		if m.previewFor.kind == previewDiff {
			hints = append(hints, [2]string{"v", "split/unified"})
		}
		return hints
	case PanelParent, PanelEntries:
		return explorerKeyHints()
	}
	return nil
}

// --- remote operations ---

// headBranch is the checked-out branch as the refs list sees it, which is where
// the upstream comes from.
func (m Model) headBranch() (git.Branch, bool) {
	for _, b := range m.snap.Branches {
		if b.Head {
			return b, true
		}
	}
	return git.Branch{}, false
}

// handleRemoteKey serves the network bindings, which mean the same in every
// panel. Each parks a note in the footer for the duration.
func (m Model) handleRemoteKey(key string) (tea.Model, tea.Cmd, bool) {
	repo, ctx := m.repo, m.ctx

	switch key {
	case "f":
		m.busy = "fetching…"
		return m, m.do("fetch", func() error { return repo.Fetch(ctx) }), true

	case "p":
		next, cmd := m.startPull()
		return next, cmd, true

	case "P":
		next, cmd := m.startPush()
		return next, cmd, true

	case "F":
		head, ok := m.headBranch()
		if !ok {
			return m, nil, true
		}
		// The branch's own upstream, not a chosen remote: overwriting one
		// place is what this is for, and it is the place the branch came from.
		m.askConfirm("Force push",
			fmt.Sprintf("Overwrite origin/%s with your local branch? Commits only on the remote would be lost — git will still refuse if it holds anything you have not fetched.", head.Name),
			true,
			func() tea.Cmd {
				return m.do("force push", func() error { return repo.ForcePush(ctx, "", head.Name) })
			})
		return m, nil, true
	}
	return m, nil, false
}

// remoteKeyHints is the footer's network section.
func remoteKeyHints() [][2]string {
	return [][2]string{{"f", "fetch"}, {"p", "pull"}, {"P", "push"}}
}
