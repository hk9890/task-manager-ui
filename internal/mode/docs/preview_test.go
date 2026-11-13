package docs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
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

// docOf is the read of doc id the shell would hand the tab.
func docOf(id, title, body string) domain.IssueDetail {
	return domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: id, Title: title, Type: "doc", Status: "open"},
		Description: body,
	}
}

// previewDocs is a docs tab wide enough for the preview, with three docs, and
// the read of the first, the tall one, already handed over.
func previewDocs(t *testing.T) *Model {
	t.Helper()
	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Auth redesign", Type: "doc"})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Session notes", Type: "doc"})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Handover", Type: "doc"})
	m := newModel(t, gw)
	m.SetSize(160, 30)
	resolve(t, m, m.Init())
	m.ShowDoc(docOf("tm-1", "Auth redesign", longBody))
	return m
}

func TestThePreviewDrawsTheReadOfTheSelectedDocOnly(t *testing.T) {
	m := previewDocs(t)
	testui.AssertContainsAll(t, m.View(0), "Preview", "Paragraph line 01.")

	// The next doc has no read yet: its header over skeleton rows, and never
	// the body of the doc before.
	m.Update(testui.Press("down"))
	view := m.View(0)
	testui.AssertContainsAll(t, view, "░")
	testui.AssertNotContainsAny(t, view, "Paragraph line 01.")

	// A read of another issue, the one the shell loaded last, is not drawn.
	m.ShowDoc(docOf("tm-9", "Some task", "Body of tm-9."))
	testui.AssertNotContainsAny(t, m.View(0), "Body of tm-9.")

	m.ShowDoc(docOf("tm-2", "Session notes", "Notes body of tm-2."))
	testui.AssertContainsAll(t, m.View(0), "Notes body of tm-2.")
}

func TestThePreviewIsLeftOutOnAScreenTooNarrowForIt(t *testing.T) {
	m := previewDocs(t)
	m.SetSize(styles.SplitMinWidth-1, 30)

	testui.AssertNotContainsAny(t, m.View(0), "Preview", "Paragraph line 01.")
	if m.Previewing() {
		t.Fatal("Previewing reports a preview the screen has no room for")
	}
}

func TestAFailedReadIsShownUntilAReadOfThatDocLands(t *testing.T) {
	m := previewDocs(t)
	m.ShowDocFailure("tm-1", "store is gone")
	view := m.View(0)
	testui.AssertContainsAll(t, view, "Auth redesign", "load failed: store is gone")
	testui.AssertNotContainsAny(t, view, "Paragraph line 01.")

	m.ShowDoc(docOf("tm-1", "Auth redesign", longBody))
	testui.AssertContainsAll(t, m.View(0), "Paragraph line 01.")
}

func TestThePageKeysScrollThePreviewAndLeaveTheSelection(t *testing.T) {
	m := previewDocs(t)

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
}

func TestEveryDocOpensAtItsTop(t *testing.T) {
	m := previewDocs(t)

	m.Update(testui.Press("pgdown"))
	m.Update(testui.Press("down"))
	if m.preview.offset != 0 {
		t.Fatalf("another doc opened scrolled to %d", m.preview.offset)
	}

	// A reload puts the selection on the first doc at once: that doc opens at
	// its top too, before the list lands.
	m.Update(testui.Press("up"))
	m.Update(testui.Press("pgdown"))
	m.Update(testui.Press("down"))
	m.ShowDoc(docOf("tm-2", "Session notes", longBody))
	m.Update(testui.Press("pgdown"))
	_ = m.Reload()
	if m.preview.offset != 0 {
		t.Fatalf("the reload left the first doc scrolled to %d", m.preview.offset)
	}
}

func TestThePreviewScrollStopsAtTheEndOfTheDoc(t *testing.T) {
	m := previewDocs(t)
	for range 20 {
		m.Update(testui.Press("pgdown"))
	}
	view := m.View(0)
	testui.AssertContainsAll(t, view, "Paragraph line 60.")
	testui.AssertNotContainsAny(t, view, "more)")
}

func TestAResizeHoldsTheScrollInsideTheDoc(t *testing.T) {
	m := previewDocs(t)
	for range 20 {
		m.Update(testui.Press("pgdown"))
	}
	m.SetSize(160, 50)

	before := m.preview.offset
	m.Update(testui.Press("pgup"))
	if m.preview.offset >= before {
		t.Fatalf("after a resize pgup left the offset at %d (was %d)", m.preview.offset, before)
	}
}

func TestTheWheelOverThePreviewScrollsItAndMovesNoSelection(t *testing.T) {
	m := previewDocs(t)
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
	m.Update(mode.MouseMsg{Kind: mode.MouseWheelDown, X: lx, Y: ly})
	if m.list.SelectedRow != 1 {
		t.Fatalf("the wheel over the list left the selection on row %d", m.list.SelectedRow)
	}
}

func TestThePreviewSaysSoWhenNothingIsSelected(t *testing.T) {
	m := previewDocs(t)
	typeText(m, "zzz")

	testui.AssertContainsAll(t, m.View(0), "Preview", "Nothing selected")
	if m.Previewing() {
		t.Fatal("Previewing reports a pane with no doc to scroll")
	}
}

func TestThePreviewIsASkeletonWhileTheFirstListLoads(t *testing.T) {
	m := newModel(t, fakes.NewTracked())
	m.SetSize(160, 30)
	_ = m.Init()

	view := m.View(0)
	testui.AssertContainsAll(t, view, "Preview", "░")
	testui.AssertNotContainsAny(t, view, "Nothing selected")
}
