package git

import (
	"context"
	"strings"
	"testing"
)

func TestCreateTagPlainAndAnnotated(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateTag(ctx, "v0.1.0", "HEAD", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if !repo.HasTag(ctx, "v0.1.0") {
		t.Error("the tag was not made")
	}

	if err := repo.CreateTag(ctx, "v0.2.0", "HEAD", "the second one"); err != nil {
		t.Fatalf("CreateTag annotated: %v", err)
	}

	// An annotated tag is an object of its own, which is what a release needs
	// and what a plain pointer is not.
	out, err := repo.run(ctx, "cat-file", "-t", "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "tag" {
		t.Errorf("v0.2.0 is a %q, want an annotated tag object", strings.TrimSpace(out))
	}

	// And the refs list shows both, which is where they are deleted from.
	refs, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, ref := range refs {
		if ref.Kind == RefTag {
			found++
		}
	}
	if found != 2 {
		t.Errorf("the refs list holds %d tags, want 2", found)
	}
}

func TestTagNamesAreCheckedBeforeReachingGit(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	for _, name := range []string{"", "--delete", "has space", "ends/"} {
		if err := repo.CreateTag(ctx, name, "HEAD", ""); err == nil {
			t.Errorf("%q was accepted as a tag name", name)
		} else if !strings.Contains(err.Error(), "tag name") {
			t.Errorf("%q: error says %q, want it to name what was wrong", name, err)
		}
	}
}

func TestDeleteTagGoesThroughTheRefsList(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateTag(ctx, "gone", "HEAD", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteBranch(ctx, Branch{Name: "gone", Kind: RefTag}, false); err != nil {
		t.Fatalf("DeleteBranch on a tag: %v", err)
	}
	if repo.HasTag(ctx, "gone") {
		t.Error("the tag survived")
	}
}
