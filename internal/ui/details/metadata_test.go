package details

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestMetadataFieldsOrderAndCoverage(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, time.April, 5, 9, 30, 0, 0, time.UTC)
	closed := time.Date(2026, time.April, 9, 8, 0, 0, 0, time.UTC)
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
		Creator:     "hans",
		ClosedAt:    closed,
		CloseReason: "done",
		Comments:    []domain.IssueComment{{ID: "c-1"}},
		BlockedBy:   []domain.IssueReference{{ID: "bw-1"}, {ID: "bw-2"}},
		Blocks:      []domain.IssueReference{{ID: "bw-3"}},
		Related:     []domain.IssueReference{{ID: "bw-4"}, {ID: "bw-5"}, {ID: "bw-6"}},
	})

	got := make([]string, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.label)
	}

	want := []string{"Type", "Priority", "Status", "Assignee", "Creator", "Created", "Updated", "Duration", "Closed", "Reason", "Comments", "Blocked by", "Blocks", "Related"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("metadata field order mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestMetadataCoreFieldsKeepStatusAsFirstEditableField(t *testing.T) {
	t.Parallel()

	groups := metadataGroups(domain.IssueDetail{
		Summary: domain.IssueSummary{Type: "task", Priority: 2, Status: "open"},
	})
	if len(groups) == 0 {
		t.Fatal("expected metadata groups")
	}

	core := groups[0]
	if core.title != "Core" {
		t.Fatalf("expected first metadata group Core, got %q", core.title)
	}
	if len(core.fields) < 3 {
		t.Fatalf("expected core fields, got %#v", core.fields)
	}

	if core.fields[1].key != MetadataFieldPriority {
		t.Fatalf("expected priority to be actionable metadata field, got key=%q", core.fields[1].key)
	}

	if core.fields[2].key != MetadataFieldStatus {
		t.Fatalf("expected status to be actionable metadata field, got key=%q", core.fields[2].key)
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
			Labels:   []string{"this-is-an-extremely-long-label-that-must-be-truncated"},
		},
	}, 20, MetadataFieldNone)

	if len(lines) == 0 {
		t.Fatal("expected metadata lines")
	}

	for _, line := range lines {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("line exceeds width: %q (%d)", line, lipgloss.Width(line))
		}
	}
}

func TestRenderMetadataRailHighlightsSelectedStatusField(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	}, 40, MetadataFieldStatus)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "› Status") {
		t.Fatalf("expected selected status indicator in metadata rail, got:\n%s", joined)
	}
}

func TestRenderMetadataRailHighlightsSelectedPriorityField(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	}, 40, MetadataFieldPriority)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "› Priority") {
		t.Fatalf("expected selected priority indicator in metadata rail, got:\n%s", joined)
	}
}

func TestMetadataGroupsIncludeLabelsAsDedicatedSection(t *testing.T) {
	t.Parallel()

	groups := metadataGroups(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui", "backend"},
		},
	})

	if len(groups) < 3 {
		t.Fatalf("expected grouped metadata sections, got %d", len(groups))
	}

	if groups[1].title != "Labels" || len(groups[1].labels) != 2 {
		t.Fatalf("expected labels group in dedicated section, got %#v", groups[1])
	}

	if groups[len(groups)-1].title != "Counts" {
		t.Fatalf("expected counts group at end, got %q", groups[len(groups)-1].title)
	}
}

func TestMetadataFieldsOmitDurationWhenCreatedMissing(t *testing.T) {
	t.Parallel()

	fields := metadataFields(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
		ClosedAt: time.Date(2026, time.April, 9, 8, 0, 0, 0, time.UTC),
	})

	for _, field := range fields {
		if field.label == "Duration" {
			t.Fatal("did not expect Duration when CreatedAt is unavailable")
		}
	}
}

func TestMetadataFieldsOmitDurationWhenClosedMissing(t *testing.T) {
	t.Parallel()

	fields := metadataFields(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:      "task",
			Priority:  1,
			Status:    "open",
			CreatedAt: time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	for _, field := range fields {
		if field.label == "Duration" {
			t.Fatal("did not expect Duration when ClosedAt is unavailable")
		}
	}
}
