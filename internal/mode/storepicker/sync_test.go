package storepicker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

var syncNow = time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)

// newSyncModel is a picker on a fixed clock that logs into the returned
// buffer.
func newSyncModel(t *testing.T, catalog storecatalog.Catalog) (*Model, *bytes.Buffer) {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	var logs bytes.Buffer
	m := NewModel(context.Background(), catalog, slog.New(slog.NewTextHandler(&logs, nil)), keys)
	m.now = func() time.Time { return syncNow }
	m.SetSize(100, 16)
	return m, &logs
}

func TestTheSyncStateLoadsWithTheStores(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{
		Entries: entries("alpha", "bravo"),
		Sync:    storecatalog.SyncStatus{SetUp: true, Branch: "machine/box", LastSync: syncNow.Add(-2 * time.Hour)},
	}
	m, _ := newSyncModel(t, catalog)

	run(t, m, m.Init())

	if got := catalog.SyncCalls(); got != 1 {
		t.Errorf("sync state read %d times, want 1", got)
	}
	if got := len(m.Entries()); got != 2 {
		t.Errorf("listed %d entries, want 2", got)
	}
	view := ansi.ReplaceAllString(m.View(0, "help"), "")
	if !strings.Contains(view, "Last sync 2 h ago · machine/box") {
		t.Errorf("expected the sync line:\n%s", view)
	}
}

// A machine where `taskmgr sync init` never ran has nothing to say about
// sync, so the picker draws no line for it.
func TestAMachineNotSetUpForSyncDrawsNoLine(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{
		Entries: entries("alpha"),
		Sync:    storecatalog.SyncStatus{Branch: "machine/box"},
	}
	m, _ := newSyncModel(t, catalog)

	run(t, m, m.Init())

	if view := m.View(0, "help"); strings.Contains(view, "machine/box") {
		t.Errorf("a machine not set up for sync drew a sync line:\n%s", view)
	}
}

// A sync state that cannot be read fails no listing: the stores are listed,
// no sync line is drawn, and the failure is a warning in the log.
func TestASyncStateErrorLeavesTheListAndDrawsNoLine(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{
		Entries: entries("alpha", "bravo"),
		SyncErr: errors.New("config.yaml: bad machine_id"),
	}
	m, logs := newSyncModel(t, catalog)

	run(t, m, m.Init())

	if got := len(m.Entries()); got != 2 {
		t.Errorf("listed %d entries, want 2", got)
	}
	view := ansi.ReplaceAllString(m.View(0, "help"), "")
	for _, unwanted := range []string{"sync", "Sync", "failed"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("the view names %q after a failed sync read:\n%s", unwanted, view)
		}
	}
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "bravo") {
		t.Errorf("the stores are not drawn:\n%s", view)
	}
	if line := logs.String(); !strings.Contains(line, "level=WARN") || !strings.Contains(line, "bad machine_id") {
		t.Errorf("expected a warning naming the failure, got %q", line)
	}
}

// A reload reads the sync state again, and the relative time counts from the
// reload.
func TestAReloadReadsTheSyncStateAgain(t *testing.T) {
	t.Parallel()

	catalog := &fakes.FakeStoreCatalog{
		Entries: entries("alpha"),
		Sync:    storecatalog.SyncStatus{SetUp: true, Branch: "machine/box", Running: true},
	}
	m, _ := newSyncModel(t, catalog)
	run(t, m, m.Init())
	if view := ansi.ReplaceAllString(m.View(0, "help"), ""); !strings.Contains(view, "Sync running · machine/box") {
		t.Fatalf("expected the running line:\n%s", view)
	}

	catalog.Sync = storecatalog.SyncStatus{SetUp: true, Branch: "machine/box", LastSync: syncNow}
	m.now = func() time.Time { return syncNow.Add(5 * time.Minute) }
	consumed, cmd := m.HandleKey(reloadKey)
	if !consumed {
		t.Fatal("expected the reload key to be consumed")
	}
	run(t, m, cmd)

	if got := catalog.SyncCalls(); got != 2 {
		t.Errorf("sync state read %d times, want 2", got)
	}
	if view := ansi.ReplaceAllString(m.View(0, "help"), ""); !strings.Contains(view, "Last sync 5 min ago · machine/box") {
		t.Errorf("expected the reloaded sync line:\n%s", view)
	}
}

// The sync line costs the rows its line: the scroll window the controller
// keeps is the one the renderer draws.
func TestTheSyncLineNarrowsTheScrollWindow(t *testing.T) {
	t.Parallel()

	names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	without, _ := newSyncModel(t, &fakes.FakeStoreCatalog{Entries: entries(names...)})
	with, _ := newSyncModel(t, &fakes.FakeStoreCatalog{
		Entries: entries(names...),
		Sync:    storecatalog.SyncStatus{SetUp: true, Branch: "machine/box"},
	})
	for _, m := range []*Model{without, with} {
		m.SetSize(100, 16)
		run(t, m, m.Init())
	}

	// Height 16 is 13 content lines: six rows and a spare line, which the
	// sync line takes; at 15 it costs a row.
	if a, b := without.itemCapacity(), with.itemCapacity(); a != b {
		t.Errorf("height 16: capacity %d without the sync line, %d with; the spare line holds it", a, b)
	}
	without.SetSize(100, 15)
	with.SetSize(100, 15)
	if a, b := without.itemCapacity(), with.itemCapacity(); b != a-1 {
		t.Errorf("height 15: capacity %d without the sync line, %d with; want one row less", a, b)
	}
}
