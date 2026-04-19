package details

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
)

var metadataANSIPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

	want := []string{"Type", "Status", "Priority", "Assignee", "Creator", "Created", "Updated", "Duration", "Closed", "Reason", "Comments", "Blocked by", "Blocks", "Related"}
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

	if core.fields[1].key != MetadataFieldStatus {
		t.Fatalf("expected status to be first actionable metadata field, got key=%q", core.fields[1].key)
	}

	if core.fields[2].key != MetadataFieldPriority {
		t.Fatalf("expected priority to be second actionable metadata field, got key=%q", core.fields[2].key)
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

	want := []string{"Type", "Status", "Priority", "Comments", "Blocked by", "Blocks", "Related"}
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

func TestRenderMetadataRailReservesIdleGutterWhenUnfocused(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui"},
		},
	}, 44, MetadataFieldNone)

	if len(lines) == 0 {
		t.Fatal("expected metadata lines")
	}

	for _, line := range lines {
		plain := metadataANSIPattern.ReplaceAllString(line, "")
		if !strings.HasPrefix(plain, "  ") {
			t.Fatalf("expected permanent idle gutter on all metadata lines, got %q", plain)
		}
		if strings.HasPrefix(plain, "› ") {
			t.Fatalf("did not expect selected marker while unfocused, got %q", plain)
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

	joined := metadataANSIPattern.ReplaceAllString(strings.Join(lines, "\n"), "")
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

	joined := metadataANSIPattern.ReplaceAllString(strings.Join(lines, "\n"), "")
	if !strings.Contains(joined, "› Priority") {
		t.Fatalf("expected selected priority indicator in metadata rail, got:\n%s", joined)
	}
}

func TestRenderMetadataRailShowsSelectedMarkerOnlyOnSelectedRow(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	}, 40, MetadataFieldStatus)

	selectedCount := 0
	for _, line := range lines {
		plain := metadataANSIPattern.ReplaceAllString(line, "")
		if strings.HasPrefix(plain, "› ") {
			selectedCount++
		}
	}

	if selectedCount != 1 {
		t.Fatalf("expected exactly one selected marker, got %d lines:\n%s", selectedCount, strings.Join(lines, "\n"))
	}
}

func TestRenderMetadataRailLabelLinesAlignWithFieldColumn(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 2,
			Status:   "open",
			Labels:   []string{"backend"},
		},
	}, 48, MetadataFieldNone)

	typeLine := findLineContaining(t, lines, "Type")
	labelLine := findLineContaining(t, lines, "• backend")

	typePlain := metadataANSIPattern.ReplaceAllString(typeLine, "")
	labelPlain := metadataANSIPattern.ReplaceAllString(labelLine, "")

	typeOffset := lipgloss.Width(typePlain[:strings.Index(typePlain, "Type")])
	bulletOffset := lipgloss.Width(labelPlain[:strings.Index(labelPlain, "•")])

	if typeOffset != bulletOffset {
		t.Fatalf("expected label bullet alignment with field column\ntype:  %q\nlabel: %q", typePlain, labelPlain)
	}
}

func TestRenderMetadataRailDividerUsesReservedGutterAlignment(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui"},
		},
	}, 32, MetadataFieldNone)

	divider := findLineContaining(t, lines, "----")
	plain := metadataANSIPattern.ReplaceAllString(divider, "")
	if !strings.HasPrefix(plain, "  -") {
		t.Fatalf("expected divider to include idle gutter, got %q", plain)
	}
}

func TestRenderMetadataRailVeryNarrowWidthsDegradeSafely(t *testing.T) {
	t.Parallel()

	for _, width := range []int{1, 2} {
		lines := renderMetadataRail(domain.IssueDetail{
			Summary: domain.IssueSummary{
				Type:     "task",
				Priority: 1,
				Status:   "open",
			},
		}, width, MetadataFieldStatus)

		if len(lines) == 0 {
			t.Fatalf("expected metadata lines for width=%d", width)
		}

		for _, line := range lines {
			if lipgloss.Width(line) > width {
				t.Fatalf("expected width-safe output at width=%d, got %q (%d)", width, line, lipgloss.Width(line))
			}
		}
	}
}

func TestRenderMetadataRailSelectedFieldKeepsLabelAlignmentStable(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	}, 40, MetadataFieldStatus)

	plainLines := make([]string, 0, len(lines))
	for _, line := range lines {
		plainLines = append(plainLines, metadataANSIPattern.ReplaceAllString(line, ""))
	}

	var priorityLine, statusLine string
	for _, line := range plainLines {
		if strings.Contains(line, "Priority") {
			priorityLine = line
		}
		if strings.Contains(line, "Status") {
			statusLine = line
		}
	}

	if priorityLine == "" || statusLine == "" {
		t.Fatalf("expected priority and status lines, got:\n%s", strings.Join(plainLines, "\n"))
	}

	priorityLabelOffset := lipgloss.Width(priorityLine[:strings.Index(priorityLine, "Priority")])
	statusLabelOffset := lipgloss.Width(statusLine[:strings.Index(statusLine, "Status")])
	if priorityLabelOffset != statusLabelOffset {
		t.Fatalf("expected stable metadata label alignment\npriority: %q\nstatus:   %q", priorityLine, statusLine)
	}
}

func TestRenderMetadataRailSelectionMovementKeepsValueColumnStable(t *testing.T) {
	t.Parallel()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "closed",
		},
	}

	prioritySelected := renderMetadataRail(detail, 48, MetadataFieldPriority)
	statusSelected := renderMetadataRail(detail, 48, MetadataFieldStatus)

	priorityLineSelected := findLineContaining(t, prioritySelected, "Priority")
	priorityLineIdle := findLineContaining(t, statusSelected, "Priority")
	statusLineIdle := findLineContaining(t, prioritySelected, "Status")
	statusLineSelected := findLineContaining(t, statusSelected, "Status")

	priorityValueOffsetSelected := valueOffset(t, priorityLineSelected)
	priorityValueOffsetIdle := valueOffset(t, priorityLineIdle)
	statusValueOffsetIdle := valueOffset(t, statusLineIdle)
	statusValueOffsetSelected := valueOffset(t, statusLineSelected)

	if priorityValueOffsetSelected != priorityValueOffsetIdle {
		t.Fatalf("expected Priority value column to stay stable when selection moves\nselected: %q\nidle:     %q", priorityLineSelected, priorityLineIdle)
	}
	if statusValueOffsetSelected != statusValueOffsetIdle {
		t.Fatalf("expected Status value column to stay stable when selection moves\nselected: %q\nidle:     %q", statusLineSelected, statusLineIdle)
	}
	if priorityValueOffsetIdle != statusValueOffsetIdle {
		t.Fatalf("expected Priority and Status value columns to align\npriority: %q\nstatus:   %q", priorityLineIdle, statusLineIdle)
	}
}

func findLineContaining(t *testing.T, lines []string, token string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, token) {
			return line
		}
	}
	t.Fatalf("expected line containing %q in metadata output", token)
	return ""
}

func valueOffset(t *testing.T, line string) int {
	t.Helper()
	idx := strings.Index(line, ": ")
	if idx < 0 {
		t.Fatalf("expected metadata line to contain ': ': %q", line)
	}
	return lipgloss.Width(line[:idx])
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
