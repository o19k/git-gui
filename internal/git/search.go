package git

import "strings"

// A LogQuery is answered by git rather than by filtering what was already read:
// the panel holds the most recent few hundred commits, and the commit being
// looked for is usually older than that.

// LogQuery narrows the commit list. The fields combine: a query with both an
// author and a message matches only commits with both.
type LogQuery struct {
	// Message matches the commit message, Author the author's name or address.
	// Both are case-insensitive.
	Message string
	Author  string

	// Content matches commits that changed the number of times the string
	// appears in a file — git's pickaxe. It is what finds the commit that
	// deleted a line.
	Content string
}

// Empty reports a query that narrows nothing.
func (q LogQuery) Empty() bool {
	return q.Message == "" && q.Author == "" && q.Content == ""
}

// args are the flags this query adds to a log command.
func (q LogQuery) args() []string {
	if q.Empty() {
		return nil
	}
	// Case-insensitivity covers --grep and --author both.
	args := []string{"--regexp-ignore-case", "--fixed-strings"}
	if q.Message != "" {
		args = append(args, "--grep="+q.Message)
	}
	if q.Author != "" {
		args = append(args, "--author="+q.Author)
	}
	if q.Content != "" {
		args = append(args, "-S"+q.Content)
	}
	return args
}

// Describe names the query for a panel title, in the order the flags are read.
func (q LogQuery) Describe() string {
	var parts []string
	if q.Message != "" {
		parts = append(parts, "message "+q.Message)
	}
	if q.Author != "" {
		parts = append(parts, "author "+q.Author)
	}
	if q.Content != "" {
		parts = append(parts, "content "+q.Content)
	}
	return strings.Join(parts, ", ")
}
