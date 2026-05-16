package board

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

func resolvedBoardKeys(t *testing.T) config.ResolvedKeyBindings {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return keys
}

// newBoardModel builds a test board model with a no-op logger.
func newBoardModel(gateway *fakes.FakeBeadsGateway, keys config.ResolvedKeyBindings) *Model {
	return NewModel(gateway, slog.Default(), keys)
}

// --- call-recording helpers ---

func countCalls(gateway *fakes.FakeBeadsGateway, method fakes.GatewayMethod) int {
	n := 0
	for _, c := range gateway.Calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

func firstQueryCall(gateway *fakes.FakeBeadsGateway) (fakes.QueryCall, bool) {
	for _, c := range gateway.Calls {
		if c.Method == fakes.MethodQuery {
			qc, ok := c.Input.(fakes.QueryCall)
			return qc, ok
		}
	}
	return fakes.QueryCall{}, false
}

func queryCallsFor(gateway *fakes.FakeBeadsGateway, expr string) []fakes.QueryCall {
	var out []fakes.QueryCall
	for _, c := range gateway.Calls {
		if c.Method == fakes.MethodQuery {
			qc, ok := c.Input.(fakes.QueryCall)
			if ok && qc.Expr == expr {
				out = append(out, qc)
			}
		}
	}
	return out
}

// --- AC: exactly 4 gateway calls dispatched in a single batch ---

func TestBoardModeInitDispatchesExact3GatewayCalls(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))

	cmds := m.Init()
	if cmds == nil {
		t.Fatalf("Init() must return a non-nil command")
	}
	// Run the batch command: it returns a tea.BatchMsg which is a []tea.Cmd slice.
	// In teatest the batch is run by the runtime; here we inspect it by
	// executing the command and checking what messages come back.
	// Run the returned command to get the BatchMsg.
	msg := cmds()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() must return a tea.Batch; got %T", msg)
	}
	if len(batch) != 4 {
		t.Fatalf("expected exactly 4 commands in the batch, got %d", len(batch))
	}

	// Execute each command in the batch to drive the gateway calls.
	for _, cmd := range batch {
		if cmd != nil {
			_ = cmd()
		}
	}

	if n := countCalls(gateway, fakes.MethodReadyExplain); n != 1 {
		t.Errorf("expected 1 ReadyExplain call, got %d", n)
	}
	if n := countCalls(gateway, fakes.MethodQuery); n != 2 {
		t.Errorf("expected 2 Query calls (in_progress + closed), got %d", n)
	}
	if n := countCalls(gateway, fakes.MethodCountIssues); n != 1 {
		t.Errorf("expected 1 CountIssues call (closed count), got %d", n)
	}
	inProgressCalls := queryCallsFor(gateway, "status=in_progress")
	if len(inProgressCalls) != 1 {
		t.Errorf("expected 1 Query(status=in_progress) call, got %d", len(inProgressCalls))
	}
	closedCalls := queryCallsFor(gateway, "status=closed")
	if len(closedCalls) != 1 {
		t.Errorf("expected 1 Query(status=closed) call, got %d", len(closedCalls))
	}
	if len(closedCalls) == 1 {
		if !closedCalls[0].Opts.IncludeClosed {
			t.Errorf("Query(status=closed) must set IncludeClosed=true")
		}
		if closedCalls[0].Opts.SortBy != domain.SortFieldClosedAt {
			t.Errorf("Query(status=closed) must sort by closed_at, got %q", closedCalls[0].Opts.SortBy)
		}
		if closedCalls[0].Opts.SortOrder != domain.SortDirectionDescending {
			t.Errorf("Query(status=closed) must sort descending, got %q", closedCalls[0].Opts.SortOrder)
		}
	}
	// Verify the CountIssues call uses status=closed.
	for _, c := range gateway.Calls {
		if c.Method == fakes.MethodCountIssues {
			cc, ok := c.Input.(fakes.CountIssuesCall)
			if !ok {
				t.Errorf("CountIssues call input is not CountIssuesCall: %T", c.Input)
				continue
			}
			if len(cc.Query.Statuses) != 1 || cc.Query.Statuses[0] != "closed" {
				t.Errorf("CountIssues must filter status=closed, got statuses=%v", cc.Query.Statuses)
			}
		}
	}
}

