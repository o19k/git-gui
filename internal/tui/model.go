package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// Tab is one workspace. The number key that opens it is its index plus one.
type Tab int

const (
	TabChanges Tab = iota
	TabLog
	TabStash
	TabFiles
	tabCount
)

var tabTitles = [tabCount]string{"Local Changes", "Log", "Stash", "Explorer"}

// Panel identifies one pane. A tab lays its panes out left to right.
type Panel int

const (
	PanelFiles Panel = iota
	PanelDiff
	PanelBranches
	PanelWorktrees
	PanelCommits
	PanelCommitFiles
	PanelDetails
	PanelStash
	PanelStashFiles
	PanelStashDiff
	PanelParent
	PanelEntries
	PanelPreview
	panelCount
)

// tabColumns is each tab's layout: columns left to right, and within a column
// the panes stacked top to bottom. tabWeights is each column's share of the
// width in percent, the last taking the rounding remainder; stackWeights does
// the same for the rows of a column that holds more than one pane.
var (
	tabColumns = [tabCount][][]Panel{
		TabChanges: {{PanelFiles}, {PanelDiff}},
		TabLog:     {{PanelBranches, PanelWorktrees}, {PanelCommits}, {PanelCommitFiles, PanelDetails}},
		TabStash:   {{PanelStash}, {PanelStashFiles}, {PanelStashDiff}},
		TabFiles:   {{PanelParent}, {PanelEntries}, {PanelPreview}},
	}
	tabWeights = [tabCount][]int{
		TabChanges: {32, 68},
		TabLog:     {24, 46, 30},
		TabStash:   {26, 26, 48},
		TabFiles:   {18, 30, 52},
	}
	stackWeights = map[Panel]int{
		PanelBranches: 60, PanelWorktrees: 40,
		PanelCommitFiles: 40, PanelDetails: 60,
	}
)

// tabPanes is a tab's panes in focus order: down each column, then across.
var tabPanes = func() [tabCount][]Panel {
	var out [tabCount][]Panel
	for t := range tabCount {
		for _, column := range tabColumns[t] {
			out[t] = append(out[t], column...)
		}
	}
	return out
}()

// landingPane is where a tab's focus starts before that tab has been visited.
// The Explorer starts on the listing rather than on the parent column.
var landingPane = func() [tabCount]Panel {
	var out [tabCount]Panel
	for t := range tabCount {
		out[t] = tabPanes[t][0]
	}
	out[TabFiles] = PanelEntries
	return out
}()

// content reports whether a pane scrolls a patch rather than selecting from a list.
func (p Panel) content() bool {
	return p == PanelDiff || p == PanelDetails || p == PanelStashDiff
}

// tabOf is the tab a pane belongs to.
func tabOf(p Panel) Tab {
	for t := range tabCount {
		if slices.Contains(tabPanes[t], p) {
			return Tab(t)
		}
	}
	return TabChanges
}

const (
	// logLimit is the fallback for a model built without settings; the stored
	// preference is what an ordinary run uses.
	logLimit = 500

	// minPaneW keeps a column wide enough for a frame and some text; below it
	// renderBox draws nothing at all. minPaneH is a frame plus one content row.
	minPaneW = 12
	minPaneH = 3

	// Bounds on the poll interval; see Model.refreshEvery.
	minRefresh = 3 * time.Second
	maxRefresh = 30 * time.Second

	// refreshDutyCycle is how many times longer than one snapshot the gap
	// between snapshots must be, capping polling at ~5% of a core.
	refreshDutyCycle = 20
)

