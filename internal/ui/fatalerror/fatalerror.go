// Package fatalerror provides a full-screen fatal error view for startup failures.
package fatalerror

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// State is the input to Render: the title/body to show and the size of the
// screen it is drawn on.
type State struct {
	Title  string
	Body   string
	Width  int
	Height int
}

// Render returns a centered full-screen error screen for the given State. It
// mirrors the stateless Render(State) entrypoint used by the other ui/* leaf
// renderers (board, search, details).
func Render(state State) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ToastBorderErrorColor)

	bodyStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	width, height := state.Width, state.Height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// The body is wrapped to the screen: a line wider than it runs through
	// the margin and off the terminal.
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(state.Title),
		"",
		bodyStyle.Render(wrapAtSpaces(state.Body, width)),
		"",
		hintStyle.Render("Press q or ctrl+c to quit."),
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// wrapAtSpaces wraps body to width at its spaces alone. textutil.WrapLines
// breaks after a hyphen too, which puts the two halves of a flag the body
// tells the operator to type on two lines. Only a word wider than width is
// still broken there.
func wrapAtSpaces(body string, width int) string {
	var lines []string
	for _, paragraph := range strings.Split(body, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
				line += " " + word
			default:
				lines = append(lines, textutil.WrapLines(line, width)...)
				line = word
			}
		}
		lines = append(lines, textutil.WrapLines(line, width)...)
	}
	return strings.Join(lines, "\n")
}