// --- AC: all-empty load ---

func TestBoardModeAllEmptyLoad(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	// No responses set: all return empty slices.

	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.pendingResults = 4
	// Use a wide enough terminal so all 4 columns are visible.
	m.SetSize(200, 30)

	// Feed all 4 results.
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{}})
	_ = m.Update(inProgressLoadedMsg{issues: nil})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 4 results arrived")
	}
	for _, col := range m.columns {
		if col.err != nil {
			t.Fatalf("expected no column errors, got col %q err: %v", col.title, col.err)
		}
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}

	view := m.View()
	// All 4 section titles must appear in a wide render.
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q in view, got: %s", title, view)
		}
	}
}

// --- AC: all 3 groups populated ---

func TestBoardModeAllGroupsPopulatedRendersGolden(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"},
			{ID: "bw-2", Title: "Ready second", Priority: 2, Status: "open", Type: "task"},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "bw-4", Title: "Blocked now", Priority: 1, Status: "blocked", Type: "bug"}},
		},
	}
	gateway.QueryResponse = []domain.IssueSummary{
		{ID: "bw-3", Title: "In progress", Priority: 2, Status: "in_progress", Type: "feature"},
	}

	tm := testui.NewTestModel(t, testui.ControllerAdapter{Controller: newBoardModel(gateway, resolvedBoardKeys(t))})
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// At width=120, 3 of 4 columns are visible (Not Ready, Ready, In Progress).
	// Done is offscreen; only check visible sections and known issue IDs.
	testui.WaitForOutputContainsAll(t, tm.Output(),
		sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress,
		"bw-1", "bw-4",
	)

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	final, ok := tm.FinalModel(t).(testui.ControllerAdapter)
	if !ok {
		t.Fatalf("expected final model adapter")
	}
	finalModel, ok := final.Controller.(*Model)
	if !ok {
		t.Fatalf("expected wrapped board model, got %T", final.Controller)
	}

	if sel := finalModel.CurrentSelection(); sel == nil || sel.Issue.ID != "bw-4" {
		t.Fatalf("expected initial selection bw-4 from Not Ready lane (earliest non-empty), got %#v", sel)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(finalModel.View()), "model_loaded.golden")
}

// --- AC: ReadyExplain error path (per-column, non-aborting) ---

func TestBoardModeReadyExplainErrorPath(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.SetSize(200, 30)
	m.pendingResults = 4

	loadErr := errors.New("network timeout")
	// Only the ReadyExplain result arrives with an error; feed the other three as
	// success so maybeCompose runs and routes the error to the correct columns.
	_ = m.Update(readyExplainLoadedMsg{err: loadErr})
	_ = m.Update(inProgressLoadedMsg{issues: nil})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 4 results arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// Not Ready (col 0) and Ready (col 1) are affected by the ReadyExplain error.
	for _, col := range m.columns[:2] {
		if col.err == nil || !strings.Contains(col.err.Error(), "network timeout") {
			t.Errorf("expected ReadyExplain error on column %q, got: %v", col.title, col.err)
		}
	}
	// In Progress (col 2) and Done (col 3) must be unaffected.
	for _, col := range m.columns[2:] {
		if col.err != nil {
			t.Errorf("expected no error on column %q, got: %v", col.title, col.err)
		}
	}

	// View must render 4-column layout (never the old loading.View).
	view := m.View()
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q in view even on error, got: %s", title, view)
		}
	}
}

// --- AC: Query in_progress error path (per-column, non-aborting) ---

