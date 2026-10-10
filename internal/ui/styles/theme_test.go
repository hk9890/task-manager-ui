package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// restoreInitialStyles puts back the theme and the glyph set the package starts
// on. The roles are package variables, so a test that calls Apply takes this
// and stays off t.Parallel.
func restoreInitialStyles(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := Apply(initialTheme, initialGlyphs); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})
}

func TestApplyAssignsEveryRoleInEveryTheme(t *testing.T) {
	restoreInitialStyles(t)

	if len(Themes()) != 4 {
		t.Fatalf("Themes() = %v, want the four Catppuccin flavours", Themes())
	}

	for _, theme := range Themes() {
		if err := Apply(theme, initialGlyphs); err != nil {
			t.Fatalf("Apply(%q): %v", theme, err)
		}
		if got, want := Dark(), theme != "catppuccin-latte"; got != want {
			t.Errorf("%s: Dark() = %v, want %v", theme, got, want)
		}

		roles := map[string]lipgloss.Color{
			"TextPrimaryColor":            TextPrimaryColor,
			"TextMutedColor":              TextMutedColor,
			"TextSecondaryColor":          TextSecondaryColor,
			"ShellTabActiveTextColor":     ShellTabActiveTextColor,
			"ShellTabActiveBgColor":       ShellTabActiveBgColor,
			"ShellTabInactiveColor":       ShellTabInactiveColor,
			"ShellTabHoverColor":          ShellTabHoverColor,
			"ShellContextColor":           ShellContextColor,
			"ShellFooterHelpColor":        ShellFooterHelpColor,
			"ShellRuleColor":              ShellRuleColor,
			"ShellActionColor":            ShellActionColor,
			"QueryAccentColor":            QueryAccentColor,
			"MatchTextColor":              MatchTextColor,
			"RowSelectedBgColor":          RowSelectedBgColor,
			"RowHoverBgColor":             RowHoverBgColor,
			"BorderDefaultColor":          BorderDefaultColor,
			"OverlayTitleColor":           OverlayTitleColor,
			"OverlayBorderColor":          OverlayBorderColor,
			"BorderHighlightFocusColor":   BorderHighlightFocusColor,
			"ButtonTextColor":             ButtonTextColor,
			"ButtonPrimaryBgColor":        ButtonPrimaryBgColor,
			"ButtonPrimaryFocusBgColor":   ButtonPrimaryFocusBgColor,
			"ButtonSecondaryBgColor":      ButtonSecondaryBgColor,
			"ButtonSecondaryFocusBgColor": ButtonSecondaryFocusBgColor,
			"ButtonDangerBgColor":         ButtonDangerBgColor,
			"ButtonDangerFocusBgColor":    ButtonDangerFocusBgColor,
			"ToastBorderSuccessColor":     ToastBorderSuccessColor,
			"ToastBorderErrorColor":       ToastBorderErrorColor,
			"ToastBorderInfoColor":        ToastBorderInfoColor,
			"ToastBorderWarnColor":        ToastBorderWarnColor,
			"StoreActiveColor":            StoreActiveColor,
			"StoreDanglingColor":          StoreDanglingColor,
			"StoreBrokenColor":            StoreBrokenColor,
			"IssueTypeBugColor":           IssueTypeBugColor,
			"IssueTypeTaskColor":          IssueTypeTaskColor,
			"IssueTypeFeatureColor":       IssueTypeFeatureColor,
			"IssueTypeEpicColor":          IssueTypeEpicColor,
			"IssueTypeChoreColor":         IssueTypeChoreColor,
			"IssueTypeDocColor":           IssueTypeDocColor,
			"IssuePriorityP0Color":        IssuePriorityP0Color,
			"IssuePriorityP1Color":        IssuePriorityP1Color,
			"IssuePriorityP2Color":        IssuePriorityP2Color,
			"IssuePriorityP3Color":        IssuePriorityP3Color,
			"IssueStatusOpenColor":        IssueStatusOpenColor,
			"IssueStatusReadyColor":       IssueStatusReadyColor,
			"IssueStatusInProgressColor":  IssueStatusInProgressColor,
			"IssueStatusBlockedColor":     IssueStatusBlockedColor,
			"IssueStatusClosedColor":      IssueStatusClosedColor,
			"IssueStatusDeferredColor":    IssueStatusDeferredColor,
		}
		for name, color := range roles {
			if !strings.HasPrefix(string(color), "#") {
				t.Errorf("%s: role %s = %q, want a hex colour", theme, name, color)
			}
		}

		if len(SkeletonShades) != 3 {
			t.Fatalf("%s: SkeletonShades has %d shades, want 3", theme, len(SkeletonShades))
		}
		for i, shade := range SkeletonShades {
			if !strings.HasPrefix(string(shade), "#") {
				t.Errorf("%s: SkeletonShades[%d] = %q, want a hex colour", theme, i, shade)
			}
		}
	}
}

// TestApplyRebuildsTheStylesWithTheRoles: a style built once from the roles
// would keep the first theme's colour after a second Apply.
func TestApplyRebuildsTheStylesWithTheRoles(t *testing.T) {
	restoreInitialStyles(t)

	if err := Apply("catppuccin-latte", initialGlyphs); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := IssueTypeBugStyle.GetForeground(); got != IssueTypeBugColor {
		t.Fatalf("IssueTypeBugStyle foreground = %v, want the applied role %v", got, IssueTypeBugColor)
	}
}

func TestValidateRejectsUnknownNamesAndListsTheValidOnes(t *testing.T) {
	t.Parallel()

	if err := Validate(initialTheme, initialGlyphs); err != nil {
		t.Fatalf("Validate rejected the initial names: %v", err)
	}

	cases := []struct {
		name, theme, glyphs string
		want                []string
	}{
		{name: "unknown theme", theme: "solarized", glyphs: "nerd", want: append([]string{`unknown theme "solarized"`}, Themes()...)},
		{name: "empty theme", theme: "", glyphs: "nerd", want: []string{`unknown theme ""`}},
		{name: "unknown glyph set", theme: initialTheme, glyphs: "emoji", want: append([]string{`unknown glyph set "emoji"`}, GlyphSets()...)},
	}
	for _, tc := range cases {
		err := Validate(tc.theme, tc.glyphs)
		if err == nil {
			t.Errorf("%s: Validate(%q, %q) = nil, want an error", tc.name, tc.theme, tc.glyphs)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not name %q", tc.name, err, want)
			}
		}
	}
}

// TestApplyLeavesTheStateAloneOnAnUnknownName: a rejected name must not leave
// the theme applied and the glyph set not.
func TestApplyLeavesTheStateAloneOnAnUnknownName(t *testing.T) {
	restoreInitialStyles(t)

	before, cursor := TextPrimaryColor, Glyphs.Cursor
	if err := Apply("catppuccin-latte", "emoji"); err == nil {
		t.Fatal("Apply accepted an unknown glyph set")
	}
	if TextPrimaryColor != before || Glyphs.Cursor != cursor || !Dark() {
		t.Fatalf("a rejected Apply changed the state: text %v, cursor %q, dark %v", TextPrimaryColor, Glyphs.Cursor, Dark())
	}
}