// Model is the whole application state.
type Model struct {
	ctx  context.Context
	repo *git.Repo

	width, height int

	// tab is the open workspace; focus is one of its panes. lastFocus is where
	// each tab's focus was when it was last left.
	tab       Tab
	focus     Panel
	lastFocus [tabCount]Panel
	cursor    [panelCount]int
	offset    [panelCount]int

	// filter narrows a list pane to entries matching a substring; filtering is
	// true while the focused pane's term is being typed.
	filter    [panelCount]string
	filtering bool

	snap git.Snapshot

	// logRef is the ref the Commits panel lists, empty for the checked-out one.
	// It follows the Branches cursor. graphOn draws the lanes the parents imply.
	logRef  string
	graphOn bool

	// logQuery narrows the commit list. It is answered by git on every refresh
	// rather than by filtering what was read: the commit being looked for is
	// usually older than the few hundred the panel holds.
	logQuery git.LogQuery

	// settings are the preferences that outlive the run.
	settings git.Settings

	// previewKey identifies the selection the content belongs to, so a git call
	// landing after the cursor moved on can be discarded.
	previewKey  string
	mainTitle   string
	mainContent string

	// mainStyled is the content coloured as source. Empty for a patch, and for a
	// file of no language the highlighter knows.
	mainStyled []string

	mainOffset int
	loading    bool
	status     string

	// busy names a network operation in flight.
	busy string

	// overlay is the modal in front of the panels.
	overlay overlay

	// commitFiles is what the selected commit touched; commitSHA says which
	// commit that is.
	commitSHA   string
	commitFiles []git.FileChange

	// stashFiles is what the selected stash entry holds; stashRef says which
	// entry that is, and stashMarks which of its paths a restore would take.
	stashRef   string
	stashFiles []git.FileChange
	stashMarks map[string]bool

	// splitDiff shows the content panes side by side instead of unified.
	splitDiff bool

	// fileMarks is the set of paths the next action in Local Changes reaches,
	// keyed by path so it survives the list being rebuilt under it.
	fileMarks map[string]bool

	// compareRef is the branch being compared with the checked-out one, and
	// compareAll whether the file list covers the whole range or just the
	// selected commit.
	compareRef string
	compareAll bool

	// pendingCommit is the message held back while the repository's checks run.
	pendingCommit string

	// blameOn replaces the content pane with the annotated file; blamePath is
	// whose annotations blameLines holds.
	blameOn    bool
	blamePath  string
	blameLines []git.BlameLine

	// blameStyled is the code column coloured as source, computed once per read
	// rather than once per frame.
	blameStyled []string

	// blameRev is the revision being annotated, empty for the working copy. It
	// is what walking back through the parents moves. blameCursor is the line
	// the annotation keys act on.
	blameRev    string
	blameCursor int

	// previewStaged records which side of the index the visible diff came from,
	// deciding whether staging a hunk adds or removes it.
	hunkMode      bool
	hunkCursor    int
	previewStaged bool

	// lineMode picks single lines out of the selected hunk. lineCursor is which
	// line of the hunk body it sits on, and lineMarks the ones that will be
	// staged — empty meaning the one under the cursor.
	lineMode   bool
	lineCursor int
	lineMarks  map[int]bool

	// --- Explorer ---

	// cwd is the directory the middle column lists, repo-relative. dirCursor
	// remembers the cursor per directory. It is applied when a listing arrives,
	// not when a directory is entered: clampCursors zeroes a cursor while the
	// list is empty, which a listing still in flight is.
	cwd       string
	dirCursor map[string]int

	// index is directory → children, built from one ls-files read, rebuilt on
	// the tick while the Explorer is open and never while it is closed.
	index   map[string][]fsEntry
	ignored []string

	// fsIndex holds the directories listed from disk instead of from git. Kept
	// apart from index, which is rebuilt wholesale. Re-listed on R, never on
	// the tick.
	fsIndex map[string][]fsEntry

	// The Explorer's preview, kept apart from mainContent.
	previewContent string
	previewTitle   string
	previewOffset  int

	// previewStyled is the content coloured as source, computed once per read.
	// Empty when the content is not source, or is of an unknown language.
	previewStyled []string

	// pendingLine is a 1-based line a search picked, applied when the content
	// it belongs to arrives.
	pendingLine  int
	previewFor   previewID
	previewLines []git.BlameLine

	// explorerMarks is separate from fileMarks: it holds clean paths too, which
	// have no entry in the status list for markedFiles to resolve.
	explorerMarks map[string]bool
	showHidden    bool

	// pendingPath is a path another tab asked to be shown, applied when the
	// listing that can show it arrives.
	pendingPath string

	// sortMode and sortReverse order every listing. stats holds the sizes and
	// times the two orders git cannot answer are sorted by, keyed by path and
	// filled one directory at a time.
	sortMode    sortMode
	sortReverse bool
	stats       map[string]fileMeta

	// loadAt marks a snapshot in flight; loadTook paces the next tick.
	loadAt   time.Time
	loadTook time.Duration
}

// New builds the initial model for repo. Options carry anything that is not
// the repository itself, so a caller wanting the defaults passes none.
func New(ctx context.Context, repo *git.Repo, options ...Option) Model {
	m := Model{
		ctx: ctx, repo: repo,
		focus: landingPane[TabChanges], lastFocus: landingPane,
		mainTitle: "Diff",
		loading:   true, loadAt: time.Now(),
		cwd:           ".",
		dirCursor:     make(map[string]int),
		index:         make(map[string][]fsEntry),
		fsIndex:       make(map[string][]fsEntry),
		explorerMarks: make(map[string]bool),
		stats:         make(map[string]fileMeta),
		settings:      git.DefaultSettings(),
	}
	for _, option := range options {
		option(&m)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), tick(minRefresh))
}

// --- messages ---

type snapshotMsg git.Snapshot

// styled is the content coloured as source, empty for a patch.
type previewMsg struct {
	key     string
	title   string
	content string
	styled  []string
	err     error
}

type tickMsg time.Time

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshEvery paces the poll to what a snapshot costs here.
func (m Model) refreshEvery() time.Duration {
	return min(max(m.loadTook*refreshDutyCycle, minRefresh), maxRefresh)
}

// reload issues a snapshot and stamps when it started.
func (m Model) reload() (Model, tea.Cmd) {
	m.loadAt = time.Now()
	return m, m.load()
}

// load reads every panel's data in one concurrent batch.
func (m Model) load() tea.Cmd {
	repo, ctx := m.repo, m.ctx
	opts := git.LoadOpts{Limit: m.logLimitOf(), Ref: m.logRef, Query: m.logQuery}
	return func() tea.Msg { return snapshotMsg(repo.Load(ctx, opts)) }
}

