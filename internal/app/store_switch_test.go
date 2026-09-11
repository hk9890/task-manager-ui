package app

// Switching the running app to another store: what the switch rebuilds, what
// it leaves alone, and what it drops.

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// twoStores is an app open on store alpha, with store bravo registered and
// openable. Each store holds one ready issue whose title names its store, so a
// frame showing the wrong store's issues says so in plain text.
type twoStores struct {
	m       Model
	catalog *fakes.FakeStoreCatalog
	runner  *fakes.FakeProcessRunner
	bravo   storecatalog.Opened
}

func newTwoStores(t *testing.T) twoStores {
	t.Helper()

	alpha := fakes.NewTracked()
	seedReady(alpha, "alp-1", "Alpha store issue", "task", 1)

	bravoRepo := fakes.NewTracked()
	seedReady(bravoRepo, "brv-1", "Bravo store issue", "task", 1)

	bravo := storecatalog.Opened{
		Repo:        bravoRepo,
		Name:        "bravo",
		ProjectPath: t.TempDir(),
		StorePath:   "/stores/bravo",
	}
	catalog := &fakes.FakeStoreCatalog{
		Entries: []storecatalog.Entry{
			{Name: "alpha", ProjectPath: "/home/hans/dev/alpha", StorePath: "/stores/alpha", Health: storecatalog.HealthOK},
			{Name: "bravo", ProjectPath: bravo.ProjectPath, StorePath: bravo.StorePath, Health: storecatalog.HealthOK},
			{Name: "gone", ProjectPath: "/home/hans/dev/gone", StorePath: "/stores/gone", Health: storecatalog.HealthDangling},
		},
		Opens: map[string]storecatalog.Opened{"bravo": bravo},
	}

	services, err := NewServices(alpha, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	runner := &fakes.FakeProcessRunner{}
	services.ProcessRunner = runner
	services.StoreCatalog = catalog
	services.ActiveStorePath = "/stores/alpha"
	services.StoreName = "alpha"

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	return twoStores{m: m, catalog: catalog, runner: runner, bravo: bravo}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, cmd := m.Update(msg)
		m = applyMessages(t, next.(Model), runBatch(cmd))
	}
	return m
}

// switchToBravo opens the picker, moves to the bravo row and opens it.
func switchToBravo(t *testing.T, m Model) Model {
	t.Helper()
	return press(t, m, "s", "j", "enter")
}

func TestSwitchingStoresReplacesEverySurfaceInPlace(t *testing.T) {
	s := newTwoStores(t)
	if !strings.Contains(pickerView(s.m), "Alpha store issue") {
		t.Fatalf("fixture: expected alpha's issue on the board before the switch:\n%s", pickerView(s.m))
	}

	m := switchToBravo(t, s.m)

	if m.active != mode.Board {
		t.Errorf("active mode after the switch: got %q, want %q", m.active, mode.Board)
	}
	view := pickerView(m)
	if !strings.Contains(view, "Bravo store issue") {
		t.Errorf("the board does not show the opened store's issue:\n%s", view)
	}
	if strings.Contains(view, "Alpha store issue") {
		t.Errorf("the board still shows the previous store's issue:\n%s", view)
	}
	if !strings.Contains(view, "bravo") {
		t.Errorf("the header does not name the opened store:\n%s", view)
	}
	if got := s.catalog.Opened(); len(got) != 1 || got[0] != "bravo" {
		t.Errorf("catalog opened %v, want [bravo]", got)
	}

	// Docs and search are rebuilt too, not only the board: their first open
	// after the switch reads the new store.
	m = press(t, m, "2")
	if m.search == nil || m.active != mode.Search {
		t.Fatalf("expected to reach Search after the switch, got %q", m.active)
	}
}

// The epoch guard. A read issued against the old store can finish after the
// switch; its result must not reach the new store's board. Cancelling the
// context cannot stop it — the command has already produced its message.
func TestResultIssuedAgainstThePreviousStoreIsDropped(t *testing.T) {
	s := newTwoStores(t)

	// A board reload against alpha: run the command so its result exists, but
	// hold the messages back until after the switch.
	next, reload := s.m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m := next.(Model)
	stale := runBatch(reload)
	if len(stale) == 0 {
		t.Fatal("fixture: the board reload produced no messages to hold back")
	}

	m = switchToBravo(t, m)
	m = applyMessages(t, m, stale)

	view := pickerView(m)
	if strings.Contains(view, "Alpha store issue") {
		t.Errorf("a result issued against the previous store reached the new board:\n%s", view)
	}
	if !strings.Contains(view, "Bravo store issue") {
		t.Errorf("the new store's issue is gone after the stale result arrived:\n%s", view)
	}
}

