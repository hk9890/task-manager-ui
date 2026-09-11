package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// Nothing resolving for the working directory used to exit 1. It now starts the
// app on the store picker, with the no-store repository and a reason naming the
// directory.
func TestStartupWithNoStoreStartsOnThePicker(t *testing.T) {
	isolateCentralHome(t)

	dir := t.TempDir()
	selected, reason, err := resolveStartupStore(startupOptions{repoFlag: "taskmgr", projectRoot: dir})
	if err != nil {
		t.Fatalf("resolveStartupStore: %v; nothing resolving must no longer be an error", err)
	}
	if !strings.Contains(reason, dir) {
		t.Errorf("reason should name the directory, got %q", reason)
	}
	assertNoStoreRepository(t, selected)
	if selected.storePath != "" {
		t.Errorf("storePath: got %q, want none with no store open", selected.storePath)
	}
}

// --store-name naming no registered store is the same situation: the picker is
// the answer to "which stores are there?".
func TestStartupWithAnUnregisteredStoreNameStartsOnThePicker(t *testing.T) {
	isolateCentralHome(t)

	selected, reason, err := resolveStartupStore(startupOptions{
		repoFlag:    "taskmgr",
		projectRoot: t.TempDir(),
		storeName:   "does-not-exist",
	})
	if err != nil {
		t.Fatalf("resolveStartupStore: %v", err)
	}
	if !strings.Contains(reason, "does-not-exist") {
		t.Errorf("reason should name the store, got %q", reason)
	}
	assertNoStoreRepository(t, selected)
}

func TestStartupWithAResolvedStoreHasNoReason(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.Init(project, "res"); err != nil {
		t.Fatalf("tasks.Init: %v", err)
	}

	selected, reason, err := resolveStartupStore(startupOptions{repoFlag: "taskmgr", projectRoot: project})
	if err != nil {
		t.Fatalf("resolveStartupStore: %v", err)
	}
	if reason != "" {
		t.Errorf("reason: got %q, want none when a store resolved", reason)
	}
	if selected.storeName != filepath.Base(project) {
		t.Errorf("storeName: got %q, want the project directory's name", selected.storeName)
	}
}

// Only "there is no store" changes. A store that exists but cannot be opened is
// still an error: it is not something the operator can pick around, and
// starting on the picker would hide it.
func TestStartupWithAnUnreadableStoreIsStillAnError(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "broken-fixture", "brk"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}
	entries, err := tasks.Stores()
	if err != nil || len(entries) != 1 {
		t.Fatalf("tasks.Stores: %v (%d entries)", err, len(entries))
	}
	// A store directory with no config is "broken", which resolution reports.
	if err := os.Remove(filepath.Join(entries[0].StorePath, "config.yaml")); err != nil {
		t.Fatalf("remove config: %v", err)
	}

	_, _, err = resolveStartupStore(startupOptions{repoFlag: "taskmgr", projectRoot: project})
	if err == nil {
		t.Fatal("expected a broken store to fail startup rather than open the picker")
	}
	if errors.Is(err, tasks.ErrNoStore) || errors.Is(err, tasks.ErrStoreNotRegistered) {
		t.Errorf("a broken store reported as no store at all: %v", err)
	}
}

func TestStartupWithAnUnloadableMemoryFixtureIsStillAnError(t *testing.T) {
	t.Parallel()

	_, _, err := resolveStartupStore(startupOptions{
		repoFlag:    "memory",
		repoFile:    filepath.Join(t.TempDir(), "missing.jsonl"),
		projectRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected a missing --repo-file to fail startup")
	}
}

func assertNoStoreRepository(t *testing.T, selected backend) {
	t.Helper()

	if selected.repo == nil {
		t.Fatal("no repository; the shell cannot be built without one")
	}
	var repoErr domain.RepositoryError
	if err := selected.repo.HealthCheck(context.Background()); !errors.As(err, &repoErr) || repoErr.Code != domain.ErrorCodeNoDatabaseFound {
		t.Errorf("HealthCheck: got %v, want the no-store repository's %q", err, domain.ErrorCodeNoDatabaseFound)
	}
}
