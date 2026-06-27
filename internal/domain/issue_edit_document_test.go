package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIssueEditDocumentRenderParseRoundTrip(t *testing.T) {
	t.Parallel()

	issue := IssueDetail{
		Summary: IssueSummary{
			ID:        "tm-12",
			Title:     "Refine editor flow",
			Status:    "in_progress",
			Type:      "task",
			Priority:  2,
			Assignee:  "hans",
			Labels:    []string{"ui", "editor"},
			CreatedAt: time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 5, 9, 30, 0, 0, time.UTC),
		},
		Description: "Current body\nwith two lines",
		Notes:       "Internal notes",
		BlockedBy:   []IssueReference{{ID: "tm-1", Title: "Predecessor"}},
		Comments:    []IssueComment{{ID: "c1", Body: "check this"}},
	}

	rendered := RenderIssueEditDocument(issue)
	if !strings.Contains(rendered, "# Task Manager UI Issue Edit") {
		t.Fatalf("expected issue edit heading, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## Read-Only Context (ignored on save)") {
		t.Fatalf("expected read-only context section, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, issueEditMarkerEditableEnd) {
		t.Fatalf("expected editable end marker before read-only context, got:\n%s", rendered)
	}

	expectedReadOnly := []string{
		fmt.Sprintf("- ID: %s", issue.Summary.ID),
		fmt.Sprintf("- Created At: %s", issueEditFormatTime(issue.Summary.CreatedAt)),
		fmt.Sprintf("- Updated At: %s", issueEditFormatTime(issue.Summary.UpdatedAt)),
		fmt.Sprintf("- Notes: %s", issueEditInline(issue.Notes)),
		fmt.Sprintf("- Blocked By: %s", issueEditRefsInline(issue.BlockedBy)),
		fmt.Sprintf("- Blocks: %s", issueEditRefsInline(issue.Blocks)),
		fmt.Sprintf("- Related: %s", issueEditRefsInline(issue.Related)),
		fmt.Sprintf("- Comments: %d", len(issue.Comments)),
	}
	for _, line := range expectedReadOnly {
		if !strings.Contains(rendered, line) {
			t.Fatalf("expected read-only context line %q, got:\n%s", line, rendered)
		}
	}

	parsed, err := ParseIssueEditDocument(rendered)
	if err != nil {
		t.Fatalf("ParseIssueEditDocument returned error: %v", err)
	}

	if parsed.Title != issue.Summary.Title {
		t.Fatalf("expected title %q, got %q", issue.Summary.Title, parsed.Title)
	}
	if parsed.Description != issue.Description {
		t.Fatalf("expected description %q, got %q", issue.Description, parsed.Description)
	}
	if parsed.Status != issue.Summary.Status || parsed.Type != issue.Summary.Type {
		t.Fatalf("expected status/type %q/%q, got %q/%q", issue.Summary.Status, issue.Summary.Type, parsed.Status, parsed.Type)
	}
	if parsed.Priority != issue.Summary.Priority {
		t.Fatalf("expected priority %d, got %d", issue.Summary.Priority, parsed.Priority)
	}
	if parsed.Assignee != issue.Summary.Assignee {
		t.Fatalf("expected assignee %q, got %q", issue.Summary.Assignee, parsed.Assignee)
	}
	if len(parsed.Labels) != 2 || parsed.Labels[0] != "ui" || parsed.Labels[1] != "editor" {
		t.Fatalf("unexpected labels: %#v", parsed.Labels)
	}
}

func TestParseIssueEditDocumentRejectsMissingMarkers(t *testing.T) {
	t.Parallel()

	_, err := ParseIssueEditDocument("# invalid")
	if err == nil {
		t.Fatalf("expected parse error for missing markers")
	}
}

func TestBuildIssueUpdateInputDetectsChanges(t *testing.T) {
	t.Parallel()

	original := IssueDetail{Summary: IssueSummary{
		Title:    "Old",
		Status:   "open",
		Type:     "task",
		Priority: 2,
		Assignee: "hans",
		Labels:   []string{"one", "two"},
	}, Description: "old body"}

	edited := IssueEditDocument{
		Title:       "New",
		Description: "new body",
		Status:      "in_progress",
		Type:        "bug",
		Priority:    1,
		Assignee:    "alice",
		Labels:      []string{"triage"},
	}

	input, changed := BuildIssueUpdateInput(original, edited)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if input.Title == nil || *input.Title != "New" {
		t.Fatalf("expected title update, got %#v", input.Title)
	}
	if input.Description == nil || *input.Description != "new body" {
		t.Fatalf("expected description update")
	}
	if input.Status == nil || *input.Status != "in_progress" {
		t.Fatalf("expected status update")
	}
	if input.Type == nil || *input.Type != "bug" {
		t.Fatalf("expected type update")
	}
	if input.Priority == nil || *input.Priority != 1 {
		t.Fatalf("expected priority update")
	}
	if input.Assignee == nil || *input.Assignee != "alice" {
		t.Fatalf("expected assignee update")
	}
	if len(input.Labels) != 1 || input.Labels[0] != "triage" {
		t.Fatalf("expected labels update, got %#v", input.Labels)
	}
}

func TestBuildIssueUpdateInputNoChanges(t *testing.T) {
	t.Parallel()

	original := IssueDetail{Summary: IssueSummary{
		Title:    "Same",
		Status:   "open",
		Type:     "task",
		Priority: 2,
		Assignee: "",
		Labels:   []string{"x"},
	}, Description: "desc"}

	edited := IssueEditDocument{
		Title:       "Same",
		Description: "desc",
		Status:      "open",
		Type:        "task",
		Priority:    2,
		Assignee:    "",
		Labels:      []string{"x"},
	}

	input, changed := BuildIssueUpdateInput(original, edited)
	if changed {
		t.Fatalf("expected no changes, got %#v", input)
	}
}

func TestBuildIssueUpdateInputClearLabels(t *testing.T) {
	t.Parallel()

	original := IssueDetail{Summary: IssueSummary{Labels: []string{"bug", "ui"}}}
	edited := IssueEditDocument{Labels: nil}

	input, changed := BuildIssueUpdateInput(original, edited)
	if !changed {
		t.Fatalf("expected changed=true when labels are cleared")
	}
	if !input.ClearLabels {
		t.Fatalf("expected ClearLabels=true when edited labels empty")
	}
	if len(input.Labels) != 0 {
		t.Fatalf("expected no label set payload when clearing, got %#v", input.Labels)
	}
}

func TestParseIssueEditDocumentPriorityBoundariesAndOutOfRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		priorityRaw string
		want        int
		wantErr     bool
	}{
		{name: "min boundary", priorityRaw: "0", want: 0},
		{name: "max boundary", priorityRaw: "4", want: 4},
		{name: "below range", priorityRaw: "-1", wantErr: true},
		{name: "above range", priorityRaw: "5", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			issue := IssueDetail{Summary: IssueSummary{
				Title:    "Priority test",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			}}

			rendered := RenderIssueEditDocument(issue)
			edited := strings.Replace(rendered,
				issueEditFieldPriorityBegin+"\n2\n"+issueEditFieldPriorityEnd,
				issueEditFieldPriorityBegin+"\n"+tc.priorityRaw+"\n"+issueEditFieldPriorityEnd,
				1,
			)

			parsed, err := ParseIssueEditDocument(edited)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected parse error for priority %q", tc.priorityRaw)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseIssueEditDocument returned error: %v", err)
			}
			if parsed.Priority != tc.want {
				t.Fatalf("parsed priority = %d, want %d", parsed.Priority, tc.want)
			}
		})
	}
}

