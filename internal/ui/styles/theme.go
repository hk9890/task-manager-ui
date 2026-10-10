package styles

import (
	"fmt"
	"sort"
	"strings"

	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/lipgloss"
)

// The palette and the glyph set are two choices, because a terminal program
// cannot ask which font is loaded. The package starts on these two, which is
// what a test draws with; a run draws with what the configuration names
// (config.Default), and Apply makes it so.
const (
	initialTheme  = "catppuccin-mocha"
	initialGlyphs = "unicode"
)

var flavors = map[string]catppuccin.Flavor{
	"catppuccin-mocha":     catppuccin.Mocha,
	"catppuccin-macchiato": catppuccin.Macchiato,
	"catppuccin-frappe":    catppuccin.Frappe,
	"catppuccin-latte":     catppuccin.Latte,
}

// lightThemes are the themes drawn on a light background.
var lightThemes = map[string]bool{"catppuccin-latte": true}

var darkTheme = true

func init() {
	if err := Apply(initialTheme, initialGlyphs); err != nil {
		panic("styles: defaults do not resolve: " + err.Error())
	}
}

// Themes and GlyphSets report the valid names, sorted, for an error message.
func Themes() []string    { return sortedKeys(flavors) }
func GlyphSets() []string { return sortedKeys(glyphSets) }

// Validate reports whether a theme and a glyph set name resolve. The error
// names what is valid, because a theme name is a guess until the operator sees
// the list.
func Validate(theme, glyphs string) error {
	if _, ok := flavors[theme]; !ok {
		return fmt.Errorf("unknown theme %q; valid: %s", theme, strings.Join(Themes(), ", "))
	}
	if _, ok := glyphSets[glyphs]; !ok {
		return fmt.Errorf("unknown glyph set %q; valid: %s", glyphs, strings.Join(GlyphSets(), ", "))
	}
	return nil
}

// Apply makes a theme and a glyph set the ones every surface draws with. It
// runs once, at startup, before the first render: the roles are package
// variables and nothing guards a change under a running program.
func Apply(theme, glyphs string) error {
	if err := Validate(theme, glyphs); err != nil {
		return err
	}
	applyFlavor(flavors[theme])
	darkTheme = !lightThemes[theme]
	Glyphs = glyphSets[glyphs]
	return nil
}

// Dark reports whether the applied theme draws on a dark background. Markdown
// takes its glamour style from it.
func Dark() bool { return darkTheme }

// applyFlavor maps a Catppuccin flavour onto the roles. It is the only place a
// colour is chosen.
func applyFlavor(f catppuccin.Flavor) {
	c := func(col catppuccin.Color) lipgloss.Color { return lipgloss.Color(col.Hex) }

	SkeletonShades = []lipgloss.Color{c(f.Surface1()), c(f.Surface2()), c(f.Overlay0())}

	TextPrimaryColor = c(f.Text())
	TextMutedColor = c(f.Overlay0())
	TextSecondaryColor = c(f.Subtext0())

	ShellTabActiveTextColor = c(f.Base())
	ShellTabActiveBgColor = c(f.Mauve())
	ShellTabInactiveColor = c(f.Overlay1())
	ShellTabHoverColor = c(f.Text())
	ShellFooterHelpColor = c(f.Overlay0())
	ShellRuleColor = c(f.Surface1())
	ShellActionColor = c(f.Subtext0())

	QueryAccentColor = c(f.Mauve())
	MatchTextColor = c(f.Yellow())

	// The two bands must stay two colours on a 256- and a 16-colour terminal,
	// where the surfaces of a dark flavour collapse into one palette entry. The
	// mantle is the nearest shade that does not.
	RowSelectedBgColor = c(f.Surface1())
	RowHoverBgColor = c(f.Mantle())

	BorderDefaultColor = c(f.Surface2())
	OverlayTitleColor = c(f.Text())
	OverlayBorderColor = c(f.Overlay1())
	BorderHighlightFocusColor = c(f.Mauve())

	ButtonTextColor = c(f.Base())
	ButtonPrimaryBgColor = c(f.Sapphire())
	ButtonPrimaryFocusBgColor = c(f.Blue())
	ButtonSecondaryBgColor = c(f.Overlay0())
	ButtonSecondaryFocusBgColor = c(f.Overlay2())
	ButtonDangerBgColor = c(f.Maroon())
	ButtonDangerFocusBgColor = c(f.Red())

	ToastBorderSuccessColor = c(f.Green())
	ToastBorderErrorColor = c(f.Red())
	ToastBorderInfoColor = c(f.Blue())
	ToastBorderWarnColor = c(f.Yellow())

	StoreActiveColor = c(f.Green())
	StoreDanglingColor = c(f.Peach())
	StoreBrokenColor = c(f.Red())

	IssueTypeBugColor = c(f.Red())
	IssueTypeTaskColor = c(f.Blue())
	IssueTypeFeatureColor = c(f.Green())
	IssueTypeEpicColor = c(f.Mauve())
	IssueTypeChoreColor = c(f.Overlay1())
	IssueTypeDocColor = c(f.Teal())

	IssuePriorityP0Color = c(f.Red())
	IssuePriorityP1Color = c(f.Peach())
	IssuePriorityP2Color = c(f.Yellow())
	IssuePriorityP3Color = c(f.Overlay1())

	IssueStatusOpenColor = c(f.Green())
	IssueStatusReadyColor = c(f.Teal())
	IssueStatusInProgressColor = c(f.Blue())
	IssueStatusBlockedColor = c(f.Red())
	IssueStatusClosedColor = c(f.Overlay1())
	IssueStatusDeferredColor = c(f.Lavender())

	buildStyles()
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
