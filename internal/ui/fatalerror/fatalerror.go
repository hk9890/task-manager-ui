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
		bodyStyle.Render(strings.Join(textutil.WrapLines(state.Body, width), "\n")),
		"",
		hintStyle.Render("Press q or ctrl+c to quit."),
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
