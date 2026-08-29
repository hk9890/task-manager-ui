package app

// What the Update dialog actually hands the repository. The mutation tests
// assert the resulting message and the error wrapping, but nothing inspected
// the UpdateIssueInput itself — so any field of that struct could be built
// wrong with the whole repository green.

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// capturedUpdateInput runs the Update submit path and returns the input the
// repository received.
func capturedUpdateInput(t *testing.T, issue domain.IssueSummary, values map[string]string) domain.UpdateIssueInput {
	t.Helper()

	gw := fakes.NewTracked()
	seedIssueDetail(gw, domain.IssueDetail{Summary: issue})
	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{kind: mutationUpdate, issue: issue}
	if msg := submitMutationCmd(services, state, values)(); msg == nil {
		t.Fatal("submitMutationCmd returned no message")
	}

	for _, call := range gw.Calls() {
		if call.Method != fakes.MethodUpdateIssue {
			continue
		}
		input, ok := call.Args.(domain.UpdateIssueInput)
		if !ok {
			t.Fatalf("UpdateIssue call carried %T, want domain.UpdateIssueInput", call.Args)
		}
		return input
	}

	t.Fatal("no UpdateIssue call reached the repository")
	return domain.UpdateIssueInput{}
}

// TestUpdateDialogClearsLabelsWhenTheFieldIsEmptied is the case the
// ClearLabels flag exists for. Without it the operator empties the Labels
// field, submits, sees a success toast — and the old labels are still there,
// because an empty Labels slice is indistinguishable from "no change".
func TestUpdateDialogClearsLabelsWhenTheFieldIsEmptied(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "tm-1", Labels: []string{"infra", "ui"}}
	input := capturedUpdateInput(t, issue, map[string]string{
		"title":    "Updated title",
		"priority": "2",
		"labels":   "",
	})

	if !input.ClearLabels {
		t.Error("ClearLabels is false after the Labels field was emptied: the old labels would survive the update")
	}
	if len(input.Labels) != 0 {
		t.Errorf("Labels = %v, want empty alongside ClearLabels", input.Labels)
	}
}

func TestUpdateDialogSendsLabelsWithoutClearingWhenTheFieldHasValues(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "tm-1", Labels: []string{"infra"}}
	input := capturedUpdateInput(t, issue, map[string]string{
		"title":    "Updated title",
		"priority": "2",
		"labels":   "infra,ui",
	})

	if input.ClearLabels {
		t.Error("ClearLabels is true although the Labels field carried values")
	}
	if len(input.Labels) != 2 || input.Labels[0] != "infra" || input.Labels[1] != "ui" {
		t.Errorf("Labels = %v, want [infra ui]", input.Labels)
	}
}

// The rest of the payload, so a wrong field anywhere in the struct fails here
// rather than reaching the store. Assignee in particular is diffed against the
// original rather than gated on non-empty, so that clearing it unassigns.
func TestUpdateDialogPayloadCarriesEveryEditedField(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "tm-1", Assignee: "hans", Status: "open", Type: "task", Priority: 3}
	input := capturedUpdateInput(t, issue, map[string]string{
		"title":    "Updated title",
		"priority": "1",
		"assignee": "",
		"labels":   "infra",
	})

	if input.Title == nil || *input.Title != "Updated title" {
		t.Errorf("Title = %v, want \"Updated title\"", input.Title)
	}
	if input.Priority == nil || *input.Priority != 1 {
		t.Errorf("Priority = %v, want 1", input.Priority)
	}
	if input.Assignee == nil || *input.Assignee != "" {
		t.Errorf("Assignee = %v, want a pointer to the empty string so the issue is unassigned", input.Assignee)
	}
}
