package details

import (
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestModelViewRendersRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("no selection", func(t *testing.T) {
		t.Parallel()

		m := Model{}
		view := m.View(100, false)
		if !strings.Contains(view, "No selected issue.") {
			t.Fatalf("expected no-selection state, got:\n%s", view)
		}
	})

	t.Run("loading", func(t *testing.T) {
		t.Parallel()

		m := Model{SelectionID: "bw-2", TargetID: "bw-2", Loading: true}
		view := m.View(100, false)
		if !strings.Contains(view, "Loading details for") || !strings.Contains(view, "bw-2") {
			t.Fatalf("expected loading detail state, got:\n%s", view)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		m := Model{SelectionID: "bw-2", Error: "boom"}
		view := m.View(100, false)
		if !strings.Contains(view, "Failed to load details for bw-2") || !strings.Contains(view, "boom") {
			t.Fatalf("expected detail error state, got:\n%s", view)
		}
	})
}

func TestModelViewSelectionChangeRendersSelectedIssueDetail(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-2",
		TargetID:    "bw-2",
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-2", Title: "Second issue", Status: "in_progress", Type: "task", Priority: 2},
		},
	}

	view := m.View(100, false)
	if !strings.Contains(view, "Second issue") || !strings.Contains(view, "bw-2 · in_progress · task · P2") {
		t.Fatalf("expected bw-2 detail rendering, got:\n%s", view)
	}

	// Simulate shell selection change to a different issue and loaded detail update.
	m.SelectionID = "bw-4"
	m.TargetID = "bw-4"
	m.Detail = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-4", Title: "Fourth issue", Status: "open", Type: "bug", Priority: 1},
	}

	view = m.View(100, false)
	if !strings.Contains(view, "Fourth issue") || !strings.Contains(view, "bw-4 · open · bug · P1") {
		t.Fatalf("expected bw-4 detail rendering after selection change, got:\n%s", view)
	}
	if strings.Contains(view, "bw-2 · in_progress · task · P2") {
		t.Fatalf("expected previous detail bw-2 to be replaced, got:\n%s", view)
	}
}
