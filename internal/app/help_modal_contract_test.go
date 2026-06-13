package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
)

// These tests pin documented help-modal behavior surfaced by the
// project-explore epic (beads-workbench-znri.4 and znri.5).
//
// docs/user-guide/key-bindings.md states:
//   - "? — toggle help"           (shell context)
//   - "esc — cancel when the modal is not required"  (modal context)
//
// Both bindings target the keyboard-help modal opened by `?`. The model
// behavior is observable as `m.showHelp` toggling between true and false.

func openHelpModal(t *testing.T) Model {
	t.Helper()
	gw := newTestRepository()
	gw.seedReady("bw-1", "Ready", "task", 1)
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Press '?' from board context to open help.
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}})
	if !m.showHelp {
		t.Fatalf("setup: expected showHelp=true after '?'; got false")
	}
	return m
}

// TestHelpModal_QuestionMarkToggleClosesModal pins znri.4: pressing '?' a
// second time while the help modal is open must dismiss the modal (the
// documented toggle semantics).
func TestHelpModal_QuestionMarkToggleClosesModal(t *testing.T) {
	m := openHelpModal(t)

	// Press '?' again — must toggle the modal closed.
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}})
	if m.showHelp {
		t.Fatalf("expected showHelp=false after second '?'; modal did not toggle closed")
	}
}

// TestHelpModal_EscapeDismissesModal pins the Esc-cancel-modal claim from
// znri.5: pressing Esc while the help modal is up must dismiss it (the
// modal context's `escape — cancel` binding).
func TestHelpModal_EscapeDismissesModal(t *testing.T) {
	m := openHelpModal(t)

	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEsc}})
	if m.showHelp {
		t.Fatalf("expected showHelp=false after Esc; modal did not dismiss")
	}
}

// TestHelpModal_EnterDismissesModal_BaselineRegressionGuard documents that
// Enter dismisses the help modal today. If a fix to znri.4 or znri.5
// accidentally regresses Enter dismissal, this test catches it.
func TestHelpModal_EnterDismissesModal_BaselineRegressionGuard(t *testing.T) {
	m := openHelpModal(t)
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEnter}})
	if m.showHelp {
		t.Fatalf("expected showHelp=false after Enter; modal did not dismiss")
	}
}

// TestHelpModal_RenderedContentVisible asserts the help modal's rendered
// View contains its title and at least one documented section heading. A
// rendering regression that drops the bottom border or truncates the body
// can show up here as a missing section, complementing the golden-file
// approach for znri.5's visual rendering claim.
func TestHelpModal_RenderedContentVisible(t *testing.T) {
	m := openHelpModal(t)
	// Apply a representative terminal size so the help overlay computes
	// its size against a known viewport.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 120, Height: 34}})

	view := m.View()
	for _, must := range []string{"Keyboard Help", "Mode switching", "Selection"} {
		if !strings.Contains(view, must) {
			t.Fatalf("expected help view to contain %q; got:\n%s", must, view)
		}
	}
}
