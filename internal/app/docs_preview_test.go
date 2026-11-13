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
// have a body each.
func docsPreviewShell(t *testing.T) (Model, *fakes.TrackedRepository) {
	t.Helper()
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Fix login prompt", "task", 1)
	seedReady(gw, "tm-8", "Auth redesign", "doc", 2, func(issue *memoryrepo.Issue) { issue.Description = "Body of the auth doc." })
	seedReady(gw, "tm-9", "Session notes", "doc", 2, func(issue *memoryrepo.Issue) { issue.Description = "Body of the session notes." })

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
