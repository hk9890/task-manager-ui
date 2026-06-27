package details

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

func assertGolden(t *testing.T, output []byte, name string) {
	t.Helper()

	if os.Getenv("TASKMGR_UI_UPDATE_GOLDEN") == "1" {
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
		SelectionID: "tm-1",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-1",
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
		SelectionID: "tm-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:        "tm-22",
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
			BlockedBy: []domain.IssueReference{{ID: "tm-10", Title: "Data model update"}},
			Blocks:    []domain.IssueReference{{ID: "tm-30", Title: "Integration checks"}},
			Related:   []domain.IssueReference{{ID: "tm-44", Title: "Renderer cleanup"}},
		},
		Width: 120,
	})

	assertGolden(t, []byte(view), "full.golden")
}

func TestRenderCommentsGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-77",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-77",
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
		SelectionID: "tm-88",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-88",
				Title:    "Dependency rich issue",
				Status:   "blocked",
				Type:     "task",
				Priority: 1,
			},
			Description: "Dependency context check",
			BlockedBy: []domain.IssueReference{
				{ID: "tm-5", Title: "Upstream gate"},
				{ID: "tm-1", Title: "Auth migration"},
			},
			Blocks: []domain.IssueReference{
				{ID: "tm-12", Title: "Release docs"},
				{ID: "tm-9", Title: "UI polish"},
			},
			Related: []domain.IssueReference{
				{ID: "tm-100", Title: "Planning umbrella"},
				{ID: "tm-42", Title: "Search sync"},
			},
		},
		Width: 100,
	})

	assertGolden(t, []byte(view), "dependency_rich.golden")
}

func TestRenderDependencyRowsHighlightSelectedIssue(t *testing.T) {
	// Use TrueColor so the subject background style would emit ANSI escapes if present.
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	view := Render(State{
		SelectionID: "tm-88",
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "tm-88", Title: "Dependency rich issue", Status: "blocked", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{{ID: "tm-1", Title: "Auth migration"}},
			Blocks:    []domain.IssueReference{{ID: "tm-9", Title: "UI polish"}},
			Related:   []domain.IssueReference{{ID: "tm-42", Title: "Search sync"}},
		},
		BrowserSelectedIssueID: "tm-9",
		Width:                  100,
	})

	// The movable cursor row uses the app-wide "› " selection prefix.
	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "› ") {
		t.Fatalf("expected cursor row to carry the › selection prefix, got:\n%s", plain)
	}
	if !strings.Contains(plain, "tm-9") {
		t.Fatalf("expected cursor issue tm-9 to appear in deps pane, got:\n%s", plain)
	}
}

func TestRenderCompactGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-22",
				Title:    "Full detail sample",
				Status:   "in_progress",
				Type:     "feature",
				Priority: 2,
				Assignee: "alice",
				Labels:   []string{"backend", "ui"},
			},
			Description: "Ship issue detail rendering for standalone mode.\nKeep shell-owned selection state.",
			Comments:    []domain.IssueComment{{ID: "c-1", Author: "reviewer", Body: "Looks good to me", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")}},
			BlockedBy:   []domain.IssueReference{{ID: "tm-10", Title: "Data model update"}},
			Blocks:      []domain.IssueReference{{ID: "tm-30", Title: "Integration checks"}},
			Related:     []domain.IssueReference{{ID: "tm-44", Title: "Renderer cleanup"}},
		},
		Width:   56,
		Compact: true,
	})

	assertGolden(t, []byte(view), "compact.golden")
}

func TestRenderCompactClosedDurationGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-23",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:        "tm-23",
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

func TestRenderCompactWithChildrenGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-epic-compact",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-epic-compact",
				Title:    "Epic with children compact",
				Status:   "in_progress",
				Type:     "epic",
				Priority: 1,
				Assignee: "alice",
			},
			Description: "An epic summary for the compact surface.",
			BlockedBy:   []domain.IssueReference{{ID: "tm-10", Title: "Gate issue"}},
			Children: []domain.IssueReference{
				{ID: "tm-c1", Title: "Child one", Type: "task", Priority: 2, Status: "open"},
				{ID: "tm-c2", Title: "Child two", Type: "task", Priority: 2, Status: "in_progress"},
				{ID: "tm-c3", Title: "Child three", Type: "task", Priority: 3, Status: "open"},
			},
		},
		Width:   56,
		Compact: true,
	})

	assertGolden(t, []byte(view), "compact_with_children.golden")
}

func TestRenderWideThreeColumnGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-wide",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-wide",
				Title:    "Wide layout sample",
				Status:   "in_progress",
				Type:     "feature",
				Priority: 1,
			},
			Description: "Three column layout should render related rail on wide terminals.",
			Notes:       "Inline related work section should be suppressed when rail is active.",
			ParentGroupBrowser: domain.ParentGroupBrowserContext{
				Parent: domain.IssueReference{ID: "tm-parent", Title: "Parent epic"},
			},
			BlockedBy: []domain.IssueReference{
				{ID: "tm-1", Title: "Auth migration", Type: "task", Priority: 1, Status: "blocked"},
			},
			Blocks: []domain.IssueReference{
				{ID: "tm-2", Title: "Docs update", Type: "docs", Priority: 2, Status: "open"},
			},
			Related: []domain.IssueReference{
				{ID: "tm-3", Title: "Renderer cleanup", Type: "chore", Priority: 3, Status: "open"},
			},
		},
		// The currently-viewed issue (tm-wide) is excluded from the browser panel;
		// the navigable list is the flattened dependency rows followed by the
		// parent row (no siblings). The cursor sits on the parent.
		BrowserItems: []domain.IssueReference{
			{ID: "tm-1", Title: "Auth migration"},
			{ID: "tm-2", Title: "Docs update"},
			{ID: "tm-3", Title: "Renderer cleanup"},
			{ID: "tm-parent", Title: "Parent epic"},
		},
		BrowserSelectedIssueID: "tm-parent",
		Width:                  InspectorThreeColumnMinWidth,
	})

	assertGolden(t, []byte(view), "wide_three_column.golden")
}

func TestRenderFallbackKeepsInlineRelatedWorkGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-fallback",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-fallback",
				Title:    "Fallback inline related work",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			},
			Description: "Below wide breakpoint should keep inline related work section.",
			BlockedBy:   []domain.IssueReference{{ID: "tm-11", Title: "Dependency A"}},
			Blocks:      []domain.IssueReference{{ID: "tm-12", Title: "Dependency B"}},
			Related:     []domain.IssueReference{{ID: "tm-13", Title: "Dependency C"}},
		},
		Width: InspectorThreeColumnMinWidth - 1,
	})

	assertGolden(t, []byte(view), "fallback_inline_related_work.golden")
}

func TestRenderUsesTwoColumnInspectorAtBreakpoint(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-22",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-22",
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
		SelectionID: "tm-23",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-23",
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
		SelectionID: "tm-rail",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-rail", Title: "Rail", Status: "open", Type: "task", Priority: 1},
			Description: "desc",
			BlockedBy:   []domain.IssueReference{{ID: "tm-1", Title: "A"}},
			Related:     []domain.IssueReference{{ID: "tm-2", Title: "B"}},
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
		SelectionID: "tm-child",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-child", Title: "Child", Status: "open", Type: "task", Priority: 1},
			Description: "desc",
			ParentGroupBrowser: domain.ParentGroupBrowserContext{
				Parent: domain.IssueReference{ID: "tm-parent", Title: "Parent"},
			},
		},
		// Parent-only: the navigable list contains just the parent row; the
		// cursor sits on it. Siblings/self are no longer surfaced.
		BrowserItems: []domain.IssueReference{
			{ID: "tm-parent", Title: "Parent"},
		},
		BrowserSelectedIssueID: "tm-parent",
		Width:                  InspectorThreeColumnMinWidth,
	})

	if !strings.Contains(view, "Dependencies") {
		t.Fatalf("expected left dependencies panel in wide layout, got:\n%s", view)
	}
	if !strings.Contains(view, "Parent (1)") {
		t.Fatalf("expected parent group (parent only, no siblings) in dependencies panel, got:\n%s", view)
	}
}

func TestRenderDependenciesOmitsParentGroupWhenNoParent(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-main",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-main", Title: "Main", Status: "open", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{
				{ID: "tm-1", Title: "A"},
			},
			Blocks: []domain.IssueReference{
				{ID: "tm-2", Title: "B"},
			},
			Related: []domain.IssueReference{
				{ID: "tm-3", Title: "C"},
			},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "tm-1", Title: "A"},
			{ID: "tm-2", Title: "B"},
			{ID: "tm-3", Title: "C"},
		},
		Width: InspectorThreeColumnMinWidth,
	})

	if strings.Contains(view, "Parent (") {
		t.Fatalf("expected dependency-only view to omit the parent group, got:\n%s", view)
	}
}

