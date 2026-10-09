package search

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uisearch "github.com/hk9890/task-manager-ui/internal/ui/search"
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func mouseSearch(t *testing.T, titles ...string) *Model {
	t.Helper()
	m := NewModel(context.Background(), memoryrepo.New(fakes.FrozenClock()), nil)
	m.SetSize(160, 24)

	var page domain.SearchResultPage
	for idx, title := range titles {
		page.Results = append(page.Results, domain.SearchResult{Issue: domain.IssueSummary{
			ID: "tm-" + string(rune('1'+idx)), Title: title, Type: "task", Status: "open",
		}})
	}
	_ = m.Update(searchLoadedMsg{page: page})
	return m
}

func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(0), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func opensDetail(cmd tea.Cmd) bool {
	for _, msg := range testui.DrainCmd(cmd) {
		if request, ok := msg.(mode.ActionRequestMsg); ok && request.Mode == mode.Search && request.Action == mode.ActionOpenDetail {
			return true
		}
	}
	return false
}

func TestClickFocusesThePaneAndSelectsTheResult(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t, "hit-one", "hit-two", "hit-three")
	if m.focus != uisearch.FocusQuery || m.selectedRow != 0 {
		t.Fatalf("fixture starts on focus %d row %d, want the query box and row 0", m.focus, m.selectedRow)
	}

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "T hit-three", 0))
	if m.focus != uisearch.FocusResults || m.selectedRow != 2 || cmd == nil || opensDetail(cmd) {
		t.Fatalf("first click: focus %d row %d cmd %v; want the results pane, row 2 and Detail not opened", m.focus, m.selectedRow, cmd != nil)
	}
	// The second click lands on the result's other line: both are that result.
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "  P0 OPN tm-3", 200)); !opensDetail(cmd) {
		t.Fatal("a second click on the same result did not open Detail")
	}

	for text, want := range map[string]uisearch.FocusPane{
		"Content ─":  uisearch.FocusContent,
		"Metadata ─": uisearch.FocusMetadata,
		"Search ─":   uisearch.FocusQuery,
	} {
		if cmd = m.Update(mouseAt(t, m, mode.MouseClick, text, 5000)); cmd != nil || m.focus != want || m.selectedRow != 2 {
			t.Errorf("click on %q: focus %d row %d; want focus %d and the selection kept", text, m.focus, m.selectedRow, want)
		}
	}
}

func TestWheelOverTheResultsMovesTheSelection(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t, "hit-one", "hit-two")

	if cmd := m.Update(mouseAt(t, m, mode.MouseWheelDown, "T hit-one", 0)); cmd == nil || m.selectedRow != 1 {
		t.Fatalf("wheel down left the selection on row %d, want row 1", m.selectedRow)
	}
	if cmd := m.Update(mouseAt(t, m, mode.MouseWheelDown, "  P0 OPN tm-1", 10)); cmd != nil || m.selectedRow != 1 {
		t.Fatal("a notch past the last result moved the selection or reported a change")
	}
	if cmd := m.Update(mouseAt(t, m, mode.MouseWheelUp, "Content ─", 20)); cmd != nil || m.selectedRow != 1 {
		t.Fatal("the wheel over the preview moved the result selection")
	}
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("the wheel moved the focus to %d; it must stay where the keys left it", m.focus)
	}
}

// TestClickWithoutResultsKeepsFocusOnTheQuery holds the rule cycleFocus has:
// with nothing to select, only the query box takes focus.
func TestClickWithoutResultsKeepsFocusOnTheQuery(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t)
	for _, text := range []string{"Results ─", "Content ─", "Metadata ─"} {
		if cmd := m.Update(mouseAt(t, m, mode.MouseClick, text, 0)); cmd != nil || m.focus != uisearch.FocusQuery {
			t.Errorf("click on %q moved the focus to %d with no results", text, m.focus)
		}
	}
}

func TestHoverFollowsThePointerAndClearsWhenItLeaves(t *testing.T) {
	testui.ForceTrueColor(t)

	m := mouseSearch(t, "hit-one", "hit-two")
	idle := m.View(0)

	_ = m.Update(mouseAt(t, m, mode.MouseMove, "T hit-two", 0))
	if m.selectedRow != 0 || m.focus != uisearch.FocusQuery {
		t.Fatal("moving the pointer changed the selection or the focus")
	}
	hovered := m.View(0)
	if hovered == idle {
		t.Fatal("the result under the pointer was not marked")
	}
	if title, id := testui.RowBand(t, hovered, "hit-two"), testui.RowBand(t, hovered, "tm-2"); title == "" || title != id {
		t.Fatalf("hover bands: title line %q, ID line %q; want one band on both lines of the result", title, id)
	}

	_ = m.Update(mouseAt(t, m, mode.MouseMove, "  P0 OPN tm-2", 10))
	if m.View(0) != hovered {
		t.Fatal("the pointer on the result's second line marked something else")
	}

	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if m.View(0) != idle {
		t.Fatal("the hover band stayed after the pointer left")
	}
}