func TestParseIssueEditDocumentPriorityPrefixP2(t *testing.T) {
	t.Parallel()

	issue := IssueDetail{Summary: IssueSummary{
		Title:    "Priority prefix",
		Status:   "open",
		Type:     "task",
		Priority: 1,
	}}

	rendered := RenderIssueEditDocument(issue)
	edited := strings.Replace(rendered,
		issueEditFieldPriorityBegin+"\n1\n"+issueEditFieldPriorityEnd,
		issueEditFieldPriorityBegin+"\nP2\n"+issueEditFieldPriorityEnd,
		1,
	)

	parsed, err := ParseIssueEditDocument(edited)
	if err != nil {
		t.Fatalf("ParseIssueEditDocument returned error: %v", err)
	}
	if parsed.Priority != 2 {
		t.Fatalf("expected P2 to parse as priority 2, got %d", parsed.Priority)
	}
}

func TestBuildIssueUpdateInputAssigneeClearing(t *testing.T) {
	t.Parallel()

	original := IssueDetail{Summary: IssueSummary{Assignee: "hans"}}
	edited := IssueEditDocument{Assignee: ""}

	input, changed := BuildIssueUpdateInput(original, edited)
	if !changed {
		t.Fatalf("expected changed=true when assignee is cleared")
	}
	if input.Assignee == nil {
		t.Fatalf("expected assignee update pointer when clearing")
	}
	if *input.Assignee != "" {
		t.Fatalf("expected cleared assignee to be empty string, got %q", *input.Assignee)
	}
}

