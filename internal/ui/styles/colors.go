// Package styles provides shared Lip Gloss colors and reusable style helpers.
package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SkeletonShades cycles through 3 muted lipgloss.AdaptiveColor values to
// produce a ~1.2 s breathing pulse on cold-start placeholders. Hand-tuned
// for both dark and light themes; the dark-theme dynamic range is wider by
// design (TextMutedColor on light themes sits close to typical backgrounds,
// so the light-theme pulse may be less perceptible than on dark themes —
// this is documented and expected; T4 verification should note if it is
// unusable on light terminals).
var SkeletonShades = []lipgloss.AdaptiveColor{
	{Light: "#C8CCC0", Dark: "#454545"}, // dim
	{Light: "#D9DCCF", Dark: "#696969"}, // mid (matches TextMutedColor dark)
	{Light: "#EAEDE0", Dark: "#7F7F7F"}, // bright
}

var (
	// Text colors.
	TextPrimaryColor   = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#CCCCCC"}
	TextMutedColor     = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#696969"}
	TextSecondaryColor = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#BBBBBB"}

	// Shell chrome colors.
	ShellTitleColor         = TextPrimaryColor
	ShellTabActiveTextColor = ButtonTextColor
	ShellTabActiveBgColor   = ButtonPrimaryFocusBgColor
	ShellTabInactiveColor   = TextMutedColor
	ShellContextColor       = TextMutedColor
	ShellFooterHelpColor    = TextMutedColor

	// Border and overlay colors.
	BorderDefaultColor        = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#696969"}
	OverlayTitleColor         = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#C9C9C9"}
	OverlayBorderColor        = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#8C8C8C"}
	BorderHighlightFocusColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}

	// Button colors.
	ButtonTextColor             = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
	ButtonPrimaryBgColor        = lipgloss.AdaptiveColor{Light: "#1A5276", Dark: "#1A5276"}
	ButtonPrimaryFocusBgColor   = lipgloss.AdaptiveColor{Light: "#3498DB", Dark: "#3498DB"}
	ButtonSecondaryBgColor      = lipgloss.AdaptiveColor{Light: "#2D3436", Dark: "#2D3436"}
	ButtonSecondaryFocusBgColor = lipgloss.AdaptiveColor{Light: "#636E72", Dark: "#636E72"}
	ButtonDangerBgColor         = lipgloss.AdaptiveColor{Light: "#922B21", Dark: "#922B21"}
	ButtonDangerFocusBgColor    = lipgloss.AdaptiveColor{Light: "#E74C3C", Dark: "#E74C3C"}

	// Toast colors.
	ToastBorderSuccessColor = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	ToastBorderErrorColor   = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	ToastBorderInfoColor    = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	ToastBorderWarnColor    = lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}

	// Compact board issue metadata colors.
	IssueTypeBugColor     = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	IssueTypeTaskColor    = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	IssueTypeFeatureColor = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	IssueTypeEpicColor    = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	IssueTypeChoreColor   = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"}

	IssuePriorityP0Color = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	IssuePriorityP1Color = lipgloss.AdaptiveColor{Light: "#FF9F43", Dark: "#FF9F43"}
	IssuePriorityP2Color = lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}
	IssuePriorityP3Color = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"}

	IssueStatusOpenColor       = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	IssueStatusInProgressColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	IssueStatusBlockedColor    = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	IssueStatusClosedColor     = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"}

	baseButtonStyle = lipgloss.NewStyle().Padding(0, 2).Bold(true)

	SelectionIndicatorStyle = lipgloss.NewStyle().Bold(true).Foreground(BorderHighlightFocusColor)
	IssueIDMutedStyle       = lipgloss.NewStyle().Foreground(TextSecondaryColor)

	IssuePriorityP0Style    = lipgloss.NewStyle().Foreground(IssuePriorityP0Color).Bold(true)
	IssuePriorityP1Style    = lipgloss.NewStyle().Foreground(IssuePriorityP1Color)
	IssuePriorityP2Style    = lipgloss.NewStyle().Foreground(IssuePriorityP2Color)
	IssuePriorityP3Style    = lipgloss.NewStyle().Foreground(IssuePriorityP3Color)
	IssueStatusOpenStyle    = lipgloss.NewStyle().Foreground(IssueStatusOpenColor)
	IssueStatusIPStyle      = lipgloss.NewStyle().Foreground(IssueStatusInProgressColor)
	IssueStatusBlockedStyle = lipgloss.NewStyle().Foreground(IssueStatusBlockedColor)
	IssueStatusClosedStyle  = lipgloss.NewStyle().Foreground(IssueStatusClosedColor)
	IssueTypeBugStyle       = lipgloss.NewStyle().Foreground(IssueTypeBugColor)
	IssueTypeTaskStyle      = lipgloss.NewStyle().Foreground(IssueTypeTaskColor)
	IssueTypeFeatureStyle   = lipgloss.NewStyle().Foreground(IssueTypeFeatureColor)
	IssueTypeEpicStyle      = lipgloss.NewStyle().Foreground(IssueTypeEpicColor)
	IssueTypeChoreStyle     = lipgloss.NewStyle().Foreground(IssueTypeChoreColor)

	PrimaryButtonStyle = baseButtonStyle.
				Foreground(ButtonTextColor).
				Background(ButtonPrimaryBgColor)

	PrimaryButtonFocusedStyle = baseButtonStyle.
					Foreground(ButtonTextColor).
					Background(ButtonPrimaryFocusBgColor).
					Underline(true).
					UnderlineSpaces(true)

	SecondaryButtonStyle = baseButtonStyle.
				Foreground(ButtonTextColor).
				Background(ButtonSecondaryBgColor)

	SecondaryButtonFocusedStyle = baseButtonStyle.
					Foreground(ButtonTextColor).
					Background(ButtonSecondaryFocusBgColor).
					Underline(true).
					UnderlineSpaces(true)

	DangerButtonStyle = baseButtonStyle.
				Foreground(ButtonTextColor).
				Background(ButtonDangerBgColor)

	DangerButtonFocusedStyle = baseButtonStyle.
					Foreground(ButtonTextColor).
					Background(ButtonDangerFocusBgColor).
					Underline(true).
					UnderlineSpaces(true)
)

