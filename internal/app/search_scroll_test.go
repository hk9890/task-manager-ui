package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func searchScrollTitle(idx int) string { return fmt.Sprintf("reach-%02d", idx) }

// newSearchScrollShell is a loaded shell on the Search tab of a store of count
// open issues, in a terminal of width by height. The query box has the focus.
func newSearchScrollShell(t *testing.T, count, width, height int) Model {
	t.Helper()

	gw := fakes.NewTracked()
	for idx := 0; idx < count; idx++ {
		seedReady(gw, fmt.Sprintf("tm-%02d", idx), searchScrollTitle(idx), "task", 2)
	}
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	x, _ := testui.FindCell(t, tabLine(m), " "+tabLabels[mode.Search]+" ")
	if m = send(t, m, leftClick(x+1, headerTabsRow)); m.active != mode.Search {
		t.Fatalf("fixture: the shell is on %q, want %q", m.active, mode.Search)
	}
	return m
}

// assertSearchSelectionDrawn fails unless result want carries the selection
// bar and both its lines are on screen.
func assertSearchSelectionDrawn(t *testing.T, m Model, want int) {
	t.Helper()

	view := m.View()
	bar, _ := styles.SelectionPrefix(true, false)
	_, y := testui.FindCell(t, view, bar+"T "+searchScrollTitle(want))
	lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")
	if len(lines) != m.height {
		t.Fatalf("View() is %d lines in a terminal of %d:\n%s", len(lines), m.height, view)
	}
	if id := fmt.Sprintf("tm-%02d", want); y+1 >= len(lines) || !strings.Contains(lines[y+1], id) {
		t.Fatalf("result %d is cut: the line under its title does not carry %s:\n%s", want, id, view)
	}
}

// TestARepeatedResizeLeavesTheSearchWindowWhereItIs: the tabs took the raw
// tea.WindowSizeMsg before the workspace size, so the scroll window was held
// for a pane as tall as the terminal and then for the real one. 13 results at
// 80x24, scrolled to the end and back to row 9.
func TestARepeatedResizeLeavesTheSearchWindowWhereItIs(t *testing.T) {
	const results = 13
	m := newSearchScrollShell(t, results, 80, 24)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	for i := 0; i < results-1; i++ {
		m = pressKey(t, m, "j")
	}
	for i := 0; i < 3; i++ {
		m = pressKey(t, m, "k")
	}
	assertSearchSelectionDrawn(t, m, 9)
	before := testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
	if strings.Contains(before, "T "+searchScrollTitle(0)) {
		t.Fatalf("fixture: the results pane is not scrolled:\n%s", before)
	}

	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if after := testui.AnsiEscapePattern.ReplaceAllString(m.View(), ""); after != before {
		t.Fatalf("a resize to the same size moved the frame.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestTheSelectedResultIsDrawnUnderATypedDraftAtTenRows: at 80x10 the
// workspace is 6 lines and the results pane has two on screen. The banner of a
// typed draft took both.
func TestTheSelectedResultIsDrawnUnderATypedDraftAtTenRows(t *testing.T) {
	m := newSearchScrollShell(t, 3, 80, 10)
	m = pressKey(t, m, "x")
	if !strings.Contains(m.View(), "draft") {
		t.Fatalf("fixture: the query box does not report a draft:\n%s", m.View())
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	assertSearchSelectionDrawn(t, m, 0)

	m = pressKey(t, m, "j")
	assertSearchSelectionDrawn(t, m, 1)
}
