package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// FormSectionConfig configures a bordered form section with auto-height.
type FormSectionConfig struct {
	Content []string
	Width   int
	Height  int

	TopLeft  string
	TopRight string
	// TopAction is a button in the top border after TopLeft, already styled.
	// It needs TopLeft and is left out when the border has no room.
	TopAction string

	Focused            bool
	FocusedBorderColor lipgloss.TerminalColor
}

// FormSection renders a rounded section box with optional titles.
// When cfg.Width < 6, returns the literal string "too narrow" with no styling.
func FormSection(cfg FormSectionConfig) string {
	if cfg.Width < 6 {
		return "too narrow"
	}

	var borderColor lipgloss.TerminalColor = BorderDefaultColor
	var titleColor lipgloss.TerminalColor = BorderDefaultColor
	if cfg.Focused && cfg.FocusedBorderColor != nil {
		borderColor = cfg.FocusedBorderColor
		titleColor = cfg.FocusedBorderColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	innerWidth := cfg.Width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	top := buildFormTopBorder(cfg, innerWidth, borderStyle, titleStyle)
	bottom := buildFormBottomBorder(innerWidth, borderStyle)

	content := cfg.Content
	if len(content) == 0 {
		content = []string{""}
	}
	if cfg.Height > 0 {
		innerHeight := cfg.Height - 2
		if innerHeight < 1 {
			innerHeight = 1
		}
		if len(content) > innerHeight {
			content = content[:innerHeight]
		}
		for len(content) < innerHeight {
			content = append(content, "")
		}
	}

	lines := make([]string, 0, len(content))
	for _, row := range content {
		lineWidth := lipgloss.Width(row)
		pad := ""
		if lineWidth < innerWidth {
			pad = strings.Repeat(" ", innerWidth-lineWidth)
		}
		lines = append(lines, borderStyle.Render(borderVertical)+row+pad+borderStyle.Render(borderVertical))
	}

	return top + "\n" + strings.Join(lines, "\n") + "\n" + bottom
}

// FormSectionActionCells is the columns TopAction covers, from x0 up to x1
// and not x1 itself, counted from the section's first cell. ok is false when
// FormSection does not draw the action.
func FormSectionActionCells(cfg FormSectionConfig) (x0, x1 int, ok bool) {
	if cfg.Width < 6 {
		return 0, 0, false
	}
	if _, drawn := topBorderDashes(cfg, cfg.Width-2); !drawn {
		return 0, 0, false
	}
	// The corner, a dash, a space, the title, a space and one dash.
	x0 = 5 + lipgloss.Width(cfg.TopLeft)
	return x0, x0 + lipgloss.Width(cfg.TopAction), true
}

// topBorderDashes is the run of dashes that fills the top border, and whether
// TopAction is drawn in front of it. The action takes its own cells and one
// dash from the run, and is left out when no dash would stay.
func topBorderDashes(cfg FormSectionConfig, innerWidth int) (dashes int, action bool) {
	leftWidth, rightWidth := lipgloss.Width(cfg.TopLeft), lipgloss.Width(cfg.TopRight)
	switch {
	case cfg.TopLeft != "" && cfg.TopRight != "":
		dashes = innerWidth - leftWidth - rightWidth - 6
	case cfg.TopLeft != "":
		dashes = innerWidth - leftWidth - 3
	default:
		dashes = innerWidth - rightWidth - 3
	}
	if cfg.TopLeft != "" && cfg.TopAction != "" {
		if rest := dashes - 1 - lipgloss.Width(cfg.TopAction); rest >= 1 {
			return rest, true
		}
	}
	return max(1, dashes), false
}

func buildFormTopBorder(cfg FormSectionConfig, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if cfg.TopLeft == "" && cfg.TopRight == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	dashes, action := topBorderDashes(cfg, innerWidth)

	var b strings.Builder
	b.WriteString(borderStyle.Render(borderTopLeft))
	if cfg.TopLeft != "" {
		b.WriteString(borderStyle.Render(borderHorizontal + " "))
		b.WriteString(titleStyle.Render(cfg.TopLeft))
		b.WriteString(borderStyle.Render(" "))
	}
	if action {
		b.WriteString(borderStyle.Render(borderHorizontal))
		b.WriteString(cfg.TopAction)
	}
	b.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, dashes)))
	if cfg.TopRight != "" {
		b.WriteString(borderStyle.Render(" "))
		b.WriteString(titleStyle.Render(cfg.TopRight))
		b.WriteString(borderStyle.Render(" " + borderHorizontal))
	}
	b.WriteString(borderStyle.Render(borderTopRight))

	return b.String()
}

func buildFormBottomBorder(innerWidth int, borderStyle lipgloss.Style) string {
	return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
}
