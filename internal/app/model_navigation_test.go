package app

// Mode switching and in-mode navigation: board selection, search entry and
// focus, tab behaviour, and the configured keybindings that drive them.

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

func TestModelBoardNavigationUpdatesShellSelectionAndDetailState(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress one", "task", 2)
	gw.seedInProgress("tm-4", "In progress two", "task", 1)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for tm-4"})
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for tm-2"})
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("expected initial board selection tm-1, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board column")
	}
	// After moving right: InProgress column sorted by priority: [tm-4(P1), tm-2(P2)].
	// First item selected is tm-4 (highest priority).
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-4" {
		t.Fatalf("expected board selection tm-4 after moving right, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "tm-4" {
		t.Fatalf("expected shell detail state to load tm-4, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board row")
	}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection tm-2 after moving down, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "tm-2" {
		t.Fatalf("expected shell detail state to update to tm-2, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected board open-detail action request command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after board enter, got %s", m.active)
	}
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if m.detail.TargetID != "tm-2" {
		t.Fatalf("expected detail target to track board selection, got %q", m.detail.TargetID)
	}
}

func TestModelSearchTextEntryIsNotHijackedByShellHotkeys(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 200
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected active mode search before typing, got %s", m.active)
	}

	mark := gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode to stay search while typing, got %s", m.active)
	}
	if gw.callCountSince(mark, fakes.MethodSearch) != 0 {
		t.Fatalf("expected typing in search query not to run search until enter, got %#v", gw.Calls())
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := m.search.SessionState().AppliedQuery; got != "b" {
		t.Fatalf("expected applied search query %q, got %q", "b", got)
	}
}

func TestModelSearchModeRendersRepresentativeErrorAndEmptyStates(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Enter search mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Trigger a repository-backed search error.
	gw.SetError(fakes.MethodSearch, errors.New("search boom"))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if view := m.View(); !strings.Contains(view, "search boom") {
		t.Fatalf("expected search error state in shell view, got:\n%s", view)
	}

	// Clear error and run another non-empty query that returns no results.
	gw.SetError(fakes.MethodSearch, nil)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if view := m.View(); !strings.Contains(view, "No matches for \"xy\".") {
		t.Fatalf("expected search empty state in shell view, got:\n%s", view)
	}

	if got := firstSelectionID(m, mode.Search); got != "" {
		t.Fatalf("expected no search selection in empty state, got %q", got)
	}
}

func TestModelCtrlSpaceTogglesSearchAndEscReturnsBoard(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected ctrl+space equivalent to enter search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected esc from search to return to board, got %s", m.active)
	}
	if m.lastBrowse != mode.Board {
		t.Fatalf("expected lastBrowse to return to board, got %s", m.lastBrowse)
	}
}

func TestModelSearchEscFromResultsFocusReturnsToBoard(t *testing.T) {
	// Regression: Esc must trigger shell escape (return to board) even when
	// search focus is on Results / Content / Metadata, not just on Query.
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// tm-3 seeded so empty-query search returns it in the results panel.
	gw.seedReady("tm-3", "Search result", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Enter search mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected search active after ctrl+space, got %s", m.active)
	}

	// Press down arrow to move search focus from Query to Results.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Confirm search focus is now on Results (CapturesShellKey must return false for Esc).
	if m.search.CapturesShellKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("expected CapturesShellKey to return false for Esc when focus is Results — shell escape must be reachable")
	}

	// Press Esc — shell escape handler should fire and return to board.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected Esc from Results focus to return to board, got %s", m.active)
	}
	if m.lastBrowse != mode.Board {
		t.Fatalf("expected lastBrowse to be board after Esc from search Results, got %s", m.lastBrowse)
	}
}

