package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/o19k/git-gui/internal/theme"
)

// overlayKind is which modal, if any, is in front of the panels.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayConfirm
	overlayInput
	overlayChoice
	overlayText
	overlayList
)

// choice is one way out of a situation with more than two, each carrying the
// command it runs.
type choice struct {
	label  string
	hint   string
	danger bool
	// busy names the work the choice starts, for the footer. The action returns
	// a command, so it cannot reach the model to say so itself.
	busy   string
	action func() tea.Cmd
}

// overlay is the state of the active modal. Destructive actions are gated
// behind a confirm that names what it is about to destroy.
type overlay struct {
	kind   overlayKind
	title  string
	body   string
	value  string // input buffer
	danger bool   // colour the confirm as destructive

	// extra is drawn verbatim under the body, for a list the body is about.
	// Wrapping would run its entries together.
	extra []string

	// busy names the work accepting starts, for the footer. The action returns
	// a command, so it cannot reach the model to say so itself.
	busy string

	// compare marks the text overlay as a branch comparison, the only one f
	// means anything in. compareRef outlives the overlay, so it cannot serve.
	compare bool

	// items and query belong to overlayList: the rows to pick from and what
	// has been typed to narrow them. cursor and offset are shared with the
	// other list-shaped kinds.
	items []listItem
	query string

	// choices and cursor belong to overlayChoice; lines and offset to the
	// scrolling text of overlayText.
	choices []choice
	cursor  int
	lines   []string
	offset  int

	// action runs on accept, receiving the input buffer (empty for a confirm).
	action func(string) tea.Cmd
}

func (m Model) confirming() bool { return m.overlay.kind == overlayConfirm }

// askConfirm puts a yes/no gate in front of an action.
func (m *Model) askConfirm(title, body string, danger bool, action func() tea.Cmd) {
	m.overlay = overlay{
		kind:   overlayConfirm,
		title:  title,
		body:   body,
		danger: danger,
		action: func(string) tea.Cmd { return action() },
	}
}

// askInput puts a single-line prompt in front of an action.
func (m *Model) askInput(title, prefill string, action func(string) tea.Cmd) {
	m.overlay = overlay{kind: overlayInput, title: title, value: prefill, action: action}
}

// askChoice offers several ways forward where there is no obvious default —
// rebase or merge for a diverged pull, theirs or ours for a conflict.
func (m *Model) askChoice(title, body string, choices []choice) {
	if len(choices) == 0 {
		return
	}
	m.overlay = overlay{kind: overlayChoice, title: title, body: body, choices: choices}
}

// showText puts a scrollable block of text in front of the panels, for
// something that is read rather than answered.
func (m *Model) showText(title string, lines []string) {
	if len(lines) == 0 {
		lines = []string{theme.DimStyle.Render("nothing to show")}
	}
	m.overlay = overlay{kind: overlayText, title: title, lines: lines}
}

// textHeight is the rows a text overlay can use, leaving the frame and the
// key line room.
func (m Model) textHeight() int { return max(m.height-6, 3) }

// takeChoice runs one of a choice overlay's options and closes it.
func (m Model) takeChoice(i int) (tea.Model, tea.Cmd) {
	picked := m.overlay.choices[i]
	m.overlay = overlay{}
	m.busy = picked.busy
	return m, picked.action()
}

