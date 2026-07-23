package git

import "testing"

func TestParseStatusOrdinaryAndUntracked(t *testing.T) {
	// porcelain=v2 -z: NUL-terminated records
	out := "1 M. N... 100644 100644 100644 abc def staged.go\x00" +
		"1 .M N... 100644 100644 100644 abc def dirty.go\x00" +
		"? new.go\x00"

	files := parseStatus(out)
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3: %+v", len(files), files)
	}

	if !files[0].Staged() || files[0].Path != "staged.go" || files[0].Code() != 'M' {
		t.Errorf("staged entry parsed wrong: %+v", files[0])
	}
	if files[1].Staged() || files[1].Code() != 'M' {
		t.Errorf("worktree-only entry should not read as staged: %+v", files[1])
	}
	if !files[2].Untracked() || files[2].Path != "new.go" {
		t.Errorf("untracked entry parsed wrong: %+v", files[2])
	}
}

func TestParseStatusRenameConsumesOrigPath(t *testing.T) {
	// A "2" record is followed by its original path as a separate NUL field;
	// mis-handling that shifts every subsequent entry by one.
	out := "2 R. N... 100644 100644 100644 abc def R100 new/name.go\x00old/name.go\x00" +
		"? after.go\x00"

	files := parseStatus(out)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(files), files)
	}
	if files[0].Path != "new/name.go" || files[0].Orig != "old/name.go" {
		t.Errorf("rename parsed wrong: %+v", files[0])
	}
	if files[0].Display() != "old/name.go → new/name.go" {
		t.Errorf("rename display = %q", files[0].Display())
	}
	if files[1].Path != "after.go" {
		t.Errorf("entry after a rename got shifted: %+v", files[1])
	}
}

func TestParseStatusUnmerged(t *testing.T) {
	out := "u UU N... 100644 100644 100644 100644 a b c conflict.go\x00"
	files := parseStatus(out)
	if len(files) != 1 || files[0].Code() != 'U' || files[0].Path != "conflict.go" {
		t.Fatalf("unmerged parsed wrong: %+v", files)
	}
}

func TestParseRefs(t *testing.T) {
	out := "refs/heads/main" + us + "origin/main" + us + "[ahead 2, behind 1]" + us + "*" + us + "abc1234" + us + "latest work\n" +
		"refs/remotes/origin/main" + us + us + us + us + "abc1234" + us + "latest work\n" +
		"refs/tags/v1.0.0" + us + us + us + us + "def5678" + us + "release\n"

	refs := parseRefs(out)
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}

	main := refs[0]
	if main.Kind != RefLocal || main.Name != "main" || !main.Head {
		t.Errorf("local branch parsed wrong: %+v", main)
	}
	if main.Ahead != 2 || main.Behind != 1 {
		t.Errorf("divergence = ↑%d ↓%d, want ↑2 ↓1", main.Ahead, main.Behind)
	}
	if main.Ref() != "refs/heads/main" {
		t.Errorf("Ref() = %q", main.Ref())
	}
	if refs[1].Kind != RefRemote || refs[1].Name != "origin/main" {
		t.Errorf("remote parsed wrong: %+v", refs[1])
	}
	if refs[2].Kind != RefTag || refs[2].Name != "v1.0.0" {
		t.Errorf("tag parsed wrong: %+v", refs[2])
	}
}

func TestParseTrackVariants(t *testing.T) {
	cases := map[string][2]int{
		"":                    {0, 0},
		"[ahead 3]":           {3, 0},
		"[behind 4]":          {0, 4},
		"[ahead 1, behind 2]": {1, 2},
		"[gone]":              {0, 0},
	}
	for input, want := range cases {
		ahead, behind := parseTrack(input)
		if ahead != want[0] || behind != want[1] {
			t.Errorf("parseTrack(%q) = %d,%d want %d,%d", input, ahead, behind, want[0], want[1])
		}
	}
}

func TestParseLog(t *testing.T) {
	out := "sha1" + us + "sha1s" + us + "Ann" + us + "2 hours ago" + us + "first" + us + "p1 p2\n" +
		"sha2" + us + "sha2s" + us + "Bo" + us + "3 days ago" + us + "second" + us + "p1\n"

	commits := parseLog(out)
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	if !commits[0].Merge() {
		t.Error("two parents should read as a merge")
	}
	if commits[1].Merge() {
		t.Error("one parent should not read as a merge")
	}
	if commits[0].Subject != "first" || commits[0].Author != "Ann" {
		t.Errorf("commit parsed wrong: %+v", commits[0])
	}
}

func TestParseStashes(t *testing.T) {
	out := "stash@{0}" + us + "WIP on main: abc123 something\n" +
		"stash@{1}" + us + "On feature: other\n"

	stashes := parseStashes(out)
	if len(stashes) != 2 {
		t.Fatalf("got %d stashes, want 2", len(stashes))
	}
	if stashes[0].Ref != "stash@{0}" || stashes[0].Subject != "WIP on main: abc123 something" {
		t.Errorf("stash parsed wrong: %+v", stashes[0])
	}
}

func TestParseEmptyOutput(t *testing.T) {
	// A clean repo returns empty strings everywhere; none of the parsers may
	// invent a phantom entry from the trailing newline.
	if got := parseStatus(""); len(got) != 0 {
		t.Errorf("parseStatus(\"\") = %+v", got)
	}
	if got := parseRefs(""); len(got) != 0 {
		t.Errorf("parseRefs(\"\") = %+v", got)
	}
	if got := parseLog(""); len(got) != 0 {
		t.Errorf("parseLog(\"\") = %+v", got)
	}
	if got := parseStashes(""); len(got) != 0 {
		t.Errorf("parseStashes(\"\") = %+v", got)
	}
}
