package git

import (
	"strconv"
	"strings"
)

// Unit separator. Safe inside a git format string because it cannot occur in a
// ref name, an author name or a commit subject.
const us = "\x1f"

const (
	refFormat       = "%(refname)" + us + "%(upstream:short)" + us + "%(upstream:track)" + us + "%(HEAD)" + us + "%(objectname:short)" + us + "%(contents:subject)"
	logFormat       = "%H" + us + "%h" + us + "%an" + us + "%ar" + us + "%s" + us + "%P"
	prettyLogFormat = "%h  %s" + us + "%an, %ar"
)

// FileChange is one entry in the Files panel.
type FileChange struct {
	Index byte // staged status letter
	Work  byte // working-tree status letter
	Path  string
	Orig  string // previous path for a rename/copy
}

// Staged reports whether the index holds a change for this path.
func (f FileChange) Staged() bool { return f.Index != '.' && f.Index != ' ' && f.Index != '?' }

// Code is the single letter the panel displays: the working-tree status when
// there is one, otherwise the staged status.
func (f FileChange) Code() byte {
	if f.Work != '.' && f.Work != ' ' && f.Work != 0 {
		return f.Work
	}
	return f.Index
}

// Untracked reports a file git has never seen.
func (f FileChange) Untracked() bool { return f.Index == '?' }

// Display is the path as shown, with renames rendered as "old → new".
func (f FileChange) Display() string {
	if f.Orig != "" {
		return f.Orig + " → " + f.Path
	}
	return f.Path
}

// parseStatus reads `git status --porcelain=v2 -z`.
func parseStatus(out string) []FileChange {
	records := strings.Split(out, "\x00")
	var files []FileChange

	for i := 0; i < len(records); i++ {
		rec := records[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '1': // ordinary change
			fields := strings.SplitN(rec, " ", 9)
			if len(fields) < 9 || len(fields[1]) < 2 {
				continue
			}
			files = append(files, FileChange{Index: fields[1][0], Work: fields[1][1], Path: fields[8]})

		case '2': // rename or copy — the original path is the next NUL-separated field
			fields := strings.SplitN(rec, " ", 10)
			if len(fields) < 10 || len(fields[1]) < 2 {
				continue
			}
			change := FileChange{Index: fields[1][0], Work: fields[1][1], Path: fields[9]}
			if i+1 < len(records) {
				i++
				change.Orig = records[i]
			}
			files = append(files, change)

		case 'u': // unmerged
			fields := strings.SplitN(rec, " ", 11)
			if len(fields) < 11 {
				continue
			}
			files = append(files, FileChange{Index: 'U', Work: 'U', Path: fields[10]})

		case '?': // untracked
			files = append(files, FileChange{Index: '?', Work: '?', Path: strings.TrimPrefix(rec, "? ")})
		}
	}
	return files
}

// parseNameStatus reads `--name-status -z`, whose records alternate between a
// status letter and the path it applies to — "M\0f.txt\0" — with a rename or
// copy spending two path records instead of one: "R100\0old\0new\0". The letter
// is its own record here, not a prefix on the path as it is without -z.
//
// There is no working-tree column as there is in porcelain=v2, so the letter
// describes the change the entry holds.
func parseNameStatus(out string) []FileChange {
	records := strings.Split(out, "\x00")
	var files []FileChange

	for i := 0; i+1 < len(records); i += 2 {
		status := records[i]
		if status == "" {
			continue
		}
		// A rename's letter carries a similarity score, as in "R100".
		code := status[0]
		if code == 'R' || code == 'C' {
			if i+2 >= len(records) {
				break
			}
			files = append(files, FileChange{
				Index: code, Work: '.', Orig: records[i+1], Path: records[i+2],
			})
			i++ // the extra path record this entry consumed
			continue
		}
		files = append(files, FileChange{Index: code, Work: '.', Path: records[i+1]})
	}
	return files
}

// RefKind distinguishes the three groups the Branches panel shows.
type RefKind int

const (
	RefLocal RefKind = iota
	RefRemote
	RefTag
)

// Branch is one ref in the Branches panel.
type Branch struct {
	Name     string
	Kind     RefKind
	Upstream string
	Ahead    int
	Behind   int
	Head     bool // currently checked out
	SHA      string
	Subject  string
}

// Ref is the fully-qualified name, needed to log or check out the branch.
func (b Branch) Ref() string {
	switch b.Kind {
	case RefRemote:
		return "refs/remotes/" + b.Name
	case RefTag:
		return "refs/tags/" + b.Name
	}
	return "refs/heads/" + b.Name
}

// parseRefs reads `git for-each-ref` in refFormat.
func parseRefs(out string) []Branch {
	var refs []Branch
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, us)
		if len(fields) < 6 {
			continue
		}
		full := fields[0]
		branch := Branch{
			Upstream: fields[1],
			Head:     fields[3] == "*",
			SHA:      fields[4],
			Subject:  fields[5],
		}
		switch {
		case strings.HasPrefix(full, "refs/heads/"):
			branch.Kind, branch.Name = RefLocal, strings.TrimPrefix(full, "refs/heads/")
		case strings.HasPrefix(full, "refs/remotes/"):
			branch.Kind, branch.Name = RefRemote, strings.TrimPrefix(full, "refs/remotes/")
		case strings.HasPrefix(full, "refs/tags/"):
			branch.Kind, branch.Name = RefTag, strings.TrimPrefix(full, "refs/tags/")
		default:
			continue
		}
		branch.Ahead, branch.Behind = parseTrack(fields[2])
		refs = append(refs, branch)
	}
	return refs
}

// parseTrack reads the "[ahead 1, behind 2]" form of %(upstream:track).
func parseTrack(track string) (ahead, behind int) {
	track = strings.Trim(track, "[]")
	for _, part := range strings.Split(track, ", ") {
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimPrefix(part, "ahead "))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimPrefix(part, "behind "))
		}
	}
	return ahead, behind
}

// Commit is one row in the Commits panel.
type Commit struct {
	SHA     string
	Short   string
	Author  string
	When    string
	Subject string
	Parents []string
}

// Merge reports a commit with more than one parent.
func (c Commit) Merge() bool { return len(c.Parents) > 1 }

// parseLog reads `git log` in logFormat.
func parseLog(out string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, us)
		if len(fields) < 6 {
			continue
		}
		commit := Commit{
			SHA:     fields[0],
			Short:   fields[1],
			Author:  fields[2],
			When:    fields[3],
			Subject: fields[4],
		}
		if fields[5] != "" {
			commit.Parents = strings.Fields(fields[5])
		}
		commits = append(commits, commit)
	}
	return commits
}

// Stash is one entry in the Stash panel.
type Stash struct {
	Ref     string // stash@{0}
	Subject string
}

// parseStashes reads `git stash list`.
func parseStashes(out string) []Stash {
	var stashes []Stash
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		ref, subject, _ := strings.Cut(line, us)
		stashes = append(stashes, Stash{Ref: ref, Subject: subject})
	}
	return stashes
}
