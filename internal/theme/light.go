package theme

// The palette is one set of names with two sets of values behind it. Switching
// swaps the values and rebuilds every style, so nothing downstream has to know
// which one is on — the same named colour just means something else.

var light bool

// IsLight reports which palette is in use.
func IsLight() bool { return light }

// UseLight swaps the palette and rebuilds the styles built from it. Switching
// to light mode uses warm neutrals suitable for a light background; dark text
// is used throughout for contrast on light panels.
func UseLight(on bool) {
	if light == on {
		return
	}
	light = on
	if on {
		currentPalette = lightPalette
	} else {
		currentPalette = darkPalette
	}
	rebuildStyles()
}
