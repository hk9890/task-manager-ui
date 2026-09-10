package app

// The store picker: the key that opens it, what it renders instead of, and
// what leaving it restores.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func pickerServices(t *testing.T, catalog *fakes.FakeStoreCatalog, activeStorePath string) Services {
	t.Helper()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress one", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.StoreCatalog = catalog
	services.ActiveStorePath = activeStorePath
	return services
}

func registryEntries() []storecatalog.Entry {
	return []storecatalog.Entry{
		{Name: "task-manager-ui", ProjectPath: "/home/hans/dev/task-manager-ui", StorePath: "/stores/task-manager-ui", Health: storecatalog.HealthOK},
		{Name: "task-manager", ProjectPath: "/home/hans/dev/task-manager", StorePath: "/stores/task-manager", Health: storecatalog.HealthOK},
		{Name: "moved-away", ProjectPath: "/home/hans/dev/moved-away", StorePath: "/stores/moved-away", Health: storecatalog.HealthDangling},
	}
}

func pickerView(m Model) string {
	return testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
}

func openPicker(t *testing.T, m Model) Model {
	t.Helper()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(Model)
	if m.active != mode.StorePicker {
		t.Fatalf("active mode after the picker key: got %q, want %q", m.active, mode.StorePicker)
	}
	return applyMessages(t, m, runBatch(cmd))
}

func TestStorePickerOpensOnItsKeyAndListsTheRegistry(t *testing.T) {
	catalog := &fakes.FakeStoreCatalog{Entries: registryEntries()}
	m := mustNewModel(t, pickerServices(t, catalog, "/stores/task-manager-ui"))
	m = applyMessages(t, m, runBatch(m.Init()))
	m = openPicker(t, m)

	view := pickerView(m)
	for _, want := range []string{"Task Stores", "task-manager-ui", "task-manager", "moved-away"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q on the picker:\n%s", want, view)
		}
	}
	// An unusable entry stays listed with its health named rather than hidden.
	if !strings.Contains(view, "dangling") {
		t.Errorf("expected the dangling entry's health to be named:\n%s", view)
	}
	if !strings.Contains(view, "active") {
		t.Errorf("expected the active store to be marked:\n%s", view)
	}
}

// The picker renders instead of the shell, so the tab strip and the shell
// footer are absent while it is up (docs/DESIGN-GUIDE.md).
func TestStorePickerReplacesTheShellChrome(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	before := testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
	if !strings.Contains(before, "Task Manager UI") {
		t.Fatalf("expected the shell title before the picker opens:\n%s", before)
	}

	m = openPicker(t, m)
	view := testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
	if strings.Contains(view, "Task Manager UI") {
		t.Errorf("the shell header is still rendered under the picker:\n%s", view)
	}
	if !strings.Contains(view, "esc back") {
		t.Errorf("expected the picker's own help line:\n%s", view)
	}
}

// Escape puts the operator back where they were. The browse tabs are untouched
// while the picker is up, so their selection and scroll position survive.
func TestStorePickerEscapeRestoresTheBoardUntouched(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	selectionBefore := firstSelectionID(m, mode.Board)
	boardBefore := m.board.View(0)

	m = openPicker(t, m)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("active mode after Escape: got %q, want %q", m.active, mode.Board)
	}
	if got := firstSelectionID(m, mode.Board); got != selectionBefore {
		t.Errorf("board selection: got %q, want the %q it was before the picker", got, selectionBefore)
	}
	if m.board.View(0) != boardBefore {
		t.Error("the board rendered differently after a round trip through the picker")
	}
}

// The picker is opened from wherever the operator was, so Escape returns
// there — not to the home tab.
func TestStorePickerEscapeReturnsToTheTabItWasOpenedFrom(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	if m.active != mode.Docs {
		t.Fatalf("expected to be on Docs, got %q", m.active)
	}

	m = openPicker(t, m)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	if m.active != mode.Docs {
		t.Errorf("active mode after Escape: got %q, want %q", m.active, mode.Docs)
	}
}