// --- update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case snapshotMsg:
		m.snap = git.Snapshot(msg)
		m.loading = false
		if !m.loadAt.IsZero() {
			m.loadTook = time.Since(m.loadAt)
			m.loadAt = time.Time{}
		}
		m.status = ""
		if len(m.snap.Errs) > 0 {
			m.status = m.snap.Errs[0].Error()
		}
		m.clampCursors()
		cmd := m.refreshPreview()
		if m.hunkMode {
			// The staged hunk is gone and the rest have renumbered.
			if ranges := hunkRanges(m.mainContent); len(ranges) == 0 {
				m.exitHunkMode()
			} else {
				m.hunkCursor = clamp(m.hunkCursor, 0, len(ranges)-1)
				m.scrollToHunk()
			}
		}
		return m, cmd

	case blameMsg:
		// Drop annotations for a file, or a revision, the cursor has left.
		if !m.blameOn || msg.path != m.blamePath || msg.rev != m.blameRev {
			return m, nil
		}
		if msg.err != nil {
			m.blameOn, m.blamePath, m.blameRev = false, "", ""
			m.status = msg.err.Error()
			return m, nil
		}
		m.blameLines, m.blameStyled = msg.lines, msg.styled
		m.blameCursor = clamp(m.blameCursor, 0, max(len(msg.lines)-1, 0))
		return m, nil

	case commitFilesMsg:
		// Drop a list for a commit the cursor has already left.
		if msg.sha != m.commitSHA {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.commitFiles = msg.files
		m.clampCursors()
		return m, nil

	case stashFilesMsg:
		// Drop a list for an entry the cursor has already left.
		if msg.ref != m.stashRef {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.stashFiles = msg.files
		m.clampCursors()
		return m, nil

	case previewMsg:
		// Drop a result for a selection the user has already moved off of.
		if msg.key != m.previewKey {
			return m, nil
		}
		m.mainTitle = msg.title
		if msg.err != nil {
			m.mainContent, m.mainStyled = "", nil
			m.status = msg.err.Error()
			return m, nil
		}
		m.mainContent, m.mainStyled = msg.content, msg.styled
		m.mainOffset = 0
		if m.hunkMode {
			m.scrollToHunk()
		}
		return m, nil

	case pullMsg:
		return m.handlePull(msg)

	case outgoingMsg:
		return m.askPush(msg)

	case compareMsg:
		return m.showCompare(msg)

	case checksMsg:
		return m.handleChecks(msg)

	case commitDateMsg:
		return m.confirmCommitDate(msg)

	case fileHistoryMsg:
		return m.showFileHistory(msg)

	case explorerPreviewMsg:
		return m.handleExplorerPreview(msg)

	case loadIndexMsg:
		return m.handleLoadIndex(msg)

	case readDirMsg:
		return m.handleReadDir(msg)

	case navigateMsg:
		return m.handleNavigate(msg)

	case grepMsg:
		return m.handleGrep(msg)

	case logMsg:
		return m.handleLog(msg)

	case sortMsg:
		return m.handleSort(msg)

	case statsMsg:
		return m.handleStats(msg)

	case editorDoneMsg:
		return m.handleEditorDone(msg)

	case settingSavedMsg:
		return m.handleSettingSaved(msg)

	case reflogMsg:
		return m.showReflog(msg)

	case worktreesMsg:
		return m.showWorktrees(msg)

	case composeDoneMsg:
		return m.handleComposeDone(msg)

	case searchFieldMsg:
		return m.handleSearchField(msg)

	case searchSetMsg:
		return m.handleSearchSet(msg)

	case reflogPickMsg:
		return m.handleReflogPick(msg)

	case reflogBranchMsg:
		return m.handleReflogBranch(msg)

	case reflogResetMsg:
		return m.handleReflogReset(msg)

	case worktreePickMsg:
		return m.handleWorktreePick(msg)

	case newWorktreeMsg:
		return m.handleNewWorktree(msg)

	case worktreeBranchMsg:
		return m.handleWorktreeBranch(msg)

	case openedMsg:
		return m.handleOpened(msg)

	case amendMsg:
		return m.handleAmend(msg)

	case stashKindMsg:
		return m.handleStashKind(msg)

	case tagNameMsg:
		return m.handleTagName(msg)

	case mutationMsg:
		m.busy = ""
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = ""
		return m.reload()

	case tickMsg:
		// Load the Explorer's index outside the in-flight guard, so a snapshot
		// already running does not skip it.
		var indexCmd tea.Cmd
		if m.tab == TabFiles {
			indexCmd = m.loadIndex()
		}

		// Stacking snapshots would spawn git faster than they finish.
		if !m.loadAt.IsZero() {
			return m, tea.Batch(indexCmd, tick(m.refreshEvery()))
		}
		next, cmd := m.reload()
		return next, tea.Batch(indexCmd, cmd, tick(next.refreshEvery()))

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.overlay.kind != overlayNone {
		return m.handleOverlayKey(msg)
	}

	// While a filter is being typed every printable key is text, so it must be
	// asked before any binding that would claim a letter.
	if m.filtering {
		return m.handleFilterKey(msg)
	}

	// Hunk mode rebinds movement and space, so it is asked first. Line mode
	// sits inside it and rebinds them again.
	if m.hunkMode {
		if m.lineMode {
			if next, cmd, handled := m.handleLineKey(msg.String()); handled {
				return next, cmd
			}
		}
		if next, cmd, handled := m.handleHunkKey(msg.String()); handled {
			return next, cmd
		}
	}

	// Annotations rebind enter and the parent walk while they are on.
	if m.blameOn {
		if next, cmd, handled := m.handleBlameKey(msg.String()); handled {
			return next, cmd
		}
	}

	// The focused panel gets first refusal, so `d` can mean discard here and
	// delete there.
	if next, cmd, handled := m.handlePanelKey(msg.String()); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleRemoteKey(msg.String()); handled {
		return next, cmd
	}

	switch msg.String() {
	case "q":
		return m.confirmQuit()

	// ctrl+c is the terminal's own abort, and a program that argues with it is
	// a program you cannot get out of.
	case "ctrl+c":
		return m, tea.Quit

	case "y", "Y":
		return m.copyPath(msg.String() == "Y")

	case "?":
		m.overlay = overlay{kind: overlayHelp}
		return m, nil

	case "R":
		m.loading = true

		// A disk listing is never re-read on the tick, by design — it is the
		// costliest read in the tab and the tree it covers is one git says
		// nothing about. That leaves it stale until something drops it, and
		// this key is the only thing that does. A fresh map rather than a
		// clear: the old one is shared with every earlier copy of the model.
		if m.tab == TabFiles {
			m.fsIndex = make(map[string][]fsEntry)
			m.stats = make(map[string]fileMeta)
			next, cmd := m.reload()
			if _, inIndex := next.index[next.cwd]; !inIndex {
				return next, tea.Batch(cmd, next.readDir(next.cwd))
			}
			return next, cmd
		}
		return m.reload()

	case "1", "2", "3", "4":
		return m.openTab(Tab(msg.String()[0] - '1'))

	case "/":
		m.startFilter()
		return m, nil

	case "b":
		return m.toggleBlame()

	case "T":
		return m.toggleTheme()

	case "w":
		return m.toggleWhitespace()

	case "[":
		return m.cycleContext(-1)

	case "]":
		return m.cycleContext(1)

	case "S":
		return m.askSearch()

	case "E":
		return m, m.loadReflog()

	case "W":
		return m, m.loadWorktrees()

	case "v":
		// Hunk marking reads line offsets straight out of the unified text.
		if m.hunkMode {
			m.status = "leave hunk mode to change the diff layout"
			return m, nil
		}
		m.splitDiff = !m.splitDiff
		m.mainOffset = m.clampMainOffset(m.mainOffset)
		// The Explorer's patch obeys the same setting and is shorter when paired.
		m.previewOffset = m.clampPreviewOffset(m.previewOffset)
		return m, nil

	case "tab", "right", "l":
		return m.movePane(1)

	case "shift+tab", "left", "h":
		return m.movePane(-1)

	case "j", "down":
		return m.moveCursor(1)

	case "k", "up":
		return m.moveCursor(-1)

	case "g":
		if m.focus.content() {
			m.mainOffset = 0
			return m, nil
		}
		m.cursor[m.focus] = 0
		m.syncOffset(m.focus)
		return m, m.refreshPreview()

	case "G":
		if m.focus.content() {
			m.mainOffset = m.clampMainOffset(m.mainLines())
			return m, nil
		}
		m.cursor[m.focus] = max(m.panelLen(m.focus)-1, 0)
		m.syncOffset(m.focus)
		return m, m.refreshPreview()

	case "ctrl+f", "pgdown":
		m.mainOffset = m.clampMainOffset(m.mainOffset + m.mainHeight())
		return m, nil

	case "ctrl+b", "pgup":
		m.mainOffset = m.clampMainOffset(m.mainOffset - m.mainHeight())
		return m, nil

	case "ctrl+d":
		m.mainOffset = m.clampMainOffset(m.mainOffset + m.mainHeight()/2)
		return m, nil

	case "ctrl+u":
		m.mainOffset = m.clampMainOffset(m.mainOffset - m.mainHeight()/2)
		return m, nil
	}
	return m, nil
}

// confirmQuit asks before leaving, naming anything uncommitted.
func (m Model) confirmQuit() (tea.Model, tea.Cmd) {
	body := "Quit?"
	if n := len(m.snap.Files); n > 0 {
		body = fmt.Sprintf("Quit with %d uncommitted %s?", n, plural(n, "change", "changes"))
	}
	m.askConfirm("Quit", body, false, func() tea.Cmd { return tea.Quit })
	return m, nil
}

// openTab switches workspace, landing where this tab was left. Reopening the
// current tab is a no-op.
func (m Model) openTab(t Tab) (tea.Model, tea.Cmd) {
	if t < 0 || t >= tabCount || t == m.tab {
		return m, nil
	}
	m.lastFocus[m.tab] = m.focus
	m.tab = t
	m.mainOffset = 0
	m.hunkMode = false

	// A zero-valued model can hold another tab's pane.
	m.focus = m.lastFocus[t]
	if !slices.Contains(tabPanes[t], m.focus) {
		m.focus = landingPane[t]
	}

	// Nothing refreshes the listing while the tab is closed, so read it on
	// every open.
	var load tea.Cmd
	if t == TabFiles {
		load = m.loadIndex()
	}

	// Split from the return: refreshPreview mutates m, and evaluation order
	// inside a single return statement is unspecified.
	preview := m.refreshPreview()
	return m, tea.Batch(load, preview)
}

// movePane walks the focus across the open tab's columns, stopping at the
// ends rather than wrapping.
func (m Model) movePane(delta int) (tea.Model, tea.Cmd) {
	panes := tabPanes[m.tab]
	i := slices.Index(panes, m.focus)
	if i < 0 {
		i = 0
	}
	next := clamp(i+delta, 0, len(panes)-1)
	if next == i {
		return m, nil
	}
	m.focus = panes[next]
	// A content pane shows what the list already selected.
	if m.focus.content() {
		return m, nil
	}
	return m, m.refreshPreview()
}

func (m Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	if m.focus.content() {
		m.mainOffset = m.clampMainOffset(m.mainOffset + delta)
		return m, nil
	}
	n := m.panelLen(m.focus)
	if n == 0 {
		return m, nil
	}
	m.cursor[m.focus] = clamp(m.cursor[m.focus]+delta, 0, n-1)
	m.syncOffset(m.focus)
	return m, m.refreshPreview()
}

// syncOffset scrolls a panel's viewport just far enough to keep its cursor
// visible. The offset persists rather than being derived each frame.
func (m *Model) syncOffset(p Panel) {
	innerH := m.paneHeight(p) - 2
	_, _, offset := window(m.panelLen(p), m.cursor[p], m.offset[p], innerH)
	m.offset[p] = offset
}

// paneWidths splits the terminal across the open tab's columns, the last
// absorbing the rounding remainder. The renderer and the pointer routing both
// read it.
func (m Model) paneWidths() []int {
	return split(m.width, tabWeights[m.tab], minPaneW)
}

// paneHeights splits a column's rows across the panes stacked in it. A column
// holding one pane gives it everything.
func (m Model) paneHeights(column []Panel) []int {
	if len(column) == 1 {
		return []int{m.bodyHeight()}
	}
	weights := make([]int, len(column))
	for i, p := range column {
		weights[i] = stackWeights[p]
	}
	return split(m.bodyHeight(), weights, minPaneH)
}

// paneHeight is the outer height of one pane, which is the whole body unless
// it shares its column.
func (m Model) paneHeight(p Panel) int {
	for _, column := range tabColumns[m.tab] {
		if i := slices.Index(column, p); i >= 0 {
			return m.paneHeights(column)[i]
		}
	}
	return m.bodyHeight()
}

// split apportions total by weight, giving the remainder to the last share.
// The result always sums to exactly total: honouring a minimum that no longer
// fits would make the panes overflow their column and tear the frame below, so
// when even the minimum will not fit for every share the panes shrink evenly
// instead.
func split(total int, weights []int, minimum int) []int {
	n := len(weights)
	out := make([]int, n)
	if n == 0 {
		return out
	}

	// Not enough room to frame every pane at its minimum: spread what there is
	// rather than overflowing.
	if total < minimum*n {
		base, rem := total/n, total%n
		for i := range out {
			out[i] = base
			if n-1-i < rem { // the remainder lands on the last panes
				out[i]++
			}
		}
		return out
	}

	used := 0
	for i := 0; i < n-1; i++ {
		share := max(total*weights[i]/100, minimum)
		// Never take so much that a later pane would drop below its minimum.
		share = min(share, total-used-minimum*(n-1-i))
		out[i] = share
		used += share
	}
	out[n-1] = total - used
	return out
}

// bodyHeight is the rows left for the panes, after the tab bar, the footer and
// the Changes tab's banner.
func (m Model) bodyHeight() int { return max(m.height-2-m.bannerHeight(), 0) }

// bannerHeight is 1 where a banner is drawn. Only the Changes tab has one:
// staged counts, or the stopped operation blocking everything else.
func (m Model) bannerHeight() int {
	if m.tab == TabChanges {
		return 1
	}
	return 0
}

func (m Model) panelLen(p Panel) int {
	switch p {
	case PanelFiles:
		return len(m.files())
	case PanelBranches:
		return len(m.branches())
	case PanelWorktrees:
		return len(m.worktrees())
	case PanelCommits:
		return len(m.commits())
	case PanelStash:
		return len(m.stashes())
	case PanelCommitFiles:
		return len(m.commitFiles)
	case PanelStashFiles:
		return len(m.stashFiles)
	case PanelParent, PanelEntries, PanelPreview:
		return m.explorerLen(p)
	}
	return 0
}

// clampCursors keeps every cursor inside its list after a refresh shrinks it.
func (m *Model) clampCursors() {
	for p := range panelCount {
		m.cursor[p] = clamp(m.cursor[p], 0, max(m.panelLen(Panel(p))-1, 0))
		m.syncOffset(Panel(p))
	}

	// dirCursor holds cursors too, and an operation can delete the entry one of
	// them points at.
	for dir, cursor := range m.dirCursor {
		n := len(m.visible(m.listingOf(dir)))
		if clamped := clamp(cursor, 0, max(n-1, 0)); clamped != cursor {
			m.dirCursor[dir] = clamped
		}
	}
}

// mainHeight is the visible rows of the tab's content pane, which is what a
// page-scroll moves by.
func (m Model) mainHeight() int {
	for _, p := range tabPanes[m.tab] {
		if p.content() {
			return max(m.paneHeight(p)-2, 1)
		}
	}
	return max(m.bodyHeight()-2, 1)
}

func (m Model) mainLines() int {
	if m.blameOn {
		return m.blameLineCount()
	}
	if m.splitDiff {
		return splitLineCount(m.mainContent)
	}
	return len(strings.Split(strings.TrimRight(m.mainContent, "\n"), "\n"))
}

func (m Model) clampMainOffset(v int) int {
	return clamp(v, 0, max(m.mainLines()-m.mainHeight(), 0))
}

// refreshPreview issues the git call for the current selection, tagged so a
// stale reply can be dropped on arrival.
func (m *Model) refreshPreview() tea.Cmd {
	repo, ctx := m.repo, m.ctx

	// Before anything that touches mainContent or blame: the default at the
	// bottom clears Local Changes' pane.
	if explorerPanel(m.focus) {
		return m.refreshExplorerPreview()
	}

	// Annotations are of a file, not of a selection: they follow the cursor
	// instead of being replaced by the selection's patch.
	if blame := m.followBlame(); m.blameOn {
		return blame
	}

	// A content pane shows what its tab's list selected.
	if m.focus.content() {
		return nil
	}

	switch m.focus {
	case PanelFiles:
		if len(m.files()) == 0 {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Diff", "", nil
			return nil
		}
		file := m.files()[m.cursor[PanelFiles]]
		key := "file:" + file.Path
		m.previewKey = key
		// A path staged with nothing left in the working tree has no unstaged
		// diff, so show the staged one.
		staged := file.Staged() && (file.Work == '.' || file.Work == 0)
		m.previewStaged = staged
		// An untracked file previews as its own source, so it is coloured as
		// source. The palette is read here: the closure cannot see the theme.
		palette := currentSyntax()
		opts := m.diffOpts()
		return func() tea.Msg {
			if file.Untracked() {
				content, err := repo.UntrackedPreview(ctx, file.Path)
				return previewMsg{
					key: key, title: "New file — " + file.Path, content: content,
					styled: highlight(file.Path, content, palette), err: err,
				}
			}
			content, err := repo.Diff(ctx, file.Path, staged, opts)
			title := "Diff — " + file.Path
			if staged {
				title = "Staged — " + file.Path
			}
			return previewMsg{key: key, title: title, content: content, err: err}
		}

	case PanelBranches:
		if len(m.branches()) == 0 {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Log", "", nil
			return nil
		}
		branch := m.branches()[m.cursor[PanelBranches]]
		key := "branch:" + branch.Ref()
		m.previewKey = key
		preview := func() tea.Msg {
			content, err := repo.BranchLog(ctx, branch.Ref(), 100)
			return previewMsg{key: key, title: "Log — " + branch.Name, content: renderPrettyLog(content), err: err}
		}

		// The Commits panel lists whichever ref is selected here, so stepping
		// right off a branch lands in its history.
		if aimed := m.aimLog(branch.Ref()); aimed != nil {
			return tea.Batch(preview, aimed)
		}
		return preview

	case PanelWorktrees:
		if len(m.worktrees()) == 0 {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Log", "", nil
			return nil
		}
		tree := m.worktrees()[m.cursor[PanelWorktrees]]
		ref := worktreeRef(tree)
		key := "worktree:" + tree.Path
		m.previewKey = key
		preview := func() tea.Msg {
			content, err := repo.BranchLog(ctx, ref, 100)
			return previewMsg{key: key, title: "Log — " + tree.Name(), content: renderPrettyLog(content), err: err}
		}

		// Aim the commit list at this checkout's tip, so stepping right lands in
		// what the worktree is sitting on.
		if aimed := m.aimLog(ref); aimed != nil {
			return tea.Batch(preview, aimed)
		}
		return preview

	case PanelCommits:
		if len(m.commits()) == 0 {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Commit", "", nil
			m.commitSHA, m.commitFiles = "", nil
			return nil
		}
		commit := m.commits()[m.cursor[PanelCommits]]
		key := "commit:" + commit.SHA
		m.previewKey = key
		// The file list belongs to the commit, so it is read alongside the patch.
		files := m.loadCommitFiles()
		opts := m.diffOpts()
		return tea.Batch(files, func() tea.Msg {
			content, err := repo.CommitDiff(ctx, commit.SHA, opts)
			return previewMsg{key: key, title: "Commit " + commit.Short, content: content, err: err}
		})

	case PanelCommitFiles:
		file, ok := m.selectedCommitFile()
		if !ok || m.commitSHA == "" {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Commit", "", nil
			return nil
		}
		sha := m.commitSHA
		key := "commitfile:" + sha + ":" + file.Path
		m.previewKey = key
		opts := m.diffOpts()
		return func() tea.Msg {
			content, err := repo.CommitFileDiff(ctx, sha, file.Path, opts)
			return previewMsg{key: key, title: "Diff — " + file.Path, content: content, err: err}
		}

	case PanelStash:
		if len(m.stashes()) == 0 {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Stash", "", nil
			m.stashRef, m.stashFiles, m.stashMarks = "", nil, nil
			return nil
		}
		stash := m.stashes()[m.cursor[PanelStash]]
		key := "stash:" + stash.Ref
		m.previewKey = key
		// The file list belongs to the entry, so it is read alongside the patch.
		files := m.loadStashFiles()
		opts := m.diffOpts()
		return tea.Batch(files, func() tea.Msg {
			content, err := repo.StashDiff(ctx, stash.Ref, opts)
			return previewMsg{key: key, title: "Stash " + stash.Ref, content: content, err: err}
		})

	case PanelStashFiles:
		file, ok := m.selectedStashFile()
		if !ok || m.stashRef == "" {
			m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Diff", "", nil
			return nil
		}
		ref := m.stashRef
		key := "stashfile:" + ref + ":" + file.Path
		m.previewKey = key
		opts := m.diffOpts()
		return func() tea.Msg {
			content, err := repo.StashFileDiff(ctx, ref, file.Path, opts)
			return previewMsg{key: key, title: "Diff — " + file.Path, content: content, err: err}
		}
	}

	m.previewKey, m.mainTitle, m.mainContent, m.mainStyled = "", "Status", "", nil
	return nil
}

// renderPrettyLog turns the unit-separated branch log into aligned columns.
func renderPrettyLog(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		subject, meta, found := strings.Cut(line, "\x1f")
		if !found {
			b.WriteString(line + "\n")
			continue
		}
		b.WriteString(subject + "  " + theme.DimStyle.Render("("+meta+")") + "\n")
	}
	return b.String()
}

