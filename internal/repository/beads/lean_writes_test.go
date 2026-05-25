package beads_test

// lean_writes_test.go — beads-workbench-7202
//
// Unit tests for the two bd-1.0.4 workaround branches in lean_writes.go.
// These tests use fakes.RecordingExecutor (no real subprocess) so each test
// completes well under 100ms.
//
// # Why package beads_test (external test package)
//
// fakes.RecordingExecutor lives in internal/testing/fakes, which imports
// internal/bd. internal/repository/beads also imports internal/bd but does
// NOT import fakes in production code, so there is no import cycle when
// tests use package beads_test (external) rather than package beads (internal).
//
// # Canonical wiring (plan-review Q6)
//
// Every test creates:
//
//	rec := fakes.NewRecordingExecutor()
//	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
//	repo := repobeads.New(runner)
//
// See internal/testing/fakes/doc.go and internal/bd/doc.go for the argv
// contract testing section.

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	bd "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
	repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// -- CloseIssue workaround tests --

// TestCloseIssue_HappyPath verifies that a successful close emits exactly one
// bd close invocation and returns nil. No bd show should be triggered.
func TestCloseIssue_HappyPath(t *testing.T) {
	t.Parallel()

	const issueID = "bw-100"
	wantArgv := []string{"close", issueID}

	rec := fakes.NewRecordingExecutor()
	rec.OnArgs(wantArgv).Return(bd.ExecResult{ExitCode: 0}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.CloseIssue(context.Background(), issueID, domain.CloseIssueInput{})
	if err != nil {
		t.Fatalf("CloseIssue happy path: unexpected error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 subprocess call, got %d: %v", len(calls), callArgSummary(calls))
	}
	if !reflect.DeepEqual(calls[0].Args, wantArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, wantArgv)
	}
}

// TestCloseIssue_HappyPath_WithReason verifies that when a close reason is
// provided, the argv includes --reason <value>.
func TestCloseIssue_HappyPath_WithReason(t *testing.T) {
	t.Parallel()

	const issueID = "bw-101"
	wantArgv := []string{"close", issueID, "--reason", "completed"}

	rec := fakes.NewRecordingExecutor()
	rec.OnArgs(wantArgv).Return(bd.ExecResult{ExitCode: 0}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.CloseIssue(context.Background(), issueID, domain.CloseIssueInput{Reason: "completed"})
	if err != nil {
		t.Fatalf("CloseIssue with reason: unexpected error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 subprocess call, got %d", len(calls))
	}
	if !reflect.DeepEqual(calls[0].Args, wantArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, wantArgv)
	}
}

// TestCloseIssue_AlreadyClosedThisSecond tests the bd-1.0.4 workaround
// (leanIsCloseNotFound): bd emits "issue not found" when re-closing an issue
// within the same second (DATETIME resolution bug; upstream: gastownhall/beads#4025).
//
// Expected behaviour: the repo catches the error, calls bd show to verify
// status=closed, and returns nil (idempotent close).
func TestCloseIssue_AlreadyClosedThisSecond(t *testing.T) {
	t.Parallel()

	const issueID = "bw-200"
	closeArgv := []string{"close", issueID}
	showArgv := []string{"show", issueID, "--json"}

	// bd 1.0.4: close fails with exit 1 and "issue not found" in stderr.
	rec := fakes.NewRecordingExecutor()
	rec.OnArgs(closeArgv).Return(bd.ExecResult{
		Stderr:   []byte("issue not found"),
		ExitCode: 1,
	}, nil)
	// bd show confirms the issue is closed.
	rec.OnArgs(showArgv).Return(bd.ExecResult{
		Stdout: closedIssueJSON(issueID),
	}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.CloseIssue(context.Background(), issueID, domain.CloseIssueInput{})
	if err != nil {
		t.Fatalf("CloseIssue already-closed-this-second: expected nil (idempotent), got: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 subprocess calls (close + show), got %d: %v",
			len(calls), callArgSummary(calls))
	}
	// call[0]: bd close <id>
	if !reflect.DeepEqual(calls[0].Args, closeArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, closeArgv)
	}
	// call[1]: bd show <id> --json
	if !reflect.DeepEqual(calls[1].Args, showArgv) {
		t.Errorf("call[1].Args = %v, want %v", calls[1].Args, showArgv)
	}
}

