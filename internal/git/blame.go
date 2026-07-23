package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// BlameLine is one line of a file with the commit that last touched it.
type BlameLine struct {
	Short  string
	Author string
	When   string
	Text   string
}

// Blame reads which commit last touched each line of a path. It uses
// --line-porcelain, which repeats the commit's details for every line rather
// than only for the first line of each run, so parsing needs no carry-over
// state between lines.
func (r *Repo) Blame(ctx context.Context, path string) ([]BlameLine, error) {
	out, err := r.run(ctx, "blame", "--line-porcelain", "--", path)
	if err != nil {
		return nil, err
	}
	return parseBlame(out), nil
}

// parseBlame reads --line-porcelain: a header naming the commit, then repeated
// "key value" lines, then the source line itself prefixed with a tab.
func parseBlame(out string) []BlameLine {
	var lines []BlameLine
	var current BlameLine

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "\t"):
			current.Text = strings.TrimPrefix(line, "\t")
			lines = append(lines, current)
			current = BlameLine{}

		case strings.HasPrefix(line, "author "):
			current.Author = strings.TrimPrefix(line, "author ")

		case strings.HasPrefix(line, "author-time "):
			current.When = formatEpoch(strings.TrimPrefix(line, "author-time "))

		default:
			// A header line starts with the commit's full hash. Anything else
			// is a key we do not use.
			if sha, _, found := strings.Cut(line, " "); found && isHex(sha) && len(sha) >= 7 {
				current.Short = sha[:7]
			}
		}
	}
	return lines
}

func formatEpoch(s string) string {
	seconds, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(seconds, 0).Format("2006-01-02")
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