// --- view ---

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.overlay.kind != overlayNone {
		return m.overlayView()
	}
	return m.frameOnly()
}

// frameOnly renders the frame without any modal on top.
func (m Model) frameOnly() string {
	widths := m.paneWidths()

	boxes := make([]string, 0, len(widths))
	for i, column := range tabColumns[m.tab] {
		w := widths[i]
		heights := m.paneHeights(column)

		stacked := make([]string, 0, len(column))
		for j, p := range column {
			h := heights[j]
			stacked = append(stacked, renderBox(
				m.panelTitle(p),
				m.panelLines(p, h-2, w-2),
				w, h, m.focus == p))
		}
		boxes = append(boxes, lipgloss.JoinVertical(lipgloss.Left, stacked...))
	}

	rows := []string{m.tabBar()}
	if m.bannerHeight() > 0 {
		rows = append(rows, m.banner())
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, boxes...), m.footer())

	// A last guard against tearing: on a terminal too short even for the tab
	// bar, banner and footer, emit exactly m.height rows so bubbletea's diff
	// never leaves a stale line behind on the alternate screen.
	return clampFrame(strings.Join(rows, "\n"), m.width, m.height)
}

// clampFrame forces a rendered frame to exactly h rows of w columns, cutting any
// overflow and padding any shortfall. Overflow would push the terminal to scroll
// and strand the top of the previous frame on screen.
func clampFrame(frame string, w, h int) string {
	lines := strings.Split(frame, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = fitLine(line, w)
	}
	return strings.Join(lines, "\n")
}

