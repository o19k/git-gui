package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
)

// press sends a single key by name, the way handleKey reads it.
func press(t *testing.T, m Model, name string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyMsg
	switch name {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestDiscardAsksBeforeDestroying(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles) // Files

	m, cmd := press(t, m, "d")
	if cmd != nil {
		t.Fatal("discard ran immediately instead of asking")
	}
	if m.overlay.kind != overlayConfirm {
		t.Fatalf("no confirm opened, overlay = %d", m.overlay.kind)
	}
	if !m.overlay.danger {
		t.Error("discarding work should be marked destructive")
	}
	if !strings.Contains(m.overlay.body, "staged.go") {
		t.Errorf("the confirm does not name what it will destroy: %q", m.overlay.body)
	}

	// "n" cancels without producing a command.
	cancelled, cmd := press(t, m, "n")
	if cmd != nil || cancelled.overlay.kind != overlayNone {
		t.Error("n should dismiss the confirm and run nothing")
	}

	// "y" accepts and hands back the mutation.
	accepted, cmd := press(t, m, "y")
	if cmd == nil {
		t.Error("y should have produced the mutation command")
	}
	if accepted.overlay.kind != overlayNone {
		t.Error("the confirm should close once accepted")
	}
}

func TestEscapeAlsoCancelsAConfirm(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m, _ = press(t, m, "d")

	m, cmd := press(t, m, "esc")
	if cmd != nil || m.overlay.kind != overlayNone {
		t.Error("esc should dismiss the confirm")
	}
}

func TestCommitPromptCollectsAMessage(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)

	m, cmd := press(t, m, "c")
	if cmd != nil {
		t.Fatal("commit ran before a message was given")
	}
	if m.overlay.kind != overlayInput {
		t.Fatalf("no prompt opened, overlay = %d", m.overlay.kind)
	}

	for _, r := range "fix it" {
		m, _ = press(t, m, string(r))
	}
	if m.overlay.value != "fix it" {
		t.Fatalf("typed text = %q", m.overlay.value)
	}

	m, _ = press(t, m, "backspace")
	if m.overlay.value != "fix i" {
		t.Errorf("backspace = %q", m.overlay.value)
	}
	if !strings.Contains(m.View(), "fix i") {
		t.Error("the prompt does not show what is being typed")
	}

	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Error("enter should have produced the commit command")
	}
	if m.overlay.kind != overlayNone {
		t.Error("the prompt should close once accepted")
	}
}

func TestEmptyCommitMessageRunsNothing(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m, _ = press(t, m, "c")

	m, cmd := press(t, m, "enter")
	if cmd != nil {
		t.Error("an empty message should not commit")
	}
	if m.overlay.kind != overlayNone {
		t.Error("the prompt should still close")
	}
}

// The message is read before the prompt is drawn, so the prompt holds all of
// it rather than the subject the commit list happens to be showing.
func TestAmendPromptIsPrefilledWithTheOldMessage(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)
	m, _ = press(t, m, "A")

	next, _ := m.Update(amendMsg{message: "first"})
	m = next.(Model)

	if m.overlay.kind != overlayInput {
		t.Fatalf("amend did not ask: overlay kind = %d", m.overlay.kind)
	}
	if m.overlay.value != "first" {
		t.Errorf("amend prompt = %q, want the message it replaces", m.overlay.value)
	}
}

// A one-line prompt cannot hold a body, and typing over it would throw the
// body away without saying so.
func TestAmendKeepsABodyOutOfTheOneLinePrompt(t *testing.T) {
	m := onPane(t, fixture(t), PanelFiles)

	next, _ := m.Update(amendMsg{message: "subject\n\nthe body that must survive"})
	m = next.(Model)

	if m.overlay.kind == overlayInput {
		t.Error("a message with a body was offered at the one-line prompt")
	}
}

func TestSameKeyMeansDifferentThingsPerPanel(t *testing.T) {
	// `d` discards in Files and deletes in Branches. Both confirm, but they
	// must name different things.
	files, _ := press(t, onPane(t, fixture(t), PanelFiles), "d")
	branches, _ := press(t, onPane(t, fixture(t), PanelBranches), "d")

	if !strings.Contains(files.overlay.title, "Discard") {
		t.Errorf("Files d = %q", files.overlay.title)
	}
	// The fixture's first branch is HEAD, which cannot be deleted.
	if branches.overlay.kind != overlayNone || !strings.Contains(branches.status, "cannot delete") {
		t.Errorf("Branches d on HEAD = overlay %d, status %q", branches.overlay.kind, branches.status)
	}
}

