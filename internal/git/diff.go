package git

import "strconv"

// DiffOpts shapes every patch the panels show. It is passed rather than held on
// the Repo: previews run concurrently with the keystroke that changes the
// setting, and a field read from both would be a race.
type DiffOpts struct {
	// Context is how many unchanged lines frame each hunk. Zero means git's own
	// default, which is three.
	Context int

	// IgnoreWhitespace drops changes that only move whitespace about. It is a
	// reading aid: a patch generated with it cannot be applied back, so hunk
	// staging never uses it.
	IgnoreWhitespace bool
}

// args are the flags these options add to a diff-producing command.
func (o DiffOpts) args() []string {
	var args []string
	if o.Context > 0 {
		args = append(args, "--unified="+strconv.Itoa(o.Context))
	}
	if o.IgnoreWhitespace {
		args = append(args, "--ignore-all-space", "--ignore-blank-lines")
	}
	return args
}

// Applicable is the same options with the parts that would make a patch
// unapplicable removed, for the diff hunk staging reads.
func (o DiffOpts) Applicable() DiffOpts {
	o.IgnoreWhitespace = false
	return o
}
