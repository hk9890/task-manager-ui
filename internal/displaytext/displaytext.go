// Package displaytext makes a string the app did not write safe to draw: a
// directory name, a path, or an error text that embeds one. It imports only
// the standard library, so the packages that read such a string and the ones
// that draw it can both use it.
package displaytext

import (
	"strings"
	"unicode"
)

const byteOrderMark rune = 0xfeff

// OneLine returns s as one drawable line: each control character and each
// line or paragraph separator becomes a space, because a newline adds a line
// to the frame and an escape sequence runs in the terminal. Each bidirectional
// control is dropped, because it changes the order in which a terminal draws
// the cells beside it, and so is the byte order mark. Every other format
// character stays: a joiner shapes an emoji sequence or a script.
func OneLine(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), unicode.In(r, unicode.Zl, unicode.Zp):
			return ' '
		case unicode.Is(unicode.Bidi_Control, r), r == byteOrderMark:
			return -1
		}
		return r
	}, s)
}

// Lines is OneLine for a text whose line feeds the app wrote itself: each line
// is cleaned, and the line feeds stay.
func Lines(s string) string {
	lines := strings.Split(s, "\n")
	for idx, line := range lines {
		lines[idx] = OneLine(line)
	}
	return strings.Join(lines, "\n")
}

// Visible reports whether s draws at least one cell that is not blank. White
// space and a format character draw none of their own, nor does a combining
// mark or a variation selector with no character before it, and a filler
// draws a blank one.
func Visible(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return !unicode.IsSpace(r) &&
			!unicode.In(r, unicode.Cf, unicode.Mn, unicode.Me, unicode.Other_Default_Ignorable_Code_Point)
	})
}
