package app

// Starting with no store to open: the app opens on the store picker and holds
// the operator there until a store is open.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository/nostore"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

type noStoreStart struct {
	m       Model
	catalog *fakes.FakeStoreCatalog
}

func newNoStoreStart(t *testing.T) noStoreStart {
	t.Helper()

	bravoRepo := fakes.NewTracked()
	seedReady(bravoRepo, "brv-1", "Bravo store issue", "task", 1)
	catalog := &fakes.FakeStoreCatalog{
		Entries: []storecatalog.Entry{
			{Name: "bravo", ProjectPath: "/home/hans/dev/bravo", StorePath: "/stores/bravo", Health: storecatalog.HealthOK},
		},
		Opens: map[string]storecatalog.Opened{"bravo": {
			Repo: bravoRepo, Name: "bravo", ProjectPath: t.TempDir(), StorePath: "/stores/bravo",
		}},
	}

	services, err := NewServices(nostore.New(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.StoreCatalog = catalog

	m := mustNewModelWithOptions(t, services, RuntimeOptions{UnresolvedStore: "No task-manager store for /tmp/empty"})
	m = applyMessages(t, m, runBatch(m.Init()))
	return noStoreStart{m: m, catalog: catalog}
}

func quits(cmd tea.Cmd) bool {
	for _, msg := range runBatch(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func TestNoStoreStartOpensOnThePickerAndSaysWhy(t *testing.T) {
	s := newNoStoreStart(t)

	if s.m.active != mode.StorePicker {
		t.Fatalf("active mode: got %q, want the store picker", s.m.active)
	}
	view := pickerView(s.m)
	if !strings.Contains(view, "Task Stores") || !strings.Contains(view, "bravo") {
		t.Errorf("expected the picker listing the registry:\n%s", view)
	}
	if !s.m.toast.Visible() || !strings.Contains(s.m.toast.View(), "/tmp/empty") {
		t.Errorf("expected a toast naming why there is no store, got %q", s.m.toast.View())
	}
	// The help line must not promise "back" when Escape quits.
	if !strings.Contains(view, "esc quit") || strings.Contains(view, "esc back") {
		t.Errorf("the help line misnames what Escape does with no store open:\n%s", view)
	}
	// The fatal "no store here" screen belongs to a store that vanished after
	// resolving, not to a start that never had one.
	if s.m.fatalErrTitle != "" {
		t.Errorf("the fatal error screen is armed: %q", s.m.fatalErrTitle)
	}
}

// With no store there is nothing to health-check and no board to load; a
// start that asked the no-store repository anything would only collect errors.
func TestNoStoreStartReadsNoStore(t *testing.T) {
	tracked := fakes.NewTracked()
	services, err := NewServices(tracked, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.StoreCatalog = &fakes.FakeStoreCatalog{}

	m := mustNewModelWithOptions(t, services, RuntimeOptions{UnresolvedStore: "no store"})
	mark := tracked.CallCount()
	applyMessages(t, m, runBatch(m.Init()))

	if got := tracked.CallCount() - mark; got != 0 {
		t.Errorf("the no-store start made %d repository calls, want none", got)
	}
}

// The refresh tick re-arms only from its own handler. A no-store start that did
// not arm it left every store opened afterwards without auto-refresh for the
// rest of the session.
func TestNoStoreStartArmsTheRefreshTick(t *testing.T) {
	services, err := NewServices(nostore.New(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.StoreCatalog = &fakes.FakeStoreCatalog{}

	for _, tc := range []struct {
		name      string
		disabled  bool
		wantArmed int
	}{
		{"auto-refresh on", false, 1},
		{"auto-refresh off", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mustNewModelWithOptions(t, services, RuntimeOptions{UnresolvedStore: "no store", DisableAutoRefresh: tc.disabled})
			armed := 0
			m.scheduleRefreshTick = func() tea.Cmd {
				armed++
				return nil
			}
			m.Init()

			if armed != tc.wantArmed {
				t.Errorf("refresh tick armed %d times, want %d", armed, tc.wantArmed)
			}
		})
	}
}

// There is nothing below the picker, so Escape leaves the app rather than
// returning to a board that does not exist.
func TestNoStoreStartEscapeQuits(t *testing.T) {
	s := newNoStoreStart(t)

	_, cmd := s.m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !quits(cmd) {
		t.Error("Escape on the picker with no store open did not quit")
	}
}

// Tab and mode keys would put a browse surface over the no-store repository.
// They are inert until a store is open.
func TestNoStoreStartHoldsTheOperatorOnThePicker(t *testing.T) {
	s := newNoStoreStart(t)

	for _, key := range []string{"1", "2", "3", "4", "e", "c"} {
		m := press(t, s.m, key)
		if m.active != mode.StorePicker {
			t.Errorf("%q left the picker for %q with no store open", key, m.active)
		}
	}
	m := press(t, s.m, "?")
	if !m.showHelp {
		t.Error("help does not open over the picker with no store open")
	}
	next, _ := s.m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(Model).active != mode.StorePicker {
		t.Error("tab left the picker with no store open")
	}
}

// Opening a store from the no-store start is an ordinary switch: its board
// loads, and from then on the picker returns to it instead of quitting.
func TestNoStoreStartOpeningAStoreLeavesTheNoStoreState(t *testing.T) {
	s := newNoStoreStart(t)

	m := press(t, s.m, "enter")

	if m.active != mode.Board {
		t.Fatalf("active mode after opening a store: got %q, want the board", m.active)
	}
	if !strings.Contains(pickerView(m), "Bravo store issue") {
		t.Errorf("the opened store's board is not shown:\n%s", pickerView(m))
	}

	m = press(t, m, "s")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if quits(cmd) {
		t.Fatal("Escape quit the app with a store open; it must return to the board")
	}
	if next.(Model).active != mode.Board {
		t.Errorf("active mode after Escape: got %q, want the board", next.(Model).active)
	}
}