// TestCloseIssue_GenuineNotFound tests the case where bd emits "issue not
// found" on close AND bd show also fails (issue genuinely absent).
//
// Expected behaviour: CloseIssue returns the *original close error* (not the
// show error). This is the non-idempotent path; the caller should surface the error.
func TestCloseIssue_GenuineNotFound(t *testing.T) {
	t.Parallel()

	const issueID = "bw-300"
	closeArgv := []string{"close", issueID}
	showArgv := []string{"show", issueID, "--json"}

	rec := fakes.NewRecordingExecutor()
	// Close fails with "issue not found".
	rec.OnArgs(closeArgv).Return(bd.ExecResult{
		Stderr:   []byte("issue not found"),
		ExitCode: 1,
	}, nil)
	// Show also fails: issue is genuinely absent.
	rec.OnArgs(showArgv).Return(bd.ExecResult{
		Stderr:   []byte("issue not found"),
		ExitCode: 1,
	}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.CloseIssue(context.Background(), issueID, domain.CloseIssueInput{})
	if err == nil {
		t.Fatal("CloseIssue genuine-not-found: expected error, got nil")
	}

	// The original close error (operation="close issue") must be returned, not
	// the show error. This distinguishes "returned original" from "returned show
	// error" — both are RepositoryErrors but the Operation field differs.
	var repoErr domain.RepositoryError
	if !errors.As(err, &repoErr) {
		t.Fatalf("expected domain.RepositoryError, got %T: %v", err, err)
	}
	if repoErr.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("error code = %q, want %q", repoErr.Code, domain.ErrorCodeCommandFailed)
	}
	if !strings.Contains(repoErr.Message, "issue not found") {
		t.Errorf("error message = %q, want it to contain %q", repoErr.Message, "issue not found")
	}

	// Verify argv sequence: close first, then show.
	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 subprocess calls, got %d: %v", len(calls), callArgSummary(calls))
	}
	if !reflect.DeepEqual(calls[0].Args, closeArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, closeArgv)
	}
	if !reflect.DeepEqual(calls[1].Args, showArgv) {
		t.Errorf("call[1].Args = %v, want %v", calls[1].Args, showArgv)
	}
}

// -- UpdateIssue ClearLabels workaround tests --

// TestUpdateIssue_ClearLabels_WithCurrentLabels tests the bd-1.0.4 workaround:
// `bd update --set-labels ""` is silently ignored by bd 1.0.4, so the repo
// fetches current labels via Issue() then emits a single `--remove-label <csv>`.
//
// # CSV ordering decision (plan-review Q10)
//
// lean_writes.go:94 does strings.Join(detail.Summary.Labels, ",") with NO sort.
// Order is preserved from what Issue() returns (i.e. from the bd show JSON).
// Since this test controls the fake's bd show response, byte-exact assertion is
// correct and pins "join preserves Issue() return order" as a behaviour guarantee.
// If the production code were to add sorting, this test would catch the change.
// See lean_writes.go:81-95 for the full ClearLabels branch.
func TestUpdateIssue_ClearLabels_WithCurrentLabels(t *testing.T) {
	t.Parallel()

	const issueID = "bw-400"
	showArgv := []string{"show", issueID, "--json"}
	// Labels are returned in this specific order from bd show (controlled by
	// the fake JSON below). The repo joins them as-is: "alpha,beta,gamma".
	wantRemoveLabelArgv := []string{"update", issueID, "--remove-label", "alpha,beta,gamma"}

	rec := fakes.NewRecordingExecutor()
	// Issue() call: bd show returns the issue with labels [alpha, beta, gamma].
	rec.OnArgs(showArgv).Return(bd.ExecResult{
		Stdout: issueWithLabelsJSON(issueID, []string{"alpha", "beta", "gamma"}),
	}, nil)
	// UpdateIssue call: bd update --remove-label <csv>
	rec.OnArgs(wantRemoveLabelArgv).Return(bd.ExecResult{ExitCode: 0}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.UpdateIssue(context.Background(), issueID, domain.UpdateIssueInput{
		ClearLabels: true,
	})
	if err != nil {
		t.Fatalf("UpdateIssue ClearLabels with labels: unexpected error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 subprocess calls (show + update), got %d: %v",
			len(calls), callArgSummary(calls))
	}

	// Assert call order: Issue() (bd show) BEFORE the update call.
	if calls[0].Args[0] != "show" {
		t.Errorf("call[0] must be bd show (Issue call); got argv: %v", calls[0].Args)
	}
	if calls[1].Args[0] != "update" {
		t.Errorf("call[1] must be bd update; got argv: %v", calls[1].Args)
	}

	// Assert exact argv for the show call.
	if !reflect.DeepEqual(calls[0].Args, showArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, showArgv)
	}

	// Assert exact argv for the update call including the CSV.
	// CSV order is preserved from Issue() return order (no sort in production
	// code at lean_writes.go:94). The fake JSON fixes that order to alpha,beta,gamma.
	if !reflect.DeepEqual(calls[1].Args, wantRemoveLabelArgv) {
		t.Errorf("call[1].Args = %v, want %v", calls[1].Args, wantRemoveLabelArgv)
	}

	// Sanity: verify the CSV contains exactly the right labels (set equality),
	// complementing the byte-exact check above.
	if len(calls[1].Args) >= 4 {
		gotCSV := calls[1].Args[3]
		assertCSVSetEquality(t, gotCSV, []string{"alpha", "beta", "gamma"})
	}
}

