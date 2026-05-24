package main

import (
	"log/slog"
	"path/filepath"

	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
)

// findLatestProjectCacheFile scans cacheBaseDir for subdirectories matching
// the pattern <projectHash>-* and returns the path to the repo.jsonl whose
// manifest has the most-recent SyncedAt timestamp.
//
// Returns "" when no valid file is found. Manifest files that are unreadable,
// corrupt, or carry a schema version different from filestorage.SchemaVersion
// are skipped; when logger is non-nil a WARN record is emitted for each skip.
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
			if logger != nil {
				logger.Warn("cache resolver: skipping manifest with wrong schema version",
					"path", manifestPath, "schema_version", m.SchemaVersion)
			}
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

	return bestJSONL
}
