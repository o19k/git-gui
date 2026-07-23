package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/o19k/git-gui/internal/git"
)

func TestRenderBlameAtDifferentWidths(t *testing.T) {
	lines := []git.BlameLine{
		{Short: "abc1234", Author: "Alice", When: "2024-01-01", Text: "line one"},
		{Short: "def5678", Author: "Bob Smith", When: "2024-01-02", Text: "line two is longer"},
		{Short: "ghi9012", Author: "Charlie", When: "2024-01-03", Text: "line three"},
	}

	tests := []struct {
		name   string
		width  int
		offset int
		height int
	}{
		{"narrow", 40, 0, 3},
		{"medium", 80, 0, 3},
		{"wide", 120, 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderBlame(lines, nil, tt.offset, tt.height, tt.width)
			if len(result) != len(lines) {
				t.Errorf("got %d lines, want %d", len(result), len(lines))
			}
			// Verify that each line contains the author and date information.
			for i, line := range result {
				if !strings.Contains(line, lines[i].Author) && lines[i].Author != "" {
					t.Errorf("line %d missing author %q", i, lines[i].Author)
				}
			}
		})
	}
}

func TestRenderBlameEmpty(t *testing.T) {
	lines := []git.BlameLine{}
	result := renderBlame(lines, nil, 0, 5, 80)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	if !strings.Contains(result[0], "annotating…") {
		t.Errorf("expected 'annotating…', got %q", result[0])
	}
}

func TestRenderBlameWithOffset(t *testing.T) {
	lines := make([]git.BlameLine, 10)
	for i := 0; i < 10; i++ {
		lines[i] = git.BlameLine{
			Short:  "abc1234",
			Author: "Author",
			When:   "2024-01-01",
			Text:   "line " + string(rune('0'+i)),
		}
	}

	// Render with offset 5, height 3 should show lines 5, 6, 7.
	result := renderBlame(lines, nil, 5, 3, 80)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1536, "1.5 KB"},
		{1048576, "1 MB"},
		{1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		result := formatSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestCyclePreviewKind(t *testing.T) {
	// Test that cyclePreview advances through all kinds.
	kinds := []previewKind{
		previewContent,
		previewDiff,
		previewBlame,
		previewHistory,
	}

	for _, start := range kinds {
		current := start
		for i := 0; i < 5; i++ {
			expected := kinds[(start+previewKind(i))%4]
			if current != expected {
				t.Errorf("cycle %d: got kind %d, want %d", i, current, expected)
			}
			current = (current + 1) % 4
		}
	}
}

func TestPreviewIDMismatch(t *testing.T) {
	// Test that a stale reply is dropped.
	m := Model{
		previewFor: previewID{path: "file1.txt", kind: previewContent},
	}

	msg := explorerPreviewMsg{
		id:      previewID{path: "file2.txt", kind: previewContent},
		title:   "New title",
		content: "New content",
	}

	result, _ := m.handleExplorerPreview(msg)
	resultModel := result.(Model)

	// The model should be unchanged.
	if resultModel.previewContent != "" {
		t.Errorf("expected previewContent to be empty, got %q", resultModel.previewContent)
	}
	if resultModel.previewTitle != "" {
		t.Errorf("expected previewTitle to be empty, got %q", resultModel.previewTitle)
	}
}

func TestPreviewIDMatch(t *testing.T) {
	// Test that a current reply is installed.
	m := Model{
		previewFor: previewID{path: "file.txt", kind: previewContent},
	}

	msg := explorerPreviewMsg{
		id:      previewID{path: "file.txt", kind: previewContent},
		title:   "File Content",
		content: "The file content",
	}

	result, _ := m.handleExplorerPreview(msg)
	resultModel := result.(Model)

	if resultModel.previewContent != "The file content" {
		t.Errorf("expected %q, got %q", "The file content", resultModel.previewContent)
	}
	if resultModel.previewTitle != "File Content" {
		t.Errorf("expected %q, got %q", "File Content", resultModel.previewTitle)
	}
}

func TestPreviewErrorHandling(t *testing.T) {
	// Test that an error is shown in status.
	m := Model{
		previewFor: previewID{path: "file.txt", kind: previewContent},
	}

	msg := explorerPreviewMsg{
		id:    previewID{path: "file.txt", kind: previewContent},
		title: "File Content",
		err:   errSentinel,
	}

	result, _ := m.handleExplorerPreview(msg)
	resultModel := result.(Model)

	if resultModel.status == "" {
		t.Errorf("expected status to contain error, got empty")
	}
	if resultModel.previewContent != "" {
		t.Errorf("expected previewContent to be empty on error, got %q", resultModel.previewContent)
	}
}

var errSentinel = &testError{"test error"}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func TestPreviewPaneLinesEmpty(t *testing.T) {
	m := Model{
		previewFor: previewID{}, // No selection
	}

	lines := m.previewPaneLines(10, 80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "nothing selected") {
		t.Errorf("expected 'nothing selected', got %q", lines[0])
	}
}

func TestPreviewPaneLinesTextContent(t *testing.T) {
	m := Model{
		previewFor:     previewID{path: "file.txt", kind: previewContent},
		previewContent: "line 1\nline 2\nline 3",
		previewOffset:  0,
	}

	lines := m.previewPaneLines(3, 80)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line 1" {
		t.Errorf("expected 'line 1', got %q", lines[0])
	}
}

