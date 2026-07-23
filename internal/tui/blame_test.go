package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

var errFake = errors.New("boom: no such path")

func blamed() []git.BlameLine {
	return []git.BlameLine{
		{Short: "aaa1111", Author: "Ada", When: "2024-01-02", Text: "package main"},
		{Short: "bbb2222", Author: "Grace", When: "2024-03-04", Text: "func main() {}"},
	}
}

func TestBlameReplacesTheDiffWithTheAnnotatedFile(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1 // dirty.go, tracked

	next, cmd := m.toggleBlame()
	m = next.(Model)
	if !m.blameOn || cmd == nil {
		t.Fatalf("b did not start annotating: on=%v cmd=%v", m.blameOn, cmd != nil)
	}
	if m.blamePath != "dirty.go" {
		t.Errorf("blamePath = %q, want the selected file", m.blamePath)
	}

	next, _ = m.Update(blameMsg{path: "dirty.go", lines: blamed()})
	m = next.(Model)

	view := m.View()
	for _, want := range []string{"Blame — dirty.go", "aaa1111", "Ada", "2024-01-02", "package main"} {
		if !strings.Contains(view, want) {
			t.Errorf("the annotated view is missing %q", want)
		}
	}
}

func TestBlameTogglesBackOff(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1

	m = key(t, m, "b")
	if !m.blameOn {
		t.Fatal("b did not turn annotations on")
	}
	m = key(t, m, "b")
	if m.blameOn || m.blamePath != "" || m.blameLines != nil {
		t.Errorf("b left annotations behind: on=%v path=%q", m.blameOn, m.blamePath)
	}
}

func TestBlameIsRefusedWhereThereIsNoHistory(t *testing.T) {
	cases := []struct {
		name   string
		cursor int
		want   string
	}{
		{"untracked", 2, "untracked"}, // new.go
	}
	for _, c := range cases {
		m := onPane(t, fixture(t), PanelFiles)
		m.cursor[PanelFiles] = c.cursor

		next, cmd := m.toggleBlame()
		after := next.(Model)
		if after.blameOn || cmd != nil {
			t.Errorf("%s: annotations started anyway", c.name)
		}
		if !strings.Contains(after.status, c.want) {
			t.Errorf("%s: status = %q, want it to explain", c.name, after.status)
		}
	}
}

func TestBlameFollowsTheSelection(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 0 // staged.go
	m = key(t, m, "b")
	next, _ := m.Update(blameMsg{path: "staged.go", lines: blamed()})
	m = next.(Model)

	m = key(t, m, "j") // dirty.go

	if m.blamePath != "dirty.go" {
		t.Errorf("blamePath = %q, want it to follow the cursor", m.blamePath)
	}
	if m.blameLines != nil {
		t.Error("the previous file's annotations were left on screen")
	}
}

func TestBlameTurnsOffWhenTheSelectionCannotBeBlamed(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1 // dirty.go
	m = key(t, m, "b")

	m = key(t, m, "j") // new.go, untracked

	if m.blameOn {
		t.Error("annotations stayed on over a file with no history")
	}
}

func TestAStaleBlameIsDiscarded(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1
	m = key(t, m, "b")
	next, _ := m.Update(blameMsg{path: "dirty.go", lines: blamed()})
	m = next.(Model)

	next, _ = m.Update(blameMsg{path: "someone-else.go", lines: nil})
	if after := next.(Model); len(after.blameLines) != 2 {
		t.Error("annotations for another file were painted over the current ones")
	}
}

func TestBlameFailureSurfacesAndTurnsOff(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1
	m = key(t, m, "b")

	next, _ := m.Update(blameMsg{path: "dirty.go", err: errFake})
	after := next.(Model)

	if after.blameOn {
		t.Error("annotations stayed on after git refused")
	}
	if !strings.Contains(after.status, "boom") {
		t.Errorf("status = %q, want git's message", after.status)
	}
}

func TestHunkModeTurnsAnnotationsOff(t *testing.T) {
	// The pane can only show one of the two, and hunks are offsets into a patch.
	m := hunkFixture(t)
	m.blameOn, m.blamePath, m.blameLines = true, "dirty.go", blamed()

	m, _ = press(t, m, "enter")

	if m.blameOn {
		t.Error("entering hunk mode left annotations on")
	}
	if !m.hunkMode {
		t.Error("hunk mode did not open")
	}
}

func TestBlameScrollsByItsOwnLength(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m.cursor[PanelFiles] = 1
	m = key(t, m, "b")
	next, _ := m.Update(blameMsg{path: "dirty.go", lines: blamed()})
	m = next.(Model)

	if got := m.mainLines(); got != 2 {
		t.Errorf("mainLines = %d, want the 2 annotated lines", got)
	}
}
