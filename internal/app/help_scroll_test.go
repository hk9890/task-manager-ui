package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// newHelpShell is a loaded shell at 120 columns and height rows with the help
// overlay open.
func newHelpShell(t *testing.T, cfg config.Model, height int) Model {
	t.Helper()
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: height})
	if m = pressKey(t, m, "?"); !m.showHelp {
		t.Fatal("fixture: the help overlay did not open")
	}
	return m
}

func helpLines(m Model) []string {
	return strings.Split(m.help.View(), "\n")
}

// TestTheDetailScrollKeysScrollAClippedHelpOverlay walks the help overlay with
// the keyboard alone: a line, a page, both ends, and out.
func TestTheDetailScrollKeysScrollAClippedHelpOverlay(t *testing.T) {
	m := newHelpShell(t, config.Default(), 16)
	top := m.View()
	if !strings.Contains(top, "more lines") || strings.Contains(top, "earlier lines") {
		t.Fatalf("fixture: the help overlay must open clipped, on its first line:\n%s", top)
	}
	topLines := helpLines(m)

	down := pressKey(t, pressKey(t, pressKey(t, m, "j"), "j"), "j")
	if !down.showHelp || !strings.Contains(down.View(), "earlier lines") {
		t.Fatalf("three presses of j did not scroll the help overlay:\n%s", down.View())
	}
	// Row 0 is the border and row 1 the indicator, so row 2 is the line that
	// stood three rows lower.
	if got := helpLines(down)[2]; got != topLines[5] {
		t.Fatalf("three presses of j moved the text to %q, want %q", got, topLines[5])
	}
	if up := send(t, down, tea.KeyMsg{Type: tea.KeyUp}); helpLines(up)[2] != topLines[4] {
		t.Fatal("up did not scroll the help overlay back one line")
	}

	page := send(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	byLine := m
	for range m.help.PageLines() {
		byLine = pressKey(t, byLine, "j")
	}
	if page.View() == top || page.View() != byLine.View() {
		t.Fatalf("pgdown did not move the help overlay one page:\n%s", page.View())
	}
	if back := send(t, page, tea.KeyMsg{Type: tea.KeyPgUp}); back.View() != top {
		t.Fatal("pgup did not move the help overlay back one page")
	}

	end := send(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	help := shellKeyHelp(m.keys)
	lastLine := strings.TrimSpace(help[strings.LastIndex(help, "\n"):])
	if !strings.Contains(end.View(), lastLine) || strings.Contains(end.View(), "more lines") {
		t.Fatalf("end must show the last line of the help and no 'more' indicator:\n%s", end.View())
	}
	if home := send(t, end, tea.KeyMsg{Type: tea.KeyHome}); home.View() != top {
		t.Fatal("home did not return the help overlay to its first view")
	}

	// Closing and reopening starts at the top, by either way out.
	for name, closeKey := range map[string]tea.KeyMsg{
		"help key": {Type: tea.KeyRunes, Runes: []rune("?")},
		"escape":   {Type: tea.KeyEsc},
	} {
		closed := send(t, end, closeKey)
		if closed.showHelp {
			t.Fatalf("%s did not close a scrolled help overlay", name)
		}
		if reopened := pressKey(t, closed, "?"); reopened.View() != top {
			t.Fatalf("the help overlay closed with the %s reopened where it was scrolled to", name)
		}
	}
}

// TestTheHelpOverlayScrollsOnReboundDetailKeys: the keys come from the detail
// context of the resolved bindings, not from literals.
func TestTheHelpOverlayScrollsOnReboundDetailKeys(t *testing.T) {
	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Detail: map[string][]string{config.DetailActionScrollDown: {"z"}},
	})
	m := newHelpShell(t, cfg, 16)
	top := m.View()

	if still := pressKey(t, m, "j"); still.View() != top || !still.showHelp {
		t.Fatal("a key no longer bound to detail scroll_down scrolled or closed the help overlay")
	}
	if down := pressKey(t, m, "z"); !strings.Contains(down.View(), "earlier lines") {
		t.Fatalf("the rebound scroll_down key did not scroll the help overlay:\n%s", down.View())
	}
}

// TestTheScrollKeysChangeNothingWhenTheHelpFits: in a terminal tall enough for
// the whole help there is nowhere to scroll to.
func TestTheScrollKeysChangeNothingWhenTheHelpFits(t *testing.T) {
	m := newHelpShell(t, config.Default(), 80)
	whole := m.View()
	if strings.Contains(whole, "more lines") {
		t.Fatalf("fixture: the help overlay is clipped at this height:\n%s", whole)
	}
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyRunes, Runes: []rune("k")},
		{Type: tea.KeyPgDown}, {Type: tea.KeyPgUp}, {Type: tea.KeyEnd}, {Type: tea.KeyHome},
	} {
		if m = send(t, m, key); m.View() != whole || !m.showHelp {
			t.Fatalf("%s changed a help overlay that fits", key)
		}
	}
}
