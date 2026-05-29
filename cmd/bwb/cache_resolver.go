package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
)

// findLatestProjectCacheFile scans cacheBaseDir for subdirectories matching
// the pattern <projectHash>-* and returns the path to the repo.jsonl whose
// manifest has the most-recent SyncedAt timestamp.
//
// Returns "" when no valid file is found. Manifest files that are unreadable
// or corrupt are skipped with a WARN log record. Manifest files that carry a
// schema version different from filestorage.SchemaVersion are collected for
// best-effort cleanup (their session subdirectory is removed) rather than
// individually warned; a single summary WARN is emitted only when at least one
// stale-schema directory was found (before cleanup).
//
// Cleanup is best-effort: errors are silently ignored and never fatal.
func findLatestProjectCacheFile(cacheBaseDir, projectHash string, logger *slog.Logger) string {
	if cacheBaseDir == "" || projectHash == "" {
		return ""
	}

	pattern := filepath.Join(cacheBaseDir, projectHash+"-*", "repo.jsonl.manifest.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	var (
		bestJSONL  string
		bestSyncNS int64 // UnixNano; 0 = no valid entry yet
		foundAny   bool

		// staleDirs accumulates session subdirectories whose manifest carries a
		// schema version that does not match the current binary. These are pruned
		// after the winning entry is selected.
		staleDirs []string
	)

	for _, manifestPath := range matches {
		m, loadErr := filestorage.LoadManifest(manifestPath)
		if loadErr != nil {
			if logger != nil {
				logger.Warn("cache resolver: skipping unreadable manifest",
					"path", manifestPath, "err", loadErr)
			}
			continue
		}
		if m.SchemaVersion != filestorage.SchemaVersion {
			// Collect the session directory for later best-effort cleanup instead
			// of emitting one WARN per stale file.
			staleDirs = append(staleDirs, filepath.Dir(manifestPath))
			continue
		}

		// Derive the repo.jsonl path: strip the ".manifest.json" suffix.
		jsonlPath := manifestPath[:len(manifestPath)-len(".manifest.json")]

		ns := m.SyncedAt.UnixNano()
		if !foundAny || ns > bestSyncNS {
			bestJSONL = jsonlPath
			bestSyncNS = ns
			foundAny = true
		}
	}

	// Best-effort cleanup of stale-schema session directories.
	// The winning directory (containing bestJSONL) is never removed even if it
	// somehow ended up in the stale list.
	if len(staleDirs) > 0 {
		if logger != nil {
			logger.Warn("cache resolver: pruning stale-schema session directories",
				"count", len(staleDirs))
		}
		bestDir := ""
		if bestJSONL != "" {
			bestDir = filepath.Dir(bestJSONL)
		}
		for _, dir := range staleDirs {
			if dir == bestDir {
				// Safety guard: never delete the selected live session's directory.
				continue
			}
			_ = os.RemoveAll(dir) // best-effort; errors silently ignored
		}
	}

	return bestJSONL
}
