package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/logging"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// TestCancelledLifecycleContextAbandonsRepositoryReads pins that the context
// threaded from startInteractive actually reaches the repository. Before it was
// wired, both mode constructors and every shell read got a context.Background()
// that was never cancelled, so quitting abandoned nothing while the godoc
// claimed a cancellation seam.
func TestCancelledLifecycleContextAbandonsRepositoryReads(t *testing.T) {
	t.Parallel()

	repo := fakes.NewTracked()
	seedIssueDetail(repo, domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-1", Title: "One", Status: "open", Type: "task"},
	})

	services, err := NewServices(repo, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := mustNewModelWithOptions(t, services, RuntimeOptions{Ctx: ctx})

	// Uncancelled: the read succeeds.
	msg, ok := loadDetailCmd(m.ctx, m.services, "tm-1")().(detailLoadedMsg)
	if !ok {
		t.Fatal("expected detailLoadedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected the read to succeed before cancellation, got %v", msg.err)
	}

	// Cancelled: the same read is abandoned.
	cancel()
	msg, ok = loadDetailCmd(m.ctx, m.services, "tm-1")().(detailLoadedMsg)
	if !ok {
		t.Fatal("expected detailLoadedMsg")
	}
	if !errors.Is(msg.err, context.Canceled) {
		t.Errorf("expected context.Canceled after cancelling the lifecycle context, got %v", msg.err)
	}
}

// TestHealthCheckFailureLogsThroughManagerLogger pins that the fatal
// startup diagnostic reaches the persistent session log. It used to go to
// slog.Default() — the stock stderr handler — from inside Update, i.e. straight
// into the alt-screen frame that main.go suppresses stderr to protect, and it
// never reached taskmgr-ui-<session_id>.log where an operator would look for it.
func TestHealthCheckFailureLogsThroughManagerLogger(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer
	manager := logging.New(logging.Options{
		Debug:     true,
		Stderr:    &stderr,
		StateDir:  stateDir,
		SessionID: "health-check-test",
	})

	services, err := NewServices(fakes.NewTracked(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.Logger = manager.Logger()

	m := mustNewModel(t, services)
	next, _ := m.Update(startupHealthCheckMsg{err: domain.RepositoryError{
		Code:      domain.ErrorCodeNoDatabaseFound,
		Operation: "health_check",
	}})
	if got := next.(Model).fatalErrTitle; got == "" {
		t.Fatal("expected the fatal error screen to be armed")
	}

	logged, err := os.ReadFile(manager.LogPath())
	if err != nil {
		t.Fatalf("reading %q: %v", manager.LogPath(), err)
	}
	if !strings.Contains(string(logged), "task-manager health check failed") {
		t.Errorf("expected the health-check failure in the persistent log %q, got:\n%s", manager.LogPath(), logged)
	}
}

// TestHealthCheckFailureLogsEveryErrorCode covers the failures with no fatal
// screen. The log call sat inside the NoDatabaseFound branch, so an unreadable
// store directory or a corrupt store reached neither the log
// docs/MONITORING.md points a diagnosing agent at nor a toast, and the operator
// saw only whatever the following Dashboard call rendered.
func TestHealthCheckFailureLogsEveryErrorCode(t *testing.T) {
	t.Parallel()

	for _, code := range []domain.ErrorCode{
		domain.ErrorCodeUnknown,
		domain.ErrorCodeCommandFailed,
		domain.ErrorCodeValidationFailed,
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			manager := logging.New(logging.Options{
				Debug:     true,
				Stderr:    &stderr,
				StateDir:  t.TempDir(),
				SessionID: "health-check-code-test",
			})

			services, err := NewServices(fakes.NewTracked(), config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}
			services.Logger = manager.Logger()

			m := mustNewModel(t, services)
			next, _ := m.Update(startupHealthCheckMsg{err: domain.RepositoryError{
				Code:      code,
				Operation: "health_check",
				Message:   "permission denied",
			}})
			if got := next.(Model).fatalErrTitle; got != "" {
				t.Fatalf("code %s armed the fatal error screen (%q); only NoDatabaseFound does", code, got)
			}

			logged, err := os.ReadFile(manager.LogPath())
			if err != nil {
				t.Fatalf("reading %q: %v", manager.LogPath(), err)
			}
			if !strings.Contains(string(logged), "task-manager health check failed") {
				t.Errorf("code %s was swallowed; the persistent log holds:\n%s", code, logged)
			}
		})
	}
}
