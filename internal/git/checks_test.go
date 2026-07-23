package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestChecksWhenNoneConfigured(t *testing.T) {
	repo, _ := newRepo(t)
	checks, err := repo.Checks(context.Background())

	if err != nil {
		t.Fatalf("Checks: %v", err)
	}
	if len(checks) != 0 {
		t.Errorf("expected no checks, got %d: %v", len(checks), checks)
	}
}

func TestChecksReadsConfiguredCommands(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Set up git config with multiple check commands.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("config", "gitgui.check", "true")
	run("config", "--add", "gitgui.check", "false")

	checks, err := repo.Checks(ctx)
	if err != nil {
		t.Fatalf("Checks: %v", err)
	}
	if len(checks) != 2 {
		t.Errorf("expected 2 checks, got %d: %v", len(checks), checks)
	}
	if checks[0] != "true" || checks[1] != "false" {
		t.Errorf("checks = %v, want [true false]", checks)
	}
}