func TestBoardModeQueryInProgressErrorPath(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.pendingResults = 4

	loadErr := errors.New("bd unavailable")
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{}})
	_ = m.Update(inProgressLoadedMsg{err: loadErr})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 4 results arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// In Progress (col 2) must carry the error.
	if m.columns[2].err == nil || !strings.Contains(m.columns[2].err.Error(), "bd unavailable") {
		t.Errorf("expected in_progress error on In Progress column, got: %v", m.columns[2].err)
	}
	// Not Ready (col 0), Ready (col 1), Done (col 3) must be unaffected.
	for _, col := range []columnData{m.columns[0], m.columns[1], m.columns[3]} {
		if col.err != nil {
			t.Errorf("expected no error on column %q, got: %v", col.title, col.err)
		}
	}
}

// --- AC: Query closed error path (per-column, non-aborting) ---

func TestBoardModeQueryClosedErrorPath(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.pendingResults = 4

	loadErr := errors.New("bd query failed")
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{}})
	_ = m.Update(inProgressLoadedMsg{issues: nil})
	_ = m.Update(closedLoadedMsg{err: loadErr})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 3 results arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// Done (col 3) must carry the error.
	if m.columns[3].err == nil || !strings.Contains(m.columns[3].err.Error(), "bd query failed") {
		t.Errorf("expected closed error on Done column, got: %v", m.columns[3].err)
	}
	// Not Ready, Ready, In Progress must be unaffected.
	for _, col := range m.columns[:3] {
		if col.err != nil {
			t.Errorf("expected no error on column %q, got: %v", col.title, col.err)
		}
	}
}

// --- Navigation tests ---

func TestBoardModeNavigationEmitsSelectionChangedAndActionRequest(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.SetSize(100, 24)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving right")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-7" {
		t.Fatalf("expected selection bw-7 after moving right, got %#v", selChanged.Selection)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving down")
	}
	msg = cmd()
	selChanged, ok = msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected selection bw-8 after moving down, got %#v", selChanged.Selection)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected action request command on enter")
	}
	msg = cmd()
	action, ok := msg.(mode.ActionRequestMsg)
	if !ok {
		t.Fatalf("expected ActionRequestMsg, got %T", msg)
	}
	if action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected action %q got %q", mode.ActionOpenDetail, action.Action)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(m.View()), "model_navigation.golden")
}

func TestBoardModeUsesConfiguredBindings(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), &config.KeyBindingOverride{
		Board: map[string][]string{
			config.BoardActionMoveLeft:   {"a"},
			config.BoardActionMoveRight:  {"d"},
			config.BoardActionMoveUp:     {"w"},
			config.BoardActionMoveDown:   {"s"},
			config.BoardActionOpenDetail: {"space"},
			config.BoardActionReload:     {"R"},
		},
	}))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, keys)
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.SetSize(100, 24)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("expected selection change after configured move-right key")
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected selection change after configured move-down key")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok || selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected configured move-down to select bw-8, got %#v", msg)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if cmd == nil {
		t.Fatal("expected action request from configured open key")
	}
	if action, ok := cmd().(mode.ActionRequestMsg); !ok || action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected open detail action request, got %#v", cmd())
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil || !m.IsLoading() {
		t.Fatal("expected configured reload key to trigger board reload")
	}
}

// --- Auto-refresh anchor tests ---

func populatedModel(gateway *fakes.FakeBeadsGateway, keys config.ResolvedKeyBindings) *Model {
	m := newBoardModel(gateway, keys)
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-2", Title: "Progress one"}, {ID: "bw-3", Title: "Progress two"}}, total: 2, exact: true},
	}
	m.focusedColumn = 1
	m.selectedRow[0] = 0
	m.selectedRow[1] = 1
	return m
}

func feedAllResults(m *Model, readyExplain domain.ReadyExplainResult, inProgress []domain.IssueSummary, closed []domain.IssueSummary) {
	m.pendingResults = 4
	_ = m.Update(readyExplainLoadedMsg{result: readyExplain})
	_ = m.Update(inProgressLoadedMsg{issues: inProgress})
	_ = m.Update(closedLoadedMsg{issues: closed})
	_ = m.Update(closedCountLoadedMsg{count: len(closed)})
}