func TestModelSearchHeaderUsesPageMetadataAndDraftQueryState(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// Seed tm-9 with "x" in title so the query "x" matches it.
	gw.seedReady("tm-9", "x Search result", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	header := m.renderHeader()
	// Memory repo returns 1 real match; searchResultCount falls back to len(Results).
	if !strings.Contains(header, "Search: 1 results") {
		t.Fatalf("expected search header to reflect result count, got:\n%s", header)
	}
	if !strings.Contains(header, "Selected: tm-9 (open)") {
		t.Fatalf("expected header to keep active search selection, got:\n%s", header)
	}
	if got := m.search.SessionState(); got.DraftQuery != "xy" || got.AppliedQuery != "x" {
		t.Fatalf("expected app shell to preserve draft/applied query split, got %#v", got)
	}
}

func TestModelSearchPreviewSyncKeepsLastLoadedPreviewDuringReloadAndError(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// tm-9 with "x" in title so query "x" finds it.
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-9", Title: "x Search result", Status: "open", Priority: 1}, Description: "cached detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.Detail.Summary.ID != "tm-9" {
		t.Fatalf("expected search selection detail to load, got %#v", m.detail.Detail)
	}
	m.renderBody()
	if got := m.search.SessionState(); len(got.Page.Results) != 1 {
		t.Fatalf("expected search page state present before reload, got %#v", got)
	}

	cmd = m.search.AutoRefresh()
	if cmd == nil {
		t.Fatal("expected search auto-refresh command")
	}
	if session := m.search.SessionState(); !session.Loading || !session.Reloading {
		t.Fatalf("expected search session to mark reload in flight, got %#v", session)
	}
	gw.SetError(fakes.MethodSearch, errors.New("refresh boom"))

	m = applyMessages(t, m, runBatch(cmd))
	m.renderBody()
	if got := m.search.SessionState(); got.Error != "refresh boom" || len(got.Page.Results) != 1 {
		t.Fatalf("expected last search page retained after refresh failure, got %#v", got)
	}
	if !strings.Contains(m.View(), "cached detail") {
		t.Fatalf("expected cached preview detail retained after refresh failure, got:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "refresh boom") || !strings.Contains(m.View(), "Search result") || !strings.Contains(m.View(), "failed") || !strings.Contains(m.View(), "x") {
		t.Fatalf("expected shell view to preserve search context on refresh failure, got:\n%s", m.View())
	}
}

func TestModelDefaultTabAndShiftTabDoNotCycleModes(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected shift+tab from board not to switch modes, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected ctrl+space to switch to search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected ctrl+space to return to board, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after hotkey 3, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected tab from detail not to cycle modes by default, got %s", m.active)
	}
}

func TestModelUsesConfiguredShellAndBoardKeyBindings(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress", Status: "in_progress", Priority: 2}, Description: "detail"})

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{
			config.ShellActionHelp:         {"F1"},
			config.ShellActionModeSearch:   {"/"},
			config.ShellActionToggleSearch: {"ctrl+s"},
			config.ShellActionQuit:         {"ctrl+q"},
		},
		Board: map[string][]string{
			config.BoardActionMoveRight: {"d"},
			config.BoardActionMoveDown:  {"s"},
		},
	})

	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); !strings.Contains(footer, "ctrl+s search") || !strings.Contains(footer, "ctrl+q quit") {
		t.Fatalf("expected footer to reflect configured bindings, got:\n%s", footer)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected configured mode_search key to switch to search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected configured toggle_search key to return to board, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected configured board move-right key to select tm-2, got %q", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.showHelp {
		t.Fatal("expected default help key to stop working after override")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = next.(Model)
	if m.showHelp {
		t.Fatal("expected plain F rune not to trigger help")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("expected configured help key to show help")
	}
	m.showHelp = false

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected configured quit key to return quit command")
	}
	msgs := runBatch(cmd)
	foundQuit := false
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			foundQuit = true
			break
		}
	}
	if !foundQuit {
		t.Fatalf("expected quit command batch, got %#v", msgs)
	}
}

// TestModeCycleDirections asserts that nextMode and prevMode traverse the mode
// cycle in opposite directions for every starting mode.
//
// Forward cycle  (nextMode): Board → Search → Board (2-mode browse toggle),
//
//	Detail → the browse mode not in lastBrowse
//
// Backward cycle (prevMode): Board → Detail → Search → Board
// (prevMode ignores lastBrowse; the cycle is fixed).
//
// Together the two functions must differ at the modes where direction matters:
// Board (next→Search vs prev→Detail) and Detail (next→browse vs prev→Search).
func TestModeCycleDirections(t *testing.T) {
	t.Parallel()

	t.Run("nextMode_forward", func(t *testing.T) {
		t.Parallel()

		if got := nextMode(mode.Board, mode.Board); got != mode.Search {
			t.Errorf("nextMode(Board, Board) = %s; want Search", got)
		}
		if got := nextMode(mode.Search, mode.Board); got != mode.Board {
			t.Errorf("nextMode(Search, Board) = %s; want Board", got)
		}
		// Detail goes to the browse mode not in lastBrowse.
		if got := nextMode(mode.Detail, mode.Search); got != mode.Board {
			t.Errorf("nextMode(Detail, Search) = %s; want Board", got)
		}
		if got := nextMode(mode.Detail, mode.Board); got != mode.Search {
			t.Errorf("nextMode(Detail, Board) = %s; want Search", got)
		}
	})

	t.Run("prevMode_backward", func(t *testing.T) {
		t.Parallel()

		// Backward cycle: Board → Detail → Search → Board
		if got := prevMode(mode.Board, mode.Board); got != mode.Detail {
			t.Errorf("prevMode(Board, _) = %s; want Detail", got)
		}
		if got := prevMode(mode.Detail, mode.Board); got != mode.Search {
			t.Errorf("prevMode(Detail, _) = %s; want Search", got)
		}
		if got := prevMode(mode.Search, mode.Board); got != mode.Board {
			t.Errorf("prevMode(Search, _) = %s; want Board", got)
		}
	})

	t.Run("next_and_prev_differ_at_board_and_detail", func(t *testing.T) {
		t.Parallel()

		// Board: next→Search, prev→Detail — must differ.
		if nextMode(mode.Board, mode.Board) == prevMode(mode.Board, mode.Board) {
			t.Errorf("nextMode(Board) and prevMode(Board) both returned %s; they must differ",
				nextMode(mode.Board, mode.Board))
		}
		// Detail: next→browse (Search when lb=Board), prev→Search — both happen
		// to return Search when lb=Board, which is expected; the distinguishing
		// arm is lb=Search where next→Board but prev→Search.
		if nextMode(mode.Detail, mode.Search) == prevMode(mode.Detail, mode.Search) {
			t.Errorf("nextMode(Detail,Search) and prevMode(Detail,Search) both returned %s; they must differ",
				nextMode(mode.Detail, mode.Search))
		}
	})
}
