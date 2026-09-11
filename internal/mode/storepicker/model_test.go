package storepicker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

func TestActiveStoreIsMarkedOnTheRowThatMatchesTheStorePath(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo", "charlie")})
	m.SetSize(100, 24)
	m.SetActiveStorePath("/home/hans/.taskmgr/stores/bravo")
	run(t, m, m.Init())

	// Line by line, so a marker on the wrong row fails rather than passing on a
	// count. This is the only coverage of the Services.ActiveStorePath ->
	// SetActiveStorePath -> Row.Active chain.
	for _, line := range strings.Split(m.View(0, "help"), "\n") {
		plain := ansi.ReplaceAllString(line, "")
		marked := strings.Contains(plain, "active")
		switch {
		case strings.Contains(plain, "bravo"):
			if !marked {
				t.Errorf("the active store's row is not marked: %q", plain)
			}
		case marked:
			t.Errorf("a row other than the active store is marked: %q", plain)
		}
	}
}

// The active store is matched on the store directory, not the project path:
// two registry entries can name the same project, and only one of them is open.
func TestActiveStoreIsNotMarkedFromTheProjectPath(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha", "bravo")})
	m.SetSize(100, 24)
	m.SetActiveStorePath("/home/hans/dev/bravo") // the project path, not the store
	run(t, m, m.Init())

	if strings.Contains(ansi.ReplaceAllString(m.View(0, "help"), ""), "active") {
		t.Error("a project path matched the active store; only the store directory may")
	}
}

// An empty active path is the no-store start. Nothing is marked, and in
// particular an entry whose StorePath is empty must not match it.
func TestNoRowIsMarkedWithoutAnActiveStore(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: []storecatalog.Entry{
		{Name: "alpha", ProjectPath: "/home/hans/dev/alpha", Health: storecatalog.HealthOK},
	}})
	m.SetSize(100, 24)
	run(t, m, m.Init())

	if strings.Contains(ansi.ReplaceAllString(m.View(0, "help"), ""), "active") {
		t.Error("a row is marked active with no store open")
	}
}

// Opening the picker always lists again, including while a listing dispatched
// for an earlier open is still on its way. The shell states that guarantee at
// the picker key; without it a store registered in another terminal never
// appears until the app restarts.
func TestEveryOpenListsAgainEvenWithOneStillInFlight(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{Entries: entries("alpha")}
	m := newModel(t, catalog)
	m.SetSize(100, 24)

	stale := m.Init() // first open; the result is deliberately not delivered yet

	catalog.Entries = entries("alpha", "registered-elsewhere")
	fresh := m.Init() // reopened
	if fresh == nil {
		t.Fatal("the second open dispatched no listing")
	}
	run(t, m, fresh)

	if got := len(m.Entries()); got != 2 {
		t.Fatalf("listed %d entries after reopening, want the 2 now registered", got)
	}

	// The first open's result arrives late and must not replace them.
	m.Update(stale())
	if got := len(m.Entries()); got != 2 {
		t.Errorf("a stale listing replaced the current one: %d entries, want 2", got)
	}
	if m.IsLoading() {
		t.Error("a stale listing put the picker back into a loading state")
	}
}

// A failed reload narrows the row window by one for the inline error line, so
// the offsets have to be re-clamped even though the rows did not change.
// docs/DESIGN-GUIDE.md: the chevron staying on a row that actually renders is a
// contract.
func TestFailedReloadKeepsTheChevronOnScreen(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		names = append(names, fmt.Sprintf("store-%02d", i))
	}
	catalog := &fakes.FakeStoreCatalog{Entries: entries(names...)}
	m := newModel(t, catalog)
	m.SetSize(100, 24)
	run(t, m, m.Init())

	for i := 0; i < 40; i++ {
		m.HandleKey(key("j"))
	}
	if m.scrollOffset == 0 {
		t.Fatal("expected a scrolled window; the fixture no longer exercises the clamp")
	}

	catalog.Err = errors.New("registry is corrupt")
	_, cmd := m.HandleKey(key("r"))
	run(t, m, cmd)

	if !strings.Contains(ansi.ReplaceAllString(m.View(0, "help"), ""), "\u203a") {
		t.Errorf("the selection chevron left the screen after a failed reload:\n%s", m.View(0, "help"))
	}
}

// The scroll window is what the renderer actually draws. Every other controller
// test here fits inside one window, so this is the only path that exercises the
// arithmetic.
func TestScrollWindowFollowsTheSelectionThroughALongList(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		names = append(names, fmt.Sprintf("store-%02d", i))
	}
	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries(names...)})
	m.SetSize(100, 24)
	run(t, m, m.Init())

	capacity := m.itemCapacity()
	for i := 0; i < 49; i++ {
		m.HandleKey(key("j"))
		if m.selectedRow < m.scrollOffset || m.selectedRow >= m.scrollOffset+capacity {
			t.Fatalf("selection %d left the window [%d,%d) after %d moves", m.selectedRow, m.scrollOffset, m.scrollOffset+capacity, i+1)
		}
	}
	if m.scrollOffset != len(names)-capacity {
		t.Errorf("scrollOffset at the end: got %d, want %d", m.scrollOffset, len(names)-capacity)
	}

	for i := 0; i < 49; i++ {
		m.HandleKey(key("k"))
	}
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset back at the top: got %d, want 0", m.scrollOffset)
	}
}

// The picker only claims movement and reload. Everything else falls through to
// the shell, which is what keeps Escape, quit and help working while it is up.
func TestUnclaimedKeysFallThroughToTheShell(t *testing.T) {
	t.Parallel()

	m := newModel(t, &fakes.FakeStoreCatalog{Entries: entries("alpha")})
	run(t, m, m.Init())

	for _, k := range []tea.KeyMsg{{Type: tea.KeyEsc}, key("?"), key("s"), key("1"), {Type: tea.KeyTab}} {
		if consumed, _ := m.HandleKey(k); consumed {
			t.Errorf("picker consumed %q; it must reach the shell", k.String())
		}
	}
}
