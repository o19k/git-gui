package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/o19k/git-gui/internal/theme"
)

// The tests run without a terminal, where lipgloss strips every escape and a
// styled string renders identically to a bare one. Forcing a colour profile
// makes the colouring observable at all.
func withColour(t *testing.T) {
	t.Helper()
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(before) })
}

// The pane scrolls by line number and a search jumps to one, so colouring must
// not change how many lines there are.
func TestHighlightKeepsTheLineCount(t *testing.T) {
	files := map[string]string{
		"main.go":   "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n",
		"app.py":    "import os\n\n\ndef main():\n    return os.getcwd()\n",
		"index.ts":  "export const x: number = 1;\nconsole.log(x);\n",
		"style.css": "body {\n  color: red;\n}\n",
		"conf.yaml": "key: value\nlist:\n  - one\n  - two\n",
	}

	for name, content := range files {
		t.Run(name, func(t *testing.T) {
			lines := highlight(name, content, currentSyntax())
			if lines == nil {
				t.Fatalf("%s was not recognised as any language", name)
			}
			want := len(strings.Split(strings.TrimRight(content, "\n"), "\n"))
			if got := len(lines); got != want {
				t.Errorf("colouring turned %d lines into %d", want, got)
			}
		})
	}
}

// Colour must not change the text, only how it is drawn: a line the width
// calculation measures differently would tear the frame.
func TestHighlightLeavesTheTextItself(t *testing.T) {
	withColour(t)

	const content = "package main\n\n// a comment\nvar n = 42\n"
	lines := highlight("main.go", content, currentSyntax())
	if lines == nil {
		t.Fatal("Go was not recognised")
	}

	plain := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i, line := range lines {
		if got := ansi.Strip(line); got != plain[i] {
			t.Errorf("line %d reads %q, want %q", i, got, plain[i])
		}
		if lipgloss.Width(line) != lipgloss.Width(plain[i]) {
			t.Errorf("line %d measures %d cells, want %d",
				i, lipgloss.Width(line), lipgloss.Width(plain[i]))
		}
	}
}

func TestHighlightColoursTheParts(t *testing.T) {
	withColour(t)

	lines := highlight("main.go", "// a comment\nvar name = \"text\"\n", currentSyntax())
	if lines == nil {
		t.Fatal("Go was not recognised")
	}
	for i, line := range lines {
		if !strings.Contains(line, "\x1b[") {
			t.Errorf("line %d has no colour at all: %q", i, line)
		}
	}
}

// Guessing at a language and colouring the guess is worse than leaving the
// text alone, so an unknown extension says so rather than picking one.
func TestHighlightDeclinesWhatItDoesNotKnow(t *testing.T) {
	if lines := highlight("notes.zzzz", "some text\n", currentSyntax()); lines != nil {
		t.Errorf("an unknown extension was coloured as %v", lines)
	}
}

// The palette is chosen as a whole, so source code has to follow the switch
// like everything else on screen.
func TestHighlightFollowsThePalette(t *testing.T) {
	withColour(t)

	theme.UseLight(false)
	dark := highlight("main.go", "var n = 42\n", currentSyntax())
	theme.UseLight(true)
	light := highlight("main.go", "var n = 42\n", currentSyntax())
	t.Cleanup(func() { theme.UseLight(false) })

	if len(dark) == 0 || len(light) == 0 {
		t.Fatal("Go was not recognised")
	}
	if dark[0] == light[0] {
		t.Error("the same line is drawn identically in both palettes")
	}
}

// The preview scrolls and searches by line number against previewLen, which
// counts the plain content, so the two views of one file have to agree.
func TestTheStyledPreviewHasAsManyLinesAsThePlainOne(t *testing.T) {
	content := strings.Repeat("var n = 1\n", 50)
	m := Model{
		previewFor:     previewID{path: "main.go", kind: previewContent},
		previewContent: content,
	}
	next, _ := m.handleExplorerPreview(explorerPreviewMsg{
		id:      m.previewFor,
		title:   "main.go",
		content: content,
		styled:  highlight("main.go", content, currentSyntax()),
	})
	after := next.(Model)

	if len(after.previewStyled) == 0 {
		t.Fatal("the content was not coloured")
	}
	if got, want := len(after.previewStyled), after.previewLen(); got != want {
		t.Errorf("the coloured view holds %d lines and the plain one %d", got, want)
	}
}

// A symlink's target and a directory's listing are content of the same kind,
// and colouring them as whatever the extension suggests would invent syntax
// for a path. Only the read of a file's own bytes attaches colouring, so a
// message without it leaves the pane plain.
func TestOnlyAFilesOwnContentIsColoured(t *testing.T) {
	m := Model{previewFor: previewID{path: "link.go", kind: previewContent}}
	next, _ := m.handleExplorerPreview(explorerPreviewMsg{
		id:      m.previewFor,
		title:   "Link — link.go",
		content: "src/main.go",
	})
	if got := next.(Model).previewStyled; got != nil {
		t.Errorf("a symlink target was coloured as source: %v", got)
	}
}

// Colouring a 1,200-line file costs tens of milliseconds, which is several
// frames: it belongs to the read, on its goroutine, not to the handler that
// installs the reply.
func TestTheHandlerDoesNotColourAnything(t *testing.T) {
	withColour(t)

	m := Model{previewFor: previewID{path: "main.go", kind: previewContent}}
	next, _ := m.handleExplorerPreview(explorerPreviewMsg{
		id:      m.previewFor,
		title:   "main.go",
		content: "package main\n",
	})
	if got := next.(Model).previewStyled; got != nil {
		t.Errorf("the handler coloured the content itself: %v", got)
	}
}

// The colouring belongs to the content it was made from.
func TestMovingToAnotherFileDropsTheOldColouring(t *testing.T) {
	m := navModel()
	m.previewStyled = []string{"stale"}

	m.refreshPreview()
	if m.previewStyled != nil {
		t.Errorf("the previous file's colouring survived into %q", m.previewFor.path)
	}
}
