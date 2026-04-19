package details

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/ui"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func assertGolden(t *testing.T, output []byte, name string) {
	t.Helper()

	if os.Getenv("BWB_UPDATE_GOLDEN") == "1" {
		path := filepath.Join("testdata", name)
		if err := os.WriteFile(path, output, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}

	ui.AssertMatchesGolden(t, output, name)
}

func TestRenderMinimalGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-1",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-1",
				Title:    "Minimal detail",
				Status:   "open",
				Type:     "task",
				Priority: 1,
			},
		},
		Width: 120,
	})

	assertGolden(t, []byte(view), "minimal.golden")
}

func TestRenderFullGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:        "bw-22",
				Title:     "Full detail sample",
				Status:    "in_progress",
				Type:      "feature",
				Priority:  2,
				Assignee:  "alice",
				Labels:    []string{"backend", "ui"},
				CreatedAt: mustTime(t, "2026-04-01T12:00:00Z"),
				UpdatedAt: mustTime(t, "2026-04-05T11:00:00Z"),
			},
			Creator:     "hans",
			Description: "Ship issue detail rendering for standalone mode.\nKeep shell-owned selection state.",
			Notes:       "Reference donor rendering patterns only.",
			ClosedAt:    mustTime(t, "2026-04-09T08:00:00Z"),
			CloseReason: "completed",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "reviewer", Body: "Looks good to me", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
			},
			BlockedBy: []domain.IssueReference{{ID: "bw-10", Title: "Data model update"}},
			Blocks:    []domain.IssueReference{{ID: "bw-30", Title: "Integration checks"}},
			Related:   []domain.IssueReference{{ID: "bw-44", Title: "Renderer cleanup"}},
		},
		Width: 120,
	})

	assertGolden(t, []byte(view), "full.golden")
}

func TestRenderCommentsGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-77",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-77",
				Title:    "Comments heavy issue",
				Status:   "open",
				Type:     "bug",
				Priority: 0,
			},
			Description: "Short description",
			Comments: []domain.IssueComment{
				{ID: "c-2", Author: "bob", Body: "Second chronologically", CreatedAt: mustTime(t, "2026-04-05T12:00:00Z")},
				{ID: "c-1", Author: "alice", Body: "First chronologically", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
				{ID: "c-3", Body: "", CreatedAt: time.Time{}},
			},
		},
		Width: 100,
	})

	assertGolden(t, []byte(view), "comments.golden")
}

func TestRenderDependencyRichGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-88",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-88",
				Title:    "Dependency rich issue",
				Status:   "blocked",
				Type:     "task",
				Priority: 1,
			},
			Description: "Dependency context check",
			BlockedBy: []domain.IssueReference{
				{ID: "bw-5", Title: "Upstream gate"},
				{ID: "bw-1", Title: "Auth migration"},
			},
			Blocks: []domain.IssueReference{
				{ID: "bw-12", Title: "Release docs"},
				{ID: "bw-9", Title: "UI polish"},
			},
			Related: []domain.IssueReference{
				{ID: "bw-100", Title: "Planning umbrella"},
				{ID: "bw-42", Title: "Search sync"},
			},
		},
		Width: 100,
	})

	assertGolden(t, []byte(view), "dependency_rich.golden")
}

func TestRenderDependencyRowsHighlightSelectedIssue(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-88",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-88", Title: "Dependency rich issue", Status: "blocked", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{{ID: "bw-1", Title: "Auth migration"}},
			Blocks:    []domain.IssueReference{{ID: "bw-9", Title: "UI polish"}},
			Related:   []domain.IssueReference{{ID: "bw-42", Title: "Search sync"}},
		},
		BrowserSelectedIssueID: "bw-9",
		Width:                  100,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "│›") || !strings.Contains(plain, "bw-9") {
		t.Fatalf("expected selected dependency row marker for bw-9, got:\n%s", plain)
	}
}

func TestRenderCompactGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-22",
				Title:    "Full detail sample",
				Status:   "in_progress",
				Type:     "feature",
				Priority: 2,
				Assignee: "alice",
				Labels:   []string{"backend", "ui"},
			},
			Description: "Ship issue detail rendering for standalone mode.\nKeep shell-owned selection state.",
			Comments:    []domain.IssueComment{{ID: "c-1", Author: "reviewer", Body: "Looks good to me", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")}},
			BlockedBy:   []domain.IssueReference{{ID: "bw-10", Title: "Data model update"}},
			Blocks:      []domain.IssueReference{{ID: "bw-30", Title: "Integration checks"}},
			Related:     []domain.IssueReference{{ID: "bw-44", Title: "Renderer cleanup"}},
		},
		Width:   56,
		Compact: true,
	})

	assertGolden(t, []byte(view), "compact.golden")
}

func TestRenderCompactClosedDurationGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-23",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:        "bw-23",
				Title:     "Closed compact duration sample",
				Status:    "closed",
				Type:      "bug",
				Priority:  1,
				Assignee:  "alice",
				CreatedAt: mustTime(t, "2026-04-01T12:00:00Z"),
			},
			Creator:     "hans",
			ClosedAt:    mustTime(t, "2026-04-03T14:30:00Z"),
			CloseReason: "completed",
			Description: "Closed issue includes duration in metadata rail.",
		},
		Width:   56,
		Compact: true,
	})

	assertGolden(t, []byte(view), "compact_closed_duration.golden")
}

func TestRenderWideThreeColumnGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-wide",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-wide",
				Title:    "Wide layout sample",
				Status:   "in_progress",
				Type:     "feature",
				Priority: 1,
			},
			Description: "Three column layout should render related rail on wide terminals.",
			Notes:       "Inline related work section should be suppressed when rail is active.",
			ParentGroupBrowser: domain.ParentGroupBrowserContext{
				Parent: domain.IssueReference{ID: "bw-parent", Title: "Parent epic"},
			},
			BlockedBy: []domain.IssueReference{
				{ID: "bw-1", Title: "Auth migration", Type: "task", Priority: 1, Status: "blocked"},
			},
			Blocks: []domain.IssueReference{
				{ID: "bw-2", Title: "Docs update", Type: "docs", Priority: 2, Status: "open"},
			},
			Related: []domain.IssueReference{
				{ID: "bw-3", Title: "Renderer cleanup", Type: "chore", Priority: 3, Status: "open"},
			},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "bw-parent", Title: "Parent epic"},
			{ID: "bw-wide", Title: "Wide layout sample"},
			{ID: "bw-sibling", Title: "Sibling issue"},
		},
		BrowserSelectedIssueID: "bw-wide",
		Width:                  InspectorThreeColumnMinWidth,
	})

	assertGolden(t, []byte(view), "wide_three_column.golden")
}

func TestRenderFallbackKeepsInlineRelatedWorkGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-fallback",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-fallback",
				Title:    "Fallback inline related work",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			},
			Description: "Below wide breakpoint should keep inline related work section.",
			BlockedBy:   []domain.IssueReference{{ID: "bw-11", Title: "Dependency A"}},
			Blocks:      []domain.IssueReference{{ID: "bw-12", Title: "Dependency B"}},
			Related:     []domain.IssueReference{{ID: "bw-13", Title: "Dependency C"}},
		},
		Width: InspectorThreeColumnMinWidth - 1,
	})

	assertGolden(t, []byte(view), "fallback_inline_related_work.golden")
}

func TestRenderUsesTwoColumnInspectorAtBreakpoint(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-22",
				Title:    "Two column sample",
				Status:   "in_progress",
				Type:     "feature",
				Priority: 2,
				Assignee: "alice",
				Labels:   []string{"backend", "ui"},
			},
			Description: "Ship issue detail rendering for standalone mode.",
		},
		Width: InspectorTwoColumnMinWidth,
	})

	if !strings.Contains(view, "Dependencies") || !strings.Contains(view, "Content") || !strings.Contains(view, "Metadata") {
		t.Fatalf("expected three-pane layout headings at width %d, got:\n%s", InspectorTwoColumnMinWidth, view)
	}
}

