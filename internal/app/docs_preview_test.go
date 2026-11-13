package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// docsPreviewShell is a shell at 160x30 on the Docs tab, with two docs that
// have a body each, both related to the task tm-1.
func docsPreviewShell(t *testing.T) (Model, *fakes.TrackedRepository) {
	t.Helper()
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Fix login prompt", "task", 1, func(issue *memoryrepo.Issue) { issue.Description = "Body of the login task." })
	related := func(body string) func(*memoryrepo.Issue) {
		return func(issue *memoryrepo.Issue) {
			issue.Description = body
			issue.Related = []string{"tm-1"}
		}
	}
	seedReady(gw, "tm-8", "Auth redesign", "doc", 2, related("Body of the auth doc."))
	seedReady(gw, "tm-9", "Session notes", "doc", 2, related("Body of the session notes."))

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 30}})
	m = applyMessages(t, m, runBatch(m.Init()))
	m = pressKey(t, m, "tab")
	if m.active != mode.Docs {
		t.Fatalf("fixture: on %q, want docs", m.active)
	}
	return m, gw
}

// docBody is the body of the selected doc the preview draws, from the shell's
// read of it.
func docBody(m Model) string {
	if m.docs.Previewing() {
		return plainShell(m)
	}
	return ""
}

func TestTheDocsPreviewDrawsTheShellsReadOfTheSelectedDoc(t *testing.T) {
	m, gw := docsPreviewShell(t)
	first := firstSelectionID(m, mode.Docs)
	other := map[string]string{"tm-8": "Body of the session notes.", "tm-9": "Body of the auth doc."}
	own := map[string]string{"tm-8": "Body of the auth doc.", "tm-9": "Body of the session notes."}

	if view := docBody(m); !strings.Contains(view, own[first]) {
		t.Fatalf("the preview does not draw the body of the selected doc %s:\n%s", first, view)
	}

	// One read a move: the shell's, for Detail. The tab reads nothing itself.
	start := gw.CallCount()
	m = pressKey(t, m, "down")
	second := firstSelectionID(m, mode.Docs)
	if reads := gw.CallCountSince(start, fakes.MethodIssue); reads != 1 {
		t.Fatalf("a move read the doc %d times, want 1", reads)
	}
	view := docBody(m)
	if !strings.Contains(view, own[second]) || strings.Contains(view, other[second]) {
		t.Fatalf("after down the preview does not draw %s alone:\n%s", second, view)
	}
}

func TestTheDocsPreviewDrawsAFailedRead(t *testing.T) {
	m, gw := docsPreviewShell(t)
	gw.SetError(fakes.MethodIssue, errors.New("store is gone"))
	m = pressKey(t, m, "down")

	testui.AssertContainsAll(t, plainShell(m), "load failed: store is gone")
}

// TestEscapeFromADrillInDrawsTheSelectedDocAgain opens a doc in Detail, drills
// into its related task, and leaves with Escape: the Detail read is now the
// task's, so the shell must read the doc again for the preview.
func TestEscapeFromADrillInDrawsTheSelectedDocAgain(t *testing.T) {
	m, _ := docsPreviewShell(t)
	doc := firstSelectionID(m, mode.Docs)
	body := map[string]string{"tm-8": "Body of the auth doc.", "tm-9": "Body of the session notes."}[doc]

	m = pressKey(t, m, "enter")
	m = pressKey(t, m, "left")
	m = pressKey(t, m, "enter")
	if m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("setup: the drill-in reached %q, want tm-1", m.detail.Detail.Summary.ID)
	}

	m = pressKey(t, m, "esc")
	if m.active != mode.Docs {
		t.Fatalf("escape left Detail for %q, want docs", m.active)
	}
	view := plainShell(m)
	if !strings.Contains(view, body) || strings.Contains(view, "Body of the login task.") {
		t.Fatalf("after the drill-in the preview does not draw the doc %s:\n%s", doc, view)
	}
}

// TestATabRoundTripKeepsThePreviewScrolled leaves the tab with the preview
// scrolled; the shell reads a board issue meanwhile, and the doc comes back
// where it was.
func TestATabRoundTripKeepsThePreviewScrolled(t *testing.T) {
	m, gw := docsPreviewShell(t)
	long := strings.Repeat("A line of the long doc.\n\n", 60)
	seedReady(gw, "tm-8", "Auth redesign", "doc", 2, func(issue *memoryrepo.Issue) { issue.Description = long })
	seedReady(gw, "tm-9", "Session notes", "doc", 2, func(issue *memoryrepo.Issue) { issue.Description = long })
	m = pressKey(t, m, "alt+r")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "up")
	m = pressKey(t, m, "pgdown")
	if !strings.Contains(plainShell(m), "earlier)") {
		t.Fatalf("setup: pgdown did not scroll the preview:\n%s", plainShell(m))
	}

	m = pressKey(t, m, "shift+tab")
	m = pressKey(t, m, "tab")
	if view := plainShell(m); !strings.Contains(view, "earlier)") {
		t.Fatalf("the round trip opened the doc at its top:\n%s", view)
	}
}
