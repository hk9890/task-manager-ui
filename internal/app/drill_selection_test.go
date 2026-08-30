package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
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

// TestUpdateDialogOnADrilledInIssueCarriesItsLabels pins what the update dialog
// is prefilled from.
//
// The drill selection is synthesised from a domain.IssueReference, which carries
// neither labels nor assignee. The dialog showed an empty labels field for an
// issue that has labels, and submitting it unchanged mapped empty to
// ClearLabels: pressing u and Enter deleted the issue's whole label set under a
// success toast.
func TestUpdateDialogOnADrilledInIssueCarriesItsLabels(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	// The child's full record arrives, as the drill-in load delivers it.
	m = applyMessages(t, m, []tea.Msg{detailLoadedMsg{
		issueID: "tm-child",
		detail: domain.IssueDetail{Summary: domain.IssueSummary{
			ID: "tm-child", Title: "Login crash", Status: "in_progress", Type: "bug", Priority: 0,
			Assignee: "hans", Labels: []string{"infra", "launcher"},
		}},
	}})

	issue, ok := m.mutationTargetIssue()
	if !ok {
		t.Fatal("no mutation target resolved for the drilled-in issue")
	}
	if issue.ID != "tm-child" {
		t.Fatalf("mutation target = %q, want tm-child", issue.ID)
	}
	if len(issue.Labels) != 2 {
		t.Errorf("the update dialog would be prefilled with labels %v; submitting that unchanged clears the issue's labels", issue.Labels)
	}
	if issue.Assignee != "hans" {
		t.Errorf("assignee prefill = %q, want hans", issue.Assignee)
	}
}

// TestSubmittingAnEmptyLabelsFieldOnlyClearsWhatTheDialogShowed pins the submit
// rule itself, the second half of the same defect: "empty" means "clear" only
// when the dialog was opened showing a label set.
func TestSubmittingAnEmptyLabelsFieldOnlyClearsWhatTheDialogShowed(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{
		ID: "tm-7", Title: "Labelled", Status: "open", Type: "task", Priority: 2,
		Labels: []string{"infra"},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	values := map[string]string{"title": "Labelled", "status": "open", "type": "task", "priority": "2", "assignee": "", "labels": ""}

	// The dialog was opened from a summary that did not carry the labels.
	unaware := mutationDialogState{kind: mutationUpdate, issue: domain.IssueSummary{ID: "tm-7", Title: "Labelled", Status: "open", Type: "task", Priority: 2}}
	if msg := submitMutationCmd(services, unaware, values)(); msg == nil {
		t.Fatal("submit produced no message")
	}
	after, err := gw.Memory.Issue(context.Background(), "tm-7")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(after.Summary.Labels) != 1 {
		t.Errorf("labels are %v after submitting a field the dialog never showed; they must be untouched", after.Summary.Labels)
	}

	// The dialog was opened showing the labels, and the operator emptied it.
	aware := mutationDialogState{kind: mutationUpdate, issue: after.Summary}
	if msg := submitMutationCmd(services, aware, values)(); msg == nil {
		t.Fatal("submit produced no message")
	}
	cleared, err := gw.Memory.Issue(context.Background(), "tm-7")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(cleared.Summary.Labels) != 0 {
		t.Errorf("emptying a labels field the dialog did show must clear the set, got %v", cleared.Summary.Labels)
	}
}
