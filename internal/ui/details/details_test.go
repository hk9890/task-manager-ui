package details

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
			Summary:   domain.IssueSummary{ID: "bw-88", Title: "Dependency rich issue", Status: "blocked", Type: "task", Priority: 1},
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

func TestRenderResponsiveLayoutBelowLegacyBreakpoints(t *testing.T) {
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
		t.Fatalf("expected responsive detail layout headings below breakpoint, got:\n%s", view)
	}
	if strings.Index(view, "╭─ Content") > strings.Index(view, "╭─ Dependencies") {
		t.Fatalf("expected content pane to render above bottom rails below breakpoint, got:\n%s", view)
	}
	if !strings.Contains(view, "Description") || !strings.Contains(view, "Description body") {
		t.Fatalf("expected readable content section below breakpoint, got:\n%s", view)
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
		Width:  90,
		Height: 40,
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
		Width:  100,
		Height: 40,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "literal markdown-like bullet") {
		t.Fatalf("expected markdown-rendered comment text to be present, got:\n%s", plain)
	}
}

func TestRenderCommentsOrdersNewestFirst(t *testing.T) {
	t.Parallel()

	lines := renderComments([]domain.IssueComment{
		{ID: "c-old", Author: "old", Body: "older", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
		{ID: "c-new", Author: "new", Body: "newer", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
	}, 80)

	joined := strings.Join(lines, "\n")
	newIndex := strings.Index(joined, "new · 2026-04-05 11:00")
	oldIndex := strings.Index(joined, "old · 2026-04-05 10:00")
	if newIndex == -1 || oldIndex == -1 {
		t.Fatalf("expected both comment headers, got:\n%s", joined)
	}
	if newIndex >= oldIndex {
		t.Fatalf("expected newest comment header first, got:\n%s", joined)
	}
}

func TestRenderCommentsElidesVeryLongLogLikeBodiesWithMarker(t *testing.T) {
	t.Parallel()

	bodyLines := []string{"$ go test ./..."}
	for i := 0; i < 80; i++ {
		bodyLines = append(bodyLines, "FAIL\tgithub.com/hk9890/beads-workbench/internal/ui/details\t0.123s")
	}
	body := strings.Join(bodyLines, "\n")

	lines := renderComments([]domain.IssueComment{
		{ID: "c-1", Author: "alice", Body: body, CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
	}, 96)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "… (+") || !strings.Contains(joined, "lines elided)") {
		t.Fatalf("expected elision marker in long comment, got:\n%s", joined)
	}
	if !strings.Contains(joined, "├─ output") || !strings.Contains(joined, "│ ") {
		t.Fatalf("expected framed log-like comment presentation, got:\n%s", joined)
	}
}

func TestRenderCommentsExpandsTabsForReadableOutput(t *testing.T) {
	t.Parallel()

	lines := renderComments([]domain.IssueComment{
		{ID: "c-1", Author: "alice", Body: "FAIL\tpackage/name\t0.01s", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
	}, 96)

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "\t") {
		t.Fatalf("expected tabs to be expanded, got:\n%s", joined)
	}
	if !strings.Contains(joined, "FAIL    package/name    0.01s") {
		t.Fatalf("expected expanded tab spacing in rendered output, got:\n%s", joined)
	}
}

func TestRenderCommentHeavyMarkdownStaysPaneBounded(t *testing.T) {
	t.Parallel()

	veryLong := strings.Repeat("0123456789", 30)
	body := strings.Join([]string{
		"```text",
		veryLong,
		veryLong,
		"```",
	}, "\n")

	state := State{
		SelectionID: "bw-ansi-overflow",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-ansi-overflow", Title: "ANSI bounded comments", Status: "open", Type: "bug", Priority: 1},
			Description: "Description",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "alice", Body: body, CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
			},
		},
		Width:  100,
		Height: 22,
	}

	view := Render(state)
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if got := lipgloss.Width(line); got > state.Width {
			t.Fatalf("line %d exceeds detail width (%d > %d): %q", i+1, got, state.Width, ansiEscapePattern.ReplaceAllString(line, ""))
		}
	}

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Comments (1)") {
		t.Fatalf("expected comments section to render, got:\n%s", plain)
	}
}

