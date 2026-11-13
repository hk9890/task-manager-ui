package docs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// longBody is a doc too tall for the preview at the height previewDocs sets.
var longBody = func() string {
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("Paragraph line %02d.", i), "")
	}
	return strings.Join(lines, "\n")
}()

// previewDocs is a docs tab wide enough for the preview, with three docs: a
// tall one first, then two short ones.
func previewDocs(t *testing.T) (*Model, *fakes.TrackedRepository) {
	t.Helper()
	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Auth redesign", Type: "doc", Description: longBody})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Session notes", Type: "doc", Description: "Notes body of tm-2."})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Handover", Type: "doc", Description: "Handover body of tm-3."})
	m := newModel(t, gw)
	m.SetSize(160, 30)
	run(m, m.Init())
	return m, gw
}

// run executes cmd and feeds every message it yields back into m, batches
// included, until no command is left. The shell's messages are dropped.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, each := range msg {
			run(m, each)
		}
	case mode.SelectionChangedMsg, mode.ActionRequestMsg:
	default:
		run(m, m.Update(msg))
	}
}

func TestThePreviewShowsTheSelectedDocAndFollowsTheSelection(t *testing.T) {
	m, _ := previewDocs(t)

	view := m.View(0)
	testui.AssertContainsAll(t, view, "Preview", "Paragraph line 01.")

	run(m, m.Update(testui.Press("down")))
	view = m.View(0)
	testui.AssertContainsAll(t, view, "Notes body of tm-2.")
	testui.AssertNotContainsAny(t, view, "Paragraph line 01.")
}

func TestThePreviewIsLeftOutOnAScreenTooNarrowForIt(t *testing.T) {
	m, _ := previewDocs(t)
	m.SetSize(styles.SplitMinWidth-1, 30)

	testui.AssertNotContainsAny(t, m.View(0), "Preview", "Paragraph line 01.")
	if m.Previewing() {
		t.Fatal("Previewing reports a preview the screen has no room for")
	}
}

func TestAReadThatLandsAfterTheSelectionMovedOnDoesNotShowTheOldDoc(t *testing.T) {
	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Auth redesign", Type: "doc", Description: "Body of tm-1."})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Session notes", Type: "doc", Description: "Body of tm-2."})
	m := newModel(t, gw)
	m.SetSize(160, 30)
	// The list lands; the read of tm-1 it starts is held back.
	held := m.Update(m.Init()())

	run(m, m.Update(testui.Press("down")))
	run(m, held)

	view := m.View(0)
	testui.AssertContainsAll(t, view, "Body of tm-2.")
	testui.AssertNotContainsAny(t, view, "Body of tm-1.")

	// The read of tm-1 is kept: selecting it again reads nothing.
	start := gw.CallCount()
	run(m, m.Update(testui.Press("up")))
	if reads := gw.CallCountSince(start, fakes.MethodIssue); reads != 0 {
		t.Fatalf("selecting a doc read before read it %d more times", reads)
	}
	testui.AssertContainsAll(t, m.View(0), "Body of tm-1.")
}

func TestADocThatChangedIsReadAgain(t *testing.T) {
	m, gw := previewDocs(t)

	start := gw.CallCount()
	run(m, m.AutoRefresh())
	if reads := gw.CallCountSince(start, fakes.MethodIssue); reads != 0 {
		t.Fatalf("a refresh that changed nothing read the selected doc %d times", reads)
	}

	m.issues[0].UpdatedAt = m.issues[0].UpdatedAt.Add(time.Hour)
	start = gw.CallCount()
	run(m, m.selectionChangedCmd())
	if reads := gw.CallCountSince(start, fakes.MethodIssue); reads != 1 {
		t.Fatalf("a doc with a later UpdatedAt was read %d times, want 1", reads)
	}
}

func TestAFailedReadIsShownAndAReloadTriesAgain(t *testing.T) {
	m, gw := previewDocs(t)
	gw.SetError(fakes.MethodIssue, errors.New("store is gone"))
	run(m, m.Update(testui.Press("down")))

	testui.AssertContainsAll(t, m.View(0), "Session notes", "load failed: store is gone")

	gw.SetError(fakes.MethodIssue, nil)
	run(m, m.Reload())
	run(m, m.Update(testui.Press("down")))
	testui.AssertContainsAll(t, m.View(0), "Notes body of tm-2.")
}

func TestThePageKeysScrollThePreviewAndLeaveTheSelection(t *testing.T) {
	m, _ := previewDocs(t)

	if cmd := m.Update(testui.Press("pgdown")); cmd != nil {
		t.Fatalf("pgdown over the preview sent %T", cmd())
	}
	if m.list.SelectedRow != 0 {
		t.Fatalf("pgdown moved the selection to row %d", m.list.SelectedRow)
	}
	view := m.View(0)
	testui.AssertContainsAll(t, view, "earlier)")
	testui.AssertNotContainsAny(t, view, "Paragraph line 01.")

	m.Update(testui.Press("pgup"))
	testui.AssertContainsAll(t, m.View(0), "Paragraph line 01.")

	// The scroll is the doc's: another doc opens at its top.
	m.Update(testui.Press("pgdown"))
	run(m, m.Update(testui.Press("down")))
	if m.preview.offset != 0 {
		t.Fatalf("another doc opened scrolled to %d", m.preview.offset)
	}
}

func TestThePreviewScrollStopsAtTheEndOfTheDoc(t *testing.T) {
	m, _ := previewDocs(t)
	for range 20 {
		m.Update(testui.Press("pgdown"))
	}
	view := m.View(0)
	testui.AssertContainsAll(t, view, "Paragraph line 60.")
	testui.AssertNotContainsAny(t, view, "more)")
}

func TestTheWheelOverThePreviewScrollsItAndMovesNoSelection(t *testing.T) {
	m, _ := previewDocs(t)
	x, y := testui.FindCell(t, m.View(0), "Paragraph line 05.")

	cmd := m.Update(mode.MouseMsg{Kind: mode.MouseWheelDown, X: x, Y: y})
	if cmd != nil || m.list.SelectedRow != 0 {
		t.Fatalf("the wheel over the preview moved the selection to row %d", m.list.SelectedRow)
	}
	if m.preview.offset != previewWheelLines {
		t.Fatalf("a wheel notch scrolled the preview to %d, want %d", m.preview.offset, previewWheelLines)
	}

	// The same notch over the list still moves the selection.
	lx, ly := testui.FindCell(t, m.View(0), "Session notes")
	run(m, m.Update(mode.MouseMsg{Kind: mode.MouseWheelDown, X: lx, Y: ly}))
	if m.list.SelectedRow != 1 {
		t.Fatalf("the wheel over the list left the selection on row %d", m.list.SelectedRow)
	}
}

func TestThePreviewSaysSoWhenNothingIsSelected(t *testing.T) {
	m, _ := previewDocs(t)
	typeText(m, "zzz")

	testui.AssertContainsAll(t, m.View(0), "Preview", "Nothing selected")
}