func TestBuildIssueUpdateInputNoSpuriousDescriptionRewriteOnNewlineTrimmedRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
	}{
		{name: "trailing newline", description: "body\n"},
		{name: "leading newline", description: "\nbody"},
		{name: "leading and trailing newline", description: "\nbody\n"},
		{name: "multiple surrounding newlines", description: "\n\nbody\n\n"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := IssueDetail{Summary: IssueSummary{
				Title:    "Round-trip body",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			}, Description: tc.description}

			// Simulate the real editor round-trip with no user edit. The parser
			// \n-trims the extracted description, so without normalization the
			// unedited save would diff as a description change.
			parsed, err := ParseIssueEditDocument(RenderIssueEditDocument(original))
			if err != nil {
				t.Fatalf("ParseIssueEditDocument returned error: %v", err)
			}

			input, changed := BuildIssueUpdateInput(original, parsed)
			if changed {
				t.Fatalf("expected changed=false for unedited newline-padded description %q, got %#v", tc.description, input)
			}
			if input.Description != nil {
				t.Fatalf("expected no spurious description rewrite for %q, got %q", tc.description, *input.Description)
			}
		})
	}
}

func TestBuildIssueUpdateInputDetectsGenuineDescriptionChangeAfterRoundTrip(t *testing.T) {
	t.Parallel()

	original := IssueDetail{Summary: IssueSummary{
		Title:    "Round-trip body",
		Status:   "open",
		Type:     "task",
		Priority: 2,
	}, Description: "\nbody\n"}

	parsed, err := ParseIssueEditDocument(RenderIssueEditDocument(original))
	if err != nil {
		t.Fatalf("ParseIssueEditDocument returned error: %v", err)
	}

	// A genuine user edit must still be detected even though the original had
	// surrounding newlines that get trimmed on round-trip.
	parsed.Description = "rewritten body"

	input, changed := BuildIssueUpdateInput(original, parsed)
	if !changed {
		t.Fatalf("expected changed=true for genuine description edit")
	}
	if input.Description == nil || *input.Description != "rewritten body" {
		t.Fatalf("expected description update to %q, got %#v", "rewritten body", input.Description)
	}
}

func TestParseIssueEditDocumentPreservesMultiParagraphDescription(t *testing.T) {
	t.Parallel()

	multi := "first paragraph\n\nstill first\n\nsecond paragraph"
	issue := IssueDetail{Summary: IssueSummary{
		Title:    "Description test",
		Status:   "open",
		Type:     "task",
		Priority: 2,
	}, Description: multi}

	rendered := RenderIssueEditDocument(issue)
	parsed, err := ParseIssueEditDocument(rendered)
	if err != nil {
		t.Fatalf("ParseIssueEditDocument returned error: %v", err)
	}

	if parsed.Description != multi {
		t.Fatalf("expected description to preserve blank lines, got %q", parsed.Description)
	}
}
