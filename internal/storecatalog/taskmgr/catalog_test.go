package taskmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// isolateCentralHome points the SDK at a registry of this test's own, so the
// listing does not depend on what the developer running the suite has
// registered on their machine.
func isolateCentralHome(t *testing.T) {
	t.Helper()
	t.Setenv("TASKMGR_HOME", filepath.Join(t.TempDir(), "taskmgr-home"))
	t.Setenv("TASKMGR_DIR", "")
}

func TestStoresListsRegisteredCentralStores(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "listed-fixture", "lst"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}

	entries, err := New().Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("listed %d entries, want 1: %+v", len(entries), entries)
	}

	entry := entries[0]
	if entry.Name != "listed-fixture" {
		t.Errorf("Name: got %q, want %q", entry.Name, "listed-fixture")
	}
	if entry.StorePath == "" {
		t.Error("StorePath is empty; the picker needs it to mark the active store")
	}
	if entry.ProjectPath == "" {
		t.Error("ProjectPath is empty; the picker shows it as the row's identity")
	}
	if entry.Health != storecatalog.HealthOK {
		t.Errorf("Health: got %q, want %q", entry.Health, storecatalog.HealthOK)
	}
}

// A registry entry whose store directory has been removed is reported as
// dangling rather than dropped: the picker names it so the operator can see
// why it never opens.
func TestStoresReportsADanglingEntry(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "dangling-fixture", "dng"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}

	before, err := New().Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("listed %d entries, want 1", len(before))
	}
	if err := os.RemoveAll(before[0].StorePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	after, err := New().Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("listed %d entries after removing the store directory, want the entry to remain", len(after))
	}
	if after[0].Health.Usable() {
		t.Errorf("Health: got %q, want an unusable health", after[0].Health)
	}
}

func TestStoresOnAnEmptyRegistryIsEmptyNotAnError(t *testing.T) {
	isolateCentralHome(t)

	entries, err := New().Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("listed %d entries on an empty registry, want none", len(entries))
	}
}

func TestStoresHonoursACancelledContext(t *testing.T) {
	isolateCentralHome(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New().Stores(ctx); err == nil {
		t.Error("expected a cancelled context to be reported")
	}
}

func TestConvertHealthCoversEverySDKCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   tasks.StoreHealth
		want storecatalog.Health
	}{
		{tasks.StoreOK, storecatalog.HealthOK},
		{tasks.StoreDangling, storecatalog.HealthDangling},
		{tasks.StoreBroken, storecatalog.HealthBroken},
		// A case this build does not know about, which is what a newer SDK
		// adding a fourth one looks like from here.
		{tasks.StoreHealth(99), storecatalog.HealthUnknown},
	}
	for _, tc := range cases {
		if got := convertHealth(tc.in); got != tc.want {
			t.Errorf("convertHealth(%v): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