func TestBoardModeAutoRefreshPreservesFocusedIssueSelectionWhenPresent(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := populatedModel(gateway, resolvedBoardKeys(t))

	cmd := m.AutoRefresh()
	if cmd == nil {
		t.Fatalf("expected auto-refresh command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-8", Title: "Progress refreshed one"},
			{ID: "bw-3", Title: "Progress two still here"},
			{ID: "bw-10", Title: "Progress refreshed three"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// bw-3 is in InProgress = column 2.
	if m.focusedColumn != 2 {
		t.Fatalf("expected focused column 2 (InProgress) to be restored via anchor, got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-3" {
		t.Fatalf("expected preserved selected issue bw-3, got %#v", sel)
	}
}

func TestBoardModeAutoRefreshDeterministicFallbackWhenSelectedIssueDisappears(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := populatedModel(gateway, resolvedBoardKeys(t))

	cmd := m.AutoRefresh()
	if cmd == nil {
		t.Fatalf("expected auto-refresh command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-11", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-12", Title: "Progress replacement"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// bw-3 (anchor issue) is gone. The anchor's prior focusedColumn was 1 (InProgress
	// in the 2-column model). Column 1 in the new 4-column model is Ready (has bw-11).
	// restoreFromAnchor clamps the prior focusedColumn (1) and selects it.
	if m.focusedColumn != 1 {
		t.Fatalf("expected fallback to clamped prior focused column 1 (Ready), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-11" {
		t.Fatalf("expected deterministic row-clamp fallback selection bw-11, got %#v", sel)
	}
}

func TestBoardModeManualReloadRemainsFullResetBehavior(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := populatedModel(gateway, resolvedBoardKeys(t))

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatalf("expected manual reload command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-21", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-22", Title: "Progress refreshed"},
		},
		nil,
	)

	// Manual reload: focus settles to first available non-empty column.
	// Columns: [NotReady(empty,0), Ready(bw-21,1), InProgress(bw-22,2), Done(empty,3)]
	// First non-empty is col 1 (Ready, has bw-21).
	if m.focusedColumn != 1 {
		t.Fatalf("expected manual reload to reset focus to first non-empty column (Ready, col 1), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-21" {
		t.Fatalf("expected manual reload selection to be bw-21 (first issue in first non-empty col), got %#v", sel)
	}
}

// --- Per-column loading state (replaces old single loading indicator) ---

func TestBoardModePerColumnLoadingState(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.SetSize(200, 30)

	// Phase 1: initial loading state — all 4 columns are loading.
	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true before any results")
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("expected column %d (%q) loading=true in cold start", i, col.title)
		}
	}
	// View must render 4-column layout with skeleton rows (░ chars), not a
	// full-screen loading message.
	view := m.View()
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q during cold-start, got: %s", title, view)
		}
	}
	if !strings.Contains(view, "░") {
		t.Fatalf("expected skeleton glyph ░ during cold-start loading, got: %s", view)
	}

	// Phase 2: only 1 result arrives — still loading (all columns).
	m.pendingResults = 4
	m.partialReadyExplain = &domain.ReadyExplainResult{}
	m.pendingResults = 3

	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true while 2 results still pending")
	}

	// Phase 3: all results arrive via maybeCompose.
	m.pendingResults = 0
	m.partialInProgress = nil
	m.partialClosed = nil
	_ = m.maybeCompose()

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all results arrived")
	}
	for i, col := range m.columns {
		if col.loading {
			t.Errorf("expected column %d (%q) loading=false after composition", i, col.title)
		}
	}
}

// --- New ticket-required tests (0x36.2) ---

// TestBoardModeColdStartAllColumnsLoading verifies that after NewModel,
// all 4 columns have loading=true and IsLoading() returns true.
func TestBoardModeColdStartAllColumnsLoading(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))

	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true immediately after NewModel")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns in cold start, got %d", len(m.columns))
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("expected column %d (%q) loading=true in cold start", i, col.title)
		}
		if col.err != nil {
			t.Errorf("expected column %d (%q) err=nil in cold start, got: %v", i, col.title, col.err)
		}
		if len(col.issues) != 0 {
			t.Errorf("expected column %d (%q) empty in cold start, got %d issues", i, col.title, len(col.issues))
		}
	}
}

