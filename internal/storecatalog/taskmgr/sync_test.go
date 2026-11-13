package taskmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// syncHome is a TASKMGR_HOME of the test's own, with the machine ID set so
// the branch does not depend on the hostname of the machine running the suite.
func syncHome(t *testing.T) string {
	t.Helper()
	isolateCentralHome(t)
	home := os.Getenv("TASKMGR_HOME")
	writeFile(t, filepath.Join(home, "config.yaml"), "version: 1\nmachine_id: test-box\n")
	return home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestSyncStatusOfAMachineNotSetUp(t *testing.T) {
	syncHome(t)

	status, err := New("tester").SyncStatus(context.Background())
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if status.SetUp || status.Running || !status.LastSync.IsZero() {
		t.Errorf("a machine where sync init never ran reports %+v", status)
	}
	if status.Branch != "machine/test-box" {
		t.Errorf("Branch: got %q, want machine/test-box", status.Branch)
	}
}

// A stores repository with a recorded remote and publish is set up, and the
// time of its last publish comes through.
func TestSyncStatusMapsTheLastPublish(t *testing.T) {
	home := syncHome(t)
	if err := os.MkdirAll(filepath.Join(home, "stores", ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(home, ".publish.state"),
		"remote: https://example.invalid/stores.git\nbranch: machine/test-box\nlast_sync: 2026-11-13T12:00:00Z\ncommit: abc123\n")

	status, err := New("tester").SyncStatus(context.Background())
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if !status.SetUp {
		t.Error("SetUp is false for a stores repository with a recorded remote")
	}
	if status.Branch != "machine/test-box" {
		t.Errorf("Branch: got %q, want machine/test-box", status.Branch)
	}
	if want := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC); !status.LastSync.Equal(want) {
		t.Errorf("LastSync: got %v, want %v", status.LastSync, want)
	}
	if status.Running {
		t.Error("Running is true with no publish lock held")
	}
}

func TestSyncStatusReportsAnUnreadableState(t *testing.T) {
	home := syncHome(t)
	writeFile(t, filepath.Join(home, ".publish.state"), "remote: [unclosed\n")

	if _, err := New("tester").SyncStatus(context.Background()); err == nil {
		t.Error("expected a damaged publish state file to be reported")
	}
}

func TestSyncStatusHonoursACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New("tester").SyncStatus(ctx); err == nil {
		t.Error("expected a cancelled context to be reported")
	}
}
