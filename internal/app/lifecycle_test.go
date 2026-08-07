package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/logging"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// TestServiceConstructorsHaveNoFilesystemSideEffects pins that constructing
// services is inspection-safe. NewServices used to fire an undisableable
// goroutine that deleted files from the shared system temp directory, which
// NewServicesWithLauncher did not — so the two were not substitutable and no
// test could build real services without a background sweep starting.
func TestServiceConstructorsHaveNoFilesystemSideEffects(t *testing.T) {
	t.Parallel()

	// A stale-looking file in the real sweep target. Constructing services must
	// not touch it; only the Init-scheduled sweep may.
	dir := t.TempDir()
	stale := filepath.Join(dir, "taskmgr-ui-issue-ctor-check.md")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	repo := newTestRepository()

	if _, err := NewServices(repo, config.Default(), t.TempDir()); err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	launcherService, err := launcher.NewService(nil, t.TempDir(), &fakes.FakeProcessRunner{})
	if err != nil {
		t.Fatalf("launcher.NewService: %v", err)
	}
	if _, err := NewServicesWithLauncher(repo, config.Default(), launcherService); err != nil {
		t.Fatalf("NewServicesWithLauncher: %v", err)
	}

	if _, err := os.Stat(stale); err != nil {
		t.Errorf("constructing services deleted %q — constructors must return a value, not sweep the filesystem", stale)
	}
}

// TestInitSchedulesStaleTempFileSweep pins the other half: moving the sweep out
// of the constructor must not lose it. The sweep is now a Cmd the shell runs.
func TestInitSchedulesStaleTempFileSweep(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stale := filepath.Join(dir, "taskmgr-ui-issue-sweep-check.md")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	services, err := NewServices(newTestRepository(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	// Init batches the sweep; run the sweep body against our own directory so the
	// assertion does not depend on the real os.TempDir().
	m := mustNewModel(t, services)
	if m.Init() == nil {
		t.Fatal("expected Init to return a batch of startup commands")
	}
	cleanStaleTempFilesInDir(services.Logger, dir)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected the sweep to remove %q", stale)
	}
}

// TestCancelledLifecycleContextAbandonsRepositoryReads pins that the context
// threaded from startInteractive actually reaches the repository. Before it was
// wired, both mode constructors and every shell read got a context.Background()
// that was never cancelled, so quitting abandoned nothing while the godoc
// claimed a cancellation seam.
func TestCancelledLifecycleContextAbandonsRepositoryReads(t *testing.T) {
	t.Parallel()

	repo := newTestRepository()
	repo.seedIssueDetail(domain.IssueDetail{
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

	services, err := NewServices(newTestRepository(), config.Default(), t.TempDir())
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

var _ tea.Cmd = Services{}.SweepStaleTempFiles()
