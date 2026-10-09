package renderhelpers_test

// CompactIssueType (glyph) and styles.IssueTypeStyle (colour) are two tables
// over the same issue-type token, extended at different times and in different
// packages. This test pins them together: a token either has both a distinct
// glyph and a distinct colour, or neither. The failure it prevents is silent —
// a row rendering a recognised glyph in the muted "unknown type" colour.
//
// Both pins run under every glyph set: a set says "unknown" with a glyph of its
// own, so the signal is the set's unknown glyph and not a fixed "?".

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// forEachGlyphSet runs check with each glyph set applied. Apply writes package
// variables, so neither the caller nor check is parallel.
func forEachGlyphSet(t *testing.T, check func(t *testing.T)) {
	t.Helper()
	t.Cleanup(func() {
		if err := styles.Apply("catppuccin-mocha", "unicode"); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})

	for _, set := range styles.GlyphSets() {
		if err := styles.Apply("catppuccin-mocha", set); err != nil {
			t.Fatalf("Apply(%q): %v", set, err)
		}
		t.Run(set, check)
	}
}

func TestIssueTypeGlyphAndStyleAgreeOnTokenSet(t *testing.T) {
	unknownStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Every task-manager SDK type, the "docs" alias, and tokens no backend
	// produces — so the test fails whichever table gains a token first.
	tokens := []string{
		"task", "bug", "feature", "epic", "chore", "doc",
		"docs", "DOC", " Doc ",
		"spike", "decision", "story", "milestone", "", "not-a-type",
	}

	forEachGlyphSet(t, func(t *testing.T) {
		unknownGlyph := renderhelpers.CompactIssueType("not-a-type")
		if unknownGlyph == "" {
			t.Fatal("the set draws nothing for an unknown issue type")
		}

		for _, token := range tokens {
			glyph, recognised := styles.Glyphs.IssueType(token)
			if got := renderhelpers.CompactIssueType(token); got != glyph {
				t.Errorf("token %q: CompactIssueType = %q, the set's glyph is %q", token, got, glyph)
			}

			hasGlyph := glyph != unknownGlyph
			hasStyle := styles.IssueTypeStyle(token).GetForeground() != unknownStyle.GetForeground()

			if hasGlyph != recognised {
				t.Errorf("token %q: glyph %q, recognised = %v — a recognised type must not draw the unknown glyph %q, and an unknown one must",
					token, glyph, recognised, unknownGlyph)
			}
			if hasGlyph != hasStyle {
				t.Errorf("token %q: distinct glyph = %v, distinct colour = %v — the two tables must support the same token set",
					token, hasGlyph, hasStyle)
			}
		}
	})
}

// TestIssueStatusGlyphAndStyleAgreeOnTokenSet is the same pin for statuses.
// It was missing, and "ready" had documented RDY/R glyphs with no entry in
// IssueStatusStyle, so the board's readiest rows rendered in the muted
// unknown-status colour.
func TestIssueStatusGlyphAndStyleAgreeOnTokenSet(t *testing.T) {
	unknownStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Every task-manager SDK status, the derived "ready" the board computes,
	// spelling variants, and tokens no backend produces.
	tokens := []string{
		"open", "in_progress", "blocked", "closed", "ready",
		"in-progress", "IN_PROGRESS", " Ready ", "RDY",
		"deferred", "archived", "", "not-a-status",
	}

	forEachGlyphSet(t, func(t *testing.T) {
		for _, token := range tokens {
			hasGlyph := explicitStatusGlyph(token)
			hasStyle := styles.IssueStatusStyle(token).GetForeground() != unknownStyle.GetForeground()

			if hasGlyph != hasStyle {
				t.Errorf("token %q: distinct glyph = %v, distinct colour = %v — the two tables must support the same token set",
					token, hasGlyph, hasStyle)
			}
			if narrow := renderhelpers.CompactIssueStateNarrow(token); lipgloss.Width(narrow) != 1 {
				t.Errorf("token %q: the dense-row status %q is not one cell", token, narrow)
			}
		}
	})
}

// explicitStatusGlyph reports whether CompactIssueState has a glyph of its own
// for the token. In a set that draws a status as an icon, that is any icon but
// the set's unknown one. In a set that spells it, it is a case rather than the
// default, which is the token's own first three characters upper-cased.
// Deriving the default here rather than listing the cases keeps the check
// honest when a case is added.
func explicitStatusGlyph(status string) bool {
	if unknownIcon, icons := styles.Glyphs.Status("not-a-status"); icons {
		return renderhelpers.CompactIssueState(status) != unknownIcon
	}

	token := renderhelpers.NormalizeToken(status)

	fallback := "---"
	if token != "" {
		fallback = strings.ToUpper(token)
		if runes := []rune(fallback); len(runes) > 3 {
			fallback = string(runes[:3])
		}
	}

	return renderhelpers.CompactIssueState(status) != fallback
}
