package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
)

// The three tests here cover one defect with three faces: handleOverlayMessage
// claimed every message that was not WindowSize/Cancel/Submit while a modal or
// the help overlay was open, so the shell's own handlers never ran. Both tick
// chains re-arm only from their own handlers, and the resize handler is the only
// caller of applyWorkspaceSizeToBrowseModes and detail.ClampScroll.

// TestSpinnerTickSurvivesAnOverlay pins that a spinner tick arriving while an
// overlay is open still reaches the shell. Swallowing it left spinnerTicking
// latched true, so ensureSpinnerTickCmd returned nil for the rest of the
// session: the spinner froze and the app looked hung while it was working.
func TestSpinnerTickSurvivesAnOverlay(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(m *Model){
		"help modal":   func(m *Model) { m.showHelp = true },
		"action modal": func(m *Model) { m.showActionModal = true },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gw := fakes.NewTracked()
			seedReady(gw, "tm-1", "Ready first", "task", 1)

			services, err := NewServices(gw, config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}

			m := mustNewModel(t, services)
			ticks := 0
			m.scheduleSpinnerTick = func() tea.Cmd {
				ticks++
				return nil
			}
			m = applyMessages(t, m, runBatch(m.Init()))

			// Something is loading, so a tick is outstanding.
			m.detail.BeginLoad("tm-1", detail.BeginLoadOptions{})
			m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})
			if !m.spinnerTicking {
				t.Fatal("setup: no spinner tick outstanding while a load is in flight")
			}

			// The tick fires while the overlay is open.
			open(&m)
			armed := ticks
			next, cmd := m.Update(loading.TickMsg{})
			m = next.(Model)
			runBatch(cmd)

			if m.spinnerTicking && ticks == armed {
				t.Fatal("the overlay swallowed the tick: spinnerTicking stayed latched and nothing re-armed, so the spinner never advances again")
			}
			if len(m.loadingStates()) > 0 && ticks == armed {
				t.Errorf("work is still in flight but no further tick was armed")
			}
		})
	}
}

// TestRefreshTickSurvivesAnOverlay pins the second chain. scheduleRefreshTick is
// called from its own handler and from Init and nowhere else, so a tick consumed
// by an overlay ended auto-refresh outright.
func TestRefreshTickSurvivesAnOverlay(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	reschedules := 0
	m.scheduleRefreshTick = func() tea.Cmd {
		reschedules++
		return nil
	}
	m = applyMessages(t, m, runBatch(m.Init()))

	m.showActionModal = true
	before := reschedules
	next, _ := m.Update(refreshTickMsg{})
	m = next.(Model)

	if reschedules == before {
		t.Error("the modal swallowed the refresh tick and nothing re-armed it: auto-refresh stops for the rest of the session")
	}
}

// TestResizeWhileAnOverlayIsOpenSizesTheBrowseTabs pins the third face. The
// browse tabs are forwarded the raw terminal size; the shell's own resize case
// is what hands them the workspace size instead, and it never ran.
func TestResizeWhileAnOverlayIsOpenSizesTheBrowseTabs(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})

	if got := lipgloss.Height(m.View()); got > m.height {
		t.Fatalf("setup: the frame is %d rows in a %d-row terminal", got, m.height)
	}

	m.showHelp = true
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 100, Height: 24}})

	if m.height != 24 || m.width != 100 {
		t.Fatalf("the shell did not record the new terminal size: %dx%d", m.width, m.height)
	}
	// The browse tab is sized by the shell's resize case alone. Left with the
	// raw terminal size it draws a frame taller than the terminal, which scrolls
	// the alt screen and leaves the stale column-top borders behind.
	if got := lipgloss.Height(m.View()); got > m.height {
		t.Errorf("frame is %d rows in a %d-row terminal after a resize behind an overlay", got, m.height)
	}

	// Closing the overlay must not be what heals it.
	m.showHelp = false
	if got := lipgloss.Height(m.View()); got > m.height {
		t.Errorf("frame is still %d rows in a %d-row terminal after the overlay closed", got, m.height)
	}
}

// TestActionRequestIsIgnoredWhenItsModeIsNoLongerActive pins the guard on the
// dialog request. The request travels as a Cmd, so the operator can leave the
// requesting surface before it arrives; answering it then resolved the target
// from a surface no longer on screen, and a submit mutated that issue.
//
// It is also what makes ESC cancel the request: ESC leaves the mode that asked.
func TestActionRequestIsIgnoredWhenItsModeIsNoLongerActive(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Board row", "task", 1)
	seedReady(gw, "tm-2", "Second row", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.active != mode.Board {
		t.Fatalf("setup: expected the board active, got %s", m.active)
	}
	// Search holds a row of its own: without it the request would resolve no
	// target and be dropped for the wrong reason.
	m.selectedByMode[mode.Search] = &mode.Selection{Issue: domain.IssueSummary{
		ID: "tm-2", Title: "Second row", Status: "open", Type: "task", Priority: 2,
	}}

	// A request from Search arrives after the operator has moved to the Board.
	// The flow is stepped by hand rather than drained: an open modal schedules a
	// repeating cursor blink, which a drain would follow forever.
	next, cmd := m.Update(mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenStatusDialog})
	m = next.(Model)
	_ = cmd

	if m.showActionModal {
		t.Error("a status dialog opened over the Board for a request Search made before the switch")
	}
	if m.pendingDialog.active {
		t.Error("a catalog load was dispatched for a surface that is no longer active")
	}

	// The same request from the active surface is answered: the guard is armed
	// and a catalog load is dispatched.
	next, cmd = m.Update(mode.ActionRequestMsg{Mode: mode.Board, Action: mode.ActionOpenStatusDialog})
	m = next.(Model)
	if !m.pendingDialog.active || cmd == nil {
		t.Error("the active surface's own request was ignored")
	}
}
