package tui

import (
	"strings"
	"testing"
)

func TestHandleChecksWhenAllPassed(t *testing.T) {
	m := fixture(t)
	msg := checksMsg{
		message: "test commit",
		results: []checkResult{
			{command: "true", ok: true, output: ""},
			{command: "echo ok", ok: true, output: "ok"},
		},
	}

	next, cmd := m.handleChecks(msg)
	model := next.(Model)

	// The model's busy flag should be cleared.
	if model.busy != "" {
		t.Errorf("busy = %q, want empty", model.busy)
	}

	// A command should be returned to commit.
	if cmd == nil {
		t.Error("expected a commit command, got nil")
	}
}

func TestHandleChecksWhenSomeFailed(t *testing.T) {
	m := fixture(t)
	msg := checksMsg{
		message: "test commit",
		results: []checkResult{
			{command: "true", ok: true, output: ""},
			{command: "false", ok: false, output: "error output"},
		},
	}

	next, _ := m.handleChecks(msg)
	model := next.(Model)

	// The model's busy flag should be cleared.
	if model.busy != "" {
		t.Errorf("busy = %q, want empty", model.busy)
	}

	// A choice overlay should be open.
	if model.overlay.kind != overlayChoice {
		t.Errorf("overlay kind = %v, want overlayChoice", model.overlay.kind)
	}

	// The title should name the failed check.
	if !strings.Contains(model.overlay.title, "false") {
		t.Errorf("title = %q, does not mention the failed check", model.overlay.title)
	}

	// There should be two choices: "Commit anyway" and "Cancel".
	if len(model.overlay.choices) != 2 {
		t.Errorf("expected 2 choices, got %d", len(model.overlay.choices))
	}
}

func TestTruncateOutputLongText(t *testing.T) {
	// Output longer than maxChars should be truncated with a note.
	output := strings.Repeat("x", 600)
	result := truncateOutput(output, 100, 10)

	totalLen := 0
	for _, line := range result {
		totalLen += len(line)
	}
	if totalLen > 150 { // Some overhead is acceptable
		t.Errorf("truncated output is too long: %d chars in %d lines", totalLen, len(result))
	}

	// The last line should mention dropped content.
	if !strings.Contains(result[len(result)-1], "…") && !strings.Contains(result[len(result)-1], "more") {
		t.Errorf("last line does not indicate truncation: %q", result[len(result)-1])
	}
}

func TestTruncateOutputManyLines(t *testing.T) {
	// Output with many lines should be capped.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	output := strings.Join(lines, "\n")

	result := truncateOutput(output, 10000, 5)
	if len(result) > 6 {
		t.Errorf("truncated output has %d lines, want ≤6", len(result))
	}

	// The last line should mention dropped content.
	if !strings.Contains(result[len(result)-1], "…") {
		t.Errorf("last line does not indicate truncation: %q", result[len(result)-1])
	}
}

func TestTruncateOutputSmall(t *testing.T) {
	// Small output should not be truncated.
	output := "small\noutput"
	result := truncateOutput(output, 1000, 10)

	if len(result) != 2 {
		t.Errorf("small output was truncated: %v", result)
	}
	if result[0] != "small" || result[1] != "output" {
		t.Errorf("output was changed: %v", result)
	}
}
