package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// sameStyle returns true when the two styles produce identical output for a
// probe string. lipgloss.Style contains unexported func fields and is not
// directly comparable with ==, so we compare rendered output instead.
func sameStyle(a, b lipgloss.Style) bool {
	const probe = "X"
	return a.Render(probe) == b.Render(probe)
}

func TestIssueTypeStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	tests := []struct {
		input string
		want  lipgloss.Style
	}{
		{"bug", IssueTypeBugStyle},
		{"Bug", IssueTypeBugStyle},
		{" bug ", IssueTypeBugStyle},
		{"task", IssueTypeTaskStyle},
		{"Task", IssueTypeTaskStyle},
		{"feature", IssueTypeFeatureStyle},
		{"Feature", IssueTypeFeatureStyle},
		{"epic", IssueTypeEpicStyle},
		{"EPIC", IssueTypeEpicStyle},
		{"chore", IssueTypeChoreStyle},
		{"Chore", IssueTypeChoreStyle},
	}

	for _, tc := range tests {
		got := IssueTypeStyle(tc.input)
		if !sameStyle(got, tc.want) {
			t.Errorf("IssueTypeStyle(%q) rendered differently than expected constant", tc.input)
		}
	}
}

func TestIssueTypeStyleDefaultBranch(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	got := IssueTypeStyle("unknown-type")

	// Default must not match any of the named type styles.
	for _, named := range []struct {
		name  string
		style lipgloss.Style
	}{
		{"bug", IssueTypeBugStyle},
		{"task", IssueTypeTaskStyle},
		{"feature", IssueTypeFeatureStyle},
		{"epic", IssueTypeEpicStyle},
		{"chore", IssueTypeChoreStyle},
	} {
		if sameStyle(got, named.style) {
			t.Fatalf("IssueTypeStyle(\"unknown-type\") unexpectedly matched %s style", named.name)
		}
	}

	// Default style should contain some ANSI (muted color).
	rendered := got.Render("X")
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("IssueTypeStyle(\"unknown-type\") default returned unstyled output: %q", rendered)
	}
}

func TestIssuePriorityStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	tests := []struct {
		priority int
		want     lipgloss.Style
	}{
		{-1, IssuePriorityP0Style},
		{0, IssuePriorityP0Style},
		{1, IssuePriorityP1Style},
		{2, IssuePriorityP2Style},
		{3, IssuePriorityP3Style},
		{4, IssuePriorityP3Style},
		{99, IssuePriorityP3Style},
	}

	for _, tc := range tests {
		got := IssuePriorityStyle(tc.priority)
		if !sameStyle(got, tc.want) {
			t.Errorf("IssuePriorityStyle(%d) rendered differently than expected constant", tc.priority)
		}
	}
}

func TestIssueStatusStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	tests := []struct {
		input string
		want  lipgloss.Style
	}{
		{"open", IssueStatusOpenStyle},
		{"Open", IssueStatusOpenStyle},
		{" open ", IssueStatusOpenStyle},
		{"in_progress", IssueStatusIPStyle},
		{"in-progress", IssueStatusIPStyle},
		{"in progress", IssueStatusIPStyle},
		{"IN_PROGRESS", IssueStatusIPStyle},
		{"blocked", IssueStatusBlockedStyle},
		{"Blocked", IssueStatusBlockedStyle},
		{"closed", IssueStatusClosedStyle},
		{"CLOSED", IssueStatusClosedStyle},
	}

	for _, tc := range tests {
		got := IssueStatusStyle(tc.input)
		if !sameStyle(got, tc.want) {
			t.Errorf("IssueStatusStyle(%q) rendered differently than expected constant", tc.input)
		}
	}
}

func TestIssueStatusStyleDefaultBranch(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	got := IssueStatusStyle("unknown-status")

	for _, named := range []struct {
		name  string
		style lipgloss.Style
	}{
		{"open", IssueStatusOpenStyle},
		{"in_progress", IssueStatusIPStyle},
		{"blocked", IssueStatusBlockedStyle},
		{"closed", IssueStatusClosedStyle},
	} {
		if sameStyle(got, named.style) {
			t.Fatalf("IssueStatusStyle(\"unknown-status\") unexpectedly matched %s style", named.name)
		}
	}

	// Default style should contain some ANSI (muted color).
	rendered := got.Render("X")
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("IssueStatusStyle(\"unknown-status\") default returned unstyled output: %q", rendered)
	}
}

func TestNormalizeIssueToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"bug", "bug"},
		{"Bug", "bug"},
		{" Bug ", "bug"},
		{"in-progress", "in_progress"},
		{"in progress", "in_progress"},
		{"IN_PROGRESS", "in_progress"},
		{"  IN-PROGRESS  ", "in_progress"},
		{"", ""},
		{"  ", ""},
	}

	for _, tc := range tests {
		got := normalizeIssueToken(tc.input)
		if got != tc.want {
			t.Errorf("normalizeIssueToken(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
