// Package filestorage provides Save and Load for persisting a
// memory.Repository to disk and restoring it.
//
// # File format
//
// Save writes two files:
//
//   - path — one JSON object per line (JSONL). In v2 format, the first line is
//     a [V2Header] carrying dashboardCache and catalogsCache snapshots; subsequent
//     lines are [memory.SnapshotIssue] records. In v1 format (legacy, no longer
//     written), every line was a SnapshotIssue.
//   - path + ".manifest.json" — a JSON object with schema_version, synced_at,
//     and bd_commit_hash fields.
//
// Load reads the manifest first. If schema_version does not match
// [SchemaVersion] it returns [repository.ErrSchemaMismatch] without attempting
// to parse the JSONL.
//
// [SaveWithHash] persists the supplied bd commit hash in the manifest so that
// [LoadWithManifest] callers can compare it against the current hash for
// staleness detection. [Save] is a thin wrapper that passes an empty hash.
//
// # Signature constraints
//
// Save accepts *memory.Repository (not the generic repository.Repository
// interface) so the serialiser can call Snapshot() directly rather than going
// through the Search-based API. This keeps the surface small and avoids
// coupling to the full Repository interface. The path is caller-supplied;
// cache-directory derivation (~/.cache/bwb/<project-hash>/) is Epic 2's
// concern.
//
// # Why a separate package
//
// file.go originally targeted the repository package, but repository already
// imports repository/memory (via the interface types), and memory imports
// repository — creating an import cycle. filestorage is a sibling package that
// imports both without participating in either cycle.
package filestorage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// SchemaVersion is the JSONL schema version written by Save.
// Load returns repository.ErrSchemaMismatch when the manifest's schema_version
// differs from this constant.
//
// History:
//
//	v1: memory snapshot only (one SnapshotIssue per line). dashboardCache and
//	    catalogsCache were NOT persisted — the root cause of the fbea bug. v1
//	    files are treated as degenerate on load (ErrSchemaMismatch returned);
//	    caching.Hydrate handles this as a cold start with dashboardDirty=true
//	    so the next Dashboard() call fans out to backing regardless of hash match.
//	v2: header line added as the first JSONL line. Header is a JSON object
//	    with V2Header sentinel field = true, plus dashboardCache and catalogsCache
//	    snapshots. Issue lines (SnapshotIssue) follow unchanged. Hydrate restores
//	    all three caches; dashboardDirty stays false on a clean v2 load with
//	    matching hash and non-empty dashboardCache.
const SchemaVersion = 2

// Manifest mirrors the on-disk manifest shape. It is returned by
// [LoadWithManifest] so callers can inspect persisted metadata such as
// BDCommitHash for staleness checks.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	SyncedAt      time.Time `json:"synced_at"`
	BDCommitHash  string    `json:"bd_commit_hash"`
}

// V2Header is the first JSONL line in a v2 snapshot file. It carries the
// dashboard and catalogs cache snapshots so that Hydrate can restore all
// three caches (memory, dashboardCache, catalogsCache) from a single file.
//
// The V2Header sentinel field is always true and is used by LoadWithManifest
// to distinguish the header line from issue lines (which lack this field).
type V2Header struct {
	// IsV2Header is a sentinel discriminator field. Always true in a v2 header
	// line. SnapshotIssue lines decode this field as false (default zero value)
	// because they do not contain it, allowing callers to distinguish the two
	// line types by checking IsV2Header after a first-pass decode.
	IsV2Header     bool                     `json:"v2header"`
	DashboardCache repository.DashboardData `json:"dashboardCache"`
	CatalogsCache  *repository.Catalogs     `json:"catalogsCache,omitempty"`
}

// Save writes r's contents to path (JSONL) and path+".manifest.json".
//
// path is the JSONL file; the manifest is written as a sibling named
// path+".manifest.json". Both files are written atomically (write to temp,
// then rename) so a concurrent Load does not read a partial write.
//
// bd_commit_hash in the manifest is written as empty string. Use [SaveWithHash]
// to persist a non-empty hash.
func Save(r *memory.Repository, path string) error {
	return SaveWithHash(r, path, "")
}

// SaveWithHash is like [Save] but persists the supplied bdCommitHash in the
// manifest. bdCommitHash may be empty (e.g. when vcStatusFunc is unavailable);
// in that case the manifest is written with an empty bd_commit_hash field.
//
// SaveWithHash calls r.Snapshot() to obtain the issue data. Callers that have
// already captured a snapshot (e.g. under a lock to prevent races) should use
// [SaveSnapshotWithHash] directly with the pre-captured slice.
func SaveWithHash(r *memory.Repository, path string, bdCommitHash string) error {
	return SaveSnapshotWithHash(r.Snapshot(), path, bdCommitHash)
}

