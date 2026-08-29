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
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestModelBoardNavigationUpdatesShellSelectionAndDetailState(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress one", "task", 2)
	seedInProgress(gw, "tm-4", "In progress two", "task", 1)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for tm-4"})
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for tm-2"})
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}})

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

	if m.detail.TargetID() != "tm-2" {
		t.Fatalf("expected detail target to track board selection, got %q", m.detail.TargetID())
	}
}

func TestModelSearchTextEntryIsNotHijackedByShellHotkeys(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

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

	mark := gw.CallCount()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode to stay search while typing, got %s", m.active)
	}
	if gw.CallCountSince(mark, fakes.MethodSearch) != 0 {
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
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

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
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

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
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	// tm-3 seeded so empty-query search returns it in the results panel.
	seedReady(gw, "tm-3", "Search result", "task", 1)

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

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	// Seed tm-9 with "x" in title so the query "x" matches it.
	seedReady(gw, "tm-9", "x Search result", "task", 1)

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

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	// tm-9 with "x" in title so query "x" finds it.
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-9", Title: "x Search result", Status: "open", Priority: 1}, Description: "cached detail"})

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

// TestSearchPreviewIsSyncedFromUpdateNotFromView pins Core Architectural Rule 9:
// selection/detail sync is event-driven, not polled. renderBody used to call
// syncSearchPreviewDetailState, so View() mutated model state and only stayed
// correct because the always-on spinner tick re-rendered ten times a second.
func TestSearchPreviewIsSyncedFromUpdateNotFromView(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "x Search result", Status: "open", Priority: 1},
		Description: "detail from the message path",
	})

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

	// The preview is populated by the message path alone — no render needed.
	if !strings.Contains(m.View(), "detail from the message path") {
		t.Fatalf("the search preview was not synced by Update, got:\n%s", m.View())
	}

	// A render must not move it. Change the shell's detail behind View's back:
	// a pure renderer leaves the preview exactly as the last message left it.
	m.detail.Detail = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "x Search result", Status: "open", Priority: 1},
		Description: "detail smuggled in through View",
	}
	if strings.Contains(m.View(), "detail smuggled in through View") {
		t.Fatal("View() mutated the search preview state")
	}
}

// tab and shift+tab drive the header tab strip: Board, Docs, Search, in
// mode.BrowseModes order. Detail is not a tab, so cycling out of it steps onto
// the strip rather than staying put.
func TestModelTabAndShiftTabCycleBrowseTabs(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	press := func(m Model, key tea.KeyMsg) Model {
		t.Helper()
		next, cmd := m.Update(key)
		m = next.(Model)
		return applyMessages(t, m, runBatch(cmd))
	}

	tab := tea.KeyMsg{Type: tea.KeyTab}
	shiftTab := tea.KeyMsg{Type: tea.KeyShiftTab}

	for _, want := range []mode.ID{mode.Docs, mode.Search, mode.Board} {
		m = press(m, tab)
		if m.active != want {
			t.Fatalf("expected tab to move to %s, got %s", want, m.active)
		}
		if m.lastBrowse != want {
			t.Fatalf("expected lastBrowse to follow the tab strip to %s, got %s", want, m.lastBrowse)
		}
	}

	for _, want := range []mode.ID{mode.Search, mode.Docs, mode.Board} {
		m = press(m, shiftTab)
		if m.active != want {
			t.Fatalf("expected shift+tab to move to %s, got %s", want, m.active)
		}
	}

	// From Detail the cycle resumes from the tab we drilled in from.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after hotkey 3, got %s", m.active)
	}
	m = press(m, tab)
	if m.active != mode.Docs {
		t.Fatalf("expected tab from detail (lastBrowse=board) to move to docs, got %s", m.active)
	}
}