// IssueTypeStyle returns the compact board style for an issue type token.
func IssueTypeStyle(issueType string) lipgloss.Style {
	switch normalizeIssueToken(issueType) {
	case "bug":
		return IssueTypeBugStyle
	case "task":
		return IssueTypeTaskStyle
	case "feature":
		return IssueTypeFeatureStyle
	case "epic":
		return IssueTypeEpicStyle
	case "chore":
		return IssueTypeChoreStyle
	default:
		return lipgloss.NewStyle().Foreground(TextMutedColor)
	}
}

// IssuePriorityStyle returns the compact board style for a priority token.
func IssuePriorityStyle(priority int) lipgloss.Style {
	switch {
	case priority <= 0:
		return IssuePriorityP0Style
	case priority == 1:
		return IssuePriorityP1Style
	case priority == 2:
		return IssuePriorityP2Style
	default:
		return IssuePriorityP3Style
	}
}

// IssueStatusStyle returns the compact board style for a status token.
func IssueStatusStyle(status string) lipgloss.Style {
	switch normalizeIssueToken(status) {
	case "open":
		return IssueStatusOpenStyle
	case "in_progress":
		return IssueStatusIPStyle
	case "blocked":
		return IssueStatusBlockedStyle
	case "closed":
		return IssueStatusClosedStyle
	default:
		return lipgloss.NewStyle().Foreground(TextMutedColor)
	}
}

func normalizeIssueToken(raw string) string {
	// Keep a local copy of renderhelpers.NormalizeToken to avoid a package cycle:
	// styles -> renderhelpers -> styles.
	tok := strings.TrimSpace(strings.ToLower(raw))
	tok = strings.ReplaceAll(tok, "-", "_")
	tok = strings.ReplaceAll(tok, " ", "_")
	return tok
}