func TestDependencyGroupsDeduplicateIssueIDsAcrossVisibleGroups(t *testing.T) {
	t.Parallel()

	groups := dependencyGroups(domain.IssueDetail{
		BlockedBy: []domain.IssueReference{{ID: "tm-1", Title: "Blocker"}},
		Blocks:    []domain.IssueReference{{ID: "tm-2", Title: "Blocked child"}},
		Related: []domain.IssueReference{
			{ID: "tm-1", Title: "Duplicate from blocked-by"},
			{ID: "tm-3", Title: "Related unique"},
		},
	}, nil)

	// Now 4 groups: Blocked by, Blocks, Related, Children (Children is empty here).
	if got := len(groups); got != 4 {
		t.Fatalf("expected 4 dependency groups, got %d", got)
	}
	if got := len(groups[0].Refs); got != 1 || groups[0].Refs[0].ID != "tm-1" {
		t.Fatalf("expected blocked-by to keep tm-1 once, got %#v", groups[0].Refs)
	}
	if got := len(groups[1].Refs); got != 1 || groups[1].Refs[0].ID != "tm-2" {
		t.Fatalf("expected blocks to keep tm-2 once, got %#v", groups[1].Refs)
	}
	if got := len(groups[2].Refs); got != 1 || groups[2].Refs[0].ID != "tm-3" {
		t.Fatalf("expected related to drop duplicate tm-1 and keep tm-3, got %#v", groups[2].Refs)
	}
	if got := len(groups[3].Refs); got != 0 {
		t.Fatalf("expected children to be empty (none provided), got %#v", groups[3].Refs)
	}
}

func TestRenderDependenciesPaneLinesDoNotRenderDuplicateIssueRowsAcrossGroups(t *testing.T) {
	t.Parallel()

	lines := renderDependenciesPaneLines(domain.IssueDetail{
		BlockedBy: []domain.IssueReference{{ID: "tm-1", Title: "Blocker"}},
		Blocks:    []domain.IssueReference{{ID: "tm-2", Title: "Blocked child"}},
		Related: []domain.IssueReference{
			{ID: "tm-1", Title: "Duplicate from blocked-by"},
			{ID: "tm-3", Title: "Related unique"},
		},
	}, nil, "tm-3", 80, false, 0)

	joined := strings.Join(lines, "\n")
	if got := strings.Count(joined, "tm-1"); got != 1 {
		t.Fatalf("expected duplicate dependency row tm-1 to render once, got %d occurrences in:\n%s", got, joined)
	}
	if !strings.Contains(joined, "Related (1)") {
		t.Fatalf("expected related group count to reflect de-duplicated rows, got:\n%s", joined)
	}
}

func TestRenderUsesMarkdownRendererForDescriptionAndNotes(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-90",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-90",
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

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	for _, want := range []string{"Ship markdown", "first", "Follow up", "link"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in rendered markdown detail:\n%s", want, plain)
		}
	}
}

func TestRenderUsesMarkdownRendererForCommentBodies(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-91",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-91", Title: "Comment plain text", Status: "open", Type: "task", Priority: 1},
			Description: "Plain",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "alice", Body: "- literal markdown-like bullet", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
			},
		},
		Width:  100,
		Height: 40,
	})

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
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
		bodyLines = append(bodyLines, "FAIL\tgithub.com/hk9890/task-manager-ui/internal/ui/details\t0.123s")
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
		SelectionID: "tm-ansi-overflow",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-ansi-overflow", Title: "ANSI bounded comments", Status: "open", Type: "bug", Priority: 1},
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
			t.Fatalf("line %d exceeds detail width (%d > %d): %q", i+1, got, state.Width, ui.AnsiEscapePattern.ReplaceAllString(line, ""))
		}
	}

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Comments (1 · newest first)") {
		t.Fatalf("expected comments section to render, got:\n%s", plain)
	}
}

func TestRenderMetadataUsesConfiguredQuickActionLabels(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-qa",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-qa", Title: "Quick actions", Status: "open", Type: "task", Priority: 1},
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

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	for _, want := range []string{"ctrl+e Edit issue", "ctrl+u Update issue", "ctrl+a Add comment", "ctrl+x Close issue", "ctrl+r Reload detail"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected configured quick action label %q in view:\n%s", want, plain)
		}
	}
}

