package app

import (
	"reflect"
	"runtime"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
)

// These tests pin the fatal-error screen quit affordances surfaced by
// beads-workbench-znri.7. The fatal screen renders the "no beads project
// here" message and tells the user "Press q or ctrl+c to quit." — the
// shell-level keybinding for ctrl+q (the documented global quit) is NOT
// honored on this screen today.
//
// The screen is the path taken when bd's startup health check fails with
// ErrorCodeNoDatabaseFound or ErrorCodeCommandUnavailable; see
// internal/app/model.go around the startupHealthCheckMsg handler.

// enterFatalErrorState constructs a Model and drives it into the fatal-error
// state by simulating a failed bd health check. Returns the model in the
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
		err: domain.RepositoryError{Code: domain.ErrorCodeNoDatabaseFound, Message: "no beads project here"},
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

// TestFatalErrorScreen_CtrlQQuits pins the real znri.7 claim: ctrl+q is
// the documented global quit shortcut (docs/user-guide/key-bindings.md
// "Shell / Global"). It must work on the fatal screen too.
//
// Regression guard for znri.7: ctrl+q must continue to quit the fatal screen.
func TestFatalErrorScreen_CtrlQQuits(t *testing.T) {
	m := enterFatalErrorState(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !isQuitCmd(cmd) {
		t.Fatalf("expected ctrl+q to produce tea.Quit on fatal screen; got cmd=%v (znri.7)", cmd)
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
