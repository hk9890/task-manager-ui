// Package repofixture writes a memory.Repository to the JSONL format that
// filestorage.Load reads. It exists so tests and fixture-building scripts can
// produce a --repo-file seed without the shipped binary carrying an export path
// nobody asked for: only Load has a production caller.
//
// The format, its schema version and the manifest shape are owned by
// internal/repository/filestorage; this package only writes them.
package repofixture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hk9890/task-manager-ui/internal/repository/filestorage"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// Save writes r's contents to path (JSONL) and path+".manifest.json".
//
// path is the JSONL file; the manifest is written as a sibling named
// path+".manifest.json". Both files are written atomically (write to temp,
// then rename) so a concurrent Load does not read a partial write.
func Save(r *memory.Repository, path string) error {
	return saveSnapshot(r.Snapshot(), path)
}

// saveSnapshot writes a pre-captured snapshot slice to path (JSONL) and
// path+".manifest.json".
func saveSnapshot(issues []memory.SnapshotIssue, path string) error {
	// Write JSONL to a temp file in the same directory as the destination so
	// that os.Rename never crosses a filesystem boundary (avoids EXDEV on
	// Linux systems where /tmp is tmpfs and ~/.cache is on the root FS).
	tmpJSONL, err := os.CreateTemp(filepath.Dir(path), "taskmgr-ui-repo-*.jsonl")
	if err != nil {
		return fmt.Errorf("repofixture.Save: create temp jsonl: %w", err)
	}
	tmpJSONLPath := tmpJSONL.Name()
	defer func() { _ = os.Remove(tmpJSONLPath) }()

	w := bufio.NewWriter(tmpJSONL)
	enc := json.NewEncoder(w)

	for _, iss := range issues {
		if err := enc.Encode(iss); err != nil {
			_ = tmpJSONL.Close()
			return fmt.Errorf("repofixture.Save: encode issue %q: %w", iss.ID, err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmpJSONL.Close()
		return fmt.Errorf("repofixture.Save: flush jsonl: %w", err)
	}
	if err := tmpJSONL.Close(); err != nil {
		return fmt.Errorf("repofixture.Save: close temp jsonl: %w", err)
	}
	if err := os.Rename(tmpJSONLPath, path); err != nil {
		return fmt.Errorf("repofixture.Save: rename jsonl to %q: %w", path, err)
	}

	// Write manifest.
	m := filestorage.Manifest{
		SchemaVersion: filestorage.SchemaVersion,
		SyncedAt:      time.Now().UTC(),
	}
	mBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("repofixture.Save: marshal manifest: %w", err)
	}

	manifestPath := path + ".manifest.json"
	tmpManifest, err := os.CreateTemp(filepath.Dir(manifestPath), "taskmgr-ui-manifest-*.json")
	if err != nil {
		return fmt.Errorf("repofixture.Save: create temp manifest: %w", err)
	}
	tmpManifestPath := tmpManifest.Name()
	defer func() { _ = os.Remove(tmpManifestPath) }()

	if _, err := tmpManifest.Write(mBytes); err != nil {
		_ = tmpManifest.Close()
		return fmt.Errorf("repofixture.Save: write manifest: %w", err)
	}
	if err := tmpManifest.Close(); err != nil {
		return fmt.Errorf("repofixture.Save: close temp manifest: %w", err)
	}
	if err := os.Rename(tmpManifestPath, manifestPath); err != nil {
		return fmt.Errorf("repofixture.Save: rename manifest to %q: %w", manifestPath, err)
	}

	return nil
}
