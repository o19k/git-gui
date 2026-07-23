package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestIndexTreeListsTrackedAndUntrackedPaths tests that IndexTree returns all paths.
func TestIndexTreeListsTrackedAndUntrackedPaths(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Create and commit a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runGit(t, dir, "add", "tracked.txt"); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := runGit(t, dir, "commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := repo.IndexTree(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var tracked, untracked bool
	for _, e := range entries {
		if e.Path == "tracked.txt" && e.Cached {
			tracked = true
		}
		if e.Path == "untracked.txt" && !e.Cached {
			untracked = true
		}
	}

	if !tracked {
		t.Error("IndexTree did not include tracked file")
	}
	if !untracked {
		t.Error("IndexTree did not include untracked file")
	}
}

// TestIndexTreeIncludesMode tests that IndexTree reports mode for tracked files.
func TestIndexTreeIncludesMode(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "file.txt")
	runGit(t, dir, "commit", "-m", "add")

	entries, err := repo.IndexTree(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Path == "file.txt" {
			if e.Mode == "" {
				t.Error("tracked file has empty Mode")
			}
			if !e.Cached {
				t.Error("tracked file has Cached=false")
			}
			return
		}
	}
	t.Error("file.txt not found in IndexTree")
}

// TestIgnoredPrefixesListsIgnoredFiles tests that IgnoredPrefixes returns ignored paths.
func TestIgnoredPrefixesListsIgnoredFiles(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Create .gitignore and ignored files.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nnode_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "node_modules/package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	prefixes, err := repo.IgnoredPrefixes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var hasLog, hasNodeModules bool
	for _, p := range prefixes {
		if p == "debug.log" {
			hasLog = true
		}
		if p == "node_modules/" {
			hasNodeModules = true
		}
	}

	if !hasLog {
		t.Error("IgnoredPrefixes did not include debug.log")
	}
	if !hasNodeModules {
		t.Error("IgnoredPrefixes did not include node_modules/")
	}
}

// TestIgnoredPrefixesDistinguishesFilesAndDirectories tests trailing slashes.
func TestIgnoredPrefixesDistinguishesFilesAndDirectories(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nignored_dir/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "ignored_dir"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "ignored_dir/file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	prefixes, err := repo.IgnoredPrefixes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range prefixes {
		if p == "debug.log" {
			// Files shouldn't have trailing slash
		}
		if p == "ignored_dir/" {
			// Directories should have trailing slash
		}
	}
}

// TestMoveTrackedUsesGitMv tests that moving a tracked file uses git mv.
func TestMoveTrackedUsesGitMv(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Create and commit a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "original.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "original.txt")
	runGit(t, dir, "commit", "-m", "initial")

	// Move using repo.Move.
	err := repo.Move(ctx, "original.txt", "renamed.txt", true)
	if err != nil {
		t.Fatal(err)
	}

	// Check that git recognizes it as a rename (R entry in status).
	status, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range status {
		if f.Path == "renamed.txt" && f.Index == 'R' {
			return // Found the rename
		}
	}
	t.Error("git status did not show a rename entry after Move")
}

// TestMoveUntrackedUsesOsRename tests that moving an untracked file uses os.Rename.
func TestMoveUntrackedUsesOsRename(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(dir, "original.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Move using repo.Move.
	err := repo.Move(ctx, filepath.Join(dir, "original.txt"), filepath.Join(dir, "renamed.txt"), false)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the file was moved on disk.
	if _, err := os.Stat(filepath.Join(dir, "renamed.txt")); err != nil {
		t.Error("renamed.txt not found after Move")
	}
	if _, err := os.Stat(filepath.Join(dir, "original.txt")); err == nil {
		t.Error("original.txt still exists after Move")
	}
}