// handleOverlayKey routes a keystroke to the active modal. It returns the
// model and, when the modal accepted, the command it produced.
func (m Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay.kind {
	case overlayHelp:
		// The list is taller than the screen, so the movement keys scroll it.
		// Any other key still dismisses.
		height := m.helpHeight()
		last := max(len(helpLines())-height, 0)
		switch msg.String() {
		case "j", "down":
			m.overlay.offset = clamp(m.overlay.offset+1, 0, last)
		case "k", "up":
			m.overlay.offset = clamp(m.overlay.offset-1, 0, last)
		case "ctrl+f", "pgdown":
			m.overlay.offset = clamp(m.overlay.offset+height, 0, last)
		case "ctrl+b", "pgup":
			m.overlay.offset = clamp(m.overlay.offset-height, 0, last)
		case "g":
			m.overlay.offset = 0
		case "G":
			m.overlay.offset = last
		default:
			m.overlay = overlay{}
		}
		return m, nil

	case overlayConfirm:
		switch msg.String() {
		case "y", "Y", "enter":
			action, busy := m.overlay.action, m.overlay.busy
			m.overlay = overlay{}
			m.busy = busy
			return m, action("")
		case "n", "N", "esc", "q", "ctrl+c":
			m.overlay = overlay{}
		}
		return m, nil

	case overlayChoice:
		switch msg.String() {
		case "j", "down":
			m.overlay.cursor = clamp(m.overlay.cursor+1, 0, len(m.overlay.choices)-1)
		case "k", "up":
			m.overlay.cursor = clamp(m.overlay.cursor-1, 0, len(m.overlay.choices)-1)
		case "enter":
			return m.takeChoice(m.overlay.cursor)
		case "esc", "q", "ctrl+c":
			m.overlay = overlay{}
		default:
			// A number key picks its row outright, so the common case is one press.
			if n := int(msg.String()[0] - '1'); len(msg.String()) == 1 && n >= 0 && n < len(m.overlay.choices) {
				return m.takeChoice(n)
			}
		}
		return m, nil

	case overlayText:
		height := m.textHeight()
		switch msg.String() {
		case "j", "down":
			m.overlay.offset = clamp(m.overlay.offset+1, 0, max(len(m.overlay.lines)-height, 0))
		case "k", "up":
			m.overlay.offset = clamp(m.overlay.offset-1, 0, max(len(m.overlay.lines)-height, 0))
		case "ctrl+f", "pgdown":
			m.overlay.offset = clamp(m.overlay.offset+height, 0, max(len(m.overlay.lines)-height, 0))
		case "ctrl+b", "pgup":
			m.overlay.offset = clamp(m.overlay.offset-height, 0, max(len(m.overlay.lines)-height, 0))
		case "g":
			m.overlay.offset = 0
		case "G":
			m.overlay.offset = max(len(m.overlay.lines)-height, 0)
		case "f":
			if m.overlay.compare {
				return m, m.toggleCompareScope()
			}
		case "esc", "q", "enter", "ctrl+c":
			m.overlay = overlay{}
		}
		return m, nil

	case overlayInput:
		switch msg.Type {
		case tea.KeyEnter:
			action, value := m.overlay.action, m.overlay.value
			m.overlay = overlay{}
			return m, action(value)
		case tea.KeyEsc, tea.KeyCtrlC:
			m.overlay = overlay{}
			return m, nil
		case tea.KeyBackspace:
			if runes := []rune(m.overlay.value); len(runes) > 0 {
				m.overlay.value = string(runes[:len(runes)-1])
			}
			return m, nil
		case tea.KeyCtrlU:
			m.overlay.value = ""
			return m, nil
		case tea.KeySpace:
			m.overlay.value += " "
			return m, nil
		case tea.KeyRunes:
			m.overlay.value += string(msg.Runes)
			return m, nil
		}
		return m, nil

	case overlayList:
		return m.handleListKey(msg)
	}
	return m, nil
}