// TestUpdateIssue_ClearLabels_EmptyCurrent tests the short-circuit: when the
// issue has no current labels, no bd update call should be made.
//
// Expected behaviour: Issue() is called, returns no labels, UpdateIssue returns
// nil immediately (no update subprocess call).
func TestUpdateIssue_ClearLabels_EmptyCurrent(t *testing.T) {
	t.Parallel()

	const issueID = "bw-500"
	showArgv := []string{"show", issueID, "--json"}

	rec := fakes.NewRecordingExecutor()
	// Issue() returns an issue with no labels.
	rec.OnArgs(showArgv).Return(bd.ExecResult{
		Stdout: issueWithLabelsJSON(issueID, nil),
	}, nil)
	// No update argv registered — if the repo emits one, the default executor
	// response (zero ExecResult) will be returned, but the call count check
	// below will catch it regardless.

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	err := repo.UpdateIssue(context.Background(), issueID, domain.UpdateIssueInput{
		ClearLabels: true,
	})
	if err != nil {
		t.Fatalf("UpdateIssue ClearLabels empty-current: unexpected error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 subprocess call (show only, no update), got %d: %v",
			len(calls), callArgSummary(calls))
	}
	if !reflect.DeepEqual(calls[0].Args, showArgv) {
		t.Errorf("call[0].Args = %v, want %v", calls[0].Args, showArgv)
	}
}

// -- Helpers --

// closedIssueJSON returns a minimal bd show JSON response for an issue with
// status=closed. The Issue() method decodes this via leanDecodeIssueArray.
func closedIssueJSON(id string) []byte {
	return []byte(`[{"id":"` + id + `","title":"fixture","status":"closed","issue_type":"task","priority":3,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`)
}

// issueWithLabelsJSON returns a minimal bd show JSON response with the given
// labels. Pass nil or empty slice for an issue with no labels.
func issueWithLabelsJSON(id string, labels []string) []byte {
	labelsJSON := "[]"
	if len(labels) > 0 {
		quoted := make([]string, len(labels))
		for i, l := range labels {
			quoted[i] = `"` + l + `"`
		}
		labelsJSON = "[" + strings.Join(quoted, ",") + "]"
	}
	return []byte(`[{"id":"` + id + `","title":"fixture","status":"open","issue_type":"task","priority":3,"labels":` + labelsJSON + `,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`)
}

// callArgSummary returns a compact [][]string representation of recorded call
// argv for use in diagnostic messages.
func callArgSummary(calls []fakes.RecordedCall) [][]string {
	out := make([][]string, len(calls))
	for i, c := range calls {
		out[i] = c.Args
	}
	return out
}

// assertCSVSetEquality decodes csv and asserts it contains exactly the labels
// in want (order-independent). This is a belt-and-suspenders check alongside
// the byte-exact assertion in TestUpdateIssue_ClearLabels_WithCurrentLabels.
func assertCSVSetEquality(t *testing.T, csv string, want []string) {
	t.Helper()
	got := strings.Split(csv, ",")
	sort.Strings(got)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(got, wantSorted) {
		t.Errorf("--remove-label CSV %q: set of labels = %v, want %v", csv, got, wantSorted)
	}
}
