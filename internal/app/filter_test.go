package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// filterShell is a loaded shell on the Board with two ready issues, one in
// progress, and two docs.
func filterShell(t *testing.T, keys ...func(*config.Model)) (Model, *fakes.TrackedRepository) {
	t.Helper()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Fix login prompt", "task", 1)
	seedReady(gw, "tm-2", "Triage inbox", "task", 2)
	seedInProgress(gw, "tm-3", "Login audit", "task", 2)
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-8", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2})
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-9", Title: "Session notes", Status: "open", Type: "doc", Priority: 2})

	cfg := config.Default()
	for _, edit := range keys {
		edit(&cfg)
	}
	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m.width, m.height = 160, 30
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 30}})
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.active != mode.Board {
		t.Fatalf("fixture: on %q, want board", m.active)
	}
	return m, gw
}

func typeInto(t *testing.T, m Model, text string) Model {
	t.Helper()
	return applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}})
}

func plainShell(m Model) string {
	return testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
}

// TestEscapeClearsTheBoardQueryBeforeItDoesAnythingElse: the first Escape goes
// to the query and is consumed; the second is the Escape there was before.
func TestEscapeClearsTheBoardQueryBeforeItDoesAnythingElse(t *testing.T) {
	m, _ := filterShell(t)
	m = typeInto(t, m, "login")
	if view := plainShell(m); !strings.Contains(view, "❯ login") || strings.Contains(view, "Triage inbox") {
		t.Fatalf("setup: the board is not filtered:\n%s", view)
	}

	_ = m.showToast("still here", toaster.StyleInfo)
	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	view := plainShell(m)
	if !strings.Contains(view, "❯ filter issues") || !strings.Contains(view, "Triage inbox") {
		t.Fatalf("esc did not clear the query:\n%s", view)
	}
	if !m.toast.Visible() {
		t.Fatal("the esc that cleared the query also dismissed the toast")
	}

	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	if m.toast.Visible() {
		t.Fatal("with an empty query esc must dismiss the toast, as it did before")
	}
}

// TestEscapeClearsTheDocsQueryThenReturnsToBoard: Docs spends the first Escape
// on its query and the second on going home.
func TestEscapeClearsTheDocsQueryThenReturnsToBoard(t *testing.T) {
	m, _ := filterShell(t)
	m = applyMessages(t, m, []tea.Msg{testKey("tab")})
	m = typeInto(t, m, "notes")
	if view := plainShell(m); !strings.Contains(view, "❯ notes") || strings.Contains(view, "Auth redesign") {
		t.Fatalf("setup: docs is not filtered:\n%s", view)
	}
	if got := firstSelectionID(m, mode.Docs); got != "tm-9" {
		t.Fatalf("the shell holds docs selection %q, want the match tm-9", got)
	}

	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	if m.active != mode.Docs {
		t.Fatalf("the esc that cleared the query left docs for %q", m.active)
	}
	if view := plainShell(m); !strings.Contains(view, "❯ filter docs") || !strings.Contains(view, "Auth redesign") {
		t.Fatalf("esc did not clear the docs query:\n%s", view)
	}

	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	if m.active != mode.Board {
		t.Fatalf("with an empty query esc left the shell on %q, want board", m.active)
	}
}