// SaveSnapshotWithHash writes a pre-captured snapshot slice to path (JSONL) and
// path+".manifest.json", persisting bdCommitHash in the manifest. The JSONL is
// written in v2 format with an empty V2Header (no dashboardCache or catalogsCache
// data). Callers that have these caches available should use [SaveSnapshotV2WithHash].
//
// This is kept for callers that only need to persist the memory snapshot
// (e.g. test helpers that do not hold a caching layer's cache state).
func SaveSnapshotWithHash(issues []memory.SnapshotIssue, path string, bdCommitHash string) error {
	return SaveSnapshotV2WithHash(issues, repository.DashboardData{}, nil, path, bdCommitHash)
}

// SaveSnapshotV2WithHash writes a pre-captured snapshot and v2 cache state to
// path (JSONL) and path+".manifest.json" in v2 format.
//
// The first JSONL line is a [V2Header] encoding dashboardCache and
// catalogsCache. Subsequent lines are SnapshotIssue records. The manifest
// carries SchemaVersion=2 and bdCommitHash.
//
// catalogsCache may be nil (written as absent in the header JSON). dashboardCache
// may be a zero-value DashboardData (written as present but empty); callers
// should only set dashboardDirty=false on Hydrate when the header's
// DashboardCache is non-empty.
//
// Callers that need to snapshot under a lock to avoid a race between snapshot
// capture and concurrent Reset should call r.Snapshot() while holding their
// lock, then call SaveSnapshotV2WithHash after releasing it — the snapshot slice
// and cache copies are value-typed and safe to use without any lock.
func SaveSnapshotV2WithHash(
	issues []memory.SnapshotIssue,
	dashboardCache repository.DashboardData,
	catalogsCache *repository.Catalogs,
	path string,
	bdCommitHash string,
) error {
	// Write JSONL to a temp file in the same directory as the destination so
	// that os.Rename never crosses a filesystem boundary (avoids EXDEV on
	// Linux systems where /tmp is tmpfs and ~/.cache is on the root FS).
	tmpJSONL, err := os.CreateTemp(filepath.Dir(path), "bwb-repo-*.jsonl")
	if err != nil {
		return fmt.Errorf("filestorage.Save: create temp jsonl: %w", err)
	}
	tmpJSONLPath := tmpJSONL.Name()
	defer func() { _ = os.Remove(tmpJSONLPath) }()

	w := bufio.NewWriter(tmpJSONL)
	enc := json.NewEncoder(w)

	// Write v2 header as first line.
	header := V2Header{
		IsV2Header:     true,
		DashboardCache: dashboardCache,
		CatalogsCache:  catalogsCache,
	}
	if err := enc.Encode(header); err != nil {
		_ = tmpJSONL.Close()
		return fmt.Errorf("filestorage.Save: encode v2 header: %w", err)
	}

	// Write issue lines.
	for _, iss := range issues {
		if err := enc.Encode(iss); err != nil {
			_ = tmpJSONL.Close()
			return fmt.Errorf("filestorage.Save: encode issue %q: %w", iss.ID, err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmpJSONL.Close()
		return fmt.Errorf("filestorage.Save: flush jsonl: %w", err)
	}
	if err := tmpJSONL.Close(); err != nil {
		return fmt.Errorf("filestorage.Save: close temp jsonl: %w", err)
	}
	if err := os.Rename(tmpJSONLPath, path); err != nil {
		return fmt.Errorf("filestorage.Save: rename jsonl to %q: %w", path, err)
	}

	// Write manifest.
	m := Manifest{
		SchemaVersion: SchemaVersion,
		SyncedAt:      time.Now().UTC(),
		BDCommitHash:  bdCommitHash,
	}
	mBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("filestorage.Save: marshal manifest: %w", err)
	}

	manifestPath := path + ".manifest.json"
	tmpManifest, err := os.CreateTemp(filepath.Dir(manifestPath), "bwb-manifest-*.json")
	if err != nil {
		return fmt.Errorf("filestorage.Save: create temp manifest: %w", err)
	}
	tmpManifestPath := tmpManifest.Name()
	defer func() { _ = os.Remove(tmpManifestPath) }()

	if _, err := tmpManifest.Write(mBytes); err != nil {
		_ = tmpManifest.Close()
		return fmt.Errorf("filestorage.Save: write manifest: %w", err)
	}
	if err := tmpManifest.Close(); err != nil {
		return fmt.Errorf("filestorage.Save: close temp manifest: %w", err)
	}
	if err := os.Rename(tmpManifestPath, manifestPath); err != nil {
		return fmt.Errorf("filestorage.Save: rename manifest to %q: %w", manifestPath, err)
	}

	return nil
}

// LoadManifest reads just the manifest file at manifestPath and returns the
// parsed Manifest. Used by callers that need to compare manifests without
// loading the full JSONL.
//
// manifestPath must be the path to the manifest file itself (i.e.
// "repo.jsonl.manifest.json"), not the JSONL path.
func LoadManifest(manifestPath string) (Manifest, error) {
	mBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("filestorage.LoadManifest: read %q: %w", manifestPath, err)
	}
	var m Manifest
	if err := json.Unmarshal(mBytes, &m); err != nil {
		return Manifest{}, fmt.Errorf("filestorage.LoadManifest: decode %q: %w", manifestPath, err)
	}
	return m, nil
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
//
// Use [LoadWithManifest] when the caller needs the persisted [Manifest]
// (e.g. to read BDCommitHash for staleness detection).
func Load(path string) (*memory.Repository, error) {
	r, _, _, err := LoadWithManifest(path)
	return r, err
}

// LoadWithManifest is like [Load] but also returns the parsed [Manifest] and
// the [V2Header] (nil if the file has no header or was not v2) so callers can
// use the persisted BDCommitHash for staleness checks and restore dashboardCache
// and catalogsCache.
//
// The V2Header pointer is non-nil only when the file is v2 format and the
// first JSONL line decodes as a valid V2Header (IsV2Header == true). A nil
// V2Header means the file had no dashboard or catalogs cache data; the caller
// should treat this as a cold start for those caches.
func LoadWithManifest(path string) (*memory.Repository, Manifest, *V2Header, error) {
	// Read and check manifest.
	manifestPath := path + ".manifest.json"
	mBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, Manifest{}, nil, fmt.Errorf("filestorage.Load: read manifest %q: %w", manifestPath, err)
	}

	var m Manifest
	if err := json.Unmarshal(mBytes, &m); err != nil {
		return nil, Manifest{}, nil, fmt.Errorf("filestorage.Load: decode manifest %q: %w", manifestPath, err)
	}

	if m.SchemaVersion != SchemaVersion {
		return nil, Manifest{}, nil, fmt.Errorf("%w: file has schema_version=%d, expected %d",
			repository.ErrSchemaMismatch, m.SchemaVersion, SchemaVersion)
	}

	// Read JSONL.
	f, err := os.Open(path)
	if err != nil {
		return nil, Manifest{}, nil, fmt.Errorf("filestorage.Load: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := memory.New()
	scanner := bufio.NewScanner(f)
	// Raise the per-token cap to 16 MiB. The default 64 KiB limit causes
	// scanner.Scan to return false with bufio.ErrTooLong for any issue whose
	// serialised SnapshotIssue JSON line exceeds that size (e.g. an issue with
	// a long markdown design doc in its Description field).
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var v2Header *V2Header
	firstLine := true

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if firstLine {
			firstLine = false
			// Attempt to decode as V2Header. If IsV2Header == true, treat as header
			// and skip to the next line. Otherwise treat as a SnapshotIssue.
			var hdr V2Header
			if jsonErr := json.Unmarshal(line, &hdr); jsonErr == nil && hdr.IsV2Header {
				v2Header = &hdr
				continue
			}
			// Not a header: fall through and decode as SnapshotIssue below.
		}

		var snap memory.SnapshotIssue
		if err := json.Unmarshal(line, &snap); err != nil {
			return nil, Manifest{}, nil, fmt.Errorf("filestorage.Load: decode issue line: %w", err)
		}

		// SeedFromSnapshot restores all fields — base issue, cross-reference
		// metadata (when present), comments, and closed state — in one call.
		// For old on-disk JSONLs that predate the ref fields, SeedFromSnapshot
		// falls back to the same re-resolution path as Seed.
		r.SeedFromSnapshot(snap)
	}
	if err := scanner.Err(); err != nil {
		return nil, Manifest{}, nil, fmt.Errorf("filestorage.Load: scan jsonl: %w", err)
	}

	return r, m, v2Header, nil
}
