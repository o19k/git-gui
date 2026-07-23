package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

// A preference changed here is written back to the user's global git config, so
// the next run starts where this one left off. The write is a git call like any
// other, and a failing one is reported in the footer rather than being allowed
// to stop the change taking effect for this run.

// Option configures a Model at construction. New takes them variadically so a
// caller that wants the defaults, which is every test, needs none.
type Option func(*Model)

// WithSettings starts the model from stored preferences.
func WithSettings(s git.Settings) Option {
	return func(m *Model) {
		m.settings = s
		theme.UseLight(s.Light)
	}
}

// contextSteps are the widths the context key cycles through. Three is git's
// own default; twenty is enough to read a hunk in the shape of its function.
var contextSteps = []int{1, 3, 8, 20}

// diffOpts is how patches are generated right now. Hunk mode drops the
// whitespace setting: a patch that hides changes cannot be applied back, and
// what is staged has to be what is on screen.
func (m Model) diffOpts() git.DiffOpts {
	opts := m.settings.Diff()
	if m.hunkMode {
		return opts.Applicable()
	}
	return opts
}

// logLimitOf is how many commits to read, falling back to the default for a
// model built without settings.
func (m Model) logLimitOf() int {
	if m.settings.LogLimit <= 0 {
		return logLimit
	}
	return m.settings.LogLimit
}

// saveSetting records a preference without blocking on it. The change is
// already in effect; this is only about the next run.
func (m Model) saveSetting(key, value string) tea.Cmd {
	repo, ctx := m.repo, m.ctx
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		if err := repo.SaveSetting(ctx, key, value); err != nil {
			return settingSavedMsg{err: err}
		}
		return settingSavedMsg{}
	}
}

// settingSavedMsg reports a preference that could not be written.
type settingSavedMsg struct{ err error }

func (m Model) handleSettingSaved(msg settingSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = "the setting holds for this run only: " + msg.err.Error()
	}
	return m, nil
}

// toggleTheme swaps the palette and remembers which one is on.
func (m Model) toggleTheme() (tea.Model, tea.Cmd) {
	light := !theme.IsLight()
	theme.UseLight(light)
	m.settings.Light = light

	value := "dark"
	if light {
		value = "light"
	}
	return m, m.saveSetting(git.SettingTheme, value)
}

// toggleWhitespace hides or shows changes that only move whitespace about.
func (m Model) toggleWhitespace() (tea.Model, tea.Cmd) {
	if m.hunkMode {
		m.status = "leave hunk mode to change how patches are read"
		return m, nil
	}
	m.settings.IgnoreWhitespace = !m.settings.IgnoreWhitespace
	m.status = "whitespace-only changes shown"
	if m.settings.IgnoreWhitespace {
		m.status = "whitespace-only changes hidden"
	}

	save := m.saveSetting(git.SettingIgnoreWhitespace, boolValue(m.settings.IgnoreWhitespace))
	return m, tea.Batch(save, m.refreshPreview())
}

// cycleContext widens or narrows the unchanged lines framing each hunk.
func (m Model) cycleContext(delta int) (tea.Model, tea.Cmd) {
	if m.hunkMode {
		// The hunks would be renumbered under the cursor, and the selection is
		// what the next keystroke stages.
		m.status = "leave hunk mode to change the context width"
		return m, nil
	}

	next := stepContext(m.settings.DiffContext, delta)
	m.settings.DiffContext = next
	m.status = fmt.Sprintf("%d lines of context", next)

	save := m.saveSetting(git.SettingDiffContext, fmt.Sprint(next))
	return m, tea.Batch(save, m.refreshPreview())
}

// stepContext moves one place along contextSteps, stopping at the ends. A width
// that is not one of the steps lands on the nearest one first.
func stepContext(current, delta int) int {
	if current <= 0 {
		current = 3
	}

	at, exact := 0, false
	for i, step := range contextSteps {
		if step <= current {
			at = i
		}
		if step == current {
			exact = true
		}
	}

	// A width set by hand sits between two steps. Narrowing from it means the
	// step below, which is the one `at` already names.
	if !exact && delta < 0 {
		return contextSteps[at]
	}
	return contextSteps[clamp(at+delta, 0, len(contextSteps)-1)]
}

func boolValue(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