func TestRenderReturnsFallbackBelowMinWidth(t *testing.T) {
	t.Parallel()

	state := State{
		SelectionID: "tm-narrow",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "tm-narrow",
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
		SelectionID: "tm-minwidth",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "tm-minwidth",
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
			ID:       "tm-99",
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
				SelectionID: "tm-99",
				TargetID:    "tm-99",
				Loading:     true,
				Width:       tc.width,
				Height:      tc.height,
			})

			// Loaded render: Loading=false, full detail present.
			loadedView := Render(State{
				SelectionID: "tm-99",
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
				SelectionID: "tm-99",
				TargetID:    "tm-99",
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

// TestRenderContentPaneLinesPlaceholderSuppressesMetaRowAndRule verifies that
// the search "no selection" placeholder (ID="(none)", Type="") renders WITHOUT
// the dashboard-style meta row or the thin rule, while a real issue summary
// still renders WITH them.
func TestRenderContentPaneLinesPlaceholderSuppressesMetaRowAndRule(t *testing.T) {
	t.Parallel()

	t.Run("placeholder omits meta row and rule", func(t *testing.T) {
		t.Parallel()

		placeholderDetail := domain.IssueDetail{
			Summary: domain.IssueSummary{
				Title:    "No selected result.",
				ID:       "(none)",
				Status:   "(none)",
				Type:     "",
				Priority: -1,
			},
			Description: "Select a result in the search rail to preview issue content.",
		}

		lines := renderContentPaneLines(placeholderDetail, 80, 20, false, 0)
		joined := strings.Join(lines, "\n")

		// Must NOT contain the junk meta row tokens.
		for _, unwanted := range []string{"P0", "(NO", "(none)"} {
			if strings.Contains(joined, unwanted) {
				t.Fatalf("placeholder content pane must not contain %q; got:\n%s", unwanted, joined)
			}
		}
		// Must NOT contain the thin rule character.
		if strings.Contains(joined, "─") {
			t.Fatalf("placeholder content pane must not contain thin rule; got:\n%s", joined)
		}
		// Must contain the placeholder title.
		if !strings.Contains(joined, "No selected result.") {
			t.Fatalf("placeholder content pane must contain the title; got:\n%s", joined)
		}
	})

	t.Run("real issue retains meta row and rule", func(t *testing.T) {
		t.Parallel()

		realDetail := domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-test",
				Title:    "Real issue title",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			},
			Description: "Real description.",
		}

		lines := renderContentPaneLines(realDetail, 80, 20, false, 0)
		joined := strings.Join(lines, "\n")

		// Must contain the thin rule (header separator).
		if !strings.Contains(joined, "─") {
			t.Fatalf("real issue content pane must contain the thin rule; got:\n%s", joined)
		}
		// Must contain the issue ID in the meta row.
		if !strings.Contains(joined, "tm-test") {
			t.Fatalf("real issue content pane must contain the issue ID; got:\n%s", joined)
		}
	})
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
		{name: "zero triggers default", total: 0, wantContent: 14, wantBottom: 10},
		{name: "negative triggers default", total: -5, wantContent: 14, wantBottom: 10},
		{name: "total=1 (tiny)", total: 1, wantContent: 1, wantBottom: 0},
		{name: "total=3", total: 3, wantContent: 1, wantBottom: 2},
		{name: "total=6 (boundary)", total: 6, wantContent: 3, wantBottom: 3},
		{name: "total=7 (just above small branch)", total: 7, wantContent: 3, wantBottom: 4},
		{name: "total=10 (bottom shift needed)", total: 10, wantContent: 4, wantBottom: 6},
		{name: "total=14 (tight bottom)", total: 14, wantContent: 8, wantBottom: 6},
		{name: "total=24 (typical terminal)", total: 24, wantContent: 14, wantBottom: 10},
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

	// For very small totals, the pane-width floors deliberately exceed `available`
	// — overflow is acceptable; the test pins observed outputs.
	tests := []struct {
		name        string
		total       int
		wantLeft    int
		wantContent int
		wantMeta    int
	}{
		{name: "zero total", total: 0, wantLeft: 8, wantContent: 1, wantMeta: 8},
		{name: "total=1", total: 1, wantLeft: 8, wantContent: 1, wantMeta: 8},
		{name: "total=4 (barely above gap)", total: 4, wantLeft: 8, wantContent: 1, wantMeta: 8},
		{name: "total=40 (narrow, reduction fires)", total: 40, wantLeft: 8, wantContent: 20, wantMeta: 8},
		{name: "total=60 (content tight)", total: 60, wantLeft: 14, wantContent: 20, wantMeta: 22},
		{name: "two-column min width", total: InspectorTwoColumnMinWidth, wantLeft: 26, wantContent: 46, wantMeta: 34},
		{name: "three-column min width", total: InspectorThreeColumnMinWidth, wantLeft: 34, wantContent: 68, wantMeta: 34},
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
			ID:     "tm-scroll",
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
			ID:     "tm-long",
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
			// The responsive bottom pane is shorter (~10 inner rows), and the
			// Children group adds 2 lines (label + "(none)"), so a dep offset > 0
			// is expected for an empty detail in narrow/short viewports.
			name:            "empty detail responsive layout: no content scroll",
			state:           State{Detail: emptyDetail, Width: InspectorTwoColumnMinWidth - 10, Height: 30},
			wantZeroContent: true,
		},
		{
			// zero width/height: defaults to defaultDetailWidth/defaultDetailHeight (80×24).
			// Width 80 < InspectorTwoColumnMinWidth, so responsive layout is used.
			// The small bottom pane means the empty detail deps may be > 0.
			name:            "zero width height uses defaults: no content scroll for empty detail",
			state:           State{Detail: emptyDetail, Width: 0, Height: 0},
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
		SelectionID:   "tm-9",
		Loading:       true,
		SkeletonPhase: phase,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-9",
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

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
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

// TestSkeletonRenderDoesNotShowLiteralZeroOrNoneInCountsAndDeps is a regression
// test for the skeleton-counts regression: with state.Skeleton==true, the Counts panel
// must not render literal "0" values and the Dependencies pane must not render
// "(none)" — both must use placeholder glyphs instead.
func TestSkeletonRenderDoesNotShowLiteralZeroOrNoneInCountsAndDeps(t *testing.T) {
	t.Parallel()

	// Use the cold-start skeleton path: Loading=true, Skeleton=true, no prior detail.
	view := Render(State{
		SelectionID: "tm-sk",
		TargetID:    "tm-sk",
		Loading:     true,
		Skeleton:    true,
		Width:       180,
		Height:      30,
	})

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Counts panel must not show literal "0" values for any count field.
	for _, forbidden := range []string{"Comments: 0", "Blocked by: 0", "Blocks: 0", "Related: 0"} {
		if strings.Contains(plain, forbidden) {
			t.Errorf("expected no literal zero count in skeleton render, but found %q:\n%s", forbidden, plain)
		}
	}

	// Dependencies pane must not show "(none)" — should show skeleton glyphs instead.
	if strings.Contains(plain, "(none)") {
		t.Errorf("expected no '(none)' in skeleton render dependencies pane, got:\n%s", plain)
	}

	// Content pane TopRight must not show "0 comments" — should show skeleton glyphs.
	if strings.Contains(plain, "0 comments") {
		t.Errorf("expected no '0 comments' in skeleton render content pane header, got:\n%s", plain)
	}

	// Skeleton glyph must be present (confirms loading-state treatment is active).
	if !strings.Contains(plain, issuerow.SkeletonGlyph) {
		t.Errorf("expected skeleton glyph %q in skeleton render, got:\n%s", issuerow.SkeletonGlyph, plain)
	}
}

// TestSkeletonRenderAtTwoColumnWidthDoesNotShowLiteralZeroOrNone tests the same
// invariant at a width that uses the responsive (two-pane bottom) layout path.
func TestSkeletonRenderAtTwoColumnWidthDoesNotShowLiteralZeroOrNone(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-sk2",
		TargetID:    "tm-sk2",
		Loading:     true,
		Skeleton:    true,
		Width:       InspectorTwoColumnMinWidth,
		Height:      30,
	})

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")

	for _, forbidden := range []string{"Comments: 0", "Blocked by: 0", "Blocks: 0", "Related: 0"} {
		if strings.Contains(plain, forbidden) {
			t.Errorf("expected no literal zero count in two-column skeleton render, but found %q:\n%s", forbidden, plain)
		}
	}

	if strings.Contains(plain, "(none)") {
		t.Errorf("expected no '(none)' in two-column skeleton render, got:\n%s", plain)
	}
}