// panelTitle is the pane's border title: a count for a list, and whatever the
// last preview named itself for a content pane.
func (m Model) panelTitle(p Panel) string {
	if p.content() {
		if m.blameOn {
			return m.blameTitle()
		}
		if m.hunkMode {
			if file, ok := m.selectedFile(); ok {
				total := len(hunkRanges(m.mainContent))
				if m.lineMode {
					return m.lineTitle(file.Path, total)
				}
				return hunkTitle(file.Path, m.hunkCursor, total, m.previewStaged)
			}
		}
		return m.mainTitle
	}

	name, unit := "", ""
	switch p {
	case PanelFiles:
		name, unit = "Changes", "file"
	case PanelBranches:
		name, unit = "Branches", "ref"
	case PanelWorktrees:
		name, unit = "Worktrees", "worktree"
	case PanelCommits:
		name, unit = "Commits", "commit"
	case PanelStash:
		name, unit = "Stash", "entry"
	case PanelCommitFiles:
		name, unit = "Files", "file"
	case PanelStashFiles:
		name, unit = "Files", "file"
	case PanelParent, PanelEntries, PanelPreview:
		return m.explorerTitle(p)
	}

	// The commit list is of one ref, and which ref belongs in its title. A
	// query narrows it further, and a short list is not a short history.
	suffix := m.filterSuffix(p)
	if p == PanelCommits {
		suffix = m.logSuffix() + m.searchSuffix() + suffix
	}

	n := m.panelLen(p)
	if n == 0 {
		return name + suffix
	}
	plural := unit + "s"
	if unit == "entry" {
		plural = "entries"
	}
	if n == 1 {
		plural = unit
	}
	return fmt.Sprintf("%s — %d %s", name, n, plural) + suffix
}

