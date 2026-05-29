package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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

	// Stale-schema directory must have been pruned.
	if _, statErr := os.Stat(badDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected stale-schema dir %q to be removed, but it still exists (stat err: %v)", badDir, statErr)
	}
}

// TestStaleSchemaCleanupNeverDeletesWinner verifies that the winning session
// directory is never deleted even when its manifest somehow appears in the
// stale list (defensive safety guard).
//
// This scenario is contrived in practice but the guard must hold regardless.
func TestStaleSchemaCleanupNeverDeletesWinner(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "778899"

	// Write multiple stale-schema directories.
	for _, sub := range []string{hash + "-sess-stale1", hash + "-sess-stale2"} {
		dir := filepath.Join(baseDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		badManifest := []byte(`{"schema_version": 999, "synced_at": "2026-05-01T06:00:00Z", "bd_commit_hash": ""}`)
		if err := os.WriteFile(filepath.Join(dir, "repo.jsonl.manifest.json"), badManifest, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "repo.jsonl"), []byte{}, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// One current-schema winner.
	syncedAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	want := writeCacheEntry(t, baseDir, hash+"-sess-winner", syncedAt)
	winnerDir := filepath.Dir(want)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected winner %q, got %q", want, got)
	}

	// Winner directory must survive.
	if _, statErr := os.Stat(winnerDir); statErr != nil {
		t.Fatalf("winner dir %q must not be deleted: %v", winnerDir, statErr)
	}
	if _, statErr := os.Stat(want); statErr != nil {
		t.Fatalf("winner jsonl %q must not be deleted: %v", want, statErr)
	}
}

// TestStaleSchemaCleanupPreservesNonWinningCurrentSchema verifies that a
// current-schema session that was not selected as the winner (older SyncedAt)
// is NOT deleted. Only stale-schema directories should be pruned.
func TestStaleSchemaCleanupPreservesNonWinningCurrentSchema(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "aabbccdde"

	// Write a stale-schema directory.
	staleDir := filepath.Join(baseDir, hash+"-sess-stale")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	badManifest := []byte(`{"schema_version": 999, "synced_at": "2026-05-01T06:00:00Z", "bd_commit_hash": ""}`)
	if err := os.WriteFile(filepath.Join(staleDir, "repo.jsonl.manifest.json"), badManifest, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "repo.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Older current-schema session (not selected as winner).
	t1 := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	olderGoodJSONL := writeCacheEntry(t, baseDir, hash+"-sess-older-good", t1)
	olderGoodDir := filepath.Dir(olderGoodJSONL)

	// Newer current-schema session (selected as winner).
	t2 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	want := writeCacheEntry(t, baseDir, hash+"-sess-newest", t2)
	winnerDir := filepath.Dir(want)

	got := findLatestProjectCacheFile(baseDir, hash, nil)
	if got != want {
		t.Fatalf("expected newest file %q, got %q", want, got)
	}

	// Stale dir must be pruned.
	if _, statErr := os.Stat(staleDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected stale dir %q to be removed (stat err: %v)", staleDir, statErr)
	}

	// Non-winning but current-schema dir must survive.
	if _, statErr := os.Stat(olderGoodDir); statErr != nil {
		t.Fatalf("non-winning current-schema dir %q must not be deleted: %v", olderGoodDir, statErr)
	}

	// Winner dir must survive.
	if _, statErr := os.Stat(winnerDir); statErr != nil {
		t.Fatalf("winner dir %q must not be deleted: %v", winnerDir, statErr)
	}
}

// TestStaleSchemaCleanupEmitsSingleWarnNotPerFile verifies that scanning N
// stale-schema manifests emits exactly one WARN record (the summary) rather
// than one per stale file — that is the core noise-reduction guarantee.
func TestStaleSchemaCleanupEmitsSingleWarnNotPerFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	const hash = "ff00ff"

	// Write five stale-schema directories.
	const staleCount = 5
	for i := range staleCount {
		dir := filepath.Join(baseDir, fmt.Sprintf("%s-sess-stale%d", hash, i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		badManifest := []byte(`{"schema_version": 999, "synced_at": "2026-05-01T06:00:00Z", "bd_commit_hash": ""}`)
		if err := os.WriteFile(filepath.Join(dir, "repo.jsonl.manifest.json"), badManifest, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "repo.jsonl"), []byte{}, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// One good file so there is a winner.
	syncedAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	writeCacheEntry(t, baseDir, hash+"-sess-good", syncedAt)

	// Capture log records with a minimal inline slog.Handler.
	var mu sync.Mutex
	var captured []capturedLogRecord
	handler := &capturingHandler{mu: &mu, records: &captured}
	logger := slog.New(handler)

	findLatestProjectCacheFile(baseDir, hash, logger)

	mu.Lock()
	all := captured
	mu.Unlock()

	// Count WARN records with the per-file message (must be zero) vs the summary
	// message (must be one).
	perFileMsg := "cache resolver: skipping manifest with wrong schema version"
	summaryMsg := "cache resolver: pruning stale-schema session directories"

	perFileCount := 0
	summaryCount := 0
	for _, r := range all {
		if r.level == slog.LevelWarn {
			if r.msg == perFileMsg {
				perFileCount++
			}
			if r.msg == summaryMsg {
				summaryCount++
			}
		}
	}
	if perFileCount != 0 {
		t.Errorf("expected 0 per-file WARN records, got %d", perFileCount)
	}
	if summaryCount != 1 {
		t.Errorf("expected 1 summary WARN record, got %d", summaryCount)
	}
}

// capturedLogRecord holds the level and message of a single captured slog
// record. Used by capturingHandler for test assertions.
type capturedLogRecord struct {
	level slog.Level
	msg   string
}

// capturingHandler is a minimal slog.Handler that records every log record's
// level and message for inspection in tests.
type capturingHandler struct {
	mu      *sync.Mutex
	records *[]capturedLogRecord
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, capturedLogRecord{r.Level, r.Message})
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }
