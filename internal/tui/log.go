package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// The Commits panel lists one ref, and which ref that is follows the Branches
// cursor.

// logMsg carries a re-read of the commit list. ref tags it, so a read for a
// branch the cursor has already left is dropped rather than drawn.
type logMsg struct {
	ref      string
	commits  []git.Commit
	unpushed map[string]bool
	err      error
}

// aimLog points the commit list at a ref, and reads it only on a change: the
// caller runs on every snapshot as well as on every keypress.
func (m *Model) aimLog(ref string) tea.Cmd {
	if ref == m.logRef {
		return nil
	}
	m.logRef = ref
	return m.loadLog()
}

// loadLog re-reads the commits of whichever ref is selected, ahead of the poll
// that would reach the same list within a few seconds.
func (m Model) loadLog() tea.Cmd {
	repo, ctx, ref := m.repo, m.ctx, m.logRef
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		commits, err := repo.LogRef(ctx, ref, logLimit)
		// Read alongside the commits, or marks from the branch just left would
		// sit on rows they do not belong to.
		unpushed, uerr := repo.Unpushed(ctx, ref, logLimit)
		if err == nil {
			err = uerr
		}
		return logMsg{ref: ref, commits: commits, unpushed: unpushed, err: err}
	}
}

func (m Model) handleLog(msg logMsg) (tea.Model, tea.Cmd) {
	if msg.ref != m.logRef {
		return m, nil
	}
	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	m.snap.Commits = msg.commits
	m.snap.Unpushed = msg.unpushed
	// Another history: a position in the old one points at nothing here.
	m.cursor[PanelCommits], m.offset[PanelCommits] = 0, 0
	m.clampCursors()
	return m, nil
}

// unpushed is the set of commits to mark. Nothing is marked without a remote,
// where every commit is unpublished.
func (m Model) unpushed() map[string]bool {
	for _, b := range m.snap.Branches {
		if b.Kind == git.RefRemote {
			return m.snap.Unpushed
		}
	}
	return nil
}

// logName is the selected ref as it is written in the Branches panel.
func (m Model) logName() string {
	name := m.logRef
	for _, prefix := range []string{"refs/heads/", "refs/remotes/", "refs/tags/"} {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			return rest
		}
	}
	return name
}

// onHead reports whether the listed commits are the checked-out branch's, the
// only history the rewriting keys can reach.
func (m Model) onHead() bool {
	return m.logRef == "" || m.logRef == "refs/heads/"+m.snap.Branch
}

// logSuffix names the ref in the panel's title when it is not the checked-out
// one.
func (m Model) logSuffix() string {
	if m.onHead() {
		return ""
	}
	return " · " + m.logName()
}
