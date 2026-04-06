package details

import (
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/ui"
)

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

	ui.AssertMatchesGolden(t, []byte(view), "minimal.golden")
}

func TestRenderFullGolden(t *testing.T) {
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
			Notes:       "Reference donor rendering patterns only.",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "reviewer", Body: "Looks good to me", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
			},
			BlockedBy: []domain.IssueReference{{ID: "bw-10", Title: "Data model update"}},
			Blocks:    []domain.IssueReference{{ID: "bw-30", Title: "Integration checks"}},
			Related:   []domain.IssueReference{{ID: "bw-44", Title: "Renderer cleanup"}},
		},
		Width: 120,
	})

	ui.AssertMatchesGolden(t, []byte(view), "full.golden")
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

	ui.AssertMatchesGolden(t, []byte(view), "comments.golden")
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

	ui.AssertMatchesGolden(t, []byte(view), "dependency_rich.golden")
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

	ui.AssertMatchesGolden(t, []byte(view), "compact.golden")
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}

	return ts
}