// TestSkeletonFalseStillShowsRealCountsAndDeps ensures the non-skeleton path
// continues to render real data correctly (regression guard for Skeleton=false).
func TestSkeletonFalseStillShowsRealCountsAndDeps(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-real",
		Skeleton:    false,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-real",
				Title:    "Real detail",
				Status:   "blocked",
				Type:     "task",
				Priority: 2,
			},
			Comments:  []domain.IssueComment{{ID: "c-1"}, {ID: "c-2"}},
			BlockedBy: []domain.IssueReference{{ID: "tm-dep", Title: "Dep"}},
		},
		Width:  180,
		Height: 30,
	})

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Counts section uses aligned label format, e.g. "Comments  : 2" — check label and value separately.
	if !strings.Contains(plain, "Comments") || !strings.Contains(plain, ": 2") {
		t.Errorf("expected real comment count '2' when skeleton=false, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Blocked by") || !strings.Contains(plain, ": 1") {
		t.Errorf("expected real blocked-by count '1' when skeleton=false, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Blocked by (1)") {
		t.Errorf("expected real dep header 'Blocked by (1)' when skeleton=false, got:\n%s", plain)
	}
	// Skeleton glyph must NOT appear as a count placeholder in the Counts section.
	if strings.Contains(plain, ": "+issuerow.SkeletonGlyph) {
		t.Errorf("expected no skeleton glyphs in non-skeleton render Counts, but found in:\n%s", plain)
	}
}

// TestRenderLongDepsWindowGolden verifies that a deps pane with more rows than
// the inner window shows "N of M" in the header after a non-zero scroll offset.
func TestRenderLongDepsWindowGolden(t *testing.T) {
	t.Parallel()

	// Build 30 dep refs: 10 BlockedBy + 10 Blocks + 10 Related.
	makeRefs := func(prefix string, n int) []domain.IssueReference {
		refs := make([]domain.IssueReference, n)
		for i := range refs {
			refs[i] = domain.IssueReference{
				ID:    fmt.Sprintf("tm-%s%02d", prefix, i),
				Title: fmt.Sprintf("%s issue %d", prefix, i),
			}
		}
		return refs
	}

	detail := domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-main", Title: "Main issue", Status: "open", Type: "task", Priority: 1},
		BlockedBy: makeRefs("bb", 10),
		Blocks:    makeRefs("bl", 10),
		Related:   makeRefs("re", 10),
	}

	// Use three-pane width and small height so the deps pane is clipped.
	// At height=15, innerHeight=13; with 30 deps + 4 group headers + 3 separators
	// = 37 rendered lines, only 13 are visible.
	view := Render(State{
		SelectionID:              "tm-main",
		Detail:                   detail,
		BrowserItems:             append(append(detail.BlockedBy, detail.Blocks...), detail.Related...),
		BrowserSelectedIssueID:   "tm-bb00",
		DependenciesScrollOffset: 10,
		Width:                    InspectorThreeColumnMinWidth,
		Height:                   15,
	})

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Header must show "N of M" since the window clips.
	if !strings.Contains(plain, "of") {
		t.Fatalf("expected 'N of M' in Dependencies header when window clips, got:\n%s", plain)
	}

	assertGolden(t, []byte(view), "long_deps_window.golden")
}