// The docs tab is the only surface that can show an open doc: task-manager
// excludes docs from the ready queue, so one never reaches a board column.
func TestModelDocsTabListsOpenDocsAndOpensThemInDetail(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-9", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2})
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-9", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2}, Description: "doc body"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Not asserted here: that the board omits the doc. Under the real backend
	// it does — the SDK keeps non-work types out of Ready — but the memory
	// fixture has no such exclusion yet, so the board view would show it and
	// the assertion would pin fixture behavior, not product behavior. The
	// fixture parity gap is tracked separately.

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Docs {
		t.Fatalf("expected tab from board to enter docs, got %s", m.active)
	}

	view := m.View()
	testui.AssertContainsAll(t, view, "Docs", "tm-9", "Auth redesign")
	testui.AssertNotContainsAny(t, view, "Ready first")

	if got := firstSelectionID(m, mode.Docs); got != "tm-9" {
		t.Fatalf("expected docs selection tm-9, got %q", got)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected enter in docs to open detail, got %s", m.active)
	}
	if m.detail.TargetID() != "tm-9" && m.detail.Detail.Summary.ID != "tm-9" {
		t.Fatalf("expected detail to track the selected doc, target=%q detail=%q", m.detail.TargetID(), m.detail.Detail.Summary.ID)
	}

	// Escape returns to the tab we drilled in from.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Docs {
		t.Fatalf("expected escape from detail to return to docs, got %s", m.active)
	}
}

func TestModelUsesConfiguredShellAndBoardKeyBindings(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress", Status: "in_progress", Priority: 2}, Description: "detail"})

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

// TestModeCycleDirections asserts that nextMode and prevMode traverse the
// header tab strip — mode.BrowseModes: Board, Docs, Search — in opposite
// directions, wrapping at both ends.
//
// Detail is not a tab. Cycling from it resumes from lastBrowse, so it steps
// onto the strip instead of staying in Detail.
func TestModeCycleDirections(t *testing.T) {
	t.Parallel()

	t.Run("nextMode_forward", func(t *testing.T) {
		t.Parallel()

		if got := nextMode(mode.Board, mode.Board); got != mode.Docs {
			t.Errorf("nextMode(Board, Board) = %s; want Docs", got)
		}
		if got := nextMode(mode.Docs, mode.Docs); got != mode.Search {
			t.Errorf("nextMode(Docs, Docs) = %s; want Search", got)
		}
		if got := nextMode(mode.Search, mode.Search); got != mode.Board {
			t.Errorf("nextMode(Search, Search) = %s; want Board", got)
		}
		// From Detail the cycle resumes from lastBrowse.
		if got := nextMode(mode.Detail, mode.Search); got != mode.Board {
			t.Errorf("nextMode(Detail, Search) = %s; want Board", got)
		}
		if got := nextMode(mode.Detail, mode.Board); got != mode.Docs {
			t.Errorf("nextMode(Detail, Board) = %s; want Docs", got)
		}
	})

	t.Run("prevMode_backward", func(t *testing.T) {
		t.Parallel()

		if got := prevMode(mode.Board, mode.Board); got != mode.Search {
			t.Errorf("prevMode(Board, Board) = %s; want Search", got)
		}
		if got := prevMode(mode.Search, mode.Search); got != mode.Docs {
			t.Errorf("prevMode(Search, Search) = %s; want Docs", got)
		}
		if got := prevMode(mode.Docs, mode.Docs); got != mode.Board {
			t.Errorf("prevMode(Docs, Docs) = %s; want Board", got)
		}
		if got := prevMode(mode.Detail, mode.Board); got != mode.Search {
			t.Errorf("prevMode(Detail, Board) = %s; want Search", got)
		}
	})

	t.Run("next_and_prev_are_inverses", func(t *testing.T) {
		t.Parallel()

		for _, tab := range mode.BrowseModes {
			if got := prevMode(nextMode(tab, tab), tab); got != tab {
				t.Errorf("prevMode(nextMode(%s)) = %s; want %s", tab, got, tab)
			}
			if nextMode(tab, tab) == prevMode(tab, tab) {
				t.Errorf("nextMode(%s) and prevMode(%s) both returned %s; they must differ",
					tab, tab, nextMode(tab, tab))
			}
		}
	})

	t.Run("unknown_mode_falls_back_to_the_strip", func(t *testing.T) {
		t.Parallel()

		// Neither current nor lastBrowse is a tab: the cycle still lands on one.
		if got := nextMode(mode.Detail, mode.Detail); got != mode.Docs {
			t.Errorf("nextMode(Detail, Detail) = %s; want Docs (fallback to Board, then forward)", got)
		}
	})
}