// overlayView draws the active modal centred over the panels.
func (m Model) overlayView() string {
	// A prompt is a sentence and reads better narrow; a listing is whatever git
	// printed and reads better wide.
	width := 60
	if m.overlay.kind == overlayText {
		width = max(m.width-8, 40)
	}

	var lines []string
	switch m.overlay.kind {
	case overlayHelp:
		return m.helpView()

	case overlayConfirm:
		lines = append(lines, "")
		for _, line := range wrap(m.overlay.body, width-4) {
			lines = append(lines, " "+theme.NormalStyle.Render(line))
		}
		if len(m.overlay.extra) > 0 {
			lines = append(lines, "")
			for _, line := range m.overlay.extra {
				lines = append(lines, " "+fitLine(line, width-2))
			}
		}
		accept := theme.FooterKeyStyle.Render("y")
		if m.overlay.danger {
			accept = theme.ErrorStyle.Render("y")
		}
		lines = append(lines, "",
			" "+accept+" "+theme.MutedStyle.Render("confirm")+
				theme.DimStyle.Render("   ")+
				theme.FooterKeyStyle.Render("n")+" "+theme.MutedStyle.Render("cancel"))

	case overlayChoice:
		lines = append(lines, "")
		for _, line := range wrap(m.overlay.body, width-4) {
			lines = append(lines, " "+theme.NormalStyle.Render(line))
		}
		// The paths the question is about sit between the body and the choices;
		// wrapping them into the prose would run them together.
		if len(m.overlay.extra) > 0 {
			lines = append(lines, "")
			for _, line := range m.overlay.extra {
				lines = append(lines, " "+fitLine(line, width-2))
			}
		}
		lines = append(lines, "")
		for i, c := range m.overlay.choices {
			label := theme.NormalStyle.Render(c.label)
			if c.danger {
				label = theme.ErrorStyle.Render(c.label)
			}
			marker := "  "
			if i == m.overlay.cursor {
				marker = theme.FooterKeyStyle.Render("▌ ")
			}
			row := marker + theme.FooterKeyStyle.Render(fmt.Sprint(i+1)) + " " + label
			// The hint sits beside the label where it fits, underneath where it
			// does not.
			inline := row + theme.DimStyle.Render(" — "+c.hint)
			switch {
			case c.hint == "":
				lines = append(lines, row)
			case lipgloss.Width(inline) <= width-2:
				lines = append(lines, inline)
			default:
				lines = append(lines, row)
				for _, line := range wrap(c.hint, width-8) {
					lines = append(lines, "     "+theme.DimStyle.Render(line))
				}
			}
		}
		lines = append(lines, "",
			" "+theme.FooterKeyStyle.Render("enter")+" "+theme.MutedStyle.Render("choose")+
				theme.DimStyle.Render("   ")+
				theme.FooterKeyStyle.Render("esc")+" "+theme.MutedStyle.Render("cancel"))

	case overlayText:
		height := m.textHeight()
		start := clamp(m.overlay.offset, 0, max(len(m.overlay.lines)-1, 0))
		end := min(start+height, len(m.overlay.lines))
		for _, line := range m.overlay.lines[start:end] {
			lines = append(lines, " "+line)
		}
		keys := " " + theme.FooterKeyStyle.Render("j/k") + " " + theme.MutedStyle.Render("scroll")
		if m.overlay.compare {
			keys += theme.DimStyle.Render("   ") +
				theme.FooterKeyStyle.Render("f") + " " + theme.MutedStyle.Render("file scope")
		}
		lines = append(lines, "", keys+
			theme.DimStyle.Render("   ")+
			theme.FooterKeyStyle.Render("esc")+" "+theme.MutedStyle.Render("close"))

	case overlayInput:
		// A block cursor at the end of the buffer.
		cursor := lipgloss.NewStyle().
			Background(lipgloss.Color(theme.Primary)).
			Foreground(lipgloss.Color(theme.BG)).
			Render(" ")
		lines = append(lines, "",
			" "+theme.NormalStyle.Render(m.overlay.value)+cursor,
			"",
			" "+theme.FooterKeyStyle.Render("enter")+" "+theme.MutedStyle.Render("accept")+
				theme.DimStyle.Render("   ")+
				theme.FooterKeyStyle.Render("esc")+" "+theme.MutedStyle.Render("cancel"))

	case overlayList:
		width = max(m.width-8, 40)
		lines = append(lines, "")
		cursor := lipgloss.NewStyle().
			Background(lipgloss.Color(theme.Primary)).
			Foreground(lipgloss.Color(theme.BG)).
			Render(" ")
		lines = append(lines, " "+theme.NormalStyle.Render(m.overlay.query)+cursor)
		lines = append(lines, "")
		listLines := m.listLines(width - 4)
		lines = append(lines, listLines...)
		lines = append(lines, "",
			" "+theme.FooterKeyStyle.Render("enter")+" "+theme.MutedStyle.Render("select")+
				theme.DimStyle.Render("   ")+
				theme.FooterKeyStyle.Render("esc")+" "+theme.MutedStyle.Render("cancel"))
	}

	boxWidth := min(width, m.width-2)
	boxHeight := len(lines) + 2
	box := renderBox(m.overlay.title, lines, boxWidth, boxHeight, true)

	x := max(0, (m.width-boxWidth)/2)
	y := max(0, (m.height-boxHeight)/2)

	// No room for the float: fill the screen instead.
	if m.width < 4 || m.height < 2 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	// Help uses the full screen.
	if m.overlay.kind == overlayHelp {
		return box
	}

	frame := m.frameOnly()
	return composite(frame, box, x, y)
}

// wrap breaks text into lines of at most w columns, on word boundaries.
func wrap(text string, w int) []string {
	if w <= 0 {
		return []string{text}
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
