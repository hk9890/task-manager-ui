package board

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestTheAsideStandsRightOfTheColumnsUnderTheListHead(t *testing.T) {
	const width, height, aside = 120, 20, 50
	state := hitTestState(width, height)
	state.Columns = state.Columns[:1]
	state.AsideWidth = aside
	pane := make([]string, ColumnHeight(height))
	for idx := range pane {
		pane[idx] = strings.Repeat("A", aside)
	}
	state.Aside = strings.Join(pane, "\n")

	view := testui.Downsample(Render(state))
	lines := strings.Split(view, "\n")
	if got := len(lines); got != height {
		t.Fatalf("the frame is %d lines, want %d", got, height)
	}
	if got := lipgloss.Width(lines[0]); got != width {
		t.Fatalf("the list head is %d cells, want the whole width %d", got, width)
	}
	for idx, line := range lines[headLines:] {
		if !strings.HasSuffix(line, strings.Repeat("A", aside)) {
			t.Fatalf("line %d does not end in the aside:\n%s", idx+headLines, line)
		}
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("line %d is %d cells, want %d", idx+headLines, got, width)
		}
	}

	x, y := testui.FindCell(t, view, "tm-aa00")
	if hit, ok := HitTest(state, x, y); !ok || hit.Row != 0 {
		t.Fatalf("HitTest on the column row reported %+v, %v", hit, ok)
	}
	if OverAside(state, x, y) {
		t.Fatal("OverAside reports the column row")
	}

	// The aside's first cell on the row: the cell left of it is the gap.
	ax := lipgloss.Width(lines[y][:strings.Index(lines[y], strings.Repeat("A", aside))])
	if hit, ok := HitTest(state, ax, y); ok {
		t.Fatalf("HitTest on the aside reported %+v", hit)
	}
	if !OverAside(state, ax, y) || !OverAside(state, ax+aside-1, y) {
		t.Fatal("OverAside misses a cell the aside is drawn on")
	}
	if OverAside(state, ax-1, y) || OverAside(state, ax, 0) {
		t.Fatal("OverAside reports the gap or the list head")
	}
}
