package tui

import (
	"strings"
	"testing"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/theme"
)

func TestWithSettingsAppliesTheStoredTheme(t *testing.T) {
	t.Cleanup(func() { theme.UseLight(false) })

	m := New(t.Context(), nil, WithSettings(git.Settings{Light: true, LogLimit: 42}))

	if !theme.IsLight() {
		t.Error("the stored theme was not applied")
	}
	if m.logLimitOf() != 42 {
		t.Errorf("log limit = %d, want the stored 42", m.logLimitOf())
	}
}

// A model built without settings, which is every test and any caller that does
// not care, still reads a sensible amount of history.
func TestLogLimitFallsBackToTheDefault(t *testing.T) {
	m := New(t.Context(), nil)
	if m.logLimitOf() != logLimit {
		t.Errorf("log limit = %d, want %d", m.logLimitOf(), logLimit)
	}
}

func TestThemeKeyTogglesAndRemembers(t *testing.T) {
	t.Cleanup(func() { theme.UseLight(false) })

	m := fixture(t)
	light := theme.IsLight()

	m, _ = press(t, m, "T")
	if theme.IsLight() == light {
		t.Error("T did not swap the palette")
	}
	if m.settings.Light != theme.IsLight() {
		t.Error("the model did not record which palette is on")
	}
}

func TestWhitespaceKeyTogglesAndSaysWhichWayItWent(t *testing.T) {
	m := fixture(t)

	m, _ = press(t, m, "w")
	if !m.settings.IgnoreWhitespace {
		t.Fatal("w did not hide whitespace-only changes")
	}
	if !strings.Contains(m.status, "hidden") {
		t.Errorf("status = %q, want it to say which way the setting went", m.status)
	}

	m, _ = press(t, m, "w")
	if m.settings.IgnoreWhitespace {
		t.Error("w did not turn back off")
	}
}

func TestContextKeysWalkTheSteps(t *testing.T) {
	m := fixture(t)

	m, _ = press(t, m, "]")
	if m.settings.DiffContext != 8 {
		t.Errorf("context = %d, want the step above the default", m.settings.DiffContext)
	}

	m, _ = press(t, m, "[")
	m, _ = press(t, m, "[")
	if m.settings.DiffContext != 1 {
		t.Errorf("context = %d, want the narrowest step", m.settings.DiffContext)
	}

	// The ends hold rather than wrap: a width nobody asked for is worse than
	// the key doing nothing.
	m, _ = press(t, m, "[")
	if m.settings.DiffContext != 1 {
		t.Errorf("context = %d, want it to stop at the narrowest", m.settings.DiffContext)
	}
}

func TestStepContextLandsOnTheNearestStepFirst(t *testing.T) {
	// A width configured by hand is not one of the steps, and stepping from it
	// must not jump past everything.
	if got := stepContext(5, 1); got != 8 {
		t.Errorf("stepping up from 5 gave %d, want 8", got)
	}
	if got := stepContext(5, -1); got != 3 {
		t.Errorf("stepping down from 5 gave %d, want 3", got)
	}
	if got := stepContext(0, 1); got != 8 {
		t.Errorf("stepping up from git's default gave %d, want 8", got)
	}
}

// The hunk numbering is what the next keystroke stages, and changing the width
// renumbers them.
func TestContextAndWhitespaceKeysAreRefusedInHunkMode(t *testing.T) {
	m := hunkFixture(t)
	m, _ = press(t, m, "enter")

	before := m.settings.DiffContext
	m, _ = press(t, m, "]")
	if m.settings.DiffContext != before {
		t.Error("the context width changed under an open hunk selection")
	}

	m, _ = press(t, m, "w")
	if m.settings.IgnoreWhitespace {
		t.Error("whitespace was hidden in the patch that is about to be applied")
	}
}
