//go:build integration

package contract_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
	beads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
)

// TestRealGatewayIssueLifecycleScenario exercises all 4 write methods on the
// real bd gateway in order (create → update → comment → close), verifying each
// mutation with a subsequent ShowIssue read.  A fresh per-test fixture is used
// (not the shared snapshot) because write operations mutate state.
func TestRealGatewayIssueLifecycleScenario(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping lifecycle scenario test")
	}

	t.Setenv("BEADS_ACTOR", "fixture-user")

	// Build a fresh mutable fixture repo for this test.
	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := bdrunner.NewCommandRunner(bdrunner.RunnerConfig{
		WorkDir: repoPath,
	})
	gw := beads.NewCLIGateway(runner)
	ctx := context.Background()

	// ---- Step 1: CreateIssue ----

	t.Logf("step 1: CreateIssue")

	priority := 2
	createInput := domain.CreateIssueInput{
		Title:       "Lifecycle test issue",
		Description: "Created by lifecycle scenario test",
		Type:        "task",
		Priority:    &priority,
	}

	createResult, err := gw.CreateIssue(ctx, createInput)
	if err != nil {
		t.Fatalf("step 1: CreateIssue: unexpected error: %v", err)
	}
	if createResult.IssueID == "" {
		t.Fatal("step 1: CreateIssue: expected non-empty IssueID")
	}

	issueID := createResult.IssueID

	// Verify via ShowIssue.
	detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if err != nil {
		t.Fatalf("step 1: ShowIssue after create: unexpected error: %v", err)
	}
	if detail.Summary.ID != issueID {
		t.Errorf("step 1: ShowIssue: ID: got %q, want %q", detail.Summary.ID, issueID)
	}
	if detail.Summary.Title != createInput.Title {
		t.Errorf("step 1: ShowIssue: Title: got %q, want %q", detail.Summary.Title, createInput.Title)
	}
	if detail.Summary.Type != createInput.Type {
		t.Errorf("step 1: ShowIssue: Type: got %q, want %q", detail.Summary.Type, createInput.Type)
	}
	if detail.Summary.Priority != priority {
		t.Errorf("step 1: ShowIssue: Priority: got %d, want %d", detail.Summary.Priority, priority)
	}
	if detail.Summary.Status == "closed" {
		t.Errorf("step 1: ShowIssue: expected non-closed status after create, got %q", detail.Summary.Status)
	}

	// ---- Step 2: UpdateIssue ----

	t.Logf("step 2: UpdateIssue (change title and priority)")

	updatedTitle := "Lifecycle test issue (updated)"
	updatedPriority := 1
	updateInput := domain.UpdateIssueInput{
		Title:    &updatedTitle,
		Priority: &updatedPriority,
	}

	if err := gw.UpdateIssue(ctx, issueID, updateInput); err != nil {
		t.Fatalf("step 2: UpdateIssue: unexpected error: %v", err)
	}

	// Verify via ShowIssue.
	detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if err != nil {
		t.Fatalf("step 2: ShowIssue after update: unexpected error: %v", err)
	}
	if detail.Summary.Title != updatedTitle {
		t.Errorf("step 2: ShowIssue: Title: got %q, want %q", detail.Summary.Title, updatedTitle)
	}
	if detail.Summary.Priority != updatedPriority {
		t.Errorf("step 2: ShowIssue: Priority: got %d, want %d", detail.Summary.Priority, updatedPriority)
	}

	// ---- Step 3: AddComment ----

	t.Logf("step 3: AddComment")

	commentBody := "This is a lifecycle test comment"
	addCommentInput := domain.AddCommentInput{
		Body: commentBody,
	}

	if err := gw.AddComment(ctx, issueID, addCommentInput); err != nil {
		t.Fatalf("step 3: AddComment: unexpected error: %v", err)
	}

	// Verify via ShowIssue that the comment appears.
	detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if err != nil {
		t.Fatalf("step 3: ShowIssue after comment: unexpected error: %v", err)
	}
	if len(detail.Comments) == 0 {
		t.Fatal("step 3: ShowIssue: expected at least 1 comment, got 0")
	}
	found := false
	for _, c := range detail.Comments {
		if c.Body == commentBody {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("step 3: ShowIssue: expected comment %q in Comments, got %v", commentBody, detail.Comments)
	}

	// ---- Step 4: CloseIssue ----

	t.Logf("step 4: CloseIssue with reason")

	closeReason := "lifecycle scenario complete"
	closeInput := domain.CloseIssueInput{
		Reason: closeReason,
	}

	if err := gw.CloseIssue(ctx, issueID, closeInput); err != nil {
		t.Fatalf("step 4: CloseIssue: unexpected error: %v", err)
	}

	// Verify via ShowIssue that status=closed and reason is stored.
	detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if err != nil {
		t.Fatalf("step 4: ShowIssue after close: unexpected error: %v", err)
	}
	if detail.Summary.Status != "closed" {
		t.Errorf("step 4: ShowIssue: Status: got %q, want %q", detail.Summary.Status, "closed")
	}
	if detail.CloseReason != closeReason {
		t.Errorf("step 4: ShowIssue: CloseReason: got %q, want %q", detail.CloseReason, closeReason)
	}

	// ---- Step 5: ListIssues (default filter) ----

	t.Logf("step 5: ListIssues default filter — closed issue must be excluded")

	issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("step 5: ListIssues: unexpected error: %v", err)
	}

	for _, issue := range issues {
		if issue.ID == issueID {
			t.Errorf("step 5: ListIssues: closed issue %s should not appear in default (open-only) list", issueID)
		}
	}
}
