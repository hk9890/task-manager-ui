package storepicker

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

func entries(names ...string) []storecatalog.Entry {
	out := make([]storecatalog.Entry, 0, len(names))
	for _, name := range names {
		out = append(out, storecatalog.Entry{
			Name:        name,
			ProjectPath: "/home/hans/dev/" + name,
			StorePath:   "/home/hans/.taskmgr/stores/" + name,
			Health:      storecatalog.HealthOK,
		})
	}
	return out
}

func newModel(t *testing.T, catalog storecatalog.Catalog) *Model {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	return NewModel(context.Background(), catalog, nil, keys)
}

// run executes a Cmd and feeds the message back, which is what the shell does
// for real.
func run(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()

	if cmd == nil {
		t.Fatal("expected a Cmd")
	}
	m.Update(cmd())
}

func key(runes string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)}
}

func TestInitListsTheCatalog(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo")}
	m := newModel(t, catalog)

	run(t, m, m.Init())

	if got := len(m.Entries()); got != 2 {
		t.Fatalf("listed %d entries, want 2", got)
	}
	if m.IsLoading() {
		t.Error("still loading after the listing arrived")
	}
	selected, ok := m.SelectedEntry()
	if !ok || selected.Name != "alpha" {
		t.Errorf("selection: got %+v (ok=%v), want the first entry", selected, ok)
	}
}

func TestListingIsInFlightUntilItArrives(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha")})

	cmd := m.Init()
	if !m.IsLoading() {
		t.Error("expected the picker to report loading while the listing is in flight")
	}
	run(t, m, cmd)
	if m.IsLoading() {
		t.Error("expected loading to clear once the listing arrived")
	}
}

// A second reload while one is in flight must not fire a second read: the
// registry answer would be identical and the first result would settle the
// model twice.
func TestReloadIsSuppressedWhileOneIsInFlight(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{Entries: entries("alpha")}
	m := newModel(t, catalog)

	first := m.Init()
	consumed, second := m.HandleKey(key("r"))
	if !consumed {
		t.Fatal("expected the reload key to be consumed")
	}
	if second != nil {
		t.Error("expected no second listing while one is in flight")
	}

	run(t, m, first)
	if got := catalog.Calls(); got != 1 {
		t.Errorf("catalog read %d times, want 1", got)
	}
}

func TestReloadAfterTheListingSettlesReadsAgain(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{Entries: entries("alpha")}
	m := newModel(t, catalog)
	run(t, m, m.Init())

	_, cmd := m.HandleKey(key("r"))
	run(t, m, cmd)

	if got := catalog.Calls(); got != 2 {
		t.Errorf("catalog read %d times, want 2", got)
	}
}

func TestMovementClampsToTheList(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo", "charlie")})
	m.SetSize(100, 24)
	run(t, m, m.Init())

	for i := 0; i < 5; i++ {
		m.HandleKey(key("j"))
	}
	selected, _ := m.SelectedEntry()
	if selected.Name != "charlie" {
		t.Errorf("after moving past the end: got %q, want the last entry", selected.Name)
	}

	for i := 0; i < 5; i++ {
		m.HandleKey(key("k"))
	}
	selected, _ = m.SelectedEntry()
	if selected.Name != "alpha" {
		t.Errorf("after moving past the start: got %q, want the first entry", selected.Name)
	}
}

func TestMovementOnAnEmptyListIsSafe(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{})
	run(t, m, m.Init())

	m.HandleKey(key("j"))
	if _, ok := m.SelectedEntry(); ok {
		t.Error("an empty registry has no selected entry")
	}
}

// A failed listing keeps whatever was on screen: the operator can still read
// the stores listed a moment ago, and the inline error says why they may be
// stale.
func TestFailedReloadKeepsTheStaleRows(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo")}
	m := newModel(t, catalog)
	run(t, m, m.Init())

	catalog.Err = errors.New("registry is corrupt")
	_, cmd := m.HandleKey(key("r"))
	run(t, m, cmd)

	if got := len(m.Entries()); got != 2 {
		t.Errorf("kept %d rows after a failed reload, want the 2 already listed", got)
	}
	if !strings.Contains(m.View(0, "help"), "registry is corrupt") {
		t.Error("expected the failure to be named on the picker")
	}
}

// A programmatic embed can build the shell without a catalog. That must reach
// the operator as an error on the picker rather than a panic.
func TestNilCatalogReportsRatherThanPanics(t *testing.T) {
	t.Parallel()

	m := newModel(t, nil)
	run(t, m, m.Init())

	if !strings.Contains(m.View(0, "help"), "no store catalog") {
		t.Errorf("expected the missing catalog to be named on the picker:\n%s", m.View(0, "help"))
	}
}

func TestActiveStoreIsMarked(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo")})
	m.SetSize(100, 24)
	m.SetActiveStorePath("/home/hans/.taskmgr/stores/bravo")
	run(t, m, m.Init())

	view := m.View(0, "help")
	if strings.Count(view, "active") != 1 {
		t.Errorf("expected exactly one row marked active:\n%s", view)
	}
}

// The picker only claims movement and reload. Everything else falls through to
// the shell, which is what keeps Escape, quit and help working while it is up.
func TestUnclaimedKeysFallThroughToTheShell(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha")})
	run(t, m, m.Init())

	for _, k := range []tea.KeyMsg{{Type: tea.KeyEsc}, key("?"), key("s"), {Type: tea.KeyEnter}} {
		if consumed, _ := m.HandleKey(k); consumed {
			t.Errorf("picker consumed %q; it must reach the shell", k.String())
		}
	}
}
