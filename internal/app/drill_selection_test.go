package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestBackgroundBrowseLoadKeepsTheDrillSelection pins the guard on
// clearDrillSelection.
//
// A board auto-refresh or a Done-column load-more completing while the operator
// is inside a drilled-in Detail emits a SelectionChangedMsg for a tab that is
// not on screen. Clearing the drill selection for it silently retargeted every
// shell mutation and every launcher at that tab's row — the exact failure the
// drill selection exists to prevent.
func TestBackgroundBrowseLoadKeepsTheDrillSelection(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	boardRow := mode.SelectionChangedMsg{
		Mode:      mode.Board,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-epic", Title: "Auth epic"}},
	}

	// The board's own load lands while Detail is showing the child.
	m = applyMessages(t, m, []tea.Msg{boardRow})

	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID != "tm-child" {
		t.Fatalf("a background board load retargeted the shell at %+v while Detail shows tm-child", selection)
	}
	if m.detail.Detail.Summary.ID != "tm-child" {
		t.Errorf("the drilled-in detail was reloaded from the board row: %q", m.detail.Detail.Summary.ID)
	}

	// The tab the operator is actually on still supersedes the drill-in.
	m.active = mode.Board
	m = applyMessages(t, m, []tea.Msg{boardRow})
	if m.drillSelection != nil {
		t.Error("the active tab moving its own selection did not supersede the drill-in")
	}
}

// The drill selection is synthesised from a domain.IssueReference, which carries
// neither labels nor assignee. An editor opened on it would show an empty
// labels field for an issue that has labels, and an unchanged save would clear
// the set. The editor reads the issue again instead.
func TestEditorOnADrilledInIssueShowsItsLabelsAndAnUnchangedSaveKeepsThem(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)
	gw.Memory.Seed(memoryrepo.Issue{
		ID: "tm-child", Title: "Login crash", Status: "in_progress", Type: "bug",
		Assignee: "hans", Labels: []string{"infra", "launcher"}, ParentID: "tm-epic",
	})

	m = pressKey(t, m, "alt+u")
	if m.active != mode.IssueEdit || m.editorTarget.issueID != "tm-child" {
		t.Fatalf("the editor opened on %q (surface %q), want tm-child", m.editorTarget.issueID, m.active)
	}
	testui.AssertContainsAll(t, plainShell(m), "infra, launcher", "hans")

	m = pressKey(t, m, "ctrl+s")
	if gw.HasCall(fakes.MethodUpdateIssue) {
		t.Errorf("an unchanged save wrote the issue: %#v", gw.CallsFor(fakes.MethodUpdateIssue))
	}
	if labels := issueState(gw, "tm-child").Summary.Labels; len(labels) != 2 {
		t.Errorf("the labels are %v after an unchanged save", labels)
	}
}