func TestRenderThreePaneLayoutBelowLegacyBreakpoints(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-23",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-23",
				Title:    "Single column sample",
				Status:   "open",
				Type:     "task",
				Priority: 1,
			},
			Description: "Description body",
		},
		Width: InspectorTwoColumnMinWidth - 1,
	})

	if !strings.Contains(view, "Dependencies") || !strings.Contains(view, "Content") || !strings.Contains(view, "Metadata") {
		t.Fatalf("expected dense three-pane layout below legacy breakpoint, got:\n%s", view)
	}
}

func TestRenderTwoColumnUsesFixedMetadataRailWidth(t *testing.T) {
	t.Parallel()

	_, metadata := splitInspectorWidths(InspectorTwoColumnMinWidth)
	if metadata != 34 {
		t.Fatalf("expected fixed metadata rail width 34, got %d", metadata)
	}
}

func TestRenderThreeColumnRailWidthsStayInApprovedRange(t *testing.T) {
	t.Parallel()

	left, _, metadata := splitThreeColumnWidths(InspectorThreeColumnMinWidth)
	if left < leftRailMinWidth || left > leftRailMaxWidth {
		t.Fatalf("expected left rail in [%d,%d], got %d", leftRailMinWidth, leftRailMaxWidth, left)
	}
	if metadata != 34 {
		t.Fatalf("expected metadata rail width 34, got %d", metadata)
	}
}

func TestSplitThreePaneWidthsTargetsQuarterLeftRailAtCommonTerminalSizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		total int
	}{
		{name: "120 columns", total: 120},
		{name: "160 columns", total: 160},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			left, _, metadata := splitThreePaneWidths(tc.total)
			available := tc.total - (detailColumnGap * 2)
			expected := clamp(available/4, leftRailMinWidth, leftRailMaxWidth)
			if left != expected {
				t.Fatalf("expected left rail %d for total=%d, got %d", expected, tc.total, left)
			}
			if metadata != metadataRailWidth {
				t.Fatalf("expected metadata rail width %d for total=%d, got %d", metadataRailWidth, tc.total, metadata)
			}
		})
	}
}

func TestRenderWideLayoutWithoutBrowserFallsBackToPairedContentMetadata(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-rail",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-rail", Title: "Rail", Status: "open", Type: "task", Priority: 1},
			Description: "desc",
			BlockedBy:   []domain.IssueReference{{ID: "bw-1", Title: "A"}},
			Related:     []domain.IssueReference{{ID: "bw-2", Title: "B"}},
		},
		Width: InspectorThreeColumnMinWidth,
	})

	if strings.Contains(view, "Issue Browser") {
		t.Fatalf("expected no special left browser without parent-group context, got:\n%s", view)
	}
	if strings.Contains(view, "Issue Browser") {
		t.Fatalf("expected no special left browser without parent-group context, got:\n%s", view)
	}
	if !strings.Contains(view, "Metadata") {
		t.Fatalf("expected paired content/metadata layout without browser panel, got:\n%s", view)
	}
}

func TestRenderWideLayoutWithBrowserUsesLeftIssueBrowserPanel(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-child",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-child", Title: "Child", Status: "open", Type: "task", Priority: 1},
			Description: "desc",
			ParentGroupBrowser: domain.ParentGroupBrowserContext{
				Parent: domain.IssueReference{ID: "bw-parent", Title: "Parent"},
			},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "bw-parent", Title: "Parent"},
			{ID: "bw-child", Title: "Child"},
			{ID: "bw-sibling", Title: "Sibling"},
		},
		BrowserSelectedIssueID: "bw-child",
		Width:                  InspectorThreeColumnMinWidth,
	})

	if !strings.Contains(view, "Dependencies") {
		t.Fatalf("expected left dependencies panel in wide layout, got:\n%s", view)
	}
	if !strings.Contains(view, "Structure (3)") {
		t.Fatalf("expected structure group in dependencies panel, got:\n%s", view)
	}
}

