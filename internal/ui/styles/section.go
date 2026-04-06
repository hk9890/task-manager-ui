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

	TopLeft     string
	TopLeftHint string
	TopRight    string
	BottomLeft  string
	BottomRight string

	Focused            bool
	FocusedBorderColor lipgloss.TerminalColor
}

// FormSection renders a rounded section box with optional titles.
func FormSection(cfg FormSectionConfig) string {
	var borderColor lipgloss.TerminalColor = BorderDefaultColor
	var titleColor lipgloss.TerminalColor = BorderDefaultColor
	if cfg.Focused && cfg.FocusedBorderColor != nil {
		borderColor = cfg.FocusedBorderColor
		titleColor = cfg.FocusedBorderColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	hintStyle := lipgloss.NewStyle().Foreground(TextMutedColor)

	innerWidth := cfg.Width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	top := buildFormTopBorder(cfg.TopLeft, cfg.TopLeftHint, cfg.TopRight, innerWidth, borderStyle, titleStyle, hintStyle)
	bottom := buildFormBottomBorder(cfg.BottomLeft, cfg.BottomRight, innerWidth, borderStyle, titleStyle)

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

func buildFormTopBorder(title, hint, rightTitle string, innerWidth int, borderStyle, titleStyle, hintStyle lipgloss.Style) string {
	if title == "" && rightTitle == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	leftPart := ""
	leftWidth := 0
	if title != "" {
		leftPart = titleStyle.Render(title)
		leftWidth = lipgloss.Width(title)
		if hint != "" {
			h := "(" + hint + ")"
			leftPart += " " + hintStyle.Render(h)
			leftWidth += 1 + lipgloss.Width(h)
		}
	}

	rightPart := ""
	rightWidth := 0
	if rightTitle != "" {
		rightPart = titleStyle.Render(rightTitle)
		rightWidth = lipgloss.Width(rightTitle)
	}

	dashes := 1
	switch {
	case title != "" && rightTitle != "":
		dashes = innerWidth - leftWidth - rightWidth - 6
	case title != "":
		dashes = innerWidth - leftWidth - 3
	default:
		dashes = innerWidth - rightWidth - 3
	}
	if dashes < 1 {
		dashes = 1
	}

	var b strings.Builder
	b.WriteString(borderStyle.Render(borderTopLeft))
	if title != "" {
		b.WriteString(borderStyle.Render(borderHorizontal + " "))
		b.WriteString(leftPart)
		b.WriteString(borderStyle.Render(" "))
	}
	b.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, dashes)))
	if rightTitle != "" {
		b.WriteString(borderStyle.Render(" "))
		b.WriteString(rightPart)
		b.WriteString(borderStyle.Render(" " + borderHorizontal))
	}
	b.WriteString(borderStyle.Render(borderTopRight))

	return b.String()
}

func buildFormBottomBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	leftWidth := lipgloss.Width(leftTitle)
	rightWidth := lipgloss.Width(rightTitle)

	dashes := 1
	switch {
	case leftTitle != "" && rightTitle != "":
		dashes = innerWidth - leftWidth - rightWidth - 6
	case leftTitle != "":
		dashes = innerWidth - leftWidth - 3
	default:
		dashes = innerWidth - rightWidth - 3
	}
	if dashes < 1 {
		dashes = 1
	}

	var b strings.Builder
	b.WriteString(borderStyle.Render(borderBottomLeft))
	if leftTitle != "" {
		b.WriteString(borderStyle.Render(borderHorizontal + " "))
		b.WriteString(leftTitle)
		b.WriteString(borderStyle.Render(" "))
	}
	b.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, dashes)))
	if rightTitle != "" {
		b.WriteString(borderStyle.Render(" "))
		b.WriteString(titleStyle.Render(rightTitle))
		b.WriteString(borderStyle.Render(" " + borderHorizontal))
	}
	b.WriteString(borderStyle.Render(borderBottomRight))

	return b.String()
}