// TestBoardModeAtomicSwapAllColumnsAfterAllResults verifies that after all 3
// gateway responses arrive, all 4 columns have loading=false.
func TestBoardModeAtomicSwapAllColumnsAfterAllResults(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.pendingResults = 4

	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one", Status: "open", Type: "task", Priority: 1}},
	}})
	_ = m.Update(inProgressLoadedMsg{issues: []domain.IssueSummary{{ID: "bw-2", Title: "IP one", Status: "in_progress", Type: "task", Priority: 1}}})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 4 results arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	for i, col := range m.columns {
		if col.loading {
			t.Errorf("expected column %d (%q) loading=false after atomic swap", i, col.title)
		}
	}
}

// TestBoardModePartialErrorOnlyAffectsCorrectColumns verifies that when one
// gateway call fails, only the affected column(s) carry an error.
func TestBoardModePartialErrorOnlyAffectsCorrectColumns(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.pendingResults = 4

	// in_progress call fails; the other three succeed.
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{}})
	_ = m.Update(inProgressLoadedMsg{err: errors.New("in_progress error")})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after all 3 results")
	}
	// Not Ready (0), Ready (1) — unaffected.
	for _, col := range m.columns[:2] {
		if col.err != nil {
			t.Errorf("expected no error on column %q, got: %v", col.title, col.err)
		}
	}
	// In Progress (2) — must have error.
	if m.columns[2].err == nil {
		t.Errorf("expected error on In Progress column, got nil")
	}
	// Done (3) — unaffected.
	if m.columns[3].err != nil {
		t.Errorf("expected no error on Done column, got: %v", m.columns[3].err)
	}
}

// TestBoardModeKeyboardNavigationNoopWhenAllColumnsEmpty verifies that
// keyboard navigation during full cold-start (all columns empty) is a no-op
// and does not panic.
func TestBoardModeKeyboardNavigationNoopWhenAllColumnsEmpty(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))
	m.SetSize(200, 30)

	// All columns are in cold-start loading state with no issues.
	// Navigation key presses must not panic.
	keyTests := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyLeft},
		tea.KeyMsg{Type: tea.KeyRight},
		tea.KeyMsg{Type: tea.KeyUp},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	}

	for _, k := range keyTests {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("keyboard navigation panicked during cold-start: %v", r)
				}
			}()
			_ = m.Update(k)
		}()
	}

	if m.CurrentSelection() != nil {
		t.Fatalf("expected nil selection when all columns empty, got: %#v", m.CurrentSelection())
	}
}

// TestBoardModeRefreshKeepsStaleIssuesVisible verifies that on auto-refresh,
// columns with existing issues keep them visible while loading=true.
func TestBoardModeRefreshKeepsStaleIssuesVisible(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := newBoardModel(gateway, resolvedBoardKeys(t))

	// Seed the model with loaded data as if a prior load already completed.
	m.columns = []columnData{
		{title: sectionTitleNotReady, issues: nil, loading: false},
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one"}}, loading: false, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-2", Title: "IP one"}}, loading: false, total: 1, exact: true},
		{title: sectionTitleDone, issues: nil, loading: false},
	}

	// Trigger auto-refresh (marks columns loading while preserving issues).
	_ = m.AutoRefresh()

	// All columns must now be loading.
	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true during auto-refresh")
	}
	// Columns with prior issues must still have them visible (stale rendering).
	if len(m.columns[1].issues) == 0 {
		t.Errorf("expected Ready column to retain stale issues during refresh, got empty")
	}
	if !m.columns[1].loading {
		t.Errorf("expected Ready column loading=true during auto-refresh")
	}
	if len(m.columns[2].issues) == 0 {
		t.Errorf("expected InProgress column to retain stale issues during refresh, got empty")
	}
}

// --- closedLimit ---

func TestClosedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		height int
		want   int
	}{
		{height: 0, want: 50},   // default: max(50, 20)
		{height: 10, want: 50},  // 10-3=7; max(50,7)=50
		{height: 53, want: 50},  // 53-3=50; max(50,50)=50
		{height: 60, want: 57},  // 60-3=57; max(50,57)=57
		{height: 100, want: 97}, // 100-3=97; max(50,97)=97
	}

	for _, tc := range tests {
		m := &Model{height: tc.height}
		got := m.closedLimit()
		if got != tc.want {
			t.Errorf("closedLimit() with height=%d: got %d, want %d", tc.height, got, tc.want)
		}
	}
}

// --- sectionItemCapacity (retained from old suite) ---

func TestSectionItemCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		height int
		want   int
	}{
		{height: 0, want: 20},  // safe default before first WindowSizeMsg
		{height: 1, want: 1},   // clamp: 1-3=-2, clamped to 1
		{height: 3, want: 1},   // clamp: 3-3=0, clamped to 1
		{height: 4, want: 1},   // 4-3=1
		{height: 24, want: 21}, // 24-3=21
		{height: 30, want: 27}, // 30-3=27
		{height: 34, want: 31}, // 34-3=31
	}

	for _, tc := range tests {
		m := &Model{height: tc.height}
		got := m.sectionItemCapacity()
		if got != tc.want {
			t.Errorf("sectionItemCapacity() with height=%d: got %d, want %d", tc.height, got, tc.want)
		}
	}
}

// --- slog warning capture ---

func TestBoardModeComposerWarningsEmittedToSlog(t *testing.T) {
	t.Parallel()

	// Capture slog output by using a custom handler.
	var capturedMessages []string
	handler := &captureHandler{capture: &capturedMessages}
	logger := slog.New(handler)

	gateway := fakes.NewFakeBeadsGateway()
	m := NewModel(gateway, logger, resolvedBoardKeys(t))
	m.pendingResults = 4

	// Feed all 4 results. No warnings expected from empty inputs.
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{}})
	_ = m.Update(inProgressLoadedMsg{issues: nil})
	_ = m.Update(closedLoadedMsg{issues: nil})
	_ = m.Update(closedCountLoadedMsg{count: 0})

	// With empty inputs no cardinality warning is expected.
	if len(capturedMessages) != 0 {
		t.Fatalf("expected no slog warnings for empty inputs, got %v", capturedMessages)
	}
}

