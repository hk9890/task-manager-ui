// Package filestorage provides Save and Load for persisting a
// memory.Repository to disk and restoring it.
//
// # File format
//
// Save writes two files:
//
//   - path — one JSON object per line (JSONL). Each line is a
//     [memory.SnapshotIssue] including all fields: DependsOn, Comments, and
//     all timestamps. The round-trip is lossless.
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
const SchemaVersion = 1

// Manifest mirrors the on-disk manifest shape. It is returned by
// [LoadWithManifest] so callers can inspect persisted metadata such as
// BDCommitHash for staleness checks.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	SyncedAt      time.Time `json:"synced_at"`
	BDCommitHash  string    `json:"bd_commit_hash"`
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
func SaveWithHash(r *memory.Repository, path string, bdCommitHash string) error {
	issues := r.Snapshot()

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
	r, _, err := LoadWithManifest(path)
	return r, err
}

// LoadWithManifest is like [Load] but also returns the parsed [Manifest] so
// callers can use the persisted BDCommitHash for staleness checks.
func LoadWithManifest(path string) (*memory.Repository, Manifest, error) {
	// Read and check manifest.
	manifestPath := path + ".manifest.json"
	mBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("filestorage.Load: read manifest %q: %w", manifestPath, err)
	}

	var m Manifest
	if err := json.Unmarshal(mBytes, &m); err != nil {
		return nil, Manifest{}, fmt.Errorf("filestorage.Load: decode manifest %q: %w", manifestPath, err)
	}

	if m.SchemaVersion != SchemaVersion {
		return nil, Manifest{}, fmt.Errorf("%w: file has schema_version=%d, expected %d",
			repository.ErrSchemaMismatch, m.SchemaVersion, SchemaVersion)
	}

	// Read JSONL.
	f, err := os.Open(path)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("filestorage.Load: open %q: %w", path, err)
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
			return nil, Manifest{}, fmt.Errorf("filestorage.Load: decode issue line: %w", err)
		}

		r.Seed(memory.Issue{
			ID:          snap.ID,
			Title:       snap.Title,
			Status:      snap.Status,
			Priority:    snap.Priority,
			Type:        snap.Type,
			Assignee:    snap.Assignee,
			Labels:      snap.Labels,
			Description: snap.Description,
			Notes:       snap.Notes,
			DependsOn:   snap.DependsOn,
			Related:     snap.Related,
			ParentID:    snap.ParentID,
			ChildrenIDs: snap.ChildrenIDs,
			Created:     snap.Created,
			Updated:     snap.Updated,
		})

		if len(snap.Comments) > 0 {
			memComments := make([]memory.Comment, len(snap.Comments))
			for i, c := range snap.Comments {
				memComments[i] = memory.Comment(c)
			}
			r.SeedComments(snap.ID, memComments...)
		}

		// Restore closed state: Seed does not accept a closed timestamp,
		// so we call SeedClosed for any issue that was closed.
		if snap.Status == "closed" && !snap.Closed.IsZero() {
			r.SeedClosed(snap.ID, snap.Closed, snap.CloseReason)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, Manifest{}, fmt.Errorf("filestorage.Load: scan jsonl: %w", err)
	}

	return r, m, nil
}