func TestSwitchingStoresCancelsThePreviousStoresContext(t *testing.T) {
	s := newTwoStores(t)
	previous := s.m.ctx

	m := switchToBravo(t, s.m)

	if previous.Err() == nil {
		t.Error("the previous store's context is still live after the switch")
	}
	if m.ctx.Err() != nil {
		t.Error("the new store's context is already cancelled")
	}
	if m.appCtx.Err() != nil {
		t.Error("switching stores cancelled the application context")
	}
}

// {{project.root}} follows the active store: a launcher with no explicit
// work_dir runs in the project of the store now open, not the one the app
// started on.
func TestLauncherRunsInTheOpenedStoresProject(t *testing.T) {
	s := newTwoStores(t)
	m := switchToBravo(t, s.m)

	m = press(t, m, "3") // Detail on bravo's issue
	if m.active != mode.Detail {
		t.Fatalf("expected Detail after the switch, got %q", m.active)
	}
	m = press(t, m, "l") // launch_shell_command; its default definition has no work_dir

	calls := s.runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("launcher ran %d processes, want 1", len(calls))
	}
	if calls[0].Dir != s.bravo.ProjectPath {
		t.Errorf("launcher ran in %q, want the opened store's project %q", calls[0].Dir, s.bravo.ProjectPath)
	}
}

// Opening the store that is already open leaves the picker and rebuilds
// nothing: every surface would reload to show exactly what was there.
func TestOpeningTheActiveStoreIsANoOp(t *testing.T) {
	s := newTwoStores(t)
	epoch := s.m.storeEpoch

	m := press(t, s.m, "s", "enter") // the first row is alpha, the active store

	if m.active != mode.Board {
		t.Errorf("active mode: got %q, want the board the picker was opened from", m.active)
	}
	if m.storeEpoch != epoch {
		t.Error("opening the active store rebuilt the surfaces")
	}
	if got := s.catalog.Opened(); len(got) != 0 {
		t.Errorf("catalog opened %v, want nothing", got)
	}
}

func TestAnUnusableStoreIsNotOpened(t *testing.T) {
	s := newTwoStores(t)
	epoch := s.m.storeEpoch

	m := press(t, s.m, "s", "j", "j", "enter") // the dangling row

	if m.storeEpoch != epoch {
		t.Error("an unusable store was switched to")
	}
	if got := s.catalog.Opened(); len(got) != 0 {
		t.Errorf("catalog opened %v for an unusable entry, want nothing", got)
	}
	if !m.toast.Visible() || !strings.Contains(m.toast.View(), "cannot be opened") {
		t.Errorf("expected a toast saying the store cannot be opened, got %q", m.toast.View())
	}
	if m.active != mode.StorePicker {
		t.Errorf("active mode: got %q, want the picker to stay up", m.active)
	}
}

// A store that fails to open leaves the app on the store it was already on.
func TestAFailedOpenKeepsTheCurrentStore(t *testing.T) {
	s := newTwoStores(t)
	s.catalog.OpenErr = errors.New("permission denied")
	epoch := s.m.storeEpoch

	m := switchToBravo(t, s.m)

	if m.storeEpoch != epoch {
		t.Error("a failed open switched stores anyway")
	}
	if m.services.StoreName != "alpha" {
		t.Errorf("active store after a failed open: got %q, want alpha", m.services.StoreName)
	}
	if !m.toast.Visible() || !strings.Contains(m.toast.View(), "permission denied") {
		t.Errorf("expected a toast naming the failure, got %q", m.toast.View())
	}
}

// After a switch the picker marks the newly opened store as the active one.
func TestThePickerMarksTheOpenedStoreActive(t *testing.T) {
	s := newTwoStores(t)
	m := switchToBravo(t, s.m)
	m = press(t, m, "s")
	// The switch raises an "Opened store bravo" toast over the picker; it would
	// match the row assertions below without being a row.
	m.toast = m.toast.Hide()

	for _, line := range strings.Split(pickerView(m), "\n") {
		marked := strings.Contains(line, "active")
		switch {
		case strings.Contains(line, "bravo"):
			if !marked {
				t.Errorf("the opened store is not marked active: %q", line)
			}
		case marked:
			t.Errorf("a store other than the opened one is marked active: %q", line)
		}
	}
}
