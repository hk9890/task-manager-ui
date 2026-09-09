package app

// The store picker: the key that opens it, what it renders instead of, and
// what leaving it restores.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
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

	view := testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
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
// without restarting the app.
func TestStorePickerRelistsOnEveryOpen(t *testing.T) {
	catalog := &fakes.FakeStoreCatalog{Entries: registryEntries()}
	m := mustNewModel(t, pickerServices(t, catalog, ""))
	m = applyMessages(t, m, runBatch(m.Init()))

	m = openPicker(t, m)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	m = openPicker(t, m)

	if got := catalog.Calls(); got != 2 {
		t.Errorf("catalog read %d times across two opens, want 2", got)
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
