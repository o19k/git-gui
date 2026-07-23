package theme

import (
	"testing"
)

// TestPaletteSwitch verifies that switching between palettes rebuilds the styles
// with new colours and that switching back restores the originals.
func TestPaletteSwitch(t *testing.T) {
	// Capture dark mode values
	IsLight() // Ensure we start in dark mode
	UseLight(false)
	darkBG := BG
	darkPrimary := Primary
	darkText := Text

	// Switch to light mode and verify the colours changed
	UseLight(true)
	if BG == darkBG {
		t.Error("switching to light mode should change BG, but it did not")
	}
	if Primary == darkPrimary {
		t.Error("switching to light mode should change Primary, but it did not")
	}
	if Text == darkText {
		t.Error("switching to light mode should change Text, but it did not")
	}
	lightBG := BG
	lightPrimary := Primary
	lightText := Text

	// Switch back to dark mode and verify the colours restored
	UseLight(false)
	if BG != darkBG {
		t.Errorf("switching to dark mode should restore BG to %q, got %q", darkBG, BG)
	}
	if Primary != darkPrimary {
		t.Errorf("switching to dark mode should restore Primary to %q, got %q", darkPrimary, Primary)
	}
	if Text != darkText {
		t.Errorf("switching to dark mode should restore Text to %q, got %q", darkText, Text)
	}

	// Switch to light again and verify the light colours returned
	UseLight(true)
	if BG != lightBG {
		t.Errorf("switching to light mode should restore BG to %q, got %q", lightBG, BG)
	}
	if Primary != lightPrimary {
		t.Errorf("switching to light mode should restore Primary to %q, got %q", lightPrimary, Primary)
	}
	if Text != lightText {
		t.Errorf("switching to light mode should restore Text to %q, got %q", lightText, Text)
	}
}

// TestStylesRebuilt verifies that styles are actually rebuilt when the palette
// changes by checking that the colour variables used to build them change.
// Since Lipgloss styles don't expose their properties, we verify the underlying
// colour values are swapped and used to construct new styles.
func TestStylesRebuilt(t *testing.T) {
	UseLight(false)
	darkText := Text
	darkPrimary := Primary

	UseLight(true)
	lightText := Text
	lightPrimary := Primary

	if darkText == lightText {
		t.Error("Text color should change when switching to light, but it did not")
	}
	if darkPrimary == lightPrimary {
		t.Error("Primary color should change when switching to light, but it did not")
	}

	UseLight(false)
	if Text != darkText {
		t.Error("Text color should restore when switching back to dark, but it did not")
	}
	if Primary != darkPrimary {
		t.Error("Primary color should restore when switching back to dark, but it did not")
	}
}

// TestSwitchIdempotent verifies that calling UseLight with the same value twice
// does not cause issues and is safe.
func TestSwitchIdempotent(t *testing.T) {
	UseLight(false)
	darkPrimary := Primary
	UseLight(false)
	if Primary != darkPrimary {
		t.Error("calling UseLight(false) twice should not change the palette")
	}

	UseLight(true)
	lightPrimary := Primary
	UseLight(true)
	if Primary != lightPrimary {
		t.Error("calling UseLight(true) twice should not change the palette")
	}
}

// A border one step off the background is not a division: three columns of
// text with nothing visible between them read as one column of noise. This is
// the complaint that produced the change, so it is asserted as a distance
// rather than as a pair of literals.
func TestBordersStandOutFromTheBackground(t *testing.T) {
	for _, light := range []bool{false, true} {
		UseLight(light)

		if gap := channelGap(Border, BG); gap < 24 {
			t.Errorf("light=%v: the border is %d off the background, too close to see", light, gap)
		}
		if gap := channelGap(BorderFocus, Border); gap < 24 {
			t.Errorf("light=%v: the focused border is %d off an unfocused one", light, gap)
		}
	}
	UseLight(false)
}

// channelGap is the largest per-channel difference between two #rrggbb colours.
func channelGap(a, b string) int {
	var worst int
	for i := 1; i < 7; i += 2 {
		x, y := hexByte(a[i:i+2]), hexByte(b[i:i+2])
		if d := x - y; d > worst {
			worst = d
		} else if -d > worst {
			worst = -d
		}
	}
	return worst
}

func hexByte(s string) int {
	var n int
	for _, c := range s {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		}
	}
	return n
}