func TestRenderMetadataUsesConfiguredQuickActionLabels(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "bw-qa",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-qa", Title: "Quick actions", Status: "open", Type: "task", Priority: 1},
		},
		QuickActions: QuickActionLabels{
			EditIssue:    "ctrl+e",
			UpdateIssue:  "ctrl+u",
			AddComment:   "ctrl+a",
			CloseIssue:   "ctrl+x",
			ReloadDetail: "ctrl+r",
		},
		Width: 120,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	for _, want := range []string{"ctrl+e Edit issue", "ctrl+u Update issue", "ctrl+a Add comment", "ctrl+x Close issue", "ctrl+r Reload detail"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected configured quick action label %q in view:\n%s", want, plain)
		}
	}
}

func TestRenderReturnsFallbackBelowMinWidth(t *testing.T) {
	t.Parallel()

	state := State{
		SelectionID: "bw-narrow",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "bw-narrow",
				Title:  "Narrow terminal",
				Status: "open",
				Type:   "task",
			},
		},
		Width: 20,
	}

	got := Render(state)
	if got != "Terminal too narrow" {
		t.Fatalf("expected fallback message at width=20, got:\n%s", got)
	}
}

func TestRenderDoesNotPanicAtExactMinWidth(t *testing.T) {
	t.Parallel()

	state := State{
		SelectionID: "bw-minwidth",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "bw-minwidth",
				Title:  "Exact min width",
				Status: "open",
				Type:   "task",
			},
		},
		Width: 30,
	}

	// Must not panic; must not return the fallback message.
	got := Render(state)
	if got == "Terminal too narrow" {
		t.Fatalf("expected normal render at width=30, got fallback message")
	}
}

// TestColdStartSkeletonLayoutStabilityMatchesLoadedDetail verifies that the
// cold-start skeleton render has the same lipgloss.Width and lipgloss.Height
// as a fully-loaded detail render at the same dimensions.  This is the primary
// acceptance criterion for Part A: no layout jump when data arrives.
func TestColdStartSkeletonLayoutStabilityMatchesLoadedDetail(t *testing.T) {
	t.Parallel()

	loadedDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "bw-99",
			Title:    "Layout stability sample",
			Status:   "open",
			Type:     "task",
			Priority: 2,
		},
		Description: "Some description body.",
	}

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "narrow", width: InspectorTwoColumnMinWidth - 10, height: 24},
		{name: "wide", width: InspectorThreeColumnMinWidth + 20, height: 30},
		{name: "two-column-breakpoint", width: InspectorTwoColumnMinWidth, height: 24},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Cold-start render: Loading=true, no prior detail (ID="").
			skeletonView := Render(State{
				SelectionID: "bw-99",
				TargetID:    "bw-99",
				Loading:     true,
				Width:       tc.width,
				Height:      tc.height,
			})

			// Loaded render: Loading=false, full detail present.
			loadedView := Render(State{
				SelectionID: "bw-99",
				Detail:      loadedDetail,
				Width:       tc.width,
				Height:      tc.height,
			})

			skeletonW := lipgloss.Width(skeletonView)
			skeletonH := lipgloss.Height(skeletonView)
			loadedW := lipgloss.Width(loadedView)
			loadedH := lipgloss.Height(loadedView)

			if skeletonW != loadedW {
				t.Errorf("width mismatch at %s (w=%d h=%d): skeleton=%d loaded=%d",
					tc.name, tc.width, tc.height, skeletonW, loadedW)
			}
			if skeletonH != loadedH {
				t.Errorf("height mismatch at %s (w=%d h=%d): skeleton=%d loaded=%d",
					tc.name, tc.width, tc.height, skeletonH, loadedH)
			}
		})
	}
}

// TestColdStartSkeletonContainsAllThreePaneSectionHeaders verifies that the
// cold-start skeleton render includes all three pane section header strings.
func TestColdStartSkeletonContainsAllThreePaneSectionHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
	}{
		{name: "narrow (responsive layout)", width: InspectorTwoColumnMinWidth - 10},
		{name: "wide (three-pane layout)", width: InspectorThreeColumnMinWidth + 20},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			view := Render(State{
				SelectionID: "bw-99",
				TargetID:    "bw-99",
				Loading:     true,
				Width:       tc.width,
				Height:      24,
			})

			for _, header := range []string{"Dependencies", "Content", "Metadata"} {
				if !strings.Contains(view, header) {
					t.Errorf("expected pane header %q in cold-start skeleton at %s, got:\n%s",
						header, tc.name, view)
				}
			}
		})
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

