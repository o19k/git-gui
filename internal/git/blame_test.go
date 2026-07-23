package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlameNamesTheCommitBehindEachLine(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, "b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add b"); err != nil {
		t.Fatal(err)
	}

	lines, err := repo.Blame(ctx, "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d blamed lines, want 2: %+v", len(lines), lines)
	}
	for i, want := range []string{"first", "second"} {
		if lines[i].Text != want {
			t.Errorf("line %d text = %q, want %q", i, lines[i].Text, want)
		}
		if len(lines[i].Short) != 7 {
			t.Errorf("line %d short sha = %q", i, lines[i].Short)
		}
		if lines[i].Author == "" {
			t.Errorf("line %d has no author", i)
		}
		if !strings.HasPrefix(lines[i].When, "20") {
			t.Errorf("line %d date = %q, want a yyyy-mm-dd", i, lines[i].When)
		}
	}
}

func TestBlameOnAnUnknownPathErrors(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.Blame(context.Background(), "nope.txt"); err == nil {
		t.Error("blaming a path git does not track should error")
	}
}