// captureHandler is a minimal slog.Handler that captures log messages for assertions.
type captureHandler struct {
	capture *[]string
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	*h.capture = append(*h.capture, record.Message)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// --- layout goldens ---

func TestBoardModeDashboardLayoutGoldensAcrossWidths(t *testing.T) {
	t.Parallel()

	readyExplainResp := domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "bw-1", Title: "Ready fix login prompt", Priority: 1, Status: "open", Type: "task"},
			{ID: "bw-5", Title: "Ready improve docs outline", Priority: 2, Status: "open", Type: "task"},
			{ID: "bw-6", Title: "Ready triage inbox", Priority: 2, Status: "open", Type: "chore"},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked: API contract pending", Priority: 0, Status: "blocked", Type: "bug"}},
			{Issue: domain.IssueSummary{ID: "bw-9", Title: "Blocked: migration sequencing", Priority: 1, Status: "blocked", Type: "task"}},
		},
	}
	inProgressIssues := []domain.IssueSummary{
		{ID: "bw-2", Title: "Implement board keyboard shortcuts", Priority: 1, Status: "in_progress", Type: "feature"},
		{ID: "bw-7", Title: "Wire detail reload behavior", Priority: 1, Status: "in_progress", Type: "task"},
		{ID: "bw-8", Title: "Polish header help copy", Priority: 2, Status: "in_progress", Type: "docs"},
	}

	tests := []struct {
		name     string
		width    int
		height   int
		golden   string
		mustShow []string
		minMeta  int
	}{
		{name: "w80", width: 80, height: 28, golden: "model_layout_w80.golden", mustShow: []string{sectionTitleNotReady, sectionTitleReady, "bw-1"}, minMeta: 4},
		{name: "w120", width: 120, height: 30, golden: "model_layout_w120.golden", mustShow: []string{sectionTitleNotReady, sectionTitleInProgress, "bw-1", "bw-3"}, minMeta: 6},
		{name: "w180", width: 180, height: 34, golden: "model_layout_w180.golden", mustShow: []string{sectionTitleNotReady, sectionTitleDone, "bw-1", "bw-2", "bw-3"}, minMeta: 8},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			subGateway := fakes.NewFakeBeadsGateway()
			subGateway.ReadyExplainResponse = readyExplainResp
			subGateway.QueryResponse = inProgressIssues

			tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: newBoardModel(subGateway, resolvedBoardKeys(t))}, tc.width, tc.height)
			t.Cleanup(func() {
				_ = tm.Quit()
			})

			tm.Send(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			_ = testui.WaitForOutputContainsAll(t, tm.Output(), tc.mustShow...)

			if err := tm.Quit(); err != nil {
				t.Fatalf("failed to quit teatest model: %v", err)
			}

			final, ok := tm.FinalModel(t).(testui.ControllerAdapter)
			if !ok {
				t.Fatalf("expected final model adapter")
			}
			finalModel, ok := final.Controller.(*Model)
			if !ok {
				t.Fatalf("expected wrapped board model, got %T", final.Controller)
			}

			// AC: exactly 1 ReadyExplain, 2 Query calls, and 1 CountIssues call.
			if n := countCalls(subGateway, fakes.MethodReadyExplain); n != 1 {
				t.Errorf("expected 1 ReadyExplain call, got %d", n)
			}
			if n := countCalls(subGateway, fakes.MethodQuery); n != 2 {
				t.Errorf("expected 2 Query calls, got %d", n)
			}
			if n := countCalls(subGateway, fakes.MethodCountIssues); n != 1 {
				t.Errorf("expected 1 CountIssues call, got %d", n)
			}

			view := finalModel.View()
			testui.AssertMatchesGoldenNormalized(t, []byte(view), tc.golden)
			assertCompactIssueRows(t, view, tc.minMeta)
		})
	}
}

// --- Real Gateway + RecordingExecutor subprocess-argv scenario ---