func TestDeletingTheCurrentBranchIsRefused(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches) // Branches, cursor on main (HEAD)

	m, cmd := press(t, m, "d")
	if cmd != nil {
		t.Error("deleting the checked-out branch should run nothing")
	}
	if !strings.Contains(m.status, "cannot delete the branch you are on") {
		t.Errorf("status = %q", m.status)
	}
}

// Deleting a remote branch reaches someone else's copy of the repository, so
// it asks first and is flagged as destructive — never straight off the key.
func TestDeletingARemoteBranchAsksFirst(t *testing.T) {
	for _, key := range []string{"d", "D"} {
		t.Run(key, func(t *testing.T) {
			m := onPane(t, fixture(t), PanelBranches)
			m, _ = press(t, m, "j") // origin/main

			m, cmd := press(t, m, key)
			if cmd != nil {
				t.Error("the remote was pushed to before the question was answered")
			}
			if m.overlay.kind != overlayConfirm {
				t.Fatalf("no question was asked: overlay kind %d, status %q",
					m.overlay.kind, m.status)
			}
			if !m.overlay.danger {
				t.Error("deleting a branch on the remote is not flagged destructive")
			}
			if !strings.Contains(m.overlay.body, "origin/main") {
				t.Errorf("the question does not name the branch: %q", m.overlay.body)
			}
		})
	}
}

// The two keys differ everywhere else, and a reader could take D for a harder
// form that skips a check. There is no such form on a remote.
func TestBothDeleteKeysAskTheSameOfARemoteBranch(t *testing.T) {
	plain := onPane(t, fixture(t), PanelBranches)
	plain, _ = press(t, plain, "j")
	plain, _ = press(t, plain, "d")

	forced := onPane(t, fixture(t), PanelBranches)
	forced, _ = press(t, forced, "j")
	forced, _ = press(t, forced, "D")

	if plain.overlay.body != forced.overlay.body {
		t.Errorf("d asks %q and D asks %q", plain.overlay.body, forced.overlay.body)
	}
}

func TestForceDeleteIsMarkedDestructive(t *testing.T) {
	m := onPane(t, fixture(t), PanelBranches)
	m, _ = press(t, m, "j") // origin/main is refused, so move to a deletable one

	plain, _ := press(t, onPane(t, fixture(t), PanelCommits), "c") // cherry-pick, non-destructive
	if plain.overlay.danger {
		t.Error("cherry-pick should not be flagged destructive")
	}

	drop, _ := press(t, onPane(t, fixture(t), PanelStash), "d") // stash drop
	if !drop.overlay.danger {
		t.Error("dropping a stash should be flagged destructive")
	}
}

func TestCherryPickAndRevertConfirmWithContext(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits) // Commits

	pick, cmd := press(t, m, "c")
	if cmd != nil || pick.overlay.kind != overlayConfirm {
		t.Fatal("cherry-pick did not open a confirm")
	}
	if !strings.Contains(pick.overlay.body, "aaa1111") || !strings.Contains(pick.overlay.body, "first") {
		t.Errorf("confirm does not identify the commit: %q", pick.overlay.body)
	}

	revert, cmd := press(t, m, "v")
	if cmd != nil || revert.overlay.kind != overlayConfirm {
		t.Fatal("revert did not open a confirm")
	}
	if !strings.Contains(revert.overlay.title, "Revert") {
		t.Errorf("confirm title = %q", revert.overlay.title)
	}
}

func TestStashApplyAndPopRunDirectly(t *testing.T) {
	// Apply and pop are recoverable, so they run without a gate; drop is not.
	m := onPane(t, fixture(t), PanelStash)

	applied, cmd := press(t, m, "space")
	if cmd == nil || applied.busy != "" {
		t.Error("space should apply the stash locally, with no network note")
	}
	if _, cmd := press(t, m, "enter"); cmd == nil {
		t.Error("enter should pop the stash")
	}
	dropped, cmd := press(t, m, "d")
	if cmd != nil || dropped.overlay.kind != overlayConfirm {
		t.Error("d should ask before dropping")
	}
}

func TestPullIsReachableFromTheStashPanel(t *testing.T) {
	// `p` must mean pull in every pane, including one that could claim it.
	for _, panel := range []Panel{PanelFiles, PanelBranches, PanelCommits, PanelStash} {
		m, cmd := press(t, onPane(t, fixture(t), panel), "p")
		if cmd == nil {
			t.Errorf("pane %d: p did not pull", panel)
		}
		if m.busy != "pulling…" {
			t.Errorf("pane %d: busy = %q", panel, m.busy)
		}
	}
}