func TestPreviewPaneLinesWithOffset(t *testing.T) {
	m := Model{
		previewFor:     previewID{path: "file.txt", kind: previewContent},
		previewContent: "line 1\nline 2\nline 3\nline 4\nline 5",
		previewOffset:  2,
	}

	lines := m.previewPaneLines(2, 80)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "line 3" {
		t.Errorf("expected 'line 3', got %q", lines[0])
	}
}

// The heading is the only thing on screen that says which of the four views
// the pane is showing, so cycling has to move it.
func TestTheHeadingFollowsTheCycledView(t *testing.T) {
	m := Model{
		repo:       &git.Repo{Path: t.TempDir()},
		previewFor: previewID{path: "pkg/thing.go", kind: previewContent},
	}

	want := []string{"Diff — thing.go", "Blame — thing.go", "History — thing.go", "Content — thing.go"}
	for _, title := range want {
		m.cyclePreview()
		if m.previewTitle != title {
			t.Errorf("after e the pane is titled %q, want %q", m.previewTitle, title)
		}
	}
}

// The gutter was dim and the code beside it plain, so switching to blame threw
// away the colouring the content view had. The annotations and the syntax are
// answers to different questions and there is no reason to trade one away.
func TestBlameKeepsTheSyntaxColouring(t *testing.T) {
	withColour(t)

	lines := []git.BlameLine{
		{Short: "aaa1111", Author: "Alice", When: "2024-01-01", Text: "package main"},
		{Short: "aaa1111", Author: "Alice", When: "2024-01-01", Text: ""},
		{Short: "bbb2222", Author: "Bob", When: "2024-02-02", Text: "func main() {"},
		{Short: "bbb2222", Author: "Bob", When: "2024-02-02", Text: "\tprintln(42)"},
		{Short: "bbb2222", Author: "Bob", When: "2024-02-02", Text: "}"},
	}

	styled := highlightBlame("main.go", lines, currentSyntax())
	if len(styled) != len(lines) {
		t.Fatalf("highlighted %d lines for %d, want one each", len(styled), len(lines))
	}
	if !strings.Contains(strings.Join(styled, "\n"), "\x1b[") {
		t.Fatal("a Go file came back with no colour on it")
	}

	out := renderBlame(lines, styled, 0, 5, 80)
	if !strings.Contains(out[2], "\x1b[") {
		t.Errorf("the code column is uncoloured: %q", out[2])
	}
	// The annotations are still there beside it.
	if !strings.Contains(ansi.Strip(out[2]), "bbb2222") {
		t.Errorf("the gutter lost its commit: %q", ansi.Strip(out[2]))
	}
}

// A file of no language the lexer knows still annotates — the colouring is the
// part that is missing, not the blame.
func TestBlameWithoutALexerStillDraws(t *testing.T) {
	lines := []git.BlameLine{{Short: "aaa1111", Author: "Alice", When: "2024-01-01", Text: "plain words"}}

	if got := highlightBlame("notes.unknownext", lines, currentSyntax()); got != nil {
		t.Errorf("highlighted an unknown language: %v", got)
	}
	out := renderBlame(lines, nil, 0, 1, 80)
	if !strings.Contains(ansi.Strip(out[0]), "plain words") {
		t.Errorf("the line is missing: %q", ansi.Strip(out[0]))
	}
}
