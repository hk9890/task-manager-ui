package renderhelpers

import (
	"strings"
	"testing"
)

func TestNormalizeToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "uppercase", input: "IN_PROGRESS", want: "in_progress"},
		{name: "lowercase", input: "ready", want: "ready"},
		{name: "mixed with spaces", input: " In Progress ", want: "in_progress"},
		{name: "hyphen separator", input: "In-Progress", want: "in_progress"},
		{name: "empty", input: "", want: ""},
		{name: "special chars unchanged", input: "feat@v2", want: "feat@v2"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizeToken(tc.input)
			if got != tc.want {
				t.Fatalf("NormalizeToken(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompactIssueType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "bug", input: "BUG", want: "B"},
		{name: "task", input: "task", want: "T"},
		{name: "feature", input: "Feature", want: "F"},
		{name: "unknown", input: "something_else", want: "?"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CompactIssueType(tc.input); got != tc.want {
				t.Fatalf("CompactIssueType(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompactPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		priority int
		want     string
	}{
		{name: "negative clamps to zero", priority: -1, want: "P0"},
		{name: "zero", priority: 0, want: "P0"},
		{name: "normal", priority: 3, want: "P3"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CompactPriority(tc.priority); got != tc.want {
				t.Fatalf("CompactPriority(%d) = %q, want %q", tc.priority, got, tc.want)
			}
		})
	}
}

func TestCompactIssueState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "blocked", status: "blocked", want: "BLK"},
		{name: "in progress", status: "in-progress", want: "IP"},
		{name: "open", status: "open", want: "OPN"},
		{name: "closed", status: "closed", want: "CLS"},
		{name: "ready", status: "ready", want: "RDY"},
		{name: "blank", status: "   ", want: "---"},
		{name: "unknown truncated", status: "something", want: "SOM"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CompactIssueState(tc.status); got != tc.want {
				t.Fatalf("CompactIssueState(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestCompactIssueStateNarrow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "blocked", status: "blocked", want: "B"},
		{name: "in progress", status: "in-progress", want: "I"},
		{name: "open", status: "open", want: "O"},
		{name: "closed", status: "closed", want: "C"},
		{name: "ready", status: "ready", want: "R"},
		{name: "blank", status: "   ", want: "-"},
		{name: "unknown first rune", status: "something", want: "S"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CompactIssueStateNarrow(tc.status); got != tc.want {
				t.Fatalf("CompactIssueStateNarrow(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestCompactIssueID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       string
		maxWidth int
		want     string
	}{
		{name: "fits as is", id: "bw-1", maxWidth: 10, want: "bw-1"},
		{name: "repo prefix dropped", id: "beads-workbench-9uk", maxWidth: 8, want: "9uk"},
		{name: "suffix ellipsis", id: "very-long-id", maxWidth: 5, want: "…g-id"},
		{name: "tiny width truncates", id: "abcdef", maxWidth: 1, want: "."},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CompactIssueID(tc.id, tc.maxWidth); got != tc.want {
				t.Fatalf("CompactIssueID(%q, %d) = %q, want %q", tc.id, tc.maxWidth, got, tc.want)
			}
		})
	}
}

func TestStyledHelpersContainUnderlyingToken(t *testing.T) {
	t.Parallel()

	if got := CompactIssueTypeStyled("bug"); !strings.Contains(got, "B") {
		t.Fatalf("CompactIssueTypeStyled should contain token B, got %q", got)
	}

	if got := CompactPriorityStyled(2); !strings.Contains(got, "P2") {
		t.Fatalf("CompactPriorityStyled should contain token P2, got %q", got)
	}

	if got := CompactIssueStateStyled("ready"); !strings.Contains(got, "RDY") {
		t.Fatalf("CompactIssueStateStyled should contain token RDY, got %q", got)
	}

	if got := CompactIssueStateNarrowStyled("in_progress"); !strings.Contains(got, "I") {
		t.Fatalf("CompactIssueStateNarrowStyled should contain token I, got %q", got)
	}

	if got := CompactIssueIDMuted("beads-workbench-9uk", 8); !strings.Contains(got, "9uk") {
		t.Fatalf("CompactIssueIDMuted should contain compact id, got %q", got)
	}
}