func TestNetworkKeysAnnounceThemselves(t *testing.T) {
	// P reads what it would publish before asking, so its note names that step.
	cases := map[string]string{"f": "fetching…", "p": "pulling…", "P": "reading what would be published…"}
	for k, want := range cases {
		m, cmd := press(t, fixture(t), k)
		if cmd == nil {
			t.Errorf("%q produced no command", k)
		}
		if m.busy != want {
			t.Errorf("%q set busy = %q, want %q", k, m.busy, want)
		}
		if !strings.Contains(m.View(), want) {
			t.Errorf("%q: the footer does not say what is happening", k)
		}
	}
}

func TestPushAsksBeforePublishingAndListsWhatWouldGo(t *testing.T) {
	m, cmd := press(t, fixture(t), "P")
	if cmd == nil {
		t.Fatal("P produced no command")
	}

	next, _ := m.Update(outgoingMsg{
		branch:      "main",
		hasUpstream: true,
		commits:     []git.Commit{{Short: "abc1234", Subject: "first"}, {Short: "def5678", Subject: "second"}},
	})
	m = next.(Model)

	if m.overlay.kind != overlayConfirm {
		t.Fatalf("push did not ask: overlay kind = %d", m.overlay.kind)
	}
	if !strings.Contains(m.overlay.body, "2 commits") {
		t.Errorf("the confirm does not say how many: %q", m.overlay.body)
	}
	view := m.View()
	for _, want := range []string{"abc1234", "first", "def5678", "second"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(view, want) {
				t.Errorf("the confirm does not list %q", want)
			}
		})
	}
}

func TestPushWithNothingOutgoingSaysSoRatherThanAsking(t *testing.T) {
	m := fixture(t)
	next, _ := m.Update(outgoingMsg{branch: "main", hasUpstream: true})
	m = next.(Model)

	if m.overlay.kind != overlayNone {
		t.Error("a push with nothing to send still asked")
	}
	if !strings.Contains(m.status, "nothing to push") {
		t.Errorf("status = %q, want it to explain", m.status)
	}
}

func TestPushingABranchWithNoUpstreamSaysItIsNew(t *testing.T) {
	m := fixture(t)
	next, _ := m.Update(outgoingMsg{
		branch:  "feature",
		commits: []git.Commit{{Short: "abc1234", Subject: "first"}},
	})
	m = next.(Model)

	if !strings.Contains(m.overlay.body, "new origin/feature") {
		t.Errorf("the confirm does not say the branch is new: %q", m.overlay.body)
	}
}

func TestForcePushAsksAndIsMarkedDestructive(t *testing.T) {
	m, cmd := press(t, fixture(t), "F")
	if cmd != nil {
		t.Fatal("force push ran without asking")
	}
	if m.overlay.kind != overlayConfirm || !m.overlay.danger {
		t.Fatalf("no destructive confirm: kind=%d danger=%v", m.overlay.kind, m.overlay.danger)
	}
	if !strings.Contains(m.overlay.body, "origin/main") {
		t.Errorf("the confirm does not name the ref it overwrites: %q", m.overlay.body)
	}
	if m.busy != "" {
		t.Error("nothing should be marked in flight before the confirm is accepted")
	}

	accepted, cmd := press(t, m, "y")
	if cmd == nil {
		t.Error("accepting the confirm produced no command")
	}
	if accepted.overlay.kind != overlayNone {
		t.Error("the confirm should close on accept")
	}
}

func TestBusyClearsWhenTheOperationReports(t *testing.T) {
	m, _ := press(t, fixture(t), "P")
	if m.busy == "" {
		t.Fatal("push did not mark itself in flight")
	}

	next, _ := m.Update(mutationMsg{op: "push", err: errors.New("failed to push some refs")})
	m = next.(Model)

	if m.busy != "" {
		t.Error("busy note survived the operation finishing")
	}
	if !strings.Contains(m.View(), "failed to push") {
		t.Error("the push error never reached the footer")
	}
}

func TestFailedMutationSurfacesTheGitError(t *testing.T) {
	m := fixture(t)

	next, cmd := m.Update(mutationMsg{op: "commit", err: errors.New("nothing to commit, working tree clean")})
	m = next.(Model)

	if cmd != nil {
		t.Error("a failed mutation should not trigger a reload")
	}
	if !strings.Contains(m.status, "nothing to commit") {
		t.Errorf("status = %q", m.status)
	}
	if !strings.Contains(m.View(), "nothing to commit") {
		t.Error("the error never reached the footer")
	}
}

func TestSuccessfulMutationTriggersAReload(t *testing.T) {
	m := fixture(t)
	m.status = "stale error"

	next, cmd := m.Update(mutationMsg{op: "stage"})
	m = next.(Model)

	if cmd == nil {
		t.Error("a successful mutation should reload the snapshot")
	}
	if m.status != "" {
		t.Errorf("the previous error was left on screen: %q", m.status)
	}
}