// panelLines builds only the rows visible in a viewport of innerH rows, each
// fitted to innerW columns.
func (m Model) panelLines(p Panel, innerH, innerW int) []string {
	if p.content() {
		// Blame is asked before hunk mode; entering one turns the other off, so
		// they can never both be on.
		if m.blameOn {
			return m.blamePaneLines(innerH, innerW)
		}
		if m.hunkMode {
			if m.lineMode {
				return m.linePaneLines(innerH)
			}
			return hunkPaneLines(m.mainContent, hunkRanges(m.mainContent), m.hunkCursor, m.mainOffset, innerH)
		}
		if m.loading && m.mainContent == "" {
			return emptyLines("loading…")
		}
		// A file rather than a patch: no second side to lay it against.
		if len(m.mainStyled) > 0 {
			start := clamp(m.mainOffset, 0, max(len(m.mainStyled)-1, 0))
			end := min(start+innerH, len(m.mainStyled))
			return m.mainStyled[start:end]
		}
		if m.splitDiff {
			return splitDiffLines(m.mainContent, m.mainOffset, innerH, innerW)
		}
		return diffLines(m.mainContent, m.mainOffset, innerH)
	}

	focused := m.focus == p
	n := m.panelLen(p)
	start, end, _ := window(n, m.cursor[p], m.offset[p], innerH)

	// An empty list means something different when a filter is on: the entries
	// exist, they just do not match.
	if n == 0 && m.filter[p] != "" {
		return emptyLines("nothing matches " + m.filter[p])
	}

	switch p {
	case PanelFiles:
		if n == 0 {
			return emptyLines("working tree clean")
		}
		return m.fileLines(m.files(), start, end, m.cursor[p], innerW, focused)
	case PanelBranches:
		if n == 0 {
			return emptyLines("no refs")
		}
		return branchLines(m.branches(), start, end, m.cursor[p], innerW, focused)
	case PanelWorktrees:
		if n == 0 {
			return emptyLines("no worktrees")
		}
		return m.worktreeLines(start, end, m.cursor[p], innerW, focused)
	case PanelCommits:
		if n == 0 {
			return emptyLines("no commits")
		}
		// The rails are of the whole list: a lane's column is decided by the
		// commits above the window, not inside it.
		var rows []graphRow
		if m.graphOn {
			rows = graphRows(m.commits())
		}
		return commitLines(m.commits(), rows, m.unpushed(), start, end, m.cursor[p], innerW, focused)
	case PanelStash:
		if n == 0 {
			return emptyLines("no stashes")
		}
		return stashLines(m.stashes(), start, end, m.cursor[p], innerW, focused)
	case PanelCommitFiles:
		if n == 0 {
			return emptyLines("no files")
		}
		return m.commitFileLines(start, end, innerW, focused)
	case PanelStashFiles:
		if n == 0 {
			return emptyLines("no stash selected")
		}
		return m.stashFileLines(start, end, innerW, focused)
	case PanelParent, PanelEntries, PanelPreview:
		return m.explorerLines(p, innerH, innerW)
	}
	return nil
}