// Every open re-lists: a store registered from another terminal must appear
// without restarting the app. Asserted on what the second open renders, not on
// a call count — a picker that issues the read and discards the result passes a
// count.
func TestStorePickerRelistsOnEveryOpen(t *testing.T) {
	catalog := &fakes.FakeStoreCatalog{Entries: registryEntries()}
	m := mustNewModel(t, pickerServices(t, catalog, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	m = openPicker(t, m)
	if strings.Contains(pickerView(m), "registered-elsewhere") {
		t.Fatal("the fixture already lists the store the reopen is supposed to discover")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	catalog.Entries = append(registryEntries(), storecatalog.Entry{
		Name:        "registered-elsewhere",
		ProjectPath: "/home/hans/dev/registered-elsewhere",
		StorePath:   "/stores/registered-elsewhere",
		Health:      storecatalog.HealthOK,
	})
	m = openPicker(t, m)

	if got := catalog.Calls(); got != 2 {
		t.Errorf("catalog read %d times across two opens, want 2", got)
	}
	if !strings.Contains(pickerView(m), "registered-elsewhere") {
		t.Errorf("the reopened picker does not show the newly registered store:\n%s", pickerView(m))
	}
}

// The picker key must not reach a browse tab as a keystroke, and typing it
// into the search query must not open the picker.
func TestStorePickerKeyDoesNotOpenFromTheSearchQueryField(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	if m.active == mode.StorePicker {
		t.Error("the picker key opened the picker while the search query field had focus")
	}
}

// The picker is not a tab, so cycling the header strip must never land on it.
func TestStorePickerIsNotInTheTabCycle(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	for i := 0; i < len(mode.BrowseModes)+2; i++ {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = applyMessages(t, next.(Model), runBatch(cmd))
		if m.active == mode.StorePicker {
			t.Fatal("tab cycling reached the store picker; it is not a tab")
		}
	}
}

// An overlay opened over the picker must be drawn. One that is live but not
// rendered still holds the keyboard, so the picker looks frozen and quit stops
// working, with no visible cause.
func TestOverlaysRenderOverThePicker(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))
	m = openPicker(t, m)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	if !m.showHelp {
		t.Fatal("expected the help overlay to open over the picker")
	}
	if !strings.Contains(pickerView(m), "Keyboard Help") {
		t.Errorf("the help overlay is open but not drawn over the picker:\n%s", pickerView(m))
	}

	// The same must hold for the action modal, whichever surface armed it.
	m.showActionModal = true
	m.actionModal = m.help
	if !strings.Contains(pickerView(m), "Keyboard Help") {
		t.Error("an open action modal is not drawn over the picker")
	}
}

// The store listing is the shell's own message. An overlay that swallows it
// leaves the picker's in-flight state latched: nothing else ever clears it, so
// it reads "Reading the central store registry…" and spins for the rest of the
// session.
func TestOpenOverlayDoesNotSwallowTheStoreListing(t *testing.T) {
	catalog := &fakes.FakeStoreCatalog{Entries: registryEntries()}
	m := mustNewModel(t, pickerServices(t, catalog, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(Model)
	listing := runBatch(cmd)

	// The help overlay opens before the listing lands.
	next, helpCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = applyMessages(t, next.(Model), runBatch(helpCmd))

	m = applyMessages(t, m, listing)

	if m.storePicker.IsLoading() {
		t.Error("the listing was swallowed by the overlay; the picker is still loading")
	}
	if len(m.storePicker.Entries()) != len(registryEntries()) {
		t.Errorf("the picker listed %d entries, want %d", len(m.storePicker.Entries()), len(registryEntries()))
	}
	if len(m.loadingStates()) != 0 {
		t.Errorf("the shell still reports work in flight: %+v", m.loadingStates())
	}
}

// Every shell action that acts on "the selected issue" resolves it from a
// browse tab that is not on screen while the picker is up. Editing, closing or
// commenting on an issue the operator cannot see is the worst outcome
// available, so the picker swallows them.
func TestIssueActionsAreInertOnThePicker(t *testing.T) {
	for _, key := range []string{"e", "c", "u", "x", "a"} {
		t.Run(key, func(t *testing.T) {
			gw := fakes.NewTracked()
			seedReady(gw, "tm-1", "Ready first", "task", 1)
			services, err := NewServices(gw, config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}
			services.StoreCatalog = &fakes.FakeStoreCatalog{Entries: registryEntries()}

			m := mustNewModel(t, services)
			m = applyMessages(t, m, runBatch(m.Init()))
			m = openPicker(t, m)

			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			m = applyMessages(t, next.(Model), runBatch(cmd))

			if m.showActionModal {
				t.Errorf("%q armed a mutation dialog from the picker", key)
			}
			if m.pendingDialog.active {
				t.Errorf("%q armed a pending dialog from the picker", key)
			}
			if m.active != mode.StorePicker {
				t.Errorf("%q moved the operator off the picker, to %q", key, m.active)
			}
		})
	}
}

// A browse tab's background load landing under the picker must not retarget the
// Detail surface: currentSelection() answers from lastBrowse for any non-browse
// mode, so the detail the operator returns to would silently change.
func TestSelectionLandingUnderThePickerDoesNotLoadDetail(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))
	m = openPicker(t, m)
	// Whatever the board had already loaded stays the detail target; the point
	// is that a selection arriving under the picker does not move it.
	targetBefore := strings.TrimSpace(m.detail.TargetID())

	next, cmd := m.Update(mode.SelectionChangedMsg{
		Mode:      mode.Board,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress"}},
	})
	m = applyMessages(t, next.(Model), runBatch(cmd))

	if m.detail.IsLoading() {
		t.Error("a selection landing under the picker started a detail load")
	}
	if got := strings.TrimSpace(m.detail.TargetID()); got != targetBefore {
		t.Errorf("the detail surface was retargeted from %q to %q while the picker was up", targetBefore, got)
	}
	// The selection itself is still recorded; it is only the detail load that waits.
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Errorf("board selection: got %q, want tm-2", got)
	}
}

// The picker's loading state belongs to the picker's own spinner. Left reported
// after the operator switched tabs, it spins a browse header for a surface that
// is not on screen.
func TestPickerLoadingIsNotReportedOnAnotherTab(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(Model)
	listing := runBatch(cmd) // held back, so the listing is still in flight

	if len(m.loadingStates()) == 0 {
		t.Fatal("expected the in-flight listing to be reported while the picker is on screen")
	}

	next, tabCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = applyMessages(t, next.(Model), runBatch(tabCmd))
	if m.active != mode.Board {
		t.Fatalf("expected to be on Board, got %q", m.active)
	}
	if got := m.loadingStates(); len(got) != 0 {
		t.Errorf("Board reports the picker's listing as in flight: %+v", got)
	}

	m = applyMessages(t, m, listing)
}
