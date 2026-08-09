package app

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// These tests pin the fatal-error screen quit affordances. The fatal screen
// renders the no-task-store message and tells
// the user "Press q or ctrl+c to quit." — the shell-level keybinding for
// ctrl+q (the documented global quit) is NOT honored on this screen today.
//
// The screen is the path taken when taskmgr's startup health check fails with
// ErrorCodeNoDatabaseFound; see internal/app/model.go around the
// startupHealthCheckMsg handler.

// enterFatalErrorState constructs a Model and drives it into the fatal-error
// state by simulating a failed taskmgr health check. Returns the model in the
// fatal state, ready to receive key messages.
func enterFatalErrorState(t *testing.T) Model {
	t.Helper()
	gw := newTestRepository()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)

	hc := startupHealthCheckMsg{
		err: domain.RepositoryError{Code: domain.ErrorCodeNoDatabaseFound, Message: "no task-manager store here"},
	}
	next, _ := m.Update(hc)
	m = next.(Model)

	if m.fatalErrTitle == "" {
		t.Fatalf("setup: expected fatalErrTitle to be set after health-check failure")
	}
	return m
}

// isQuitCmd returns true when cmd, when executed, yields tea.QuitMsg.
// tea.Quit is itself a Cmd, so calling cmd() returns the QuitMsg value.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	// Defensive: tea.QuitMsg is the canonical quit message type. Compare
	// by reflect.Type so we don't depend on internal aliasing.
	return reflect.TypeOf(msg) == reflect.TypeOf(tea.QuitMsg{})
}

// TestFatalErrorScreen_QKeyQuits documents that 'q' DOES quit the fatal
// screen today (the docs' "Press q or ctrl+c to quit" hint holds for 'q').
// Regression guard against future changes accidentally removing this.
func TestFatalErrorScreen_QKeyQuits(t *testing.T) {
	m := enterFatalErrorState(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !isQuitCmd(cmd) {
		t.Fatalf("expected 'q' to produce tea.Quit on fatal screen; got cmd=%v", cmd)
	}
}

// TestFatalErrorScreen_CtrlQQuits pins the ctrl+q-is-global-quit claim:
// ctrl+q is the documented global quit shortcut (docs/user-guide/key-bindings.md
// "Shell / Global"). It must work on the fatal screen too.
//
// Regression guard: ctrl+q must continue to quit the fatal screen.
func TestFatalErrorScreen_CtrlQQuits(t *testing.T) {
	m := enterFatalErrorState(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !isQuitCmd(cmd) {
		t.Fatalf("expected ctrl+q to produce tea.Quit on fatal screen; got cmd=%v", cmd)
	}
}

// TestFatalErrorScreen_CtrlCQuits documents the ctrl+c path that the
// screen's hint text advertises. The handler at model.go:354 matches
// "ctrl+c" by string; this test prevents accidental regression of that
// path (which is the user's only reliable way out today).
func TestFatalErrorScreen_CtrlCQuits(t *testing.T) {
	// macOS Bubble Tea key handling for Ctrl+C may diverge; keep the test
	// platform-portable by skipping if needed. Today both linux and darwin
	// handle this identically.
	_ = runtime.GOOS
	m := enterFatalErrorState(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatalf("expected ctrl+c to produce tea.Quit on fatal screen; got cmd=%v", cmd)
	}
}

func TestModelStartupHealthCheckClearsPathOnSuccess(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle != "" {
		t.Fatalf("expected fatalErr to be empty after successful health check, got %q", m.fatalErrTitle)
	}
}

func TestModelFatalErrViewRendersFatalErrorScreen(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:    domain.ErrorCodeNoDatabaseFound,
		Message: "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	view := m.View()
	if !strings.Contains(view, "no task-manager store here") {
		t.Fatalf("expected fatal error title in View(), got %q", view)
	}
	if !strings.Contains(view, "taskmgr") {
		t.Fatalf("expected 'taskmgr' mention in View(), got %q", view)
	}
	// A store can be central, so the screen must not present `taskmgr init` as
	// the only remedy: running it in a project whose store was moved centrally
	// creates a second, empty store beside the real one.
	for _, want := range []string{"central", "--store-name"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in the fatal error screen, got %q", want, view)
		}
	}
}

func TestModelFatalErrUpdateOnlyHandlesQuitAndResize(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:    domain.ErrorCodeNoDatabaseFound,
		Message: "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle == "" {
		t.Fatal("precondition: expected fatalErr to be set")
	}

	// Window resize should update dimensions.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("expected width=120 height=40 after resize, got %d %d", m.width, m.height)
	}

	// Quit key should return tea.Quit.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd from 'q' key when fatalErr is set, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}

	// Arbitrary key should be swallowed (no cmd).
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Fatalf("expected nil cmd for non-quit key when fatalErr is set, got non-nil")
	}
}

func TestModelStartupHealthCheckSetsFatalErrOnNoDatabaseFound(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:      domain.ErrorCodeNoDatabaseFound,
		Operation: "health check",
		Message:   "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle == "" {
		t.Fatal("expected fatalErrTitle to be set after NoDatabaseFound health check")
	}
	view := m.View()
	if !strings.Contains(view, "no task-manager store here") {
		t.Fatalf("expected no-database title in View(), got %q", view)
	}
	if !strings.Contains(view, "taskmgr") {
		t.Fatalf("expected 'taskmgr' hint in View(), got %q", view)
	}
}

func TestModelFatalErrIgnoresNonRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, errors.New("some plain error"))

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	// A non-RepositoryError does not set fatalErr — app loads normally.
	if m.fatalErrTitle != "" {
		t.Fatalf("expected fatalErr to be empty for non-RepositoryError, got %q", m.fatalErrTitle)
	}
}
