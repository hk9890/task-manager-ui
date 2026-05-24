package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// errInjected is the sentinel used by all mutation error-injection tests.
// Each test asserts both the documented toast prefix (strings.HasPrefix) AND
// errors.Is(res.err, errInjected). The errors.Is assertion catches a silent
// %w → %v demotion that would leave Error() identical but break the unwrap chain.
var errInjected = errors.New("injected mutation failure")

// newMutationErrorServices returns a minimal Services container backed by the
// supplied repository. It uses config.Default() and a temp dir so no external
// editor or launcher process is ever spawned.
func newMutationErrorServices(t *testing.T, gw *appTestRepository) Services {
	t.Helper()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	return services
}

// TestMutationUpdateRepositoryError verifies that a repository failure in
// mutationUpdate produces a result whose error begins with "update issue
// failed:" and wraps the original error (i.e. errors.Is works).
func TestMutationUpdateRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1"}})
	gw.SetError(repository.MethodUpdateIssue, errInjected)

	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{
		kind:  mutationUpdate,
		issue: domain.IssueSummary{ID: "bw-1"},
		// nil statusNames/typeNames/labelNames → len(nil)==0, guards skip
	}
	// priority must be a valid integer so parseRequiredPriority passes; value
	// must survive all pre-repository validation to reach the repository call.
	values := map[string]string{
		"title":    "Updated title",
		"priority": "2",
		"labels":   "",
	}

	msg := submitMutationCmd(services, state, values)()
	res, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected non-nil error from mutationUpdate with injected repository failure")
	}
	const wantPrefix = "update issue failed:"
	if !strings.HasPrefix(res.err.Error(), wantPrefix) {
		t.Errorf("error %q does not begin with %q", res.err.Error(), wantPrefix)
	}
	if !errors.Is(res.err, errInjected) {
		t.Errorf("errors.Is(res.err, errInjected) = false; error is %q (wrapper may have used %%v instead of %%w)", res.err.Error())
	}
}

// TestMutationCloseRepositoryError verifies that a repository failure in
// mutationClose produces a result whose error begins with "close issue
// failed:" and wraps the original error.
func TestMutationCloseRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1"}})
	gw.SetError(repository.MethodCloseIssue, errInjected)

	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{
		kind:  mutationClose,
		issue: domain.IssueSummary{ID: "bw-1"},
	}
	values := map[string]string{"reason": "done"}

	msg := submitMutationCmd(services, state, values)()
	res, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected non-nil error from mutationClose with injected repository failure")
	}
	const wantPrefix = "close issue failed:"
	if !strings.HasPrefix(res.err.Error(), wantPrefix) {
		t.Errorf("error %q does not begin with %q", res.err.Error(), wantPrefix)
	}
	if !errors.Is(res.err, errInjected) {
		t.Errorf("errors.Is(res.err, errInjected) = false; error is %q (wrapper may have used %%v instead of %%w)", res.err.Error())
	}
}

// TestMutationCommentRepositoryError verifies that a repository failure in
// mutationComment produces a result whose error begins with "add comment
// failed:" and wraps the original error.
func TestMutationCommentRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1"}})
	gw.SetError(repository.MethodAddComment, errInjected)

	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{
		kind:  mutationComment,
		issue: domain.IssueSummary{ID: "bw-1"},
	}
	// body must be non-empty to pass the pre-repository guard.
	values := map[string]string{"body": "looks good"}

	msg := submitMutationCmd(services, state, values)()
	res, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected non-nil error from mutationComment with injected repository failure")
	}
	const wantPrefix = "add comment failed:"
	if !strings.HasPrefix(res.err.Error(), wantPrefix) {
		t.Errorf("error %q does not begin with %q", res.err.Error(), wantPrefix)
	}
	if !errors.Is(res.err, errInjected) {
		t.Errorf("errors.Is(res.err, errInjected) = false; error is %q (wrapper may have used %%v instead of %%w)", res.err.Error())
	}
}

// TestMutationStatusRepositoryError verifies that a repository failure in
// mutationStatus (status-only UpdateIssue) produces a result whose error
// begins with "update status failed:" and wraps the original error.
func TestMutationStatusRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1"}})
	gw.SetError(repository.MethodUpdateIssue, errInjected)

	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{
		kind: mutationStatus,
		// issue.Status is "" (zero value); values["status"] = "in_progress"
		// differs, so the noChange short-circuit is skipped.
		issue: domain.IssueSummary{ID: "bw-1"},
		// nil statusNames → len(nil)==0, unknown-status guard skips.
	}
	values := map[string]string{"status": "in_progress"}

	msg := submitMutationCmd(services, state, values)()
	res, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected non-nil error from mutationStatus with injected repository failure")
	}
	const wantPrefix = "update status failed:"
	if !strings.HasPrefix(res.err.Error(), wantPrefix) {
		t.Errorf("error %q does not begin with %q", res.err.Error(), wantPrefix)
	}
	if !errors.Is(res.err, errInjected) {
		t.Errorf("errors.Is(res.err, errInjected) = false; error is %q (wrapper may have used %%v instead of %%w)", res.err.Error())
	}
}

// TestMutationPriorityRepositoryError verifies that a repository failure in
// mutationPriority (priority-only UpdateIssue) produces a result whose error
// begins with "update priority failed:" and wraps the original error.
func TestMutationPriorityRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1"}})
	gw.SetError(repository.MethodUpdateIssue, errInjected)

	services := newMutationErrorServices(t, gw)

	state := mutationDialogState{
		kind: mutationPriority,
		// issue.Priority is 0 (zero value); values["priority"] = "2"
		// differs, so the noChange short-circuit is skipped.
		issue: domain.IssueSummary{ID: "bw-1"},
	}
	values := map[string]string{"priority": "2"}

	msg := submitMutationCmd(services, state, values)()
	res, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected non-nil error from mutationPriority with injected repository failure")
	}
	const wantPrefix = "update priority failed:"
	if !strings.HasPrefix(res.err.Error(), wantPrefix) {
		t.Errorf("error %q does not begin with %q", res.err.Error(), wantPrefix)
	}
	if !errors.Is(res.err, errInjected) {
		t.Errorf("errors.Is(res.err, errInjected) = false; error is %q (wrapper may have used %%v instead of %%w)", res.err.Error())
	}
}
