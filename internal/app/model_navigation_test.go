package app

// Mode switching and in-mode navigation: board selection, tab behaviour, and
// the configured keybindings that drive them.

import (
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

// tab and shift+tab drive the header tab strip: Board, Docs, in
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

	for _, want := range []mode.ID{mode.Docs, mode.Board} {
		m = press(m, tab)
		if m.active != want {
			t.Fatalf("expected tab to move to %s, got %s", want, m.active)
		}
		if m.lastBrowse != want {
			t.Fatalf("expected lastBrowse to follow the tab strip to %s, got %s", want, m.lastBrowse)
		}
	}

	for _, want := range []mode.ID{mode.Docs, mode.Board} {
		m = press(m, shiftTab)
		if m.active != want {
			t.Fatalf("expected shift+tab to move to %s, got %s", want, m.active)
		}
	}

	// From Detail the cycle resumes from the tab we drilled in from.
	m = press(m, testKey("enter"))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after enter, got %s", m.active)
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
			config.ShellActionHelp: {"F1"},
			config.ShellActionQuit: {"ctrl+q"},
		},
		Board: map[string][]string{
			config.BoardActionMoveRight: {"alt+d"},
			config.BoardActionMoveDown:  {"alt+s"},
		},
	})

	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); !strings.Contains(footer, "f1 help") || !strings.Contains(footer, "ctrl+q quit") {
		t.Fatalf("expected footer to reflect configured bindings, got:\n%s", footer)
	}

	next, cmd := m.Update(testKey("alt+d"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected configured board move-right key to select tm-2, got %q", got)
	}

	next, _ = m.Update(testKey("alt+h"))
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
// header tab strip — mode.BrowseModes: Board, Docs — wrapping at both ends.
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
		if got := nextMode(mode.Docs, mode.Docs); got != mode.Board {
			t.Errorf("nextMode(Docs, Docs) = %s; want Board", got)
		}
		// From Detail the cycle resumes from lastBrowse.
		if got := nextMode(mode.Detail, mode.Docs); got != mode.Board {
			t.Errorf("nextMode(Detail, Docs) = %s; want Board", got)
		}
		if got := nextMode(mode.Detail, mode.Board); got != mode.Docs {
			t.Errorf("nextMode(Detail, Board) = %s; want Docs", got)
		}
	})

	t.Run("prevMode_backward", func(t *testing.T) {
		t.Parallel()

		if got := prevMode(mode.Board, mode.Board); got != mode.Docs {
			t.Errorf("prevMode(Board, Board) = %s; want Docs", got)
		}
		if got := prevMode(mode.Docs, mode.Docs); got != mode.Board {
			t.Errorf("prevMode(Docs, Docs) = %s; want Board", got)
		}
		if got := prevMode(mode.Detail, mode.Board); got != mode.Docs {
			t.Errorf("prevMode(Detail, Board) = %s; want Docs", got)
		}
	})

	t.Run("next_and_prev_are_inverses", func(t *testing.T) {
		t.Parallel()

		for _, tab := range mode.BrowseModes {
			if got := prevMode(nextMode(tab, tab), tab); got != tab {
				t.Errorf("prevMode(nextMode(%s)) = %s; want %s", tab, got, tab)
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

// TestBarePrintableKeysTypeIntoTheFilterAndRunNoAction: on a browse tab a bare
// letter, digit or sign is typed into the query and runs no action: it changes
// no surface and opens no overlay. The store is read only for the detail of a
// selection the filter moved.
func TestBarePrintableKeysTypeIntoTheFilterAndRunNoAction(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-9", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := applyMessages(t, mustNewModel(t, services), nil)
	m = applyMessages(t, m, runBatch(m.Init()))

	for _, tab := range []mode.ID{mode.Board, mode.Docs} {
		if m.active != tab {
			m = applyMessages(t, m, []tea.Msg{testKey("tab")})
		}
		if m.active != tab {
			t.Fatalf("fixture: on %q, want %q", m.active, tab)
		}

		for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789?/>" {
			mark := len(gw.Calls())
			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}})
			if m.active != tab || m.overlayOpen() {
				t.Errorf("%q on %s: active %q, overlay %v", string(r), tab, m.active, m.overlayOpen())
			}
			if !strings.Contains(m.View(), "❯ "+string(r)) {
				t.Errorf("%q on %s is not on the query line:\n%s", string(r), tab, m.View())
			}
			for _, call := range gw.Calls()[mark:] {
				if call.Method != fakes.MethodIssue {
					t.Errorf("%q on %s called %s", string(r), tab, call.Method)
				}
			}

			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlU}})
		}
	}
}

// TestEscapeFromDocsReturnsToBoard: Board is the home tab.
func TestEscapeFromDocsReturnsToBoard(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = applyMessages(t, m, []tea.Msg{testKey("tab")})
	if m.active != mode.Docs {
		t.Fatalf("fixture: on %q, want docs", m.active)
	}

	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	if m.active != mode.Board || m.lastBrowse != mode.Board {
		t.Fatalf("esc from docs left the shell on %q (last browse %q)", m.active, m.lastBrowse)
	}
}
