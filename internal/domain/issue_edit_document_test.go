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
			ID:        "bw-12",
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
		BlockedBy:   []IssueReference{{ID: "bw-1", Title: "Predecessor"}},
		Comments:    []IssueComment{{ID: "c1", Body: "check this"}},
	}

	rendered := RenderIssueEditDocument(issue)
	if !strings.Contains(rendered, "# Beads Workbench Issue Edit") {
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
