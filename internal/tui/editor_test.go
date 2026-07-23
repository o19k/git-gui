package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// $VISUAL wins over $EDITOR: $EDITOR may be a line editor kept for scripts,
// and the convention exists so a full-screen editor can be named separately.
func TestVisualWinsOverEditor(t *testing.T) {
	t.Setenv("EDITOR", "ed")
	t.Setenv("VISUAL", "vim")

	if got := editorCommand(); !slices.Equal(got, []string{"vim"}) {
		t.Errorf("editorCommand() = %v", got)
	}

	t.Setenv("VISUAL", "")
	if got := editorCommand(); !slices.Equal(got, []string{"ed"}) {
		t.Errorf("with no $VISUAL, editorCommand() = %v", got)
	}
}

// "code -w" and "emacsclient -nw" are ordinary values of these variables, and
// running the whole string as one program name would fail with a name nobody
// typed.
func TestAnEditorKeepsItsArguments(t *testing.T) {
	t.Setenv("VISUAL", "code -w")

	if got := editorCommand(); !slices.Equal(got, []string{"code", "-w"}) {
		t.Errorf("editorCommand() = %v", got)
	}
}

func TestNoEditorConfiguredSaysWhichVariablesToSet(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	m := revealModel()
	next, cmd := m.openInEditor("src/main.go")

	if cmd != nil {
		t.Fatal("something was run with no editor configured")
	}
	status := next.(Model).status
	if !strings.Contains(status, "EDITOR") || !strings.Contains(status, "VISUAL") {
		t.Errorf("status = %q, want both variables named", status)
	}
}

// enter is the key with no direction, so it is the one that can mean "open".
// h and l keep their single meaning: the columns.
func TestOnlyEnterOpensAFile(t *testing.T) {
	t.Setenv("VISUAL", "true")

	m := revealModel()
	m.repo = &git.Repo{Path: t.TempDir()}
	m.cwd = "src"
	m.focus = PanelEntries

	if _, cmd, _ := m.explorerKey("l"); cmd != nil {
		t.Error("l ran the editor instead of staying in the columns")
	}
	if _, cmd, _ := m.explorerKey("right"); cmd != nil {
		t.Error("→ ran the editor instead of staying in the columns")
	}
	if _, cmd, _ := m.explorerKey("enter"); cmd == nil {
		t.Error("enter did not open the file")
	}
}

// A directory is somewhere to step into, whichever key asks.
func TestEnterOnADirectoryStillStepsIntoIt(t *testing.T) {
	t.Setenv("VISUAL", "true")

	m := revealModel()
	m.cursor[PanelEntries] = 0 // src

	next, _, _ := m.explorerKey("enter")
	if got := next.(Model).cwd; got != "src" {
		t.Errorf("cwd = %q, want src", got)
	}
}

// The file may have been changed by the editor, so what was read before it ran
// is not to be trusted — the sizes and times least of all.
func TestReturningFromTheEditorDropsWhatMayHaveChanged(t *testing.T) {
	m := revealModel()
	m.stats = map[string]fileMeta{"src/main.go": {size: 1}}
	m.previewStyled = []string{"stale"}

	next, cmd := m.handleEditorDone(editorDoneMsg{})
	after := next.(Model)

	if len(after.stats) != 0 {
		t.Errorf("sizes from before the edit survived: %v", after.stats)
	}
	if after.previewStyled != nil {
		t.Error("the colouring of the old content survived")
	}
	if cmd == nil {
		t.Error("nothing was re-read after the editor exited")
	}
}
