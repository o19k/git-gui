package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

func dateCommit() git.Commit {
	return git.Commit{SHA: "abc1234567890", Short: "abc1234", Subject: "test commit"}
}

func TestChangingADateAsksForItBeforeWarningAboutIt(t *testing.T) {
	// The warning is about replaying history, which is not worth putting in
	// front of someone who has not yet decided on a date.
	m, cmd := fixture(t).askCommitDate(dateCommit())

	if cmd != nil {
		t.Error("t ran something before asking for a date")
	}
	if m.overlay.kind != overlayInput {
		t.Fatalf("t did not prompt: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.title, "abc1234") {
		t.Errorf("the prompt does not name the commit: %q", m.overlay.title)
	}
	if !strings.Contains(m.overlay.title, "YYYY-MM-DD") {
		t.Errorf("the prompt does not say what form to type: %q", m.overlay.title)
	}
}

func TestTheAnsweredDateIsGatedAsARewrite(t *testing.T) {
	next, _ := fixture(t).Update(commitDateMsg{
		sha: "abc1234567890", short: "abc1234", date: "2020-05-05",
	})
	m := next.(Model)

	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Fatalf("the rewrite is not gated: kind=%d danger=%v", m.overlay.kind, m.overlay.danger)
	}
	for _, want := range []string{"abc1234", "2020-05-05", "replayed"} {
		if !strings.Contains(m.overlay.body, want) {
			t.Errorf("the confirm does not mention %q: %q", want, m.overlay.body)
		}
	}
}

func TestAnEmptyDateChangesNothing(t *testing.T) {
	m, _ := fixture(t).askCommitDate(dateCommit())

	// Answering the prompt with nothing is how it is backed out of.
	if cmd := m.overlay.action(""); cmd != nil {
		t.Error("an empty date still started a rewrite")
	}
}

func TestTheDatePromptCarriesTheCommitThroughToTheConfirm(t *testing.T) {
	// The prompt's callback returns a command and never sees the model again,
	// so the commit has to survive the round trip for the confirm to name it.
	m, _ := fixture(t).askCommitDate(dateCommit())

	cmd := m.overlay.action("2019-01-02")
	if cmd == nil {
		t.Fatal("a typed date produced nothing")
	}
	msg, ok := cmd().(commitDateMsg)
	if !ok {
		t.Fatalf("the prompt produced %T, want the date to come back through the loop", cmd())
	}
	if msg.sha != "abc1234567890" || msg.date != "2019-01-02" {
		t.Errorf("the round trip lost something: %+v", msg)
	}
}
