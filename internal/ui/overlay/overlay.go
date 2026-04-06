// Package overlay provides ANSI-aware overlay placement helpers.
package overlay

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Position specifies where to place overlay content.
type Position int

const (
	// Center places foreground in the center.
	Center Position = iota
	// Top places foreground at the top center.
	Top
	// Bottom places foreground at the bottom center.
	Bottom
	// BottomLeft places foreground at the bottom-left.
	BottomLeft
)

// Config controls overlay placement.
type Config struct {
	Width    int
	Height   int
	Position Position
	PadX     int
	PadY     int
}

// Place renders foreground over background while preserving ANSI styles.
func Place(cfg Config, fg, bg string) string {
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	for len(bgLines) < cfg.Height {
		bgLines = append(bgLines, strings.Repeat(" ", cfg.Width))
	}

	fgHeight := len(fgLines)
	fgWidth := lipgloss.Width(fg)
	startX, startY := calculatePosition(cfg, fgWidth, fgHeight)

	for i, fgLine := range fgLines {
		bgY := startY + i
		if bgY >= len(bgLines) {
			break
		}

		bgLine := bgLines[bgY]
		fgLineWidth := ansi.StringWidth(fgLine)

		leftPart := ansi.Truncate(bgLine, startX, "")
		leftWidth := ansi.StringWidth(leftPart)
		if leftWidth < startX {
			leftPart += strings.Repeat(" ", startX-leftWidth)
		}

		endX := startX + fgLineWidth
		bgWidth := ansi.StringWidth(bgLine)
		rightPart := ""
		if endX < bgWidth {
			rightPart = ansi.TruncateLeft(bgLine, endX, "")
		}

		bgLines[bgY] = leftPart + fgLine + rightPart
	}

	return strings.Join(bgLines, "\n")
}

func calculatePosition(cfg Config, fgWidth, fgHeight int) (x, y int) {
	switch cfg.Position {
	case Top:
		x = (cfg.Width - fgWidth) / 2
		y = cfg.PadY
	case Bottom:
		x = (cfg.Width - fgWidth) / 2
		y = cfg.Height - fgHeight - cfg.PadY
	case BottomLeft:
		x = cfg.PadX
		y = cfg.Height - fgHeight - cfg.PadY
	default:
		x = (cfg.Width - fgWidth) / 2
		y = (cfg.Height - fgHeight) / 2
	}

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return x, y
}
