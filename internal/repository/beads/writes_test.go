package beads

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
)

func TestGatewayCreateIssueMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte(`{"id":"bd-123"}`)}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))
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

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte(`{"id":"bd-999"}`)}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))
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
	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte(`{"id":""}`)}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode create issue output")
}

// TestGatewayCreateIssueRejectsInvalidJSON verifies that a non-JSON stdout
// (e.g. unexpected diagnostic output) is rejected with ErrorCodeDecodeFailed.
func TestGatewayCreateIssueRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("not-json\n")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

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

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

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

// TestGatewayUpdateIssueClearLabelsEmitsRemoveLabel verifies that ClearLabels=true
// first fetches the current labels via bd show and then emits bd update --remove-label
// with the comma-separated list of existing labels.
// Workaround for bd 1.0.4 --set-labels "" silent no-op (see [[ubav]]).
// Note: the correct flag is --remove-label (singular), NOT --remove-labels (plural).
// The plural form is not a recognized bd 1.0.4 flag and causes a silent failure.
func TestGatewayUpdateIssueClearLabelsEmitsRemoveLabel(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	// ShowIssue call to fetch current labels.
	rec.OnArgs([]string{"show", "bd-42", "--json"}).Return(bdrunner.ExecResult{Stdout: []byte(`[
		{"id":"bd-42","title":"some issue","status":"open","issue_type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","labels":["alpha","beta"]}
	]`)}, nil)
	// UpdateIssue call that removes the labels using the correct singular flag.
	rec.OnArgs([]string{"update", "bd-42", "--remove-label", "alpha,beta"}).Return(bdrunner.ExecResult{Stdout: []byte("ok")}, nil)

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

	wantUpdateArgs := []string{"update", "bd-42", "--remove-label", "alpha,beta"}
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
	rec.OnArgs([]string{"show", "bd-99", "--json"}).Return(bdrunner.ExecResult{Stdout: []byte(`[
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

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	err := gateway.CloseIssue(context.Background(), "bd-7", domain.CloseIssueInput{Reason: "completed"})
	if err != nil {
		t.Fatalf("CloseIssue returned error: %v", err)
	}

	wantArgs := []string{"close", "bd-7", "--reason", "completed"}
	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// verbDispatchExecutor returns a different ExecResult/err per leading argv
// token. The leading token (args[0]) corresponds to the bd subcommand
// ("close", "show", etc.). Use to simulate multi-call sequences such as the
// CloseIssue → ShowIssue idempotency-recovery path.
type verbDispatchExecutor struct {
	byVerb map[string]struct {
		result bdrunner.ExecResult
		err    error
	}
	calls [][]string // ordered argv per invocation, for assertions
}

func (e *verbDispatchExecutor) Run(_ context.Context, _ string, args []string, _ string, _ []string) (bdrunner.ExecResult, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	if len(args) == 0 {
		return bdrunner.ExecResult{}, errors.New("verbDispatchExecutor: empty args")
	}
	got, ok := e.byVerb[args[0]]
	if !ok {
		return bdrunner.ExecResult{}, errors.New("verbDispatchExecutor: no response for verb " + args[0])
	}
	return got.result, got.err
}

// TestGatewayCloseIssueEmulatesIdempotencyOnBdNotFound exercises the gateway's
// emulation of CloseIssue idempotency over the bd 1.0.4 close bug (re-closing
// an already-closed issue within the same wall-clock second produces
// RowsAffected==0 in bd's UPDATE, which bd misreports as "issue not found:
// <id>"). Filed upstream as gastownhall/beads#4025. The gateway detects this
// specific failure and probes via ShowIssue: when the issue exists with
// status=closed, return nil. See writes.go and the CloseIssue contract note
// in interface.go. Delete this test together with the recovery block in
// writes.go once the upstream fix ships and we bump the mise-pinned bd
// version.
func TestGatewayCloseIssueEmulatesIdempotencyOnBdNotFound(t *testing.T) {
	t.Parallel()

	// Real bd 1.0.4 stderr for the close-lookup bug. The runner wraps it as
	// "command exited with code 1: <stderr>" — matching the substring
	// "issue not found" in the resulting GatewayError.Message.
	bdStderr := []byte("Error closing bd-7: issue not found: bd-7\n")

	// ShowIssue payload: closed issue with the same id.
	showJSON := []byte(`[{
		"id": "bd-7",
		"title": "already closed",
		"status": "closed",
		"priority": 2,
		"issue_type": "task",
		"created_at": "2026-05-17T00:00:00Z",
		"updated_at": "2026-05-17T00:00:00Z",
		"closed_at": "2026-05-17T00:00:00Z"
	}]`)

	exec := &verbDispatchExecutor{byVerb: map[string]struct {
		result bdrunner.ExecResult
		err    error
	}{
		"close": {result: bdrunner.ExecResult{Stderr: bdStderr, ExitCode: 1}},
		"show":  {result: bdrunner.ExecResult{Stdout: showJSON}},
	}}

	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: exec}))

	if err := gateway.CloseIssue(context.Background(), "bd-7", domain.CloseIssueInput{}); err != nil {
		t.Fatalf("CloseIssue: expected nil (idempotency emulated), got %v", err)
	}

	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 bd calls (close + show), got %d: %v", len(exec.calls), exec.calls)
	}
	if exec.calls[0][0] != "close" {
		t.Errorf("call[0]: expected close, got %v", exec.calls[0])
	}
	if exec.calls[1][0] != "show" {
		t.Errorf("call[1]: expected show, got %v", exec.calls[1])
	}
}

// TestGatewayCloseIssuePropagatesNotFoundWhenIssueTrulyMissing verifies the
// emulation does NOT mask a real not-found: if ShowIssue also returns
// not-found (truly missing issue), the original close error surfaces.
func TestGatewayCloseIssuePropagatesNotFoundWhenIssueTrulyMissing(t *testing.T) {
	t.Parallel()

	bdStderr := []byte("Error closing bd-missing: issue not found: bd-missing\n")
	showStderr := []byte(`Error: resolving ID bd-missing: no issue found matching "bd-missing"` + "\n")

	exec := &verbDispatchExecutor{byVerb: map[string]struct {
		result bdrunner.ExecResult
		err    error
	}{
		"close": {result: bdrunner.ExecResult{Stderr: bdStderr, ExitCode: 1}},
		"show":  {result: bdrunner.ExecResult{Stderr: showStderr, ExitCode: 1}},
	}}

	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: exec}))

	err := gateway.CloseIssue(context.Background(), "bd-missing", domain.CloseIssueInput{})
	if err == nil {
		t.Fatal("CloseIssue: expected error for truly missing issue, got nil")
	}
	var gwErr domain.GatewayError
	if !errors.As(err, &gwErr) {
		t.Fatalf("CloseIssue: expected domain.GatewayError, got %T: %v", err, err)
	}
	if gwErr.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("CloseIssue: expected ErrorCodeCommandFailed, got %q", gwErr.Code)
	}
}

// TestGatewayCloseIssuePropagatesNotFoundWhenShowReturnsOpen guards against a
// silly recovery: if ShowIssue returns an OPEN issue (not closed), the
// original close error must still surface — recovery only applies when the
// issue's end-state is already what close was trying to achieve.
func TestGatewayCloseIssuePropagatesNotFoundWhenShowReturnsOpen(t *testing.T) {
	t.Parallel()

	bdStderr := []byte("Error closing bd-7: issue not found: bd-7\n")
	showJSON := []byte(`[{
		"id": "bd-7",
		"title": "still open",
		"status": "open",
		"priority": 2,
		"issue_type": "task",
		"created_at": "2026-05-17T00:00:00Z",
		"updated_at": "2026-05-17T00:00:00Z"
	}]`)

	exec := &verbDispatchExecutor{byVerb: map[string]struct {
		result bdrunner.ExecResult
		err    error
	}{
		"close": {result: bdrunner.ExecResult{Stderr: bdStderr, ExitCode: 1}},
		"show":  {result: bdrunner.ExecResult{Stdout: showJSON}},
	}}

	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: exec}))

	err := gateway.CloseIssue(context.Background(), "bd-7", domain.CloseIssueInput{})
	if err == nil {
		t.Fatal("CloseIssue: expected error when ShowIssue returns open issue, got nil")
	}
}

func TestGatewayAddCommentMapsCommandArgs(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

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
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.UpdateIssue(context.Background(), "bd-1", domain.UpdateIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.CloseIssue(context.Background(), "bd-1", domain.CloseIssueInput{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)

	err = gateway.AddComment(context.Background(), "bd-1", domain.AddCommentInput{Body: "x"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

// --- wgz0: test gap coverage ---

// TestGatewayCreateIssueLabelsAbsentFromResponse verifies that CreateIssue
// succeeds when labels are passed via --labels but the bd create JSON response
// does NOT include a "labels" field. bd 1.0.4 omits labels from the create
// response payload; the gateway must not treat this as a decode failure.
func TestGatewayCreateIssueLabelsAbsentFromResponse(t *testing.T) {
	t.Parallel()

	// Response has only "id" — no "labels" field, matching real bd 1.0.4 behavior.
	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte(`{"id":"bd-500"}`)}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	result, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title:  "labelled issue",
		Labels: []string{"x", "y"},
	})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}
	if result.IssueID != "bd-500" {
		t.Fatalf("unexpected IssueID: %q", result.IssueID)
	}

	// Assert the --labels flag was emitted.
	wantArgs := []string{"create", "--json", "--title", "labelled issue", "--labels", "x,y"}
	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// TestGatewayUpdateIssueAllNilFieldsNoOp verifies that UpdateIssueInput{} with
// ClearLabels=false causes the gateway to emit `bd update <id>` with no
// additional flags. bd 1.0.4 exits 0 with "No updates specified" in this case
// and the gateway returns nil. The test asserts the exact argv shape.
func TestGatewayUpdateIssueAllNilFieldsNoOp(t *testing.T) {
	t.Parallel()

	wantArgs := []string{"update", "bd-77"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgs).Return(bdrunner.ExecResult{Stdout: []byte("No updates specified")}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.UpdateIssue(context.Background(), "bd-77", domain.UpdateIssueInput{})
	if err != nil {
		t.Fatalf("UpdateIssue no-op returned error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 bd call, got %d: %#v", len(calls), calls)
	}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", calls[0].Args, wantArgs)
	}
}

// TestGatewayUpdateIssueStartedAtOnInProgressTransition verifies that setting
// status to "in_progress" emits `bd update <id> --status in_progress`. The
// gateway's responsibility is only to pass the flag correctly; started_at is
// set by bd internally and is not part of the gateway's argv contract.
func TestGatewayUpdateIssueStartedAtOnInProgressTransition(t *testing.T) {
	t.Parallel()

	status := "in_progress"
	wantArgs := []string{"update", "bd-88", "--status", "in_progress"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgs).Return(bdrunner.ExecResult{Stdout: []byte("ok")}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.UpdateIssue(context.Background(), "bd-88", domain.UpdateIssueInput{Status: &status})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 bd call, got %d: %#v", len(calls), calls)
	}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", calls[0].Args, wantArgs)
	}
}

// TestGatewayCloseIssueDefaultCloseReasonOmitsFlag verifies that when
// CloseIssueInput.Reason is empty, the gateway emits `bd close <id>` WITHOUT
// the --reason flag. bd 1.0.4 then defaults the close_reason to "Closed".
// The gateway must not inject a --reason flag when none was provided.
func TestGatewayCloseIssueDefaultCloseReasonOmitsFlag(t *testing.T) {
	t.Parallel()

	wantArgs := []string{"close", "bd-99"}

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("ok")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	err := gateway.CloseIssue(context.Background(), "bd-99", domain.CloseIssueInput{})
	if err != nil {
		t.Fatalf("CloseIssue returned error: %v", err)
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v (--reason must be absent when Reason is empty)", execStub.args, wantArgs)
	}
}

// TestGatewayAddCommentArgvShapeEmptyBodyAllowed verifies that AddComment
// emits `bd comments add <id> <body>` even when body is an empty string.
// The gateway does not validate body content; bd decides whether to accept it.
// Behavioral verification (updated_at unchanged, not idempotent, works on
// closed issues, long body stored verbatim, markdown stored verbatim) is bd's
// responsibility and belongs in contract/integration tests, not unit tests.
func TestGatewayAddCommentArgvShapeEmptyBodyAllowed(t *testing.T) {
	t.Parallel()

	wantArgs := []string{"comments", "add", "bd-10", ""}

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("Comment added to bd-10")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	err := gateway.AddComment(context.Background(), "bd-10", domain.AddCommentInput{Body: ""})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// TestGatewayAddCommentArgvShapeLongBody verifies that AddComment passes a
// long body (1000 chars) as a single argv element without truncation or
// modification. Whether bd stores the full body verbatim is bd's concern and
// belongs in contract/integration tests.
func TestGatewayAddCommentArgvShapeLongBody(t *testing.T) {
	t.Parallel()

	longBody := ""
	for i := 0; i < 1000; i++ {
		longBody += "x"
	}
	wantArgs := []string{"comments", "add", "bd-11", longBody}

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("Comment added to bd-11")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	err := gateway.AddComment(context.Background(), "bd-11", domain.AddCommentInput{Body: longBody})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// TestGatewayAddCommentArgvShapeMarkdownBody verifies that AddComment passes a
// markdown body as-is without escaping or modification. Whether bd stores
// markdown verbatim is bd's responsibility and belongs in contract/integration
// tests.
func TestGatewayAddCommentArgvShapeMarkdownBody(t *testing.T) {
	t.Parallel()

	markdownBody := "## heading\n\n- item 1\n- item 2\n\n```go\nfmt.Println(\"hello\")\n```"
	wantArgs := []string{"comments", "add", "bd-12", markdownBody}

	execStub := &stubExecutor{result: bdrunner.ExecResult{Stdout: []byte("Comment added to bd-12")}}
	gateway := NewCLIGateway(bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Executor: execStub}))

	err := gateway.AddComment(context.Background(), "bd-12", domain.AddCommentInput{Body: markdownBody})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	if !reflect.DeepEqual(execStub.args, wantArgs) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", execStub.args, wantArgs)
	}
}

// stubExecutor is a minimal CommandExecutor that records the last Run call
// and returns a configured result. Mirrored from runner_test.go (gateway/beads)
// which stays in its original package after the gateway code moved here.
type stubExecutor struct {
	mu      sync.Mutex
	command string
	args    []string
	workDir string
	env     []string

	result bdrunner.ExecResult
	err    error
}

func (s *stubExecutor) Run(_ context.Context, command string, args []string, workDir string, env []string) (bdrunner.ExecResult, error) {
	s.mu.Lock()
	s.command = command
	s.args = append([]string(nil), args...)
	s.workDir = workDir
	s.env = append([]string(nil), env...)
	result, err := s.result, s.err
	s.mu.Unlock()

	return result, err
}