// TestDependencyRefLineIndexChildrenConsistency asserts that BrowserSelectedIndex
// ↔ DependencyRefLineIndex returns the correct rendered-line position when a
// Children group is present. This is the key correctness risk from plan-review Q2:
// if DependencyRefLineIndex were not updated, arrow navigation onto a child would
// scroll to the wrong line.
//
// The test uses IDs that are not in input order to catch sort-mismatch bugs
// between orderedReferences (used by (a)/(c)) and sort.SliceStable (used by (b)).
func TestDependencyRefLineIndexChildrenConsistency(t *testing.T) {
	t.Parallel()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-parent", Title: "Parent epic", Status: "open", Type: "epic", Priority: 1},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-z9", Title: "Blocker Z"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-a1", Title: "Downstream A"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-m5", Title: "Related M"},
		},
		// Children deliberately out of input order to expose sort-mismatch bugs.
		Children: []domain.IssueReference{
			{ID: "tm-c3", Title: "Child C (third)"},
			{ID: "tm-c1", Title: "Child C (first)"},
			{ID: "tm-c2", Title: "Child C (second)"},
		},
	}

	// browserItems is the flat list in group/sort order matching (b).
	// Order: BlockedBy asc, Blocks asc, Related asc, Children asc.
	// BlockedBy: tm-z9
	// Blocks: tm-a1
	// Related: tm-m5
	// Children sorted asc: tm-c1, tm-c2, tm-c3
	browserItems := []domain.IssueReference{
		{ID: "tm-z9", Title: "Blocker Z"},
		{ID: "tm-a1", Title: "Downstream A"},
		{ID: "tm-m5", Title: "Related M"},
		{ID: "tm-c1", Title: "Child C (first)"},
		{ID: "tm-c2", Title: "Child C (second)"},
		{ID: "tm-c3", Title: "Child C (third)"},
	}

	// Verify every BrowserSelectedIndex maps to a rendered-line that contains
	// the expected issue ID.
	lines := renderDependenciesPaneLines(detail, browserItems, "", 80, false, 0)
	joinedLines := strings.Join(lines, "\n")

	for i, ref := range browserItems {
		lineIdx := DependencyRefLineIndex(i, browserItems, detail)
		if lineIdx < 0 || lineIdx >= len(lines) {
			t.Errorf("browserItems[%d] (ID=%q): DependencyRefLineIndex=%d out of range [0,%d)",
				i, ref.ID, lineIdx, len(lines))
			continue
		}
		if !strings.Contains(lines[lineIdx], ref.ID) {
			t.Errorf("browserItems[%d] (ID=%q): DependencyRefLineIndex=%d points to line %q, expected to contain %q\nAll lines:\n%s",
				i, ref.ID, lineIdx, lines[lineIdx], ref.ID, joinedLines)
		}
	}

	// Verify the Children group header is present and correctly placed (before the Parent
	// group, which is absent here, per group order: Blocked by, Blocks, Related, Children, Parent).
	if !strings.Contains(joinedLines, "Children (3)") {
		t.Errorf("expected 'Children (3)' in rendered deps pane, got:\n%s", joinedLines)
	}

	// Children group must appear after Related group in the rendered output.
	relatedIdx := strings.Index(joinedLines, "Related (")
	childrenIdx := strings.Index(joinedLines, "Children (")
	if relatedIdx < 0 || childrenIdx < 0 {
		t.Errorf("expected both 'Related (' and 'Children (' in output:\n%s", joinedLines)
	} else if childrenIdx <= relatedIdx {
		t.Errorf("expected Children group after Related group, relatedIdx=%d childrenIdx=%d", relatedIdx, childrenIdx)
	}
}

