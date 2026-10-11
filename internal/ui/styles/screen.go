package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// screenChromeRows is the rows Screen spends around the body: the title line,
// the rule under it, the subtitle, the rule under that and the legend.
const screenChromeRows = 5

// screenVersionGap is the least space between the title and the version.
const screenVersionGap = 2

// ScreenConfig is a full screen that stands in the shell's place: the help
// and the configuration screen.
type ScreenConfig struct {
	// Title names the screen where the shell has its menu bar.
	Title string
	// Version is the build version, at the right edge of the title line.
	Version string
	// Subtitle says what the body is, on the line under the title.
	Subtitle string
	// Body is the lines between the second rule and the legend. Screen draws
	// the first ScreenBodyRows of them, each cut to Width.
	Body []string
	// Legend is the key legend on the last line. It arrives styled, from
	// KeyLegend at ScreenTextWidth.
	Legend string
	Width  int
	Height int
}

// ScreenBodyRows is the rows a screen of height has for its body.
func ScreenBodyRows(height int) int {
	return max(height-screenChromeRows, 0)
}

// ScreenTextWidth is the cells a line of text has on a screen of width: the
// title, the subtitle and the legend start one cell in.
func ScreenTextWidth(width int) int {
	return max(width-1, 0)
}

// Screen draws the chrome the full screens share: the bold title in the menu
// bar's place with the version flush right, the rule the shell has under its
// bar, the subtitle, a rule across the whole width, the body, and the legend
// on the last line. The title, the first rule, the subtitle and the legend
// start where a button's label starts on the bar, one cell in. The version is
// left off when the title leaves it no room. A body line wider than the screen
// ends at the last cell, with no mark.
func Screen(cfg ScreenConfig) string {
	text := ScreenTextWidth(cfg.Width)
	title := " " + lipgloss.NewStyle().Foreground(TextPrimaryColor).Bold(true).
		Render(textutil.TruncateString(cfg.Title, text))
	if gap := cfg.Width - lipgloss.Width(title) - lipgloss.Width(cfg.Version); gap >= screenVersionGap {
		title += strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(ShellFooterHelpColor).Render(cfg.Version)
	}

	lines := []string{
		title,
		InsetRule(cfg.Width),
		" " + lipgloss.NewStyle().Foreground(ScreenSubtitleColor).Render(textutil.TruncateString(cfg.Subtitle, text)),
		Rule(cfg.Width),
	}
	for row := range ScreenBodyRows(cfg.Height) {
		line := ""
		if row < len(cfg.Body) {
			line = ansi.Truncate(cfg.Body[row], cfg.Width, "")
		}
		lines = append(lines, line)
	}
	return strings.Join(append(lines, " "+textutil.TruncateString(cfg.Legend, text)), "\n")
}

// SectionHeading opens a section of a full screen: the title, bold, and a rule
// to width, so a section reads as a block and not as one more line.
func SectionHeading(title string, width int) string {
	title = textutil.TruncateString(title, width)
	heading := lipgloss.NewStyle().Foreground(SectionHeadingColor).Bold(true).Render(title)
	return heading + " " + Rule(width-lipgloss.Width(title)-1)
}
