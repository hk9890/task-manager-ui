package details

import (
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestMetadataFieldsOrderAndCoverage(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, time.April, 5, 9, 30, 0, 0, time.UTC)
	fields := metadataFields(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:      "feature",
			Priority:  2,
			Status:    "in_progress",
			Assignee:  "alice",
			Labels:    []string{"ui", "backend"},
			CreatedAt: created,
			UpdatedAt: updated,
		},
		Comments:  []domain.IssueComment{{ID: "c-1"}},
		BlockedBy: []domain.IssueReference{{ID: "bw-1"}, {ID: "bw-2"}},
		Blocks:    []domain.IssueReference{{ID: "bw-3"}},
		Related:   []domain.IssueReference{{ID: "bw-4"}, {ID: "bw-5"}, {ID: "bw-6"}},
	})

	got := make([]string, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.label)
	}

	want := []string{"Type", "Priority", "Status", "Assignee", "Labels", "Created", "Updated", "Comments", "Blocked by", "Blocks", "Related"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("metadata field order mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestMetadataFieldsOmitEmptyOptionalValues(t *testing.T) {
	t.Parallel()

	fields := metadataFields(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	})

	got := make([]string, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.label)
	}

	want := []string{"Type", "Priority", "Status", "Comments", "Blocked by", "Blocks", "Related"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected field set\n got: %v\nwant: %v", got, want)
	}
}

func TestRenderMetadataRailRespectsWidth(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "feature-with-very-long-name",
			Priority: 1,
			Status:   "open",
		},
	}, 20)

	if len(lines) == 0 {
		t.Fatal("expected metadata lines")
	}

	for _, line := range lines {
		if len(line) > 20 {
			t.Fatalf("line exceeds width: %q (%d)", line, len(line))
		}
	}
}
