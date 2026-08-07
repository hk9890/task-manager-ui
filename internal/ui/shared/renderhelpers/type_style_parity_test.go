package renderhelpers_test

// CompactIssueType (glyph) and styles.IssueTypeStyle (colour) are two switches
// over the same issue-type token, extended at different times and in different
// packages. This test pins them together: a token either has both a distinct
// glyph and a distinct colour, or neither. The failure it prevents is silent —
// a row rendering a recognised glyph in the muted "unknown type" colour.

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func TestIssueTypeGlyphAndStyleAgreeOnTokenSet(t *testing.T) {
	t.Parallel()

	unknownStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Every task-manager SDK type, the "docs" alias, and tokens no backend
	// produces — so the test fails whichever table gains a token first.
	tokens := []string{
		"task", "bug", "feature", "epic", "chore", "doc",
		"docs", "DOC", " Doc ",
		"spike", "decision", "story", "milestone", "", "not-a-type",
	}

	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			t.Parallel()

			hasGlyph := renderhelpers.CompactIssueType(token) != "?"
			hasStyle := styles.IssueTypeStyle(token).GetForeground() != unknownStyle.GetForeground()

			if hasGlyph != hasStyle {
				t.Errorf("token %q: distinct glyph = %v, distinct colour = %v — the two tables must support the same token set",
					token, hasGlyph, hasStyle)
			}
		})
	}
}