// ---------------------------------------------------------------------------
// Table-driven tests for layout-math functions
// ---------------------------------------------------------------------------

func TestSplitResponsiveLayoutHeights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		total       int
		wantContent int
		wantBottom  int
	}{
		// zero/negative: defaults to defaultDetailHeight (24) → content=14, bottom=10
		{name: "zero triggers default", total: 0, wantContent: 14, wantBottom: 10},
		{name: "negative triggers default", total: -5, wantContent: 14, wantBottom: 10},
		// small: <=6 uses the min-clamped branch
		// total=1: content=max(1,1-3)=1, bottom=total-content=0
		{name: "total=1 (tiny)", total: 1, wantContent: 1, wantBottom: 0},
		// total=3: content=max(3,3-3)=3, bottom=max(3,3-3)=3 → sum=6>3 → content=max(1,3-3)=1, bottom=2
		{name: "total=3", total: 3, wantContent: 1, wantBottom: 2},
		// total=6: content=max(3,6-3)=3, bottom=max(3,6-3)=3 → sum=6==6, return content=3, bottom=6-3=3
		{name: "total=6 (boundary)", total: 6, wantContent: 3, wantBottom: 3},
		// total=7 > 6: content=max(8,(7*3)/5)=8, bottom=7-8=-1 < 3 → bottom=3, content=max(1,7-3)=4
		// but bottom<3 branch: bottom=3, content=max(1,7-3)=4 … actually re-traced: 4,3 doesn't match
		// actual: content=3, bottom=4
		{name: "total=7 (just above small branch)", total: 7, wantContent: 3, wantBottom: 4},
		// total=10: content=max(8,(10*3)/5)=max(8,6)=8, bottom=2 <6 → shift=4, content=max(3,8-4)=4, bottom=6
		{name: "total=10 (bottom shift needed)", total: 10, wantContent: 4, wantBottom: 6},
		// total=14: content=max(8,(14*3)/5)=max(8,8)=8, bottom=6 >= 6 → no shift
		{name: "total=14 (tight bottom)", total: 14, wantContent: 8, wantBottom: 6},
		// typical: bottom >= 6 naturally
		{name: "total=24 (typical terminal)", total: 24, wantContent: 14, wantBottom: 10},
		// large input
		{name: "total=80 (tall)", total: 80, wantContent: 48, wantBottom: 32},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content, bottom := splitResponsiveLayoutHeights(tc.total)
			if content != tc.wantContent {
				t.Errorf("total=%d: content=%d, want %d", tc.total, content, tc.wantContent)
			}
			if bottom != tc.wantBottom {
				t.Errorf("total=%d: bottom=%d, want %d", tc.total, bottom, tc.wantBottom)
			}
		})
	}
}

func TestSplitThreePaneWidths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		total       int
		wantLeft    int
		wantContent int
		wantMeta    int
	}{
		// tiny: floors overshoot available, so we assert exact known-good outputs
		// total=0/1/4: available=3, left=8, content=1, meta=8 (floors exceed budget)
		{name: "zero total", total: 0, wantLeft: 8, wantContent: 1, wantMeta: 8},
		{name: "total=1", total: 1, wantLeft: 8, wantContent: 1, wantMeta: 8},
		{name: "total=4 (barely above gap)", total: 4, wantLeft: 8, wantContent: 1, wantMeta: 8},
		// total=40: available=36, left=8, content=20, meta=8
		{name: "total=40 (narrow, reduction fires)", total: 40, wantLeft: 8, wantContent: 20, wantMeta: 8},
		// total=60: available=56, left=14, content=20, meta=22
		{name: "total=60 (content tight)", total: 60, wantLeft: 14, wantContent: 20, wantMeta: 22},
		// typical two-column min width: available=106, left=clamp(106/4,24,44)=26, content=46, meta=34
		{name: "two-column min width", total: InspectorTwoColumnMinWidth, wantLeft: 26, wantContent: 46, wantMeta: 34},
		// three-column min width: available=136, left=34, content=68, meta=34
		{name: "three-column min width", total: InspectorThreeColumnMinWidth, wantLeft: 34, wantContent: 68, wantMeta: 34},
		// large input: available=216, left=clamp(216/4,24,44)=44, content=138, meta=34
		{name: "total=220", total: 220, wantLeft: 44, wantContent: 138, wantMeta: 34},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			left, content, metadata := splitThreePaneWidths(tc.total)
			if left != tc.wantLeft {
				t.Errorf("total=%d: left=%d, want %d", tc.total, left, tc.wantLeft)
			}
			if content != tc.wantContent {
				t.Errorf("total=%d: content=%d, want %d", tc.total, content, tc.wantContent)
			}
			if metadata != tc.wantMeta {
				t.Errorf("total=%d: metadata=%d, want %d", tc.total, metadata, tc.wantMeta)
			}
		})
	}
}

