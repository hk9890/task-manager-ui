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

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("bd-123\n")}}
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))
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
		"--silent",
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

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("bd-999\n")}}
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))
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
		"--silent",
		"--title", "P0 issue",
		"--priority", "0",
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayCreateIssueRequiresNonEmptyIssueID(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("\n")}}
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode create issue output")
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
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

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

func TestGatewayUpdateIssueClearsLabelsWhenRequested(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	err := gateway.UpdateIssue(context.Background(), "bd-42", domain.UpdateIssueInput{ClearLabels: true})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	wantArgs := []string{"update", "bd-42", "--set-labels", ""}
	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

func TestGatewayCloseIssueMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

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
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

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
	gateway := NewGateway(NewCommandRunner(RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.UpdateIssue(context.Background(), "bd-1", domain.UpdateIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.CloseIssue(context.Background(), "bd-1", domain.CloseIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.AddComment(context.Background(), "bd-1", domain.AddCommentInput{Body: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}
