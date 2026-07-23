package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogQueryArgs(t *testing.T) {
	if empty := (LogQuery{}); !empty.Empty() {
		t.Error("an unset query should narrow nothing")
	}

	args := LogQuery{Message: "fix", Author: "ada", Content: "func main"}.args()
	for _, want := range []string{"--grep=fix", "--author=ada", "-Sfunc main", "--regexp-ignore-case", "--fixed-strings"} {
		if !contains(args, want) {
			t.Errorf("%q missing from %v", want, args)
		}
	}

	// The panel title is the only thing saying why the list is short.
	if got := (LogQuery{Author: "ada"}).Describe(); got != "author ada" {
		t.Errorf("Describe = %q", got)
	}
}

// The point of searching in git rather than filtering the panel: the commit is
// found even when it is beyond the limit the panel reads.
func TestSearchLogFindsACommitBeyondTheLimit(t *testing.T) {
	repo := searchRepo(t)
	ctx := context.Background()

	found, err := repo.SearchLog(ctx, "", 2, LogQuery{Message: "needle"})
	if err != nil {
		t.Fatalf("SearchLog: %v", err)
	}
	if len(found) != 1 || !strings.Contains(found[0].Subject, "needle") {
		t.Fatalf("the message search found %v", logSubjects(found))
	}

	// And the pickaxe finds the commit by what it changed, not by its message.
	byContent, err := repo.SearchLog(ctx, "", 50, LogQuery{Content: "distinctive"})
	if err != nil {
		t.Fatalf("SearchLog: %v", err)
	}
	if len(byContent) != 1 || !strings.Contains(byContent[0].Subject, "needle") {
		t.Fatalf("the content search found %v", logSubjects(byContent))
	}

	empty, err := repo.SearchLog(ctx, "", 50, LogQuery{Message: "nothing has this"})
	if err != nil {
		t.Fatalf("SearchLog: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("a query nothing matches returned %v", logSubjects(empty))
	}

}

// searchRepo makes a history where the commit being looked for is old enough
// that a short limit would miss it.
func searchRepo(t *testing.T) *Repo {
	t.Helper()
	repo, dir := newRepo(t)
	ctx := context.Background()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("hay.txt", "a distinctive line\n")
	if err := repo.Stage(ctx, "hay.txt"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "the needle", CommitOpts{}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"a", "b", "c", "d"} {
		write(name+".txt", name+"\n")
		if err := repo.Stage(ctx, name+".txt"); err != nil {
			t.Fatal(err)
		}
		if err := repo.Commit(ctx, "filler "+name, CommitOpts{}); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func logSubjects(commits []Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.Subject)
	}
	return out
}