// TestRenderChildrenGroupGolden verifies rendering of an epic with children
// in the Dependencies pane (placed per Q3 ordering: after Related, before Structure).
func TestRenderChildrenGroupGolden(t *testing.T) {
	t.Parallel()

	view := Render(State{
		SelectionID: "tm-epic",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:       "tm-epic",
				Title:    "Epic with children",
				Status:   "in_progress",
				Type:     "epic",
				Priority: 1,
			},
			Description: "An epic that has child tasks.",
			BlockedBy:   []domain.IssueReference{{ID: "tm-blocker", Title: "Upstream dependency"}},
			Children: []domain.IssueReference{
				{ID: "tm-child1", Title: "First child task", Type: "task", Priority: 2, Status: "open"},
				{ID: "tm-child2", Title: "Second child task", Type: "task", Priority: 2, Status: "in_progress"},
			},
		},
		Width:  InspectorTwoColumnMinWidth,
		Height: 24,
	})

	assertGolden(t, []byte(view), "children_group.golden")
}

// --- Browser-panel cursor golden assertions ---
//
// The browser panel has exactly one marker: the movable cursor, rendered with the
// app-wide "› " selection prefix (identical to board/search/metadata). The
// currently-viewed issue is excluded from the panel entirely (see the model's
// browserItemsFromDependencies), so there is no second "subject" marker.

// TestRenderCursorRowUsesSelectionPrefixGolden: when BrowserSelectedIssueID is set,
// the cursor row carries the app-wide "› " selection prefix and idle rows carry the
// blank gutter.
func TestRenderCursorRowUsesSelectionPrefixGolden(t *testing.T) {
	// Pin the color profile so the golden is deterministic regardless of other
	// tests mutating the global lipgloss profile.
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	// Render a narrow detail (responsive layout) with cursor=tm-cursor.
	view := Render(State{
		SelectionID: "tm-main",
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "tm-main", Title: "Main issue", Status: "open", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{{ID: "tm-cursor", Title: "Cursor issue", Type: "task", Priority: 2, Status: "open"}},
			Blocks:    []domain.IssueReference{{ID: "tm-other", Title: "Other issue", Type: "task", Priority: 3, Status: "open"}},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "tm-cursor", Title: "Cursor issue"},
			{ID: "tm-other", Title: "Other issue"},
		},
		BrowserSelectedIssueID: "tm-cursor", // cursor on this row
		Width:                  InspectorTwoColumnMinWidth,
		Height:                 18,
	})

	assertGolden(t, []byte(view), "cursor_row_selection_prefix.golden")

	// The cursor row must carry the › selection prefix.
	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "›") {
		t.Errorf("expected cursor row to carry the › selection prefix, got:\n%s", plain)
	}
}

// TestRenderEpicChildrenCursorPrefixGolden: opening an epic renders its Children
// group; the cursor row carries the "› " prefix, the epic itself appears only in the
// Content pane (never in the deps pane), and no second marker exists.
func TestRenderEpicChildrenCursorPrefixGolden(t *testing.T) {
	// Pin the color profile so the golden is deterministic regardless of other
	// tests mutating the global lipgloss profile.
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	view := Render(State{
		SelectionID: "tm-epic",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-epic", Title: "Top-level epic", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{
				{ID: "tm-child1", Title: "Child task one", Type: "task", Priority: 2, Status: "open"},
				{ID: "tm-child2", Title: "Child task two", Type: "task", Priority: 2, Status: "open"},
			},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "tm-child1", Title: "Child task one"},
			{ID: "tm-child2", Title: "Child task two"},
		},
		BrowserSelectedIssueID: "tm-child1", // cursor on child1 → carries ›
		Width:                  InspectorThreeColumnMinWidth,
		Height:                 18,
	})

	assertGolden(t, []byte(view), "epic_children_cursor.golden")

	plain := ui.AnsiEscapePattern.ReplaceAllString(view, "")
	// The cursor row (tm-child1) must carry the › selection prefix.
	if !strings.Contains(plain, "›") {
		t.Errorf("expected cursor row (tm-child1) to carry the › selection prefix, got:\n%s", plain)
	}
	// The epic ID must appear in the Content pane (title/summary line).
	if !strings.Contains(plain, "tm-epic") {
		t.Errorf("expected tm-epic to appear in Content pane, got:\n%s", plain)
	}
}

// TestContentBodySkeletonIsProseNotBoardRows verifies the AC for
// the Content body skeleton must NOT produce board
// issue-row shapes (issuerow.RenderCompactSkeleton output) and must include at
// least one blank-line gap separating prose blocks.
//
// Two sub-tests exercise representative widths so both narrow (responsive) and
// wide (three-pane) layout paths are covered.
func TestContentBodySkeletonIsProseNotBoardRows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		width int
		n     int
	}{
		{"narrow", InspectorTwoColumnMinWidth - 10, 18},
		{"wide", InspectorThreeColumnMinWidth + 20, 24},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const phase = 1 // representative mid-cycle phase

			proseLines := renderProseContentSkeleton(tc.width, tc.n, phase)

			// AC: total line count must equal n.
			if len(proseLines) != tc.n {
				t.Fatalf("want %d prose skeleton lines, got %d", tc.n, len(proseLines))
			}

			// AC: at least one blank line must exist to separate prose blocks.
			hasBlank := false
			for _, line := range proseLines {
				if line == "" {
					hasBlank = true
					break
				}
			}
			if !hasBlank {
				t.Errorf("prose skeleton has no blank lines — expected at least one blank-line gap between prose blocks; lines:\n%v", proseLines)
			}

			// AC: no prose skeleton line must equal the corresponding
			// issuerow.RenderCompactSkeleton output (board-row shape).
			for i, line := range proseLines {
				boardRow := issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
					Width:  tc.width,
					Seed:   i,
					Phase:  phase,
					Styled: true,
				})
				if line == boardRow {
					t.Errorf("prose skeleton line %d equals board-row skeleton shape — Content body must not look like board rows; line=%q", i, line)
				}
			}
		})
	}
}