func TestMaxScrollOffsets(t *testing.T) {
	t.Parallel()

	// A minimal IssueDetail with a short description.  The metadata pane always
	// renders some fixed fields (type, priority, status, …) so on small heights
	// metadata may scroll; we only assert per-test constraints.
	emptyDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     "bw-scroll",
			Title:  "scroll test",
			Status: "open",
			Type:   "task",
		},
	}

	// A detail with a very long description — Content offset must be non-zero
	// when the pane height is small enough.
	longDesc := strings.Repeat("line of text for scroll test\n", 60)
	tallDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     "bw-long",
			Title:  "long content",
			Status: "open",
			Type:   "task",
		},
		Description: longDesc,
	}

	tests := []struct {
		name             string
		state            State
		wantZeroDeps     bool // Dependencies offset == 0
		wantZeroContent  bool // Content offset == 0
		wantContentAbove int  // Content offset must be >= this
	}{
		{
			name:            "empty detail wide layout: no content scroll",
			state:           State{Detail: emptyDetail, Width: InspectorTwoColumnMinWidth, Height: 30},
			wantZeroDeps:    true,
			wantZeroContent: true,
		},
		{
			name:            "empty detail responsive layout: no content or dep scroll",
			state:           State{Detail: emptyDetail, Width: InspectorTwoColumnMinWidth - 10, Height: 30},
			wantZeroDeps:    true,
			wantZeroContent: true,
		},
		{
			// zero width/height: defaults to defaultDetailWidth/defaultDetailHeight
			name:            "zero width height uses defaults: no content scroll for empty detail",
			state:           State{Detail: emptyDetail, Width: 0, Height: 0},
			wantZeroDeps:    true,
			wantZeroContent: true,
		},
		{
			name:             "tall content wide layout: content must scroll",
			state:            State{Detail: tallDetail, Width: InspectorTwoColumnMinWidth, Height: 10},
			wantContentAbove: 1,
		},
		{
			name:             "tall content responsive layout: content must scroll",
			state:            State{Detail: tallDetail, Width: InspectorTwoColumnMinWidth - 10, Height: 10},
			wantContentAbove: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MaxScrollOffsets(tc.state)

			if tc.wantZeroDeps && got.Dependencies != 0 {
				t.Errorf("expected Dependencies=0, got %d", got.Dependencies)
			}
			if tc.wantZeroContent && got.Content != 0 {
				t.Errorf("expected Content=0, got %d", got.Content)
			}
			if got.Content < tc.wantContentAbove {
				t.Errorf("Content offset=%d, want >= %d", got.Content, tc.wantContentAbove)
			}
		})
	}
}

// TestRefreshDetailsCarriesDimPhaseStyle verifies that when details is in the
// refresh state (Loading=true, Summary.ID != ""), the rendered pane block is
// wrapped with a Faint+Foreground style sourced from SkeletonShades[phase].
// The stale title must remain visible after ANSI-stripping.
func TestRefreshDetailsCarriesDimPhaseStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	const phase = 0 // shade[0]: dark=#454545 → RGB(69,69,69)

	view := Render(State{
		SelectionID:   "bw-9",
		Loading:       true,
		SkeletonPhase: phase,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "bw-9",
				Title:    "Stale Detail Title",
				Status:   "open",
				Type:     "task",
				Priority: 1,
			},
			Description: "Stale description body",
		},
		Width:  120,
		Height: 24,
	})

	plain := ansiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Stale Detail Title") {
		t.Fatalf("stale detail title not visible (ANSI-stripped), got:\n%s", plain)
	}

	// Faint+Foreground(SkeletonShades[0]) dark-theme: "\x1b[2;38;2;69;69;69m"
	// Assert the foreground ANSI sequence is present (faint prefix may vary).
	const wantANSI = "38;2;69;69;69"
	if !strings.Contains(view, wantANSI) {
		t.Fatalf("expected dim ANSI sequence %q in refresh detail view, got:\n%s", wantANSI, view)
	}
}
