package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// lipgloss.Style has unexported func fields, so == doesn't work — compare via
// rendered output for the same probe instead.
func sameStyle(a, b lipgloss.Style) bool {
	const probe = "X"
	return a.Render(probe) == b.Render(probe)
}

func forceTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestIssueTypeStyle(t *testing.T) {
	forceTrueColor(t)

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
	forceTrueColor(t)

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
	forceTrueColor(t)

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
	forceTrueColor(t)

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
	forceTrueColor(t)

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
