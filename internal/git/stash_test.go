package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNameStatusReadsTheZeroSeparatedForm(t *testing.T) {
	// The letter is its own record, and a rename spends two path records.
	files := parseNameStatus("M\x00f.txt\x00R100\x00g.txt\x00renamed.txt\x00A\x00new.txt\x00")

	if len(files) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(files), files)
	}
	if files[0].Path != "f.txt" || files[0].Index != 'M' {
		t.Errorf("first entry = %+v", files[0])
	}
	if files[1].Index != 'R' || files[1].Orig != "g.txt" || files[1].Path != "renamed.txt" {
		t.Errorf("rename entry = %+v", files[1])
	}
	if got := files[1].Display(); got != "g.txt → renamed.txt" {
		t.Errorf("rename display = %q", got)
	}
	if files[2].Path != "new.txt" || files[2].Index != 'A' {
		t.Errorf("entry after the rename = %+v", files[2])
	}
}

func TestParseNameStatusOnEmptyOutput(t *testing.T) {
	if got := parseNameStatus(""); len(got) != 0 {
		t.Errorf("empty output produced %+v", got)
	}
}

// stashRepo commits two files, changes both, and stashes.
func stashRepo(t *testing.T) *Repo {
	t.Helper()
	repo, dir := newRepo(t)
	ctx := context.Background()

	for _, name := range []string{"f.txt", "g.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add both", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"f.txt", "g.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.StashPush(ctx, StashOpts{Message: "wip", Untracked: true}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestStashFilesListsWhatTheEntryHolds(t *testing.T) {
	repo := stashRepo(t)

	files, err := repo.StashFiles(context.Background(), "stash@{0}")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(files), files)
	}
	paths := []string{files[0].Path, files[1].Path}
	for _, want := range []string{"f.txt", "g.txt"} {
		if !slicesContains(paths, want) {
			t.Errorf("%s missing from %v", want, paths)
		}
	}
}

func TestStashFileDiffIsScopedToOnePath(t *testing.T) {
	// `stash show` rejects a pathspec, so this must not be built on it.
	repo := stashRepo(t)

	patch, err := repo.StashFileDiff(context.Background(), "stash@{0}", "f.txt", DiffOpts{})
	if err != nil {
		t.Fatalf("StashFileDiff: %v", err)
	}
	if !strings.Contains(patch, "f.txt") || !strings.Contains(patch, "+changed") {
		t.Errorf("the path's own change is missing:\n%s", patch)
	}
	if strings.Contains(patch, "g.txt") {
		t.Errorf("the patch leaked another path:\n%s", patch)
	}
}

func TestStashApplyFilesRestoresOnlyTheChosenPaths(t *testing.T) {
	repo := stashRepo(t)
	ctx := context.Background()

	if err := repo.StashApplyFiles(ctx, "stash@{0}", []string{"f.txt"}); err != nil {
		t.Fatalf("StashApplyFiles: %v", err)
	}

	f, err := os.ReadFile(filepath.Join(repo.Path, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(f)) != "changed" {
		t.Errorf("the chosen path was not restored: %q", f)
	}

	g, err := os.ReadFile(filepath.Join(repo.Path, "g.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(g)) != "original" {
		t.Errorf("an unchosen path was restored too: %q", g)
	}

	// The entry stays on the stack, so the paths left behind are still recoverable.
	stashes, err := repo.Stashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range stashes {
		if strings.Contains(s.Subject, "wip") {
			found = true
		}
	}
	if !found {
		t.Errorf("restoring some files consumed the stash: %+v", stashes)
	}
}

func TestStashApplyFilesRefusesAnEmptySelection(t *testing.T) {
	repo := stashRepo(t)
	if err := repo.StashApplyFiles(context.Background(), "stash@{0}", nil); err == nil {
		t.Error("restoring no paths should be an error, not a no-op checkout")
	}
}

func slicesContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestCommitFilesListsWhatTheCommitTouched(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	for _, name := range []string{"x.txt", "y.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add x and y", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	commits, err := repo.Log(ctx, 1)
	if err != nil || len(commits) == 0 {
		t.Fatalf("Log: %v %+v", err, commits)
	}
	sha := commits[0].SHA

	files, err := repo.CommitFiles(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	for _, want := range []string{"x.txt", "y.txt"} {
		if !slicesContains(paths, want) {
			t.Errorf("%s missing from %v", want, paths)
		}
	}

	patch, err := repo.CommitFileDiff(ctx, sha, "x.txt", DiffOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "x.txt") {
		t.Errorf("the path's own change is missing:\n%s", patch)
	}
	if strings.Contains(patch, "y.txt") {
		t.Errorf("the patch leaked another path:\n%s", patch)
	}
}
