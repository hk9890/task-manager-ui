// Package styles provides shared Lip Gloss colors and reusable style helpers.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Text colors.
	TextPrimaryColor = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#CCCCCC"}
	TextMutedColor   = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#696969"}

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

	baseButtonStyle = lipgloss.NewStyle().Padding(0, 2).Bold(true)

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
