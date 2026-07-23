package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/o19k/git-gui/internal/theme"
)

// Highlighting runs where the file is read — on a background goroutine, once
// per read — never in Update and never per frame: a 1,200-line file costs
// ~46 ms against a 16 ms frame budget. The result travels with the content it
// belongs to, and the pane slices it like any other lines.
//
// The colours come from the palette rather than from a highlighter theme, so
// the light/dark switch reaches source code too.

// syntax is the palette the colouring uses, resolved before the work starts:
// the styles are package-level variables the light/dark switch rewrites, and
// the walk runs on a background goroutine.
type syntax struct {
	keyword, text, comment, number, function, typ, punct lipgloss.Style
}

// currentSyntax reads the palette. It is called where the model is, not where
// the highlighting happens.
func currentSyntax() syntax {
	return syntax{
		keyword:  theme.SyntaxKeyword,
		text:     theme.NormalStyle,
		comment:  theme.SyntaxComment,
		number:   theme.SyntaxNumber,
		function: theme.SyntaxFunc,
		typ:      theme.SyntaxType,
		punct:    theme.SyntaxPunct,
	}
}

// styleFor maps a token to a palette entry. Chroma's token types are a tree —
// Keyword covers KeywordDeclaration and the rest — so the categories are
// tested with chroma's own InCategory rather than by listing every leaf.
func (s syntax) styleFor(t chroma.TokenType) (lipgloss.Style, bool) {
	switch {
	case t.InCategory(chroma.Comment):
		return s.comment, true
	case t.InCategory(chroma.LiteralString):
		return theme.SyntaxString, true
	case t.InCategory(chroma.LiteralNumber):
		return s.number, true
	case t.InCategory(chroma.Keyword):
		return s.keyword, true
	case t.InCategory(chroma.NameFunction), t.InCategory(chroma.NameClass):
		return s.function, true
	case t.InCategory(chroma.NameBuiltin), t.InCategory(chroma.KeywordType):
		return s.typ, true
	case t.InCategory(chroma.Operator), t.InCategory(chroma.Punctuation):
		return s.punct, true
	}
	return s.text, false
}

// highlight returns content as styled lines, one per line of the input, and
// nil for a file of no language it knows — the caller's signal to draw the
// text as it stands rather than colour a guess.
func highlight(path, content string, palette syntax) []string {
	lexer := lexers.Match(path)
	if lexer == nil {
		return nil
	}
	// Coalescing merges adjacent tokens of one type: fewer escape sequences for
	// the same output.
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, content)
	if err != nil {
		return nil
	}

	// The line count has to match the plain text exactly: the pane scrolls by
	// line number, and a search jumps to one.
	lines := make([]string, 0, strings.Count(content, "\n")+1)
	var current strings.Builder

	for _, token := range iterator.Tokens() {
		style, styled := palette.styleFor(token.Type)

		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
			if part == "" {
				continue
			}
			if styled {
				current.WriteString(style.Render(part))
			} else {
				current.WriteString(part)
			}
		}
	}
	lines = append(lines, current.String())

	// Tokenising adds a trailing newline of its own when the file ends with
	// one, which would put every line number out by one.
	if n := len(lines); n > 0 && lines[n-1] == "" && strings.HasSuffix(content, "\n") {
		lines = lines[:n-1]
	}
	return lines
}
