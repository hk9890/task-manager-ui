// Package styles provides shared Lip Gloss colors and reusable style helpers.
package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SkeletonShades are the three shades a cold-start placeholder cycles through
// for a ~1.2 s breathing pulse.
var SkeletonShades []lipgloss.Color

// Every colour role. Apply assigns each from the theme's flavour: a colour is
// chosen once, in applyFlavor (theme.go), and nowhere else.
var (
	TextPrimaryColor            lipgloss.Color
	TextMutedColor              lipgloss.Color
	TextSecondaryColor          lipgloss.Color
	ShellTabActiveTextColor     lipgloss.Color
	ShellTabActiveBgColor       lipgloss.Color
	ShellTabInactiveColor       lipgloss.Color
	ShellTabHoverColor          lipgloss.Color
	ShellFooterHelpColor        lipgloss.Color
	RowSelectedBgColor          lipgloss.Color
	RowHoverBgColor             lipgloss.Color
	BorderDefaultColor          lipgloss.Color
	OverlayTitleColor           lipgloss.Color
	OverlayBorderColor          lipgloss.Color
	BorderHighlightFocusColor   lipgloss.Color
	ButtonTextColor             lipgloss.Color
	ButtonPrimaryBgColor        lipgloss.Color
	ButtonPrimaryFocusBgColor   lipgloss.Color
	ButtonSecondaryBgColor      lipgloss.Color
	ButtonSecondaryFocusBgColor lipgloss.Color
	ButtonDangerBgColor         lipgloss.Color
	ButtonDangerFocusBgColor    lipgloss.Color
	ToastBorderSuccessColor     lipgloss.Color
	ToastBorderErrorColor       lipgloss.Color
	ToastBorderInfoColor        lipgloss.Color
	ToastBorderWarnColor        lipgloss.Color
	StoreActiveColor            lipgloss.Color
	StoreDanglingColor          lipgloss.Color
	StoreBrokenColor            lipgloss.Color
	IssueTypeBugColor           lipgloss.Color
	IssueTypeTaskColor          lipgloss.Color
	IssueTypeFeatureColor       lipgloss.Color
	IssueTypeEpicColor          lipgloss.Color
	IssueTypeChoreColor         lipgloss.Color
	IssueTypeDocColor           lipgloss.Color
	IssuePriorityP0Color        lipgloss.Color
	IssuePriorityP1Color        lipgloss.Color
	IssuePriorityP2Color        lipgloss.Color
	IssuePriorityP3Color        lipgloss.Color
	IssueStatusOpenColor        lipgloss.Color
	IssueStatusReadyColor       lipgloss.Color
	IssueStatusInProgressColor  lipgloss.Color
	IssueStatusBlockedColor     lipgloss.Color
	IssueStatusClosedColor      lipgloss.Color
	IssueStatusDeferredColor    lipgloss.Color
	// ShellRuleColor is the rule under the menu bar.
	ShellRuleColor lipgloss.Color
	// ShellActionColor is a menu-bar button's label.
	ShellActionColor lipgloss.Color
	// QueryAccentColor is the prompt and the cursor of a query line.
	QueryAccentColor lipgloss.Color
	// MatchTextColor is the text of a row that a query word matched.
	MatchTextColor lipgloss.Color
	// SectionHeadingColor is the title of a section of the configuration screen.
	SectionHeadingColor lipgloss.Color
	// SettingLabelColor is the name of a setting, in front of its value.
	SettingLabelColor lipgloss.Color
)

// The styles built from the roles. Apply rebuilds them with the roles.
var (
	SelectionIndicatorStyle     lipgloss.Style
	IssueIDMutedStyle           lipgloss.Style
	MatchTextStyle              lipgloss.Style
	IssuePriorityP0Style        lipgloss.Style
	IssuePriorityP1Style        lipgloss.Style
	IssuePriorityP2Style        lipgloss.Style
	IssuePriorityP3Style        lipgloss.Style
	IssueStatusOpenStyle        lipgloss.Style
	IssueStatusReadyStyle       lipgloss.Style
	IssueStatusIPStyle          lipgloss.Style
	IssueStatusBlockedStyle     lipgloss.Style
	IssueStatusClosedStyle      lipgloss.Style
	IssueStatusDeferredStyle    lipgloss.Style
	IssueTypeBugStyle           lipgloss.Style
	IssueTypeTaskStyle          lipgloss.Style
	IssueTypeFeatureStyle       lipgloss.Style
	IssueTypeEpicStyle          lipgloss.Style
	IssueTypeChoreStyle         lipgloss.Style
	IssueTypeDocStyle           lipgloss.Style
	PrimaryButtonStyle          lipgloss.Style
	PrimaryButtonFocusedStyle   lipgloss.Style
	SecondaryButtonStyle        lipgloss.Style
	SecondaryButtonFocusedStyle lipgloss.Style
	DangerButtonStyle           lipgloss.Style
	DangerButtonFocusedStyle    lipgloss.Style
)

func buildStyles() {
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	button := func(bg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(ButtonTextColor).Background(bg)
	}
	focused := func(bg lipgloss.Color) lipgloss.Style {
		return button(bg).Underline(true).UnderlineSpaces(true)
	}

	SelectionIndicatorStyle = fg(BorderHighlightFocusColor).Bold(true)
	IssueIDMutedStyle = fg(TextSecondaryColor)
	MatchTextStyle = fg(MatchTextColor).Bold(true)

	IssuePriorityP0Style = fg(IssuePriorityP0Color).Bold(true)
	IssuePriorityP1Style = fg(IssuePriorityP1Color)
	IssuePriorityP2Style = fg(IssuePriorityP2Color)
	IssuePriorityP3Style = fg(IssuePriorityP3Color)
	IssueStatusOpenStyle = fg(IssueStatusOpenColor)
	IssueStatusReadyStyle = fg(IssueStatusReadyColor)
	IssueStatusIPStyle = fg(IssueStatusInProgressColor)
	IssueStatusBlockedStyle = fg(IssueStatusBlockedColor)
	IssueStatusClosedStyle = fg(IssueStatusClosedColor)
	IssueStatusDeferredStyle = fg(IssueStatusDeferredColor)
	IssueTypeBugStyle = fg(IssueTypeBugColor)
	IssueTypeTaskStyle = fg(IssueTypeTaskColor)
	IssueTypeFeatureStyle = fg(IssueTypeFeatureColor)
	IssueTypeEpicStyle = fg(IssueTypeEpicColor)
	IssueTypeChoreStyle = fg(IssueTypeChoreColor)
	IssueTypeDocStyle = fg(IssueTypeDocColor)

	PrimaryButtonStyle = button(ButtonPrimaryBgColor)
	PrimaryButtonFocusedStyle = focused(ButtonPrimaryFocusBgColor)
	SecondaryButtonStyle = button(ButtonSecondaryBgColor)
	SecondaryButtonFocusedStyle = focused(ButtonSecondaryFocusBgColor)
	DangerButtonStyle = button(ButtonDangerBgColor)
	DangerButtonFocusedStyle = focused(ButtonDangerFocusBgColor)
}

// IssueTypeStyle returns the compact board style for an issue type token. The
// token set must match renderhelpers.CompactIssueType; see the parity test in
// internal/ui/shared/renderhelpers.
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
	case "doc", "docs":
		return IssueTypeDocStyle
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
	case "ready":
		return IssueStatusReadyStyle
	case "in_progress":
		return IssueStatusIPStyle
	case "blocked":
		return IssueStatusBlockedStyle
	case "closed":
		return IssueStatusClosedStyle
	case "deferred":
		return IssueStatusDeferredStyle
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
