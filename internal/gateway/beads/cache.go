package beads

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// lastTouchedFile is the bd-maintained sentinel file whose mtime advances on
// every write to the beads database. It is the canonical change-token source.
// Verified empirically: mtime advances on bd writes; the embeddeddolt dir mtime
// does NOT advance reliably and must not be used. See epic beads-workbench-wcgy
// NOTES for the audit trail.
const lastTouchedFile = ".beads/last-touched"

// cacheEntry holds one cached read result keyed by argv.
type cacheEntry struct {
	token  time.Time // mtime of .beads/last-touched at exec time
	stdout []byte
}

// readCache is an in-process cache for bd read results.
//
// # Cache key
//
// The key is the resolved argv string (post --readonly prefix) joined with a
// NUL separator. This is the same argv passed to the executor.
//
// # Change token
//
// The token is the mtime of .beads/last-touched, resolved relative to the
// runner's bound WorkDir. bd advances this file on every write. Belt-and-
// suspenders: IsWrite=true requests also clear the cache directly so that
// bwb-initiated writes invalidate even before the file mtime advances.
//
// # Cache semantics
//
//   - Only successful reads (exit 0, nil error) are stored.
//   - Failures and write requests are never cached.
//   - The cache is bypassed when WorkDir is empty or when the last-touched file
//     cannot be stat'd (ENOENT, permission error, etc.).
//
// # Concurrency
//
// cacheMu guards all cache state and is independent of the runner's runMu.
// Hits are served under cacheMu.RLock without acquiring runMu or sem.
type readCache struct {
	cacheMu sync.RWMutex
	// entries is unbounded in principle but bounded in practice by user-driven argv variation:
	// each distinct query/show argv adds one entry, all entries clear on any write
	// (see invalidate()). No TTL/LRU until measurement shows steady-state growth.
	entries map[string]cacheEntry
	workDir string // the runner's bound WorkDir; empty = cache disabled
}

func newReadCache(workDir string) *readCache {
	return &readCache{
		entries: make(map[string]cacheEntry),
		workDir: workDir,
	}
}

// argvKey converts a resolved argv slice into a map key.
func argvKey(argv []string) string {
	return strings.Join(argv, "\x00")
}

// tokenPath returns the absolute path to .beads/last-touched.
func (c *readCache) tokenPath() string {
	return filepath.Join(c.workDir, lastTouchedFile)
}

// currentToken stats .beads/last-touched and returns its mtime.
// Returns (zero, false) if workDir is empty or the stat fails for any reason
// (including ENOENT — file not yet created). A zero token disables caching for
// that call.
func (c *readCache) currentToken() (time.Time, bool) {
	if c.workDir == "" {
		return time.Time{}, false
	}
	fi, err := os.Stat(c.tokenPath())
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}

// get returns the cached stdout for argv if an entry exists with a matching
// token. Returns (nil, false) on any miss.
func (c *readCache) get(argv []string) ([]byte, bool) {
	token, ok := c.currentToken()
	if !ok {
		return nil, false
	}

	key := argvKey(argv)

	c.cacheMu.RLock()
	entry, found := c.entries[key]
	c.cacheMu.RUnlock()

	if !found || !entry.token.Equal(token) {
		return nil, false
	}
	// Return a copy so callers cannot mutate the cached slice. All gateway callers
	// pass the bytes to json.NewDecoder which does not mutate, but copying here
	// prevents a future caller from silently corrupting the cache.
	out := make([]byte, len(entry.stdout))
	copy(out, entry.stdout)
	return out, true
}

// set stores a successful read result for argv under the given token.
func (c *readCache) set(argv []string, token time.Time, stdout []byte) {
	key := argvKey(argv)
	stored := make([]byte, len(stdout))
	copy(stored, stdout)

	c.cacheMu.Lock()
	c.entries[key] = cacheEntry{token: token, stdout: stored}
	c.cacheMu.Unlock()
}

// invalidate clears all cached entries. Called by write requests.
func (c *readCache) invalidate() {
	c.cacheMu.Lock()
	c.entries = make(map[string]cacheEntry)
	c.cacheMu.Unlock()
}