func TestRenderDependenciesOmitsStructureWhenBrowserItemsComeFromDependencyFallback(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-main",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-main", Title: "Main", Status: "open", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{
				{ID: "bw-1", Title: "A"},
			},
			Blocks: []domain.IssueReference{
				{ID: "bw-2", Title: "B"},
			},
			Related: []domain.IssueReference{
				{ID: "bw-3", Title: "C"},
			},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "bw-1", Title: "A"},
			{ID: "bw-2", Title: "B"},
			{ID: "bw-3", Title: "C"},
		},
		Width: InspectorThreeColumnMinWidth,
	})

	if strings.Contains(view, "Structure (") {
		t.Fatalf("expected dependency-fallback view to omit structure group, got:\n%s", view)
	}
}

func TestDependencyGroupsDeduplicateIssueIDsAcrossVisibleGroups(t *testing.T) {
	t.Parallel()

	groups := dependencyGroups(domain.IssueDetail{
		BlockedBy: []domain.IssueReference{{ID: "bw-1", Title: "Blocker"}},
		Blocks:    []domain.IssueReference{{ID: "bw-2", Title: "Blocked child"}},
		Related: []domain.IssueReference{
			{ID: "bw-1", Title: "Duplicate from blocked-by"},
			{ID: "bw-3", Title: "Related unique"},
		},
	}, nil)

	if got := len(groups); got != 3 {
		t.Fatalf("expected 3 dependency groups, got %d", got)
	}
	if got := len(groups[0].Refs); got != 1 || groups[0].Refs[0].ID != "bw-1" {
		t.Fatalf("expected blocked-by to keep bw-1 once, got %#v", groups[0].Refs)
	}
	if got := len(groups[1].Refs); got != 1 || groups[1].Refs[0].ID != "bw-2" {
		t.Fatalf("expected blocks to keep bw-2 once, got %#v", groups[1].Refs)
	}
	if got := len(groups[2].Refs); got != 1 || groups[2].Refs[0].ID != "bw-3" {
		t.Fatalf("expected related to drop duplicate bw-1 and keep bw-3, got %#v", groups[2].Refs)
	}
}

func TestRenderDependenciesPaneLinesDoNotRenderDuplicateIssueRowsAcrossGroups(t *testing.T) {
	t.Parallel()

	lines := renderDependenciesPaneLines(domain.IssueDetail{
		BlockedBy: []domain.IssueReference{{ID: "bw-1", Title: "Blocker"}},
		Blocks:    []domain.IssueReference{{ID: "bw-2", Title: "Blocked child"}},
		Related: []domain.IssueReference{
			{ID: "bw-1", Title: "Duplicate from blocked-by"},
			{ID: "bw-3", Title: "Related unique"},
		},
	}, nil, "bw-3", 80)

	joined := strings.Join(lines, "\n")
	if got := strings.Count(joined, "bw-1"); got != 1 {
		t.Fatalf("expected duplicate dependency row bw-1 to render once, got %d occurrences in:\n%s", got, joined)
	}
	if !strings.Contains(joined, "Related (1)") {
		t.Fatalf("expected related group count to reflect de-duplicated rows, got:\n%s", joined)
	}
}

func TestRenderUsesMarkdownRendererForDescriptionAndNotes(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-90",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-90",
				Title:    "Markdown check",
				Status:   "open",
				Type:     "task",
				Priority: 1,
			},
			Description: "# Ship markdown\n\n- first\n- second",
			Notes:       "## Follow up\n\n[link](https://example.com)",
		},
		Width: 90,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	for _, want := range []string{"Ship markdown", "first", "Follow up", "link"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in rendered markdown detail:\n%s", want, plain)
		}
	}
}

func TestRenderUsesMarkdownRendererForCommentBodies(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-91",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-91", Title: "Comment plain text", Status: "open", Type: "task", Priority: 1},
			Description: "Plain",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "alice", Body: "- literal markdown-like bullet", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
			},
		},
		Width: 100,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "literal markdown-like bullet") {
		t.Fatalf("expected markdown-rendered comment text to be present, got:\n%s", plain)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}

	return ts
}