// footer shows the keys that apply to the focused panel, plus any error.
func (m Model) footer() string {
	if m.status != "" {
		return fitLine(theme.ErrorStyle.Render(" "+m.status), m.width)
	}
	if m.busy != "" {
		return fitLine(" "+theme.FooterKeyStyle.Render("⟳ ")+theme.NormalStyle.Render(m.busy), m.width)
	}

	// Annotations replace the pane's own keys with their own.
	if m.blameOn {
		return keyLine(append(blameKeyHints(), [2]string{"?", "help"}), m.width)
	}

	// While a filter is being typed the letters are text, not bindings.
	if m.filtering {
		return keyLine(filterKeyHints(), m.width)
	}

	// In hunk mode the panel bindings are rebound, and line mode rebinds them
	// again.
	if m.hunkMode {
		hints := hunkKeyHints()
		if m.lineMode {
			hints = lineKeyHints()
		}
		return keyLine(append(hints, [2]string{"?", "help"}), m.width)
	}

	// The focused panel's own bindings come first: those are what change.
	pairs := append(m.panelKeyHints(m.focus), remoteKeyHints()...)
	pairs = append(pairs, [2]string{"?", "help"}, [2]string{"q", "quit"})
	return keyLine(pairs, m.width)
}

// keyLine renders key/description pairs as one footer line.
func keyLine(pairs [][2]string, width int) string {
	var parts []string
	for _, p := range pairs {
		parts = append(parts, theme.FooterKeyStyle.Render(p[0])+" "+theme.FooterDescStyle.Render(p[1]))
	}
	return fitLine(" "+strings.Join(parts, theme.DimStyle.Render(" · ")), width)
}

