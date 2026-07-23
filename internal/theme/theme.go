// Package theme holds the dark and light palettes. Everything downstream
// references a named colour here rather than a literal, which is what makes
// switching between them possible.
package theme

import "github.com/charmbracelet/lipgloss"

// Dark palette (zinc-dark). These are variables so the palette can be swapped
// at runtime; every style below is rebuilt from them when the palette changes.
var (
	darkPalette = map[string]string{
		"BG":          "#09090b",
		"Panel":       "#111113",
		"PanelAlt":    "#18181b",
		"Border":      "#3f3f46",
		"BorderFocus": "#d97757",
		"Text":        "#fafafa",
		"TextMuted":   "#a1a1aa",
		"TextDim":     "#71717a",
		"Primary":     "#d97757",
		"PrimaryDim":  "#4a241b",
		"Blue":        "#60a5fa",
		"Cyan":        "#22d3ee",
		"Green":       "#6a9f73",
		"Red":         "#ef4444",
		"Yellow":      "#eab308",
		"Purple":      "#a78bfa",
	}

	// Light palette: warm neutrals, no purple/lavender/blue-grey. Dark text on
	// light panels for legibility; accent must contrast against light fill.
	lightPalette = map[string]string{
		"BG":          "#fdf8f3",
		"Panel":       "#fff5ee",
		"PanelAlt":    "#f5ebe0",
		"Border":      "#d8c9b6",
		"BorderFocus": "#c65d2b",
		"Text":        "#1a1410",
		"TextMuted":   "#6b5d52",
		"TextDim":     "#8b7966",
		"Primary":     "#c65d2b",
		"PrimaryDim":  "#ead5c4",
		"Blue":        "#5a7a8f",
		"Cyan":        "#4a8a94",
		"Green":       "#5a8a52",
		"Red":         "#c63535",
		"Yellow":      "#ad8a00",
		"Purple":      "#7a6a8f",
	}

	// Currently active palette
	currentPalette = darkPalette

	// Exported names that downstream code uses. These are rebuilt on palette change.
	BG          string
	Panel       string
	PanelAlt    string
	Border      string
	BorderFocus string
	Text        string
	TextMuted   string
	TextDim     string
	Primary     string
	PrimaryDim  string
	Blue        string
	Cyan        string
	Green       string
	Red         string
	Yellow      string
	Purple      string
)

var (
	// Styles are rebuilt from palette on every change. Focus draws its border
	// in the accent — the fill is never flooded, but a border one step off the
	// background is not a division, and three columns of text with nothing
	// visible between them read as one column of noise.
	PanelStyle      lipgloss.Style
	PanelFocusStyle lipgloss.Style

	TitleStyle      lipgloss.Style
	TitleFocusStyle lipgloss.Style

	// A tinted background rather than a full-accent bar, so a long list of text
	// stays readable.
	SelectedStyle lipgloss.Style

	// An unfocused selection recedes but stays visible, so you keep your place.
	SelectedBlurStyle lipgloss.Style

	// Tabs read as a bar, not as buttons: the open one is brighter and sits on
	// a lighter fill.
	TabStyle       lipgloss.Style
	TabActiveStyle lipgloss.Style

	// TabNumStyle marks the digit that opens a tab, inside the label.
	TabNumStyle       lipgloss.Style
	TabActiveNumStyle lipgloss.Style

	NormalStyle     lipgloss.Style
	MutedStyle      lipgloss.Style
	DimStyle        lipgloss.Style
	FooterKeyStyle  lipgloss.Style
	FooterDescStyle lipgloss.Style
	ErrorStyle      lipgloss.Style
)

func init() {
	rebuildStyles()
}

