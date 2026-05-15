// Package fatalerror provides a full-screen fatal error view for startup failures.
package fatalerror

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const installURL = "https://github.com/hk9890/beads-workbench"

// View renders a centered full-screen error screen. Call this when a fatal
// startup error prevents normal app operation.
func View(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ToastBorderErrorColor)

	bodyStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	title := titleStyle.Render("beads is not available")

	body := bodyStyle.Render("The bd CLI tool was not found in your PATH.\n\nInstall beads to use this app.\nSee " + installURL + " for setup instructions.")

	hint := hintStyle.Render("Press q or ctrl+c to quit.")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