// helpLines is the whole key list, built without a model so its length is
// known before there is anything to draw.
func helpLines() []string {
	sections := []struct {
		heading string
		rows    [][2]string
	}{
		{"Everywhere", [][2]string{
			{"1 – 4", "open a tab"},
			{"tab / shift+tab", "move between panes"},
			{"h / l, ← / →", "move between panes"},
			{"j / k, ↓ / ↑", "move selection, or scroll"},
			{"g / G", "first / last"},
			{"/", "filter the focused list"},
			{"S", "search commits, in git"},
			{"v", "unified or side-by-side diff"},
			{"b", "annotate the selected file (blame)"},
			{"ctrl+f / ctrl+b", "page the diff"},
			{"w", "hide whitespace-only changes"},
			{"[ / ]", "narrower / wider diff context"},
			{"E", "reflog: where HEAD has been"},
			{"W", "worktrees: open, add, remove"},
			{"T", "light or dark"},
			{"R", "refresh now"},
			{"y / Y", "copy the path / the full path"},
			{"q", "quit — asks first"},
		}},
		{"Tabs", [][2]string{
			{"1", "Local Changes — files and their diff"},
			{"2", "Log — branches, worktrees, commits"},
			{"3", "Stash — entries and their diff"},
			{"4", "Explorer — the tree, with a preview"},
		}},
		{"Remote", [][2]string{
			{"f", "fetch and prune"},
			{"p", "pull — offers a way out if it cannot"},
			{"P", "push, after listing what would go"},
			{"F", "force push (--force-with-lease)"},
		}},
		{"Files", [][2]string{
			{"space", "stage / unstage"},
			{"m", "mark — one key then acts on every marked file"},
			{"a / u", "stage all / unstage all"},
			{"enter", "pick hunks in the diff pane"},
			{"c", "commit the index"},
			{"C", "write the message in $EDITOR"},
			{"A", "amend the last commit"},
			{"s", "stash: asks what goes in"},
			{"d", "discard changes"},
			{"H", "history of this file"},
			{"t", "untrack, keeping the file"},
			{"i", "add to .gitignore"},
			{"x", "delete from disk"},
			{"o", "show this file in the Explorer"},
		}},
		{"Conflicts", [][2]string{
			{"r", "keep ours, keep theirs, or mark resolved"},
			{"space", "mark resolved as it stands"},
		}},
		{"Branches", [][2]string{
			{"enter", "check out"},
			{"n", "new branch"},
			{"c", "compare with the current branch"},
			{"m", "rename"},
			{"M", "merge into current"},
			{"r", "rebase the current branch onto this"},
			{"t", "tag this ref's tip"},
			{"P", "on a tag: publish it"},
			{"d / D", "delete / force-delete — on a remote branch, on the remote"},
		}},
		{"Worktrees (Log tab)", [][2]string{
			{"enter", "open this checkout in the tool"},
			{"n", "add a worktree"},
			{"d", "remove this worktree"},
			{"▸", "marks the one open now"},
		}},
		{"Hunks (enter, in Changes)", [][2]string{
			{"j / k", "next / previous hunk"},
			{"space", "stage / unstage this hunk"},
			{"enter", "pick single lines out of it"},
			{"esc", "back to the file list"},
		}},
		{"Lines (enter, in hunks)", [][2]string{
			{"j / k", "next / previous changed line"},
			{"space", "pick this line"},
			{"a", "pick every line in the hunk"},
			{"enter", "stage what is picked"},
			{"esc", "back to the hunks"},
		}},
		{"Blame (b, then the diff pane)", [][2]string{
			{"j / k", "move down the file"},
			{"enter", "open this line's commit"},
			{"<", "annotate as the parent held it"},
			{">", "back to the working copy"},
			{"b", "back to the diff"},
		}},
		{"Commits", [][2]string{
			{"s", "squash into parent"},
			{"r", "reword"},
			{"d", "drop"},
			{"K / J", "move later / earlier in history"},
			{"c", "cherry-pick onto current"},
			{"v", "revert"},
			{"z", "undo the last commit, keeping its changes"},
			{"t", "change the date"},
			{"n", "new branch at this commit"},
			{"P", "push only as far as this commit"},
			{"L", "draw the branch graph"},
			{"↑", "marks a commit no remote holds"},
		}},
		{"Stash", [][2]string{
			{"space", "apply"},
			{"enter", "pop"},
			{"d", "drop"},
		}},
		{"Explorer", [][2]string{
			{"h / l, ← / →", "out of a directory, into one — on a file, into its preview"},
			{"enter", "into a directory, or open a file in $EDITOR"},
			{"e", "content, diff, blame, history"},
			{"o", "go to any path in the repository"},
			{"O", "show this file in Local Changes"},
			{"s", "search file contents"},
			{",", "order the listing"},
			{".", "show or hide dotfiles"},
			{"space", "stage / unstage"},
			{"M", "mark — one key then acts on every marked path"},
			{"n / N", "new file / new directory"},
			{"m", "rename"},
			{"d", "discard changes"},
			{"x", "delete"},
			{"i", "add to .gitignore"},
		}},
	}

	var lines []string
	for i, section := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, " "+theme.TitleFocusStyle.Render(section.heading))
		for _, r := range section.rows {
			lines = append(lines, fmt.Sprintf("   %-18s %s",
				theme.FooterKeyStyle.Render(r[0]), theme.MutedStyle.Render(r[1])))
		}
	}
	return lines
}

// helpHeight is how many key rows fit, leaving room for the frame and the
// scroll hint.
func (m Model) helpHeight() int { return max(m.height-4, 3) }

// helpView draws a window on the key list: it is longer than a terminal is
// tall.
func (m Model) helpView() string {
	all := helpLines()
	height := m.helpHeight()

	start := clamp(m.overlay.offset, 0, max(len(all)-height, 0))
	end := min(start+height, len(all))
	lines := append([]string{}, all[start:end]...)

	keys := theme.DimStyle.Render(" any key closes")
	if len(all) > height {
		keys = " " + theme.FooterKeyStyle.Render("j/k") + " " +
			theme.MutedStyle.Render(fmt.Sprintf("scroll (%d–%d of %d)", start+1, end, len(all))) +
			theme.DimStyle.Render("   ") + theme.DimStyle.Render("any other key closes")
	}
	lines = append(lines, keys)

	box := renderBox("Keys", lines, min(m.width-4, 52), min(len(lines)+2, m.height), true)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
