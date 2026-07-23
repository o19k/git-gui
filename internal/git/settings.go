package git

import (
	"context"
	"strconv"
	"strings"
)

// Preferences live in git config under the gitgui namespace rather than in a
// file of their own: there is nowhere new to look for them, `git config` reads
// and edits them as well as the tool does, and a repository can override the
// user's default the way it already can for anything else git keeps.

// The keys, spelled as git reports them: it lowercases the variable part.
const (
	SettingTheme            = "gitgui.theme"
	SettingMouse            = "gitgui.mouse"
	SettingLogLimit         = "gitgui.loglimit"
	SettingSignoff          = "gitgui.signoff"
	SettingDiffContext      = "gitgui.diffcontext"
	SettingIgnoreWhitespace = "gitgui.ignorewhitespace"
)

// Bounds on what the numeric settings may be set to. A log limit of zero would
// leave the panel permanently empty, and one of a million would spend the
// refresh on a list nobody scrolls to the end of.
const (
	minLogLimit    = 10
	maxLogLimit    = 100000
	maxDiffContext = 100
)

// Settings is what the tool remembers between runs.
type Settings struct {
	// Light selects the light palette.
	Light bool

	// Mouse captures the pointer for scrolling and click-to-select. The flag
	// turns it on for one run; this turns it on for every run.
	Mouse bool

	// LogLimit is how many commits the Log tab reads.
	LogLimit int

	// Signoff adds a Signed-off-by trailer to every commit made here.
	Signoff bool

	// DiffContext is how many unchanged lines frame each hunk, and
	// IgnoreWhitespace drops hunks that only move whitespace about.
	DiffContext      int
	IgnoreWhitespace bool
}

// DefaultSettings is what an unconfigured repository behaves as.
func DefaultSettings() Settings {
	return Settings{LogLimit: 500, DiffContext: 3}
}

// Diff is the settings that shape a patch, in the form the diff calls take.
func (s Settings) Diff() DiffOpts {
	return DiffOpts{Context: s.DiffContext, IgnoreWhitespace: s.IgnoreWhitespace}
}

// LoadSettings reads every gitgui key in one call. A repository with none set
// is not an error: git exits non-zero when the pattern matches nothing, which
// is the ordinary case.
func (r *Repo) LoadSettings(ctx context.Context) Settings {
	settings := DefaultSettings()

	out, err := r.run(ctx, "config", "--get-regexp", `^gitgui\.`)
	if err != nil {
		return settings
	}

	for _, line := range strings.Split(out, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		settings.apply(key, value)
	}
	return settings
}

// apply sets one key, ignoring names it does not know: gitgui.check lives in
// the same namespace, and a key from a newer version is not an error either.
func (s *Settings) apply(key, value string) {
	switch key {
	case SettingTheme:
		s.Light = strings.EqualFold(value, "light")
	case SettingMouse:
		s.Mouse = configBool(value)
	case SettingSignoff:
		s.Signoff = configBool(value)
	case SettingIgnoreWhitespace:
		s.IgnoreWhitespace = configBool(value)
	case SettingLogLimit:
		if n, err := strconv.Atoi(value); err == nil {
			s.LogLimit = min(max(n, minLogLimit), maxLogLimit)
		}
	case SettingDiffContext:
		if n, err := strconv.Atoi(value); err == nil {
			s.DiffContext = min(max(n, 0), maxDiffContext)
		}
	}
}

// configBool reads the spellings git itself accepts for a boolean. An empty
// value is git's "set but not given a value", which it reads as true.
func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "on", "1", "":
		return true
	}
	return false
}

// SaveSetting records one preference in the user's global config, so a choice
// made in one repository holds in the next. --replace-all because a key set
// twice would otherwise make `--get` fail rather than answer.
func (r *Repo) SaveSetting(ctx context.Context, key, value string) error {
	_, err := r.run(ctx, "config", "--global", "--replace-all", key, value)
	return err
}

// SaveBool records a boolean preference in git's own spelling.
func (r *Repo) SaveBool(ctx context.Context, key string, value bool) error {
	return r.SaveSetting(ctx, key, strconv.FormatBool(value))
}
