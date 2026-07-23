package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// Searching is git's own, not a filter over what the panel holds: / narrows the
// few hundred commits already read, and the commit being looked for is usually
// older than those. The query stays on until it is cleared, so the panel goes
// on answering it as the repository changes.

// searchFieldMsg says which part of a commit the next prompt is about.
type searchFieldMsg struct{ field searchField }

// searchSetMsg carries the answer back, empty meaning that part is no longer
// searched on.
type searchSetMsg struct {
	field searchField
	value string
}

type searchField int

const (
	searchMessage searchField = iota
	searchAuthor
	searchContent
	searchClear
)

// askSearch offers the three things git can look for.
func (m Model) askSearch() (tea.Model, tea.Cmd) {
	field := func(f searchField) func() tea.Cmd {
		return func() tea.Cmd {
			return func() tea.Msg { return searchFieldMsg{field: f} }
		}
	}

	choices := []choice{
		{label: "Message", hint: "commits whose message holds this text", action: field(searchMessage)},
		{label: "Author", hint: "commits by a name or address", action: field(searchAuthor)},
		{label: "Content", hint: "commits that added or removed this text — what finds a deleted line",
			action: field(searchContent)},
	}
	if !m.logQuery.Empty() {
		choices = append(choices, choice{
			label:  "Clear",
			hint:   "list the whole history again",
			action: field(searchClear),
		})
	}

	m.askChoice("Search commits", "Answered by git over the whole history, not by narrowing the list on screen.", choices)
	return m, nil
}

// handleSearchField asks for the text, pre-filled with whatever that part of
// the query already holds.
func (m Model) handleSearchField(msg searchFieldMsg) (tea.Model, tea.Cmd) {
	if msg.field == searchClear {
		return m.applySearch(git.LogQuery{})
	}

	field := msg.field
	m.askInput(searchTitle(field), queryField(m.logQuery, field), func(value string) tea.Cmd {
		return func() tea.Msg { return searchSetMsg{field: field, value: value} }
	})
	return m, nil
}

// handleSearchSet puts the answer into the query and reads the history again.
func (m Model) handleSearchSet(msg searchSetMsg) (tea.Model, tea.Cmd) {
	query := m.logQuery
	setQueryField(&query, msg.field, msg.value)
	return m.applySearch(query)
}

// applySearch swaps the query in, opens the tab the result is in and re-reads.
func (m Model) applySearch(query git.LogQuery) (tea.Model, tea.Cmd) {
	m.logQuery = query

	// A list of another shape: the old position points at nothing in it.
	m.cursor[PanelCommits], m.offset[PanelCommits] = 0, 0
	m.loading = true

	if m.tab != TabLog {
		next, cmd := m.openTab(TabLog)
		model := next.(Model)
		reloaded, reload := model.reload()
		return reloaded, tea.Batch(cmd, reload)
	}
	return m.reload()
}

func searchTitle(f searchField) string {
	switch f {
	case searchAuthor:
		return "Search by author"
	case searchContent:
		return "Search commits that changed this text"
	}
	return "Search commit messages"
}

// queryField reads one part of a query, so a prompt can be pre-filled with it.
func queryField(q git.LogQuery, f searchField) string {
	switch f {
	case searchAuthor:
		return q.Author
	case searchContent:
		return q.Content
	}
	return q.Message
}

// setQueryField writes one part of a query.
func setQueryField(q *git.LogQuery, f searchField, value string) {
	switch f {
	case searchAuthor:
		q.Author = value
	case searchContent:
		q.Content = value
	default:
		q.Message = value
	}
}

// searchSuffix names the query in the Commits panel's title, so a short list is
// never mistaken for a short history.
func (m Model) searchSuffix() string {
	if m.logQuery.Empty() {
		return ""
	}
	return " · " + m.logQuery.Describe()
}