// TestQueriesSurviveTabSwitchDetailRoundTripAndAutoRefresh: each tab keeps its
// own query while the operator is elsewhere and while the rows reload.
func TestQueriesSurviveTabSwitchDetailRoundTripAndAutoRefresh(t *testing.T) {
	m, _ := filterShell(t)
	m = typeInto(t, m, "login")
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("board selection %q, want tm-1", got)
	}

	// To Docs, a query there, and back.
	m = applyMessages(t, m, []tea.Msg{testKey("tab")})
	if view := plainShell(m); !strings.Contains(view, "❯ filter docs") {
		t.Fatalf("the board query leaked into docs:\n%s", view)
	}
	m = typeInto(t, m, "auth")
	m = applyMessages(t, m, []tea.Msg{testKey("shift+tab")})
	if view := plainShell(m); m.active != mode.Board || !strings.Contains(view, "❯ login") || strings.Contains(view, "Triage inbox") {
		t.Fatalf("the board lost its query over a tab switch (active %q):\n%s", m.active, view)
	}

	// Into Detail and back with Escape, which is Detail's here and not the
	// query's.
	m = applyMessages(t, m, []tea.Msg{testKey("enter")})
	if m.active != mode.Detail || m.detail.SelectionID() != "tm-1" {
		t.Fatalf("enter opened %q on %q, want detail on tm-1", m.active, m.detail.SelectionID())
	}
	m = applyMessages(t, m, []tea.Msg{testKey("esc")})
	if view := plainShell(m); m.active != mode.Board || !strings.Contains(view, "❯ login") || strings.Contains(view, "Triage inbox") {
		t.Fatalf("the board lost its query over a detail round trip (active %q):\n%s", m.active, view)
	}

	// An auto refresh reloads the rows under the query.
	m = applyMessages(t, m, []tea.Msg{refreshTickMsg{}})
	if view := plainShell(m); !strings.Contains(view, "❯ login") || strings.Contains(view, "Triage inbox") || !strings.Contains(view, "Fix login prompt") {
		t.Fatalf("the board lost its query over an auto refresh:\n%s", view)
	}

	m = applyMessages(t, m, []tea.Msg{testKey("tab")})
	if view := plainShell(m); !strings.Contains(view, "❯ auth") || strings.Contains(view, "Session notes") {
		t.Fatalf("docs lost its query over a tab switch:\n%s", view)
	}
}

// TestAQueryKeyRunsNoShellAction: a key the query takes is the query's on a
// browse tab, also when a shell action is bound to it; in Detail the binding
// works. Config refuses a binding on every single key the query takes, so the
// binding here is a name of two runes, which is how a paste arrives.
func TestAQueryKeyRunsNoShellAction(t *testing.T) {
	m, _ := filterShell(t, func(cfg *config.Model) {
		cfg.KeyBindings.Shell[config.ShellActionHelp] = []string{"ab"}
	})
	m = typeInto(t, m, "login")

	pasted := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")}
	m = applyMessages(t, m, []tea.Msg{pasted})
	if m.showHelp {
		t.Fatal("a query key on the board opened help")
	}
	if view := plainShell(m); !strings.Contains(view, "❯ loginab") {
		t.Fatalf("the query did not take the key:\n%s", view)
	}

	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlU}, testKey("enter")})
	if m.active != mode.Detail {
		t.Fatalf("fixture: on %q, want detail", m.active)
	}
	next, _ := m.Update(pasted)
	if !next.(Model).showHelp {
		t.Fatal("the key in detail did not run the action bound to it")
	}
}

// TestAnOverlayKeepsTypedKeysFromTheQuery: a key typed into a dialog is the
// dialog's.
func TestAnOverlayKeepsTypedKeysFromTheQuery(t *testing.T) {
	m, _ := filterShell(t)
	next, _ := m.Update(testKey("alt+h"))
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("fixture: help is not open")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("login")})
	m = next.(Model)
	next, _ = m.Update(testKey("alt+h"))
	m = next.(Model)
	if m.showHelp {
		t.Fatal("fixture: the help key did not close help")
	}
	if view := plainShell(m); !strings.Contains(view, "❯ filter issues") {
		t.Fatalf("keys typed under the help overlay reached the board query:\n%s", view)
	}
}

// TestBrowseLegendsNameTheFilter: no key enters the filter, so the legend is
// where an operator learns that typing does.
func TestBrowseLegendsNameTheFilter(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	for _, active := range []mode.ID{mode.Board, mode.Docs} {
		legend := styles.KeyLegend(footerHints(active, keys), 0)
		filter, clear := strings.Index(legend, "type filter"), strings.Index(legend, "esc clear")
		if filter < 0 || clear < filter || strings.Index(legend, "new") < clear {
			t.Errorf("%s legend %q: want type filter, then esc clear, both ahead of new", active, legend)
		}
	}
	if legend := styles.KeyLegend(footerHints(mode.Detail, keys), 0); strings.Contains(legend, "filter") {
		t.Errorf("detail has no filter, and its legend names one: %q", legend)
	}
}
