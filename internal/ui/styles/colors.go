// Package styles provides shared Lip Gloss colors and reusable style helpers.
package styles

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/revier"
)

// SkeletonShades are the three shades a cold-start placeholder cycles through
// for a ~1.2 s breathing pulse.
var SkeletonShades []color.Color

// Every colour role. Apply assigns each from the theme's flavour: a colour is
// chosen once, in applyFlavor (theme.go), and nowhere else.
var (
	TextPrimaryColor            color.Color
	TextMutedColor              color.Color
	TextSecondaryColor          color.Color
	ShellTabActiveTextColor     color.Color
	ShellTabActiveBgColor       color.Color
	ShellTabInactiveColor       color.Color
	ShellTabHoverColor          color.Color
	ShellHoverBgColor           color.Color
	ShellFooterHelpColor        color.Color
	RowSelectedBgColor          color.Color
	RowHoverBgColor             color.Color
	BorderDefaultColor          color.Color
	OverlayTitleColor           color.Color
	OverlayBorderColor          color.Color
	BorderHighlightFocusColor   color.Color
	ButtonTextColor             color.Color
	ButtonPrimaryBgColor        color.Color
	ButtonPrimaryFocusBgColor   color.Color
	ButtonSecondaryBgColor      color.Color
	ButtonSecondaryFocusBgColor color.Color
	ButtonDangerBgColor         color.Color
	ButtonDangerFocusBgColor    color.Color
	ToastBorderSuccessColor     color.Color
	ToastBorderErrorColor       color.Color
	ToastBorderInfoColor        color.Color
	ToastBorderWarnColor        color.Color
	StoreActiveColor            color.Color
	StoreDanglingColor          color.Color
	StoreBrokenColor            color.Color
	IssueTypeBugColor           color.Color
	IssueTypeTaskColor          color.Color
	IssueTypeFeatureColor       color.Color
	IssueTypeEpicColor          color.Color
	IssueTypeChoreColor         color.Color
	IssueTypeDocColor           color.Color
	IssuePriorityP0Color        color.Color
	IssuePriorityP1Color        color.Color
	IssuePriorityP2Color        color.Color
	IssuePriorityP3Color        color.Color
	IssueStatusOpenColor        color.Color
	IssueStatusReadyColor       color.Color
	IssueStatusInProgressColor  color.Color
	IssueStatusBlockedColor     color.Color
	IssueStatusClosedColor      color.Color
	IssueStatusDeferredColor    color.Color
	// ShellRuleColor is the rule under the menu bar.
	ShellRuleColor color.Color
	// ShellActionColor is a menu-bar button's label.
	ShellActionColor color.Color
	// QueryAccentColor is the prompt and the cursor of a query line.
	QueryAccentColor color.Color
	// MatchTextColor is the text of a row that a query word matched.
	MatchTextColor color.Color
	// ScreenSubtitleColor is the line under the title of a full screen.
	ScreenSubtitleColor color.Color
	// SectionHeadingColor is the title of a section of a full screen.
	SectionHeadingColor color.Color
	// SettingLabelColor is the name of a setting, in front of its value.
	SettingLabelColor color.Color
	// ErrorTextColor is the text of a refused save on the issue editor.
	ErrorTextColor color.Color
	// AgentRunningColor, AgentIdleColor and AgentAttentionColor are the
	// live-agent mark of a revier agent that works, waits for a prompt, and
	// waits for the operator.
	AgentRunningColor   color.Color
	AgentIdleColor      color.Color
	AgentAttentionColor color.Color
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
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	button := func(bg color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(ButtonTextColor).Background(bg)
	}
	focused := func(bg color.Color) lipgloss.Style {
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

// AgentStatusStyle returns the style of the live-agent mark for a revier
// agent status. A status revier does not document draws as idle.
func AgentStatusStyle(status string) lipgloss.Style {
	switch status {
	case revier.StatusRunning:
		return lipgloss.NewStyle().Foreground(AgentRunningColor)
	case revier.StatusAttention:
		return lipgloss.NewStyle().Foreground(AgentAttentionColor)
	default:
		return lipgloss.NewStyle().Foreground(AgentIdleColor)
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
