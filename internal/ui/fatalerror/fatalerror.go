// Package fatalerror provides a full-screen fatal error view for startup failures.
package fatalerror

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

// View renders a centered full-screen error screen with the given title and body.
func View(title, body string, width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ToastBorderErrorColor)

	bodyStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		"",
		bodyStyle.Render(body),
		"",
		hintStyle.Render("Press q or ctrl+c to quit."),
	)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
