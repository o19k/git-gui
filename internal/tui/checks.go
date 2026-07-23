package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/theme"
)

// A repository can name commands that must pass before a commit is recorded.
// They are the user's own shell command lines, so they never go through the
// git layer; a failing one stops the commit and shows its output.

// checkResult is one command's outcome.
type checkResult struct {
	command string
	ok      bool
	output  string
}

// checksMsg carries a finished run of every configured check. message is the
// commit the run is gating, so it survives the round trip; file is where that
// message is kept when it was written in the editor, since a message with a
// body cannot go on a command line.
type checksMsg struct {
	message string
	file    string
	results []checkResult
	err     error
}

// startCommit runs the repository's checks, then commits if they pass. A
// repository with none configured commits straight away.
func (m Model) startCommit(message string) (Model, tea.Cmd) {
	return m.runCommitChecks(message, "")
}

// startComposeCommit is startCommit for a message written in the editor.
func (m Model) startComposeCommit(path string) (Model, tea.Cmd) {
	return m.runCommitChecks(m.pendingCommit, path)
}

func (m Model) runCommitChecks(message, file string) (Model, tea.Cmd) {
	repo, ctx := m.repo, m.ctx

	// Reading the configuration is itself a git call, so it runs in the command
	// with the checks. No configured checks means no results, which counts as
	// every check having passed.
	m.busy = "committing…"
	m.pendingCommit = message
	return m, func() tea.Msg {
		checks, err := repo.Checks(ctx)
		if err != nil {
			return checksMsg{message: message, file: file, err: err}
		}
		return checksMsg{message: message, file: file, results: runChecks(ctx, repo.Path, checks)}
	}
}

// commit records what the checks were gating, from the file when the message
// was written in the editor and from the string when it was typed at the
// prompt.
func (m Model) commit(msg checksMsg) func() error {
	repo, ctx, opts := m.repo, m.ctx, m.commitOpts()
	if msg.file != "" {
		return func() error { return repo.CommitFile(ctx, msg.file, opts) }
	}
	return func() error { return repo.Commit(ctx, msg.message, opts) }
}

// runChecks executes each check in the repository directory, capturing its
// output and exit status. Each command runs with a deadline.
func runChecks(ctx context.Context, repoDir string, checks []string) []checkResult {
	const checkTimeout = 30 * time.Second

	results := make([]checkResult, len(checks))
	for i, command := range checks {
		results[i] = runCheck(ctx, repoDir, command, checkTimeout)
	}
	// Every check runs even after one has failed, so the list is complete.
	return results
}

// runCheck is one command. It is its own function so the deadline is released
// when the command finishes rather than when the whole run does.
func runCheck(ctx context.Context, repoDir, command string, timeout time.Duration) checkResult {
	// A deadline already on the context is tighter than ours, so leave it.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// The shell, because these are command lines rather than argument vectors.
	// Never inherit stdin: this program owns the terminal.
	proc := exec.CommandContext(ctx, "sh", "-c", command)
	proc.Dir = repoDir
	proc.Env = os.Environ()

	var out bytes.Buffer
	proc.Stdout = &out
	proc.Stderr = &out

	err := proc.Run()
	result := checkResult{command: command, ok: err == nil, output: strings.TrimSpace(out.String())}
	if ctx.Err() == context.DeadlineExceeded {
		result.output = "timed out after " + timeout.String()
	}
	return result
}

// handleChecks records the commit when every check passed, and otherwise puts
// the failure in front of the user with the way past it.
func (m Model) handleChecks(msg checksMsg) (tea.Model, tea.Cmd) {
	m.busy = ""

	if msg.err != nil {
		m.status = msg.err.Error()
		return m, nil
	}

	var failedIdx int
	var anyFailed bool
	for i, r := range msg.results {
		if !r.ok {
			failedIdx = i
			anyFailed = true
			break
		}
	}

	if !anyFailed {
		return m, m.do("commit", m.commit(msg))
	}

	// Some failed: show the first failure and ask whether to commit anyway.
	failed := msg.results[failedIdx]
	title := "Check failed — " + failed.command

	body := fmt.Sprintf("%d of %s did not pass. The index and your message are untouched either way.",
		countFailed(msg.results), count(len(msg.results), "check", "checks"))
	extra := dimmed(truncateOutput(failed.output, 54, 10))

	// A copy for the choice action, which runs after this model value has been
	// replaced.
	self := m
	m.askChoice(title, body, []choice{
		{
			label:  "Commit anyway",
			hint:   "record the commit despite the failed check",
			busy:   "committing…",
			action: func() tea.Cmd { return self.do("commit", self.commit(msg)) },
		},
		{
			label:  "Cancel",
			hint:   "keep the index staged and your message",
			action: func() tea.Cmd { return nil },
		},
	})

	m.overlay.extra = extra

	return m, nil
}

// truncateOutput keeps output to a manageable size. If the output is longer
// than maxChars, it is split into lines and capped at maxLines, saying how
// much was dropped.
// countFailed is how many of a run's checks did not pass.
func countFailed(results []checkResult) (n int) {
	for _, r := range results {
		if !r.ok {
			n++
		}
	}
	return n
}

// dimmed renders a command's own output as secondary to the question being
// asked about it.
func dimmed(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, theme.DimStyle.Render(line))
	}
	return out
}

// Both the line count and each line's width are capped, and what was dropped
// is counted rather than silently left out.
func truncateOutput(output string, maxChars int, maxLines int) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Cut by runes, not bytes: a byte cut can land inside a character.
	for i, line := range lines {
		if runes := []rune(line); len(runes) > maxChars {
			lines[i] = string(runes[:maxChars-1]) + "…"
		}
	}

	if len(lines) <= maxLines {
		return lines
	}
	// The head, not the tail: a check names what went wrong first.
	kept := lines[:maxLines-1]
	return append(kept, fmt.Sprintf("… %s more", count(len(lines)-len(kept), "line", "lines")))
}
