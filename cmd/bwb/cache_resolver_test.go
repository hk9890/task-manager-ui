package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// writeCacheEntry creates a <baseDir>/<subDir>/repo.jsonl file with a manifest
// whose SyncedAt is overwritten to syncedAt. Returns the repo.jsonl path.
func writeCacheEntry(t *testing.T, baseDir, subDir string, syncedAt time.Time) string {
	t.Helper()
	dir := filepath.Join(baseDir, subDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("writeCacheEntry: mkdir %q: %v", dir, err)
	}
	jsonlPath := filepath.Join(dir, "repo.jsonl")
	r := memory.New()
	r.Seed(memory.Issue{
		ID:     "entry-issue",
		Title:  "test",
		Status: "open",
		Type:   "task",
	})
	if err := filestorage.Save(r, jsonlPath); err != nil {
		t.Fatalf("writeCacheEntry: Save: %v", err)
	}
	// Overwrite the manifest with a controlled SyncedAt.
	overwriteManifestSyncedAt(t, jsonlPath+".manifest.json", syncedAt)
	return jsonlPath
}

// overwriteManifestSyncedAt reads the manifest at manifestPath, sets SyncedAt
// to syncedAt, and writes it back.
func overwriteManifestSyncedAt(t *testing.T, manifestPath string, syncedAt time.Time) {
	t.Helper()
	m, err := filestorage.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("overwriteManifestSyncedAt: LoadManifest: %v", err)
	}
	type manifestJSON struct {
		SchemaVersion int       `json:"schema_version"`
		SyncedAt      time.Time `json:"synced_at"`
		BDCommitHash  string    `json:"bd_commit_hash"`
	}
	data, err := json.MarshalIndent(manifestJSON{
		SchemaVersion: m.SchemaVersion,
		SyncedAt:      syncedAt,
		BDCommitHash:  m.BDCommitHash,
	}, "", "  ")
	if err != nil {
		t.Fatalf("overwriteManifestSyncedAt: marshal: %v", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("overwriteManifestSyncedAt: WriteFile: %v", err)
	}
}

// ---- findLatestProjectCacheFile tests ----

func TestFindLatestEmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got := findLatestProjectCacheFile(dir, "abc123", nil)
	if got != "" {
		t.Fatalf("expected empty result for empty dir, got %q", got)
	}
}

func TestFindLatestNonExistentDir(t *testing.T) {
	t.Parallel()

	got := findLatestProjectCacheFile(filepath.Join(t.TempDir(), "does-not-exist"), "abc123", nil)
	if got != "" {
		t.Fatalf("expected empty result for non-existent dir, got %q", got)
	}
}

func TestFindLatestSingleFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "aabbcc"
	syncedAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	want := writeCacheEntry(t, baseDir, hash+"-session1", syncedAt)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFindLatestMultipleFiles(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "ddeeff"

	// Write three files with different synced_at times.
	t1 := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) // newest
	t3 := time.Date(2026, 5, 1, 6, 0, 0, 0, time.UTC)

	writeCacheEntry(t, baseDir, hash+"-sess-old1", t1)
	want := writeCacheEntry(t, baseDir, hash+"-sess-newest", t2)
	writeCacheEntry(t, baseDir, hash+"-sess-old2", t3)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected newest file %q, got %q", want, got)
	}
}

func TestFindLatestSkipsCorruptManifest(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "112233"

	// One corrupt manifest.
	corruptDir := filepath.Join(baseDir, hash+"-sess-corrupt")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	corruptManifestPath := filepath.Join(corruptDir, "repo.jsonl.manifest.json")
	if err := os.WriteFile(corruptManifestPath, []byte("not-valid-json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Also write the (irrelevant) jsonl file.
	if err := os.WriteFile(filepath.Join(corruptDir, "repo.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// One good manifest.
	syncedAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	want := writeCacheEntry(t, baseDir, hash+"-sess-good", syncedAt)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected good file %q, got %q", want, got)
	}
}

func TestFindLatestSkipsDifferentProject(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hashABC = "aabbcc"
	const hashDEF = "ddeeff"

	// Write files for two different project hashes. DEF's file is newer than ABC's.
	syncedABC := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	syncedDEF := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)

	wantABC := writeCacheEntry(t, baseDir, hashABC+"-sess1", syncedABC)
	writeCacheEntry(t, baseDir, hashDEF+"-sess2", syncedDEF)

	// Ask for hashABC: should only return ABC's file, ignoring the newer DEF one.
	got := findLatestProjectCacheFile(baseDir, hashABC, nil)
	if got != wantABC {
		t.Fatalf("expected ABC file %q, got %q", wantABC, got)
	}
}

func TestFindLatestSkipsWrongSchemaVersion(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "445566"

	// Write a file with a wrong schema version.
	badDir := filepath.Join(baseDir, hash+"-sess-bad-schema")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	badManifest := []byte(`{"schema_version": 999, "synced_at": "2026-05-01T12:00:00Z", "bd_commit_hash": ""}`)
	if err := os.WriteFile(filepath.Join(badDir, "repo.jsonl.manifest.json"), badManifest, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "repo.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// One good file.
	syncedAt := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	want := writeCacheEntry(t, baseDir, hash+"-sess-good", syncedAt)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected good file %q, got %q", want, got)
	}
}
