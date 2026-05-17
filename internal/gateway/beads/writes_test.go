package beads

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestGatewayCreateIssueMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-123"}`)}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))
	priority := 1

	result, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title:       "Gateway write operation",
		Description: "Use official command",
		Type:        "task",
		Priority:    &priority,
		Assignee:    "hans",
		Labels:      []string{"gateway", "phase1"},
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	if result.IssueID != "bd-123" {
		t.Fatalf("unexpected issue id: %q", result.IssueID)
	}

	if execStub.command != "bd" {
		t.Fatalf("unexpected command: %q", execStub.command)
	}

	wantArgs := []string{
		"create",
		"--json",
		"--title", "Gateway write operation",
		"--description", "Use official command",
		"--type", "task",
		"--priority", "1",
		"--assignee", "hans",
		"--labels", "gateway,phase1",
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayCreateIssueIncludesExplicitZeroPriority(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-999"}`)}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))
	priority := 0

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title:    "P0 issue",
		Priority: &priority,
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	wantArgs := []string{
		"create",
		"--json",
		"--title", "P0 issue",
		"--priority", "0",
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayCreateIssueRequiresNonEmptyIssueID(t *testing.T) {
	t.Parallel()

	// bd returns a valid JSON payload but with an empty id field; the gateway
	// must reject this as a decode failure rather than returning an empty IssueID.
	execStub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":""}`)}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode create issue output")
}

// TestGatewayCreateIssueRejectsInvalidJSON verifies that a non-JSON stdout
// (e.g. unexpected diagnostic output) is rejected with ErrorCodeDecodeFailed.
func TestGatewayCreateIssueRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("not-json\n")}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayUpdateIssueMapsCommandArgs(t *testing.T) {
	t.Parallel()

	title := "Updated title"
	description := "Updated description"
	status := "in_progress"
	typ := "feature"
	priority := 0
	assignee := "jane"

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	err := gateway.UpdateIssue(context.Background(), "bd-42", domain.UpdateIssueInput{
		Title:       &title,
		Description: &description,
		Status:      &status,
		Type:        &typ,
		Priority:    &priority,
		Assignee:    &assignee,
		Labels:      []string{"alpha", "beta"},
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	wantArgs := []string{
		"update", "bd-42",
		"--title", "Updated title",
		"--description", "Updated description",
		"--status", "in_progress",
		"--type", "feature",
		"--priority", "0",
		"--assignee", "jane",
		"--set-labels", "alpha,beta",
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// TestGatewayUpdateIssueClearLabelsEmitsRemoveLabels verifies that ClearLabels=true
// first fetches the current labels via bd show and then emits bd update --remove-labels
// with the comma-separated list of existing labels.
// Workaround for bd 1.0.4 --set-labels ” silent no-op (see [[ubav]]).
func TestGatewayUpdateIssueClearLabelsEmitsRemoveLabels(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	// ShowIssue call to fetch current labels.
	rec.OnArgs([]string{"show", "bd-42", "--json"}).Return(ExecResult{Stdout: []byte(`[
		{"id":"bd-42","title":"some issue","status":"open","issue_type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","labels":["alpha","beta"]}
	]`)}, nil)
	// UpdateIssue call that removes the labels.
	rec.OnArgs([]string{"update", "bd-42", "--remove-labels", "alpha,beta"}).Return(ExecResult{Stdout: []byte("ok")}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.UpdateIssue(context.Background(), "bd-42", domain.UpdateIssueInput{ClearLabels: true})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 bd calls (show + update), got %d: %#v", len(calls), calls)
	}

	wantShowArgs := []string{"show", "bd-42", "--json"}
	if !reflect.DeepEqual(calls[0].Args, wantShowArgs) {
		t.Fatalf("unexpected first call args:\n got: %#v\nwant: %#v", calls[0].Args, wantShowArgs)
	}

	wantUpdateArgs := []string{"update", "bd-42", "--remove-labels", "alpha,beta"}
	if !reflect.DeepEqual(calls[1].Args, wantUpdateArgs) {
		t.Fatalf("unexpected second call args:\n got: %#v\nwant: %#v", calls[1].Args, wantUpdateArgs)
	}
}

// TestGatewayUpdateIssueClearLabelsNoOpWhenNoLabels verifies that ClearLabels=true
// skips the bd update call entirely when the issue has no labels (nothing to remove).
func TestGatewayUpdateIssueClearLabelsNoOpWhenNoLabels(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	// ShowIssue call returns issue with no labels.
	rec.OnArgs([]string{"show", "bd-99", "--json"}).Return(ExecResult{Stdout: []byte(`[
		{"id":"bd-99","title":"no labels issue","status":"open","issue_type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
	]`)}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.UpdateIssue(context.Background(), "bd-99", domain.UpdateIssueInput{ClearLabels: true})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 bd call (show only, no update since no labels), got %d: %#v", len(calls), calls)
	}

	wantShowArgs := []string{"show", "bd-99", "--json"}
	if !reflect.DeepEqual(calls[0].Args, wantShowArgs) {
		t.Fatalf("unexpected call args:\n got: %#v\nwant: %#v", calls[0].Args, wantShowArgs)
	}
}

func TestGatewayCloseIssueMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	err := gateway.CloseIssue(context.Background(), "bd-7", domain.CloseIssueInput{Reason: "completed"})
	if err != nil {
		t.Fatalf("CloseIssue returned error: %v", err)
	}

	wantArgs := []string{"close", "bd-7", "--reason", "completed"}
	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayAddCommentMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	err := gateway.AddComment(context.Background(), "bd-55", domain.AddCommentInput{Body: "Looks good"})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	wantArgs := []string{"comments", "add", "bd-55", "Looks good"}
	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayWriteOperationsPropagateNormalizedRunnerErrors(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{err: errors.New("fork/exec failed")}
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.UpdateIssue(context.Background(), "bd-1", domain.UpdateIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.CloseIssue(context.Background(), "bd-1", domain.CloseIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.AddComment(context.Background(), "bd-1", domain.AddCommentInput{Body: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}
