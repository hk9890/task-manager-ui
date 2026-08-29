// Package filestorage loads a memory.Repository from disk. It backs the
// `--repo memory --repo-file <path>` inspection flag.
//
// Loading is the whole shipped surface: nothing this project ships writes the
// file. internal/testing/repofixture writes it, for tests and for building a
// fixture by hand (docs/RUNNING.md).
//
// # File format
//
// The fixture is two files:
//
//   - path — one JSON object per line (JSONL); each line is a
//     [memory.SnapshotIssue] record.
//   - path + ".manifest.json" — a JSON object with schema_version and synced_at.
//
// Load reads the manifest first. If schema_version does not match
// [SchemaVersion] it returns [repository.ErrSchemaMismatch] without attempting
// to parse the JSONL.
//
// # Why a separate package
//
// This logic originally targeted the repository package, but repository already
// imports repository/memory (via the interface types), and memory imports
// repository — creating an import cycle. filestorage is a sibling package that
// imports both without participating in either cycle.
package filestorage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// SchemaVersion is the JSONL schema version Load accepts.
// Load returns repository.ErrSchemaMismatch when the manifest's schema_version
// differs from this constant.
//
// History:
//
//	v1: memory snapshot only (one SnapshotIssue per line).
//	v2: a header line carrying dashboard/catalogs cache snapshots preceded the
//	    issue lines. The caching layer that produced and consumed it was never
//	    built and has been removed.
//	v3: plain memory snapshot (one SnapshotIssue per line); no header line.
//	    v1/v2 files are rejected on load with ErrSchemaMismatch.
const SchemaVersion = 3

// Manifest mirrors the on-disk manifest shape.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	SyncedAt      time.Time `json:"synced_at"`
}

// Load reads a JSONL file from path and returns a populated *memory.Repository.
//
// Load reads the manifest from path+".manifest.json" first. If schema_version
// does not match [SchemaVersion], Load returns [repository.ErrSchemaMismatch]
// without reading the JSONL. Load does not panic on malformed input; it
// returns a descriptive error.
//
// The returned repository uses the default real-time clock and default ID
// generator; timestamps from the JSONL file are preserved in the seeded
// issues.
func Load(path string) (*memory.Repository, error) {
	// Read and check manifest.
	manifestPath := path + ".manifest.json"
	mBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("filestorage.Load: read manifest %q: %w", manifestPath, err)
	}

	var m Manifest
	if err := json.Unmarshal(mBytes, &m); err != nil {
		return nil, fmt.Errorf("filestorage.Load: decode manifest %q: %w", manifestPath, err)
	}

	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: file has schema_version=%d, expected %d",
			repository.ErrSchemaMismatch, m.SchemaVersion, SchemaVersion)
	}

	// Read JSONL.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("filestorage.Load: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := memory.New()
	scanner := bufio.NewScanner(f)
	// Raise the per-token cap to 16 MiB. The default 64 KiB limit causes
	// scanner.Scan to return false with bufio.ErrTooLong for any issue whose
	// serialised SnapshotIssue JSON line exceeds that size (e.g. an issue with
	// a long markdown design doc in its Description field).
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var snap memory.SnapshotIssue
		if err := json.Unmarshal(line, &snap); err != nil {
			return nil, fmt.Errorf("filestorage.Load: decode issue line: %w", err)
		}

		// SeedFromSnapshot restores all fields — base issue, cross-reference
		// metadata (when present), comments, and closed state — in one call.
		// For old on-disk JSONLs that predate the ref fields, SeedFromSnapshot
		// falls back to the same re-resolution path as Seed.
		r.SeedFromSnapshot(snap)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("filestorage.Load: scan jsonl: %w", err)
	}

	return r, nil
}
