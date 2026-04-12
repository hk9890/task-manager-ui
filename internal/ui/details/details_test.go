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
		Width: InspectorThreeColumnMinWidth,
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

	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected rendered output at width %d", InspectorTwoColumnMinWidth)
	}
	if !strings.Contains(lines[0], "Metadata") {
		t.Fatalf("expected first row to include metadata rail at width %d, got:\n%s", InspectorTwoColumnMinWidth, view)
	}
	if !strings.Contains(view, "Description") {
		t.Fatalf("expected content pane section at width %d, got:\n%s", InspectorTwoColumnMinWidth, view)
	}
}

func TestRenderFallsBackToSingleColumnBelowBreakpoint(t *testing.T) {
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

	if strings.Contains(view, "Description  Metadata") {
		t.Fatalf("expected no two-column header row below breakpoint, got:\n%s", view)
	}

	if !strings.Contains(view, "\nMetadata\n") {
		t.Fatalf("expected single-column metadata section below breakpoint, got:\n%s", view)
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
	if left < 22 || left > 28 {
		t.Fatalf("expected left rail in [22,28], got %d", left)
	}
	if metadata != 34 {
		t.Fatalf("expected metadata rail width 34, got %d", metadata)
	}
}

func TestRenderWideLayoutSuppressesInlineRelatedWork(t *testing.T) {
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

	if strings.Contains(view, "\nRelated Work\n") {
		t.Fatalf("expected inline related work section to be suppressed in wide layout, got:\n%s", view)
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

func TestRenderKeepsCommentBodiesPlainText(t *testing.T) {
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

	if !strings.Contains(view, "  - literal markdown-like bullet") {
		t.Fatalf("expected comment body to remain plain text, got:\n%s", view)
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