// rebuildStyles rebuilds every style from the current palette.
func rebuildStyles() {
	BG = currentPalette["BG"]
	Panel = currentPalette["Panel"]
	PanelAlt = currentPalette["PanelAlt"]
	Border = currentPalette["Border"]
	BorderFocus = currentPalette["BorderFocus"]
	Text = currentPalette["Text"]
	TextMuted = currentPalette["TextMuted"]
	TextDim = currentPalette["TextDim"]
	Primary = currentPalette["Primary"]
	PrimaryDim = currentPalette["PrimaryDim"]
	Blue = currentPalette["Blue"]
	Cyan = currentPalette["Cyan"]
	Green = currentPalette["Green"]
	Red = currentPalette["Red"]
	Yellow = currentPalette["Yellow"]
	Purple = currentPalette["Purple"]

	PanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(Border)).
		Foreground(lipgloss.Color(Text))

	PanelFocusStyle = PanelStyle.
		BorderForeground(lipgloss.Color(BorderFocus))

	TitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextMuted)).
		Padding(0, 1)

	TitleFocusStyle = TitleStyle.
		Foreground(lipgloss.Color(Primary)).
		Bold(true)

	SelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(PrimaryDim)).
		Foreground(lipgloss.Color(Text))

	SelectedBlurStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(PanelAlt)).
		Foreground(lipgloss.Color(TextMuted))

	TabStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextDim)).
		Padding(0, 2)

	// The open tab is named by the accent and underlined, not by a fill one
	// step off the background: at these two greys a tinted box reads as a
	// smudge rather than as a selection, and it has to survive being told
	// apart at a glance from three unopened neighbours.
	TabActiveStyle = TabStyle.
		Foreground(lipgloss.Color(Primary)).
		Bold(true).
		Underline(true)

	// The digit is what the accent is for on an unopened tab. On the open one
	// the label already carries it, so the digit steps back rather than
	// competing with it.
	TabNumStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(Primary)).
		Bold(true)

	TabActiveNumStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(Text)).
		Bold(true).
		Underline(true)

	NormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(Text))
	MutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(TextMuted))
	DimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(TextDim))

	FooterKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(Primary))
	FooterDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(TextMuted))

	ErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(Red))

	SyntaxKeyword = lipgloss.NewStyle().Foreground(lipgloss.Color(Purple))
	SyntaxString = lipgloss.NewStyle().Foreground(lipgloss.Color(Green))
	SyntaxComment = lipgloss.NewStyle().Foreground(lipgloss.Color(TextDim)).Italic(true)
	SyntaxNumber = lipgloss.NewStyle().Foreground(lipgloss.Color(Yellow))
	SyntaxFunc = lipgloss.NewStyle().Foreground(lipgloss.Color(Blue))
	SyntaxType = lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan))
	SyntaxPunct = lipgloss.NewStyle().Foreground(lipgloss.Color(TextMuted))
}

// Syntax names the parts of a source file the preview colours. They are
// palette entries rather than colours of their own: the two palettes are
// chosen as wholes, and a highlighter with its own idea of green would be the
// one thing on screen that ignores the light/dark switch.
var (
	SyntaxKeyword lipgloss.Style
	SyntaxString  lipgloss.Style
	SyntaxComment lipgloss.Style
	SyntaxNumber  lipgloss.Style
	SyntaxFunc    lipgloss.Style
	SyntaxType    lipgloss.Style
	SyntaxPunct   lipgloss.Style
)

// StatusColor maps a git status letter to its palette entry.
func StatusColor(code byte) lipgloss.Color {
	switch code {
	case 'A':
		return lipgloss.Color(Green)
	case 'M':
		return lipgloss.Color(Blue)
	case 'D':
		return lipgloss.Color(Red)
	case 'R', 'C':
		return lipgloss.Color(Cyan)
	case 'T':
		return lipgloss.Color(Purple)
	case '?':
		return lipgloss.Color(Yellow)
	case 'U':
		return lipgloss.Color(Red)
	}
	return lipgloss.Color(TextMuted)
}

// DiffLineStyle colours a unified-diff line by its leading marker.
func DiffLineStyle(line string) lipgloss.Style {
	if line == "" {
		return NormalStyle
	}
	switch {
	case len(line) >= 3 && (line[:3] == "+++" || line[:3] == "---"):
		return DimStyle
	case line[0] == '@':
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan))
	case line[0] == '+':
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Green))
	case line[0] == '-':
		return lipgloss.NewStyle().Foreground(lipgloss.Color(Red))
	case line[0] == 'd' || line[0] == 'i' || line[0] == 'n':
		return DimStyle
	}
	return NormalStyle
}

// GraphLane returns the colour for commit-graph lane n, cycling through seven.
func GraphLane(n int) lipgloss.Color {
	lanes := []string{Primary, Green, Purple, Cyan, Yellow, Red, "#8fb3a2"}
	return lipgloss.Color(lanes[((n%len(lanes))+len(lanes))%len(lanes)])
}