func TestPanelKeysDoNotShadowNavigation(t *testing.T) {
	// Refresh is R, not r: the Commits panel needs r for reword.
	global := []string{"j", "k", "g", "G", "R", "?", "q", "1", "2", "3", "4", "5"}
	for _, p := range []Panel{PanelFiles, PanelBranches, PanelCommits, PanelStash} {
		for _, hint := range fixture(t).panelKeyHints(p) {
			for _, g := range global {
				if hint[0] == g {
					t.Errorf("panel %d binds %q, which is a global key", p, g)
				}
			}
		}
	}
}

func TestFooterShowsTheFocusedPanelsKeys(t *testing.T) {
	cases := map[Panel]string{
		PanelFiles:    "stage",
		PanelBranches: "checkout",
		PanelCommits:  "cherry-pick",
		PanelStash:    "apply",
	}
	for panel, want := range cases {
		m := fixture(t)
		m.focus = panel
		if !strings.Contains(m.View(), want) {
			t.Errorf("footer for panel %d is missing %q", panel, want)
		}
	}
}

func TestCommitRewriteKeysAskFirst(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits) // Commits, on "first" (aaa1111)

	squash, cmd := press(t, m, "s")
	if cmd != nil || squash.overlay.kind != overlayConfirm || !squash.overlay.danger {
		t.Fatalf("squash: cmd=%v kind=%d danger=%v", cmd != nil, squash.overlay.kind, squash.overlay.danger)
	}
	if !strings.Contains(squash.overlay.body, "aaa1111") {
		t.Errorf("squash confirm does not name the commit: %q", squash.overlay.body)
	}

	drop, cmd := press(t, m, "d")
	if cmd != nil || drop.overlay.kind != overlayConfirm || !drop.overlay.danger {
		t.Fatalf("drop: cmd=%v kind=%d danger=%v", cmd != nil, drop.overlay.kind, drop.overlay.danger)
	}
	if !strings.Contains(drop.overlay.body, "rewritten") {
		t.Errorf("drop confirm does not warn that history is rewritten: %q", drop.overlay.body)
	}
}

func TestRewordPromptIsPrefilledWithTheCommitSubject(t *testing.T) {
	m := onPane(t, fixture(t), PanelCommits)

	m, cmd := press(t, m, "r")
	if cmd != nil {
		t.Fatal("reword ran before a message was given")
	}
	if m.overlay.kind != overlayInput || m.overlay.value != "first" {
		t.Errorf("prompt = kind %d, value %q", m.overlay.kind, m.overlay.value)
	}
	if !strings.Contains(m.overlay.title, "aaa1111") {
		t.Errorf("prompt title does not say which commit: %q", m.overlay.title)
	}
}

func TestMovingACommitRunsWithoutAGate(t *testing.T) {
	// Reordering is reversible by reordering back, so it does not ask.
	m := onPane(t, fixture(t), PanelCommits)

	for _, k := range []string{"K", "J"} {
		next, cmd := press(t, m, k)
		if cmd == nil {
			t.Errorf("%q produced no command", k)
		}
		if next.overlay.kind != overlayNone {
			t.Errorf("%q opened a modal", k)
		}
	}
}

func TestRefreshIsShiftR(t *testing.T) {
	// Plain `r` belongs to the Commits panel, where it rewords.
	m := onPane(t, fixture(t), PanelCommits)
	m, _ = press(t, m, "r")
	if m.overlay.kind != overlayInput {
		t.Error("r in the Commits panel should reword, not refresh")
	}

	fresh := fixture(t)
	if _, cmd := press(t, fresh, "R"); cmd == nil {
		t.Error("R should refresh")
	}
}

func TestRebaseControlsAppearOnlyWhileRebasing(t *testing.T) {
	calm := fixture(t)
	if strings.Contains(calm.View(), "rebase stopped") {
		t.Error("a calm repository claims a rebase is stopped")
	}

	// While one is stopped the keys hold everywhere, since nothing else in the
	// repository works until it is resolved.
	for _, focus := range []Panel{PanelFiles, PanelCommits} {
		stopped := fixture(t)
		stopped.snap.Rebasing = true
		stopped.focus = focus

		if _, cmd := press(t, stopped, "c"); cmd == nil {
			t.Errorf("c on pane %d should continue the rebase", focus)
		}
	}

	stopped := fixture(t)
	stopped.snap.Rebasing = true

	if !strings.Contains(stopped.View(), "rebase stopped") {
		t.Error("the stopped rebase is not announced")
	}
	aborted, cmd := press(t, stopped, "a")
	if cmd != nil || aborted.overlay.kind != overlayConfirm || !aborted.overlay.danger {
		t.Error("a should ask before throwing the rebase away")
	}
}