// TestBoardInitRealGatewaySubprocessArgvCardinality wires the board model against
// a real *beads.Gateway + *beads.CommandRunner backed by a *fakes.RecordingExecutor
// (no FakeBeadsGateway). It asserts:
//   - Exactly 4 subprocess invocations occur on Init (ReadyExplain + 2 Query + 1 Count).
//   - Each invocation's argv matches the expected shape.
//   - No "list --status" argv ever appears (regression guard against the pre-lgln
//     per-section data layer).
//
// NOTE: The board model does NOT call bd ping --json during its own Init; that
// health-check subprocess is dispatched at the app.Model layer. The board's Init
// produces exactly 4 subprocess calls. See internal/app/model.go for the
// HealthCheck dispatch context.
func TestBoardInitRealGatewaySubprocessArgvCardinality(t *testing.T) {
	t.Parallel()

	// Expected argv shapes for the 4 subprocess invocations the board fires.
	// Closed query uses the default height=0 closedLimit() = 50.
	argvReadyExplain := []string{"ready", "--explain", "--json"}
	argvQueryInProgress := []string{"query", "status=in_progress", "--json"}
	argvQueryClosed := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"}
	argvCountClosed := []string{"count", "--by-status", "--json", "--status", "closed"}

	rec := fakes.NewRecordingExecutor()

	// Pre-register canned responses so the gateway parse path succeeds.
	rec.OnArgs(argvReadyExplain).Return(beads.ExecResult{Stdout: []byte(`{
		"ready": [
			{"id":"bw-r1","title":"Ready one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
		],
		"blocked": [],
		"summary": {"total_ready": 1, "total_blocked": 0, "cycle_count": 0}
	}`)}, nil)

	rec.OnArgs(argvQueryInProgress).Return(beads.ExecResult{Stdout: []byte(`[
		{"id":"bw-p1","title":"In progress one","status":"in_progress","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
	]`)}, nil)

	rec.OnArgs(argvQueryClosed).Return(beads.ExecResult{Stdout: []byte(`[
		{"id":"bw-c1","title":"Closed one","status":"closed","issue_type":"task","priority":1,"owner":"carol","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
	]`)}, nil)

	rec.OnArgs(argvCountClosed).Return(beads.ExecResult{Stdout: []byte(`{
		"groups": [{"group": "closed", "count": 452}],
		"total": 452,
		"schema_version": 1
	}`)}, nil)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		Command:  "bd",
		Executor: rec,
	})
	gateway := beads.NewCLIGateway(runner)

	m := NewModel(gateway, slog.Default(), resolvedBoardKeys(t))

	// Drive Init: board.Init() returns a tea.Batch wrapping 4 commands.
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatalf("Init() must return a non-nil command")
	}

	// Execute the outer command to unwrap the tea.BatchMsg.
	batchMsg := initCmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() must produce a tea.BatchMsg; got %T", batchMsg)
	}
	if len(batch) != 4 {
		t.Fatalf("expected exactly 4 commands in Init batch, got %d", len(batch))
	}

	// Run each command to drive the subprocess calls through the real gateway.
	for _, cmd := range batch {
		if cmd != nil {
			_ = cmd()
		}
	}

	calls := rec.Calls()

	// AC: exactly 4 subprocess invocations (bd ping is dispatched at app level, not here).
	if len(calls) != 4 {
		t.Fatalf("expected exactly 4 subprocess invocations on board Init, got %d: %v",
			len(calls), formatArgvList(calls))
	}

	// AC: argv for each matches the expected shape. The 4 calls run in a tea.Batch
	// so order is not guaranteed; match by content.
	assertArgvPresent(t, calls, argvReadyExplain)
	assertArgvPresent(t, calls, argvQueryInProgress)
	assertArgvPresent(t, calls, argvQueryClosed)
	assertArgvPresent(t, calls, argvCountClosed)

	// AC: regression guard — no "list --status" argv (old data layer).
	for _, c := range calls {
		// Detect "list" followed by a "--status" flag anywhere in the same call.
		if hasArg(c.Args, "list") && hasArg(c.Args, "--status") {
			t.Errorf("forbidden 'list --status' pattern observed in call %v (old data layer regression)", c.Args)
		}
	}
}

// assertArgvPresent fails the test if none of the recorded calls has args
// that exactly match want.
func assertArgvPresent(t *testing.T, calls []fakes.RecordedCall, want []string) {
	t.Helper()
	for _, c := range calls {
		if reflect.DeepEqual(c.Args, want) {
			return
		}
	}
	t.Errorf("expected subprocess call with argv %v; got calls: %v", want, formatArgvList(calls))
}

// hasArg reports whether args contains the given token.
func hasArg(args []string, token string) bool {
	for _, a := range args {
		if a == token {
			return true
		}
	}
	return false
}

// formatArgvList returns a readable list of all recorded argv slices.
func formatArgvList(calls []fakes.RecordedCall) [][]string {
	out := make([][]string, len(calls))
	for i, c := range calls {
		out[i] = c.Args
	}
	return out
}

func assertCompactIssueRows(t *testing.T, view string, minIssueMetaLines int) {
	t.Helper()

	lines := strings.Split(view, "\n")
	issueRows := 0
	for _, line := range lines {
		issueRows += strings.Count(line, " P0 ")
		issueRows += strings.Count(line, " P1 ")
		issueRows += strings.Count(line, " P2 ")
		issueRows += strings.Count(line, " P3 ")
	}

	if issueRows < minIssueMetaLines {
		t.Fatalf("expected at least %d rendered issue rows, got %d\nview:\n%s", minIssueMetaLines, issueRows, view)
	}

	for _, forbidden := range []string{"Title:", "Description:", "Assignee:", "Labels:"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("expected board layout to keep compact one-line issue rows without detail-field chrome %q\nview:\n%s", forbidden, view)
		}
	}
}
