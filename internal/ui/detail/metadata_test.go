package detail

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/revier"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
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
		BlockedBy:   []domain.IssueReference{{ID: "tm-1"}, {ID: "tm-2"}},
		Blocks:      []domain.IssueReference{{ID: "tm-3"}},
		Related:     []domain.IssueReference{{ID: "tm-4"}, {ID: "tm-5"}, {ID: "tm-6"}},
	})

	got := make([]string, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.label)
	}

	want := []string{"Type", "Status", "Priority", "Assignee", "Owner", "Created", "Updated", "Duration", "Closed", "Reason", "Comments", "Blocked by", "Blocks", "Related", "Children"}
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

	want := []string{"Type", "Status", "Priority", "Comments", "Blocked by", "Blocks", "Related", "Children"}
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
	}, 20, false)

	if len(lines) == 0 {
		t.Fatal("expected metadata lines")
	}

	for _, line := range lines {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("line exceeds width: %q (%d)", line, lipgloss.Width(line))
		}
	}
}

func TestRenderMetadataRailIndentsEveryLineAndMarksNone(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui"},
		},
	}, 44, false)

	if len(lines) == 0 {
		t.Fatal("expected metadata lines")
	}

	for _, line := range lines {
		plain := metadataANSIPattern.ReplaceAllString(line, "")
		if !strings.HasPrefix(plain, "  ") {
			t.Fatalf("expected the indent on all metadata lines, got %q", plain)
		}
		if strings.HasPrefix(plain, selectionGutter()) {
			t.Fatalf("the view-only rail drew a selection marker: %q", plain)
		}
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
	}, 48, false)

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

func TestRenderMetadataRailDividerIsIndented(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui"},
		},
	}, 32, false)

	divider := findLineContaining(t, lines, "----")
	plain := metadataANSIPattern.ReplaceAllString(divider, "")
	if !strings.HasPrefix(plain, "  -") {
		t.Fatalf("expected the divider to be indented, got %q", plain)
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
		}, width, false)

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

func TestRenderMetadataRailAlignsTheLabels(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
		},
	}, 40, false)

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

func TestMetadataGroupsIncludeLabelsAsDedicatedSection(t *testing.T) {
	t.Parallel()

	groups := metadataGroups(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "task",
			Priority: 1,
			Status:   "open",
			Labels:   []string{"ui", "backend"},
		},
	}, false)

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

func TestRenderMetadataRailWithChildrenGolden(t *testing.T) {
	t.Parallel()

	lines := renderMetadataRail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			Type:     "epic",
			Priority: 1,
			Status:   "in_progress",
		},
		Children: []domain.IssueReference{
			{ID: "tm-c1", Title: "Child one"},
			{ID: "tm-c2", Title: "Child two"},
			{ID: "tm-c3", Title: "Child three"},
		},
	}, 44, false)

	testui.AssertMatchesGolden(t, []byte(strings.Join(lines, "\n")), "metadata_with_children_w44.golden")
}

// The Agents group follows Core, one field per linked agent in the order
// given, with attention said as "needs you", so a short pane keeps it. A pane
// with no linked agent draws no group.
func TestTheMetadataPaneListsTheLinkedAgentsRightAfterCore(t *testing.T) {
	t.Parallel()

	state := MetadataPaneState{
		Detail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Type: "task", Status: "in_progress"}},
		Width:  metadataRailWidth,
		Height: 40,
	}
	if view := testui.Downsample(RenderMetadataPane(state)); strings.Contains(view, "Agents") {
		t.Errorf("a pane with no linked agent draws an Agents group:\n%s", view)
	}

	state.Agents = []revier.Agent{
		{Panel: "2", Status: revier.StatusAttention, Session: "s-2"},
		{Panel: "1\x1b", Status: revier.StatusRunning, Session: "s-1"},
		{Panel: "10", Status: revier.StatusIdle, Session: "s-10"},
	}
	view := RenderMetadataPane(state)
	if strings.Contains(view, "Tab 1\x1b") {
		t.Errorf("a revier value reaches the screen uncleaned:\n%q", view)
	}
	view = testui.Downsample(view)
	_, priorityY := testui.FindCell(t, view, "Priority")
	_, agentsY := testui.FindCell(t, view, "Agents")
	if agentsY != priorityY+2 {
		t.Errorf("the Agents group on row %d does not follow Core, which ends on row %d:\n%s", agentsY, priorityY, view)
	}
	previous := agentsY
	for _, want := range []string{"Tab 2 : needs you", "Tab 1 : running", "Tab 10: idle"} {
		_, y := testui.FindCell(t, view, want)
		if y != previous+1 {
			t.Errorf("%q on row %d, want row %d:\n%s", want, y, previous+1, view)
		}
		previous = y
	}

	state.Height = 12
	if view := testui.Downsample(RenderMetadataPane(state)); !strings.Contains(view, "Tab 10: idle") {
		t.Errorf("a short pane cuts the Agents group:\n%s", view)
	}
}
