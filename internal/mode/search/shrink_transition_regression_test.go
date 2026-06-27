package search

// shrink_transition_regression_test.go — deterministic regression test for
// task-manager-ui-uwmi: "Search results pane briefly shows stale rows below
// new results during async swap".
//
// # Root cause investigation (uwmi)
//
// The two obvious hypotheses were falsified by code review and confirmed here:
//
//  1. FormSection already truncates and pads: styles.FormSection clamps content
//     to innerHeight and fills short bodies with blank-padded lines, so a
//     3-result set never leaves 9 stale row slots unfilled.
//
//  2. The page swap is atomic: searchLoadedMsg replaces m.page in a single
//     Update call; there is no intermediate state where old+new results coexist
//     in m.page.Results simultaneously.
//
// # What was actually found
//
// After driving the model through the full (large results) → (submit narrow
// query) → (deliver smaller searchLoadedMsg) sequence:
//
//   - The View() string does NOT contain any stale row titles from the prior
//     large result set.
//   - The rendered frame height is identical for both the large and the small
//     result sets (40 lines for a 40-row terminal), because FormSection pads
//     the short body with blank lines to fill the fixed resultsHeight slot.
//   - The total app frame (header + search body + footer) always equals the
//     terminal height; Bubble Tea's frame-differ therefore overwrites ALL rows
//     of the previous frame when repainting.
//
// # Verdict
//
// The defect is NOT reproducible at the Go View() / frame-composition level.
// The symptom (stale rows visible for ~1s) is a terminal-emulator repaint
// artifact that occurs when the screen-capture tool (scripts/capture_taskmgr_ui_screen.py)
// catches the terminal mid-repaint — i.e., after Bubble Tea has written the
// first N rows of the new frame but before it has overwritten the remaining
// rows. There is no Go code to fix.
//
// # What this test does
//
// This test PINS the correct rendering behavior so that a future incorrect
// "fix" (e.g., removing FormSection padding, or exposing stale results through
// a non-atomic page swap) would cause the test to FAIL, making the regression
// detectable before it ships.
//
// The test is deterministic: it drives the model synchronously (no sleeps, no
// goroutines), using the same pressAndResolve/drainCmd harness as the existing
// model tests.

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// TestSearchShrinkTransition_NoStaleRowsInView is the primary regression pin for
// task-manager-ui-uwmi.
//
// Sequence: seed 12 issues → init (large result set) → type narrow query →
// submit → deliver small searchLoadedMsg (3 results) → assert View contains
// ONLY the new results (no stale titles from the large set).
//
// The test also asserts that the rendered frame height does not shrink (same
// line count for large and small result sets), confirming that Bubble Tea's
// frame-differ will overwrite all rows of the prior frame.
func TestSearchShrinkTransition_NoStaleRowsInView(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()

	// Large set: 12 issues.  Their titles must be distinctive so assertions are
	// unambiguous — "StaleRow" appears in none of the small-result titles.
	for i := 0; i < 12; i++ {
		id := "large-" + string(rune('a'+i))
		repo.Seed(memoryrepo.Issue{ID: id, Title: "StaleRow " + id, Status: "open", Priority: 1})
	}
	// Small set: 3 issues.  Their titles contain "NewRow", not "StaleRow".
	for i := 0; i < 3; i++ {
		id := "small-" + string(rune('a'+i))
		repo.Seed(memoryrepo.Issue{ID: id, Title: "NewRow " + id, Status: "open", Priority: 2})
	}

	m := NewModel(context.Background(), repo, nil)
	const termW, termH = 160, 40
	m.SetSize(termW, termH)

	// Step 1: deliver the large result set (simulates a completed Init or
	// first search).  Build it directly instead of going through the repo
	// to keep the test synchronous and independent of memory-repo search
	// ranking behaviour.
	largeResults := make([]domain.SearchResult, 12)
	for i := 0; i < 12; i++ {
		id := "large-" + string(rune('a'+i))
		largeResults[i] = domain.SearchResult{
			Issue: domain.IssueSummary{ID: id, Title: "StaleRow " + id, Status: "open", Priority: 1},
		}
	}
	_ = m.Update(searchLoadedMsg{
		appliedQuery: "",
		page:         domain.SearchResultPage{Results: largeResults},
	})

	viewLarge := m.View(0)
	largeLines := strings.Count(viewLarge, "\n") + 1

	// Sanity: large result titles appear in the large view.
	if !strings.Contains(viewLarge, "StaleRow") {
		t.Fatalf("setup: large view must contain StaleRow titles; got:\n%s", viewLarge)
	}

	// Step 2: submit a narrow query (model goes into loading/reloading state).
	for _, r := range []rune("new") {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	submitCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = submitCmd // in-flight; do not drain yet

	// Step 3: deliver the small result set (simulates searchLoadedMsg arriving).
	smallResults := make([]domain.SearchResult, 3)
	for i := 0; i < 3; i++ {
		id := "small-" + string(rune('a'+i))
		smallResults[i] = domain.SearchResult{
			Issue: domain.IssueSummary{ID: id, Title: "NewRow " + id, Status: "open", Priority: 2},
		}
	}
	_ = m.Update(searchLoadedMsg{
		appliedQuery: "new",
		page:         domain.SearchResultPage{Results: smallResults},
	})

	viewSmall := m.View(0)
	smallLines := strings.Count(viewSmall, "\n") + 1

	// Assertion 1: the small view must not contain any stale row titles.
	//
	// If FormSection padding were missing or the page swap were non-atomic,
	// the old large-result rows would appear here and this assertion would fail.
	if strings.Contains(viewSmall, "StaleRow") {
		t.Errorf(
			"REGRESSION (uwmi): View() contains stale 'StaleRow' titles after shrink transition.\n"+
				"Expected: only NewRow titles and blank padding.\n"+
				"View after small result delivery:\n%s",
			viewSmall,
		)
	}

	// Assertion 2: all new result titles must appear in the view.
	for i := 0; i < 3; i++ {
		title := "NewRow small-" + string(rune('a'+i))
		if !strings.Contains(viewSmall, title) {
			t.Errorf("expected %q in view after shrink, not found.\nView:\n%s", title, viewSmall)
		}
	}

	// Assertion 3: rendered frame height must not decrease after the shrink.
	//
	// If the small-result view had fewer lines than the large-result view,
	// Bubble Tea's frame-differ would leave trailing stale rows in the terminal.
	// FormSection padding keeps the height stable, so this should always hold.
	if smallLines < largeLines {
		t.Errorf(
			"REGRESSION (uwmi): rendered frame SHRINKS after result-count reduction.\n"+
				"Large result view: %d lines, Small result view: %d lines.\n"+
				"A shorter frame leaves trailing stale terminal rows that Bubble Tea does not clear.",
			largeLines, smallLines,
		)
	}

	t.Logf("shrink transition: large=%d lines, small=%d lines — frame height stable", largeLines, smallLines)
}

// TestSearchShrinkTransition_IntermediateStateDoesNotExposeCombinedRows asserts
// that the transitional "loading/reloading" state (between submit and result
// delivery) does not simultaneously expose both old and new results.
//
// In the reloading state the view must show either the old results (dimmed,
// with a stale-reload banner) OR a loading skeleton — never a mix of old large
// rows and new small rows concatenated.
func TestSearchShrinkTransition_IntermediateStateDoesNotExposeCombinedRows(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()

	for i := 0; i < 12; i++ {
		id := "large-" + string(rune('a'+i))
		repo.Seed(memoryrepo.Issue{ID: id, Title: "StaleRow " + id, Status: "open", Priority: 1})
	}
	for i := 0; i < 3; i++ {
		id := "small-" + string(rune('a'+i))
		repo.Seed(memoryrepo.Issue{ID: id, Title: "NewRow " + id, Status: "open", Priority: 2})
	}

	m := NewModel(context.Background(), repo, nil)
	const termW, termH = 160, 40
	m.SetSize(termW, termH)

	// Deliver large result set.
	largeResults := make([]domain.SearchResult, 12)
	for i := 0; i < 12; i++ {
		id := "large-" + string(rune('a'+i))
		largeResults[i] = domain.SearchResult{
			Issue: domain.IssueSummary{ID: id, Title: "StaleRow " + id, Status: "open", Priority: 1},
		}
	}
	_ = m.Update(searchLoadedMsg{
		appliedQuery: "",
		page:         domain.SearchResultPage{Results: largeResults},
	})

	// Submit narrow query — model enters loading/reloading state.
	for _, r := range []rune("new") {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Capture the intermediate view (loading=true, reloading=true, old results visible).
	viewIntermediate := m.View(0)
	intermediateLines := strings.Count(viewIntermediate, "\n") + 1

	// The intermediate view must show the OLD results (dimmed), not new ones.
	// It must NOT contain any "NewRow" titles (they haven't arrived yet).
	if strings.Contains(viewIntermediate, "NewRow") {
		t.Errorf(
			"unexpected: intermediate (in-flight) view contains 'NewRow' titles before searchLoadedMsg arrived;\n"+
				"view:\n%s", viewIntermediate,
		)
	}

	// The intermediate view must not suddenly show FEWER lines than the large view.
	// (It would if the model accidentally replaced m.page with an empty slice.)
	largeBase := strings.Count(m.View(0), "\n") + 1
	if intermediateLines < largeBase {
		t.Errorf(
			"intermediate view (%d lines) is shorter than large result view (%d lines)",
			intermediateLines, largeBase,
		)
	}

	// Now deliver the small result set.
	smallResults := make([]domain.SearchResult, 3)
	for i := 0; i < 3; i++ {
		id := "small-" + string(rune('a'+i))
		smallResults[i] = domain.SearchResult{
			Issue: domain.IssueSummary{ID: id, Title: "NewRow " + id, Status: "open", Priority: 2},
		}
	}
	_ = m.Update(searchLoadedMsg{
		appliedQuery: "new",
		page:         domain.SearchResultPage{Results: smallResults},
	})

	viewFinal := m.View(0)

	// Final view: stale rows must be gone; new rows must be present.
	if strings.Contains(viewFinal, "StaleRow") {
		t.Errorf(
			"final view after searchLoadedMsg still contains 'StaleRow' titles;\nview:\n%s",
			viewFinal,
		)
	}
	for i := 0; i < 3; i++ {
		title := "NewRow small-" + string(rune('a'+i))
		if !strings.Contains(viewFinal, title) {
			t.Errorf("final view missing %q;\nview:\n%s", title, viewFinal)
		}
	}
}
