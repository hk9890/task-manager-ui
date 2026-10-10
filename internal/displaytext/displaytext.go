// Package displaytext makes a string the app did not write safe to draw: a
// directory name, a path, or an error text that embeds one. It imports only
// the standard library, so the packages that read such a string and the ones
// that draw it can both use it.
package displaytext

import (
	"strings"
	"unicode"
)

// reordering holds the format characters that change the order in which a
// terminal draws the cells beside them, and the byte order mark. Every other
// format character stays: a joiner shapes an emoji sequence or a script.
var reordering = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x061c, Hi: 0x061c, Stride: 1},
		{Lo: 0x200e, Hi: 0x200f, Stride: 1},
		{Lo: 0x202a, Hi: 0x202e, Stride: 1},
		{Lo: 0x2066, Hi: 0x2069, Stride: 1},
		{Lo: 0xfeff, Hi: 0xfeff, Stride: 1},
	},
}

// OneLine returns s as one drawable line: each control character and each
// line or paragraph separator becomes a space, because a newline adds a line
// to the frame and an escape sequence runs in the terminal, and each
// reordering character is dropped.
func OneLine(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), unicode.In(r, unicode.Zl, unicode.Zp):
			return ' '
		case unicode.Is(reordering, r):
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

// Visible reports whether s draws at least one cell that is not blank.
func Visible(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r)
	})
}
