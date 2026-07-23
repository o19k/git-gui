package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadComposedDropsCommentsAndTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), composeFile)
	body := "the subject\n\nthe body, which a one-line prompt could not hold\n" +
		"# a comment git would strip\n   # an indented one too\n\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	message, err := readComposed(path)
	if err != nil {
		t.Fatalf("readComposed: %v", err)
	}

	if strings.Contains(message, "#") {
		t.Errorf("a comment survived:\n%s", message)
	}
	if !strings.Contains(message, "the body") {
		t.Errorf("the body was lost:\n%s", message)
	}
	if strings.HasSuffix(message, "\n") {
		t.Error("trailing blank lines were kept")
	}
}

// An editor left with nothing in it means the commit is off, which is how git
// itself reads it.
func TestEmptyComposedMessageCommitsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), composeFile)
	if err := os.WriteFile(path, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := fixture(t)
	next, _ := m.Update(composeDoneMsg{path: path})
	m = next.(Model)

	if m.pendingCommit != "" {
		t.Error("an empty message was carried forward as a commit")
	}
	if !strings.Contains(m.status, "nothing committed") {
		t.Errorf("status = %q, want it to say the commit did not happen", m.status)
	}
}

func TestComposeNeedsAnEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	m := onPane(t, fixture(t), PanelFiles)
	m, cmd := press(t, m, "C")

	if cmd != nil {
		t.Error("something was run without an editor to run")
	}
	if !strings.Contains(m.status, "EDITOR") {
		t.Errorf("status = %q, want it to name what is missing", m.status)
	}
}

// The signoff setting reaches the commit rather than being read and forgotten.
func TestSignoffSettingReachesTheCommit(t *testing.T) {
	m := fixture(t)
	m.settings.Signoff = true

	if !m.commitOpts().Signoff {
		t.Error("the setting did not reach the options a commit is made with")
	}
}
