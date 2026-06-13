package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestSearchPostSubmitState_StaleMarkersClearedWhenAppliedQueryMatches pins
// beads-workbench-znri.6: after Enter is processed and a successful
// searchLoadedMsg arrives, the rendered view must not still show the
// "draft" / "stale" markers that are only meant to appear when the user's
// typed draft differs from the last applied query.
//
// State machine invariant: after a successful submit + response, the
// rendered View must contain neither the queryStatusBadge "draft" badge nor
// the "Results below are stale" banner — those should only appear AFTER
// the user has typed additional characters that diverge from the applied
// query.
func TestSearchPostSubmitState_StaleMarkersClearedWhenAppliedQueryMatches(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Backend search candidate", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "bw-2", Title: "Backend implementation", Status: "open", Type: "task", Priority: 2})

	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("backend")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Sanity: results loaded and applied query matches draft.
	if m.appliedQuery != "backend" {
		t.Fatalf("expected appliedQuery=%q after Enter, got %q", "backend", m.appliedQuery)
	}
	if m.draftQuery != "backend" {
		t.Fatalf("expected draftQuery=%q after Enter, got %q", "backend", m.draftQuery)
	}

	view := m.View(0)

	if strings.Contains(view, "Results below are stale") {
		t.Fatalf("expected 'stale' banner NOT to appear after successful submit; view:\n%s", view)
	}

	// The queryStatusBadge renders "draft" inside the Search section header
	// when the draft diverges from the applied query. After Enter, draft and
	// applied are equal, so the badge should be "shown" or empty — never
	// "draft". Use a fragment check that only matches the badge cell.
	if strings.Contains(view, "── draft ─") {
		t.Fatalf("expected Search section header NOT to show 'draft' badge after successful submit; view:\n%s", view)
	}
}

// TestSearchPostSubmitState_StaleMarkerReappearsWhenUserTypesAfterSubmit
// is the companion regression guard: the stale affordance MUST keep
// working when the user resumes typing after a submit. This prevents a
// fix to the post-submit case from accidentally removing the stale
// banner entirely.
func TestSearchPostSubmitState_StaleMarkerReappearsWhenUserTypesAfterSubmit(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Backend search candidate", Status: "open", Type: "task", Priority: 1})

	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("backend")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Now type one more character — draft diverges from applied.
	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)

	if m.draftQuery == m.appliedQuery {
		t.Fatalf("setup: expected draftQuery to diverge from appliedQuery after typing; got equal %q", m.draftQuery)
	}

	view := m.View(0)
	if !strings.Contains(view, "Results below are stale") {
		t.Fatalf("expected 'stale' banner to appear when draft diverges from applied; view:\n%s", view)
	}
}
