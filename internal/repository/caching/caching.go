// Package caching provides a CachingRepository decorator that wraps any
// repository.Repository with a fast in-memory read path. Reads are served
// from the in-memory cache on hit; misses are fetched from the backing store
// and stored in the cache. Writes are applied to the backing store first; on
// success, the cache is updated or invalidated synchronously.
//
// # Cache semantics
//
// Issue: per-ID cache backed by memory.Repository. Cache miss fetches from
// backing and seeds the result via SeedDetail.
//
// Dashboard: single cached DashboardData value protected by a dirty flag.
// The flag is set on any write; cleared on the next successful Dashboard fetch.
//
// Catalogs: TTL-cached. Default TTL is 5 minutes; configurable via
// WithCatalogsTTL. Any successful write (CreateIssue, UpdateIssue, CloseIssue,
// AddComment) also invalidates the catalogs cache so newly introduced labels or
// types are visible immediately on the next Catalogs() call. The TTL remains as
// a safety net for external catalog changes that occur outside this process.
//
// Search: always passes through to backing (not cached).
//
// HealthCheck: always passes through to backing.
//
// # Concurrency
//
// All methods are safe for concurrent use. The backing store is never called
// while c.mu is held. memory.Repository methods (SeedDetail, Forget, Reset,
// Snapshot) are called while c.mu is held; they take their own internal lock
// (memory.mu). Lock order is always c.mu → memory.mu; memory never acquires
// c.mu, so no deadlock cycle is possible.
package caching

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// CachingRepository decorates a backing Repository with an in-memory cache.
// Zero value is not usable; construct with New.
type CachingRepository struct {
	mu      sync.RWMutex
	backing repository.Repository

	// audit: memory — per-issue cache. Populated by Issue() cache misses via
	// SeedDetail. Also seeded from the v2 JSONL memory snapshot by Hydrate.
	// Snapshotted by SaveNow via memory.Snapshot() and persisted in the v2
	// JSONL issue lines. Reset by RefreshIfChanged on hash change.
	// Per-issue cache IS c.memory (via SeedDetail); there is no separate
	// per-issue map. Dashboard() does NOT seed memory — that path was removed
	// to prevent blank-detail regressions when the user opens an issue whose
	// full IssueDetail was never fetched (fbea).
	memory *memory.Repository

	// audit: dashboardCache — single-entry Dashboard result cache. Populated by
	// Dashboard() on a cache miss (backing call), and by Hydrate on a clean v2
	// load with matching hash (restored directly from v2 header). Persisted by
	// SaveNow via SaveSnapshotV2WithHash. Protected by dashboardDirty flag;
	// invalidated (dirty=true) by any write mutation or RefreshIfChanged hash
	// change.
	dashboardCache           repository.DashboardData
	dashboardDirty           bool
	lastDashboardClosedLimit int // last ClosedLimit used for a successful Dashboard fetch

	// audit: catalogsCache — TTL-cached Catalogs result. Populated by Catalogs()
	// on a cache miss (backing call). Persisted by SaveNow via
	// SaveSnapshotV2WithHash. Restored by Hydrate from v2 header. Invalidated
	// (nil) by any write mutation (CreateIssue, UpdateIssue, CloseIssue,
	// AddComment) so new labels/types are visible immediately. TTL is the
	// remaining safety net for external catalog changes outside this process.
	catalogsCache   *repository.Catalogs
	catalogsFetched time.Time
	catalogsTTL     time.Duration

	clock func() time.Time

	// Background refresh state.
	refreshInterval time.Duration                         // default 60s; used by tickLoop
	vcStatusFunc    func(context.Context) (string, error) // nil = no polling
	lastHash        string                                // last observed VCS hash ("" = uninitialized)

	cancel  context.CancelFunc // set by Start; nil until Start is called
	done    chan struct{}      // closed when tickLoop goroutine exits
	started bool               // guards against double-start

	// Persistence state.
	cacheFilePath string        // set by Hydrate(); empty = no persistence
	saveInterval  time.Duration // default 30s; used by tickLoop save ticker
}

// Compile-time assertion: CachingRepository must implement repository.Repository.
var _ repository.Repository = (*CachingRepository)(nil)

// New constructs a CachingRepository wrapping backing. A fresh memory.Repository
// is created internally and is not shared with any other caller.
func New(backing repository.Repository, opts ...Option) *CachingRepository {
	c := &CachingRepository{
		backing:         backing,
		memory:          memory.New(),
		dashboardDirty:  true, // first read always hits backing
		catalogsTTL:     5 * time.Minute,
		clock:           time.Now,
		refreshInterval: 60 * time.Second,
		saveInterval:    30 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Start launches the background refresh goroutine. Idempotent: a second call
// while the goroutine is already running is a no-op. If vcStatusFunc is nil,
// Start does nothing (no goroutine is spawned). The goroutine exits when ctx
// is cancelled OR Stop is called.
func (c *CachingRepository) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started || c.vcStatusFunc == nil {
		return
	}
	c.started = true
	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	go c.tickLoop(childCtx)
}

// Stop signals the background refresh goroutine to exit and waits for it to
// finish. Safe to call multiple times; subsequent calls after the first are
// no-ops. If Start was never called (or vcStatusFunc was nil), Stop is a
// no-op.
func (c *CachingRepository) Stop() {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return
	}
	c.cancel()
	done := c.done
	c.mu.Unlock()

	<-done

	c.mu.Lock()
	c.started = false
	c.cancel = nil
	c.done = nil
	c.mu.Unlock()
}

// RefreshIfChanged polls vcStatusFunc and, if the returned hash differs from
// the last observed hash, marks the Dashboard dirty and resets the per-ID
// Issue cache. On the first call (lastHash == ""), the hash is recorded as
// baseline without invalidating anything.
//
// This method is exported so tests can drive invalidation deterministically
// without relying on real timers. The background tickLoop also calls it on
// each tick.
func (c *CachingRepository) RefreshIfChanged(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	// Call vcStatusFunc without holding the lock — same pattern as backing
	// calls in Dashboard/Issue/etc.
	h, err := c.vcStatusFunc(ctx)
	if err != nil {
		// Silently skip — a failed tick must not corrupt cache state.
		// kxci.5 will wire a logger here.
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastHash == "" {
		// First observation: record as baseline, do not invalidate.
		c.lastHash = h
		return
	}
	if h == c.lastHash {
		return
	}
	// Hash changed: invalidate Dashboard and all per-ID Issue cache entries.
	c.lastHash = h
	c.dashboardDirty = true
	c.memory.Reset()
	// If the backing store maintains its own cache (e.g. beads.Repository's
	// parentSiblingCache), drain it too. The type assertion keeps the
	// dependency narrow — no import of internal/repository/beads required.
	if r, ok := c.backing.(interface{ Invalidate() }); ok {
		r.Invalidate()
	}
}

// tickLoop is the body of the background refresh goroutine. It runs two
// independent tickers:
//   - refreshT: fires every refreshInterval, calling RefreshIfChanged.
//   - saveT: fires every saveInterval, calling SaveNow.
//
// Both tickers exit when ctx is cancelled. If cacheFilePath is empty the save
// tick is a cheap no-op (SaveNow returns immediately).
//
// NOTE: periodic save only fires when the goroutine is running, which requires
// vcStatusFunc != nil (Start guard). If vcStatusFunc is nil but cacheFilePath
// is set, periodic save does not run; use SaveNow at shutdown. App wiring
// (kxci.5) is responsible for ensuring both are set together.
func (c *CachingRepository) tickLoop(ctx context.Context) {
	defer close(c.done)
	refreshT := time.NewTicker(c.refreshInterval)
	defer refreshT.Stop()
	saveT := time.NewTicker(c.saveInterval)
	defer saveT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-refreshT.C:
			c.RefreshIfChanged(ctx)
		case <-saveT.C:
			_ = c.SaveNow() // errors are best-effort; kxci.5 will wire a logger
		}
	}
}

// ---- persistence methods ----

// Hydrate loads the in-memory cache from loadPath (read source) and sets
// writePath as the destination for subsequent SaveNow / periodic save calls.
//
// Hydrate MUST be called before Start. Calling Hydrate after Start returns
// an error immediately, because the background refresh goroutine running
// concurrently could cause in-flight mutations to be overwritten by the state
// swap that Hydrate performs at the end of its execution.
//
// ctx is propagated to the vcStatusFunc call (with a 2-second sub-timeout).
// Callers that have no meaningful context may pass context.Background().
//
// loadPath and writePath may differ — this is the primary use case: load from
// the most-recent prior session's file while writing to the current session's
// own file.  Both may be empty.
//
// Behavior matrix:
//   - loadPath == "": skip load; just set cacheFilePath = writePath
//   - loadPath file does not exist: cold start; set cacheFilePath = writePath
//   - filestorage.ErrSchemaMismatch on load (v1 files): cold start; discard any
//     memory snapshot from the file. dashboardDirty stays true so the next
//     Dashboard() call fans out to backing. v1 files are click-trail-tainted
//     (the fbea bug) and must never populate c.memory.
//   - other read error on load: return error; still set cacheFilePath = writePath
//   - load success, hash matches, v2 header has non-empty dashboardCache: replace
//     memory with loaded data, restore dashboardCache and catalogsCache directly
//     from the v2 header. dashboardDirty=false so the first Dashboard() call is
//     served from the persisted dashboardCache (fast-paint). Also seed lastHash
//     so future ticks detect changes rather than treating the first tick as a
//     fresh baseline.
//   - load success, hash matches, v2 header is absent or has empty dashboardCache:
//     replace memory with loaded data; dashboardDirty stays true (degenerate file
//     — the loaded memory may be a click-trail or the file was saved before any
//     Dashboard call). The next Dashboard() call fans out to backing. Note: the
//     old behaviour was to pre-compute dashboardCache from the loaded memory; this
//     is now intentionally removed because it was the root cause of Defect #1.
//   - load success, hash mismatch (confirmed stale): leave c.memory as the
//     fresh empty memory created by New — do NOT swap in the loaded stale data,
//     which would cause per-ID Issue cache hits to serve session-A values.
//     dashboardDirty stays true (safe default). lastHash is seeded with the
//     current hash so future ticks can detect subsequent external changes.
//   - load success, hash unknown (empty persisted hash, nil vcStatusFunc, or
//     vcStatusFunc error): swap loaded into memory (best-effort warm start),
//     dashboardDirty stays true; lastHash stays "" (RefreshIfChanged will
//     record the first tick as baseline).
//
// Hash comparison: if the persisted hash is empty, or vcStatusFunc is nil, or
// vcStatusFunc returns an error, Hydrate always sets dashboardDirty=true.
func (c *CachingRepository) Hydrate(ctx context.Context, loadPath, writePath string) error {
	// Enforce the precondition: Hydrate must be called before Start.
	c.mu.RLock()
	alreadyStarted := c.started
	c.mu.RUnlock()
	if alreadyStarted {
		return errors.New("caching: Hydrate must be called before Start")
	}

	// Always set the write path regardless of load outcome.
	setWritePath := func() {
		c.mu.Lock()
		c.cacheFilePath = writePath
		c.mu.Unlock()
	}

	if loadPath == "" {
		setWritePath()
		return nil
	}

	// Load outside the lock — file IO must not hold c.mu.
	loaded, manifest, v2Header, err := filestorage.LoadWithManifest(loadPath)
	if err != nil {
		// Always set writePath even on load failure so future saves work.
		setWritePath()
		if errors.Is(err, repository.ErrSchemaMismatch) {
			// Schema mismatch: v1 files are click-trail-tainted (the fbea bug).
			// Treat as cold start — discard any memory snapshot. dashboardDirty
			// stays true so next Dashboard() fans out to backing regardless of
			// hash match. Do NOT load v1 memory into c.memory.
			return nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			// No prior session: cold start; future saves use writePath.
			return nil
		}
		// Unknown error: return it, but writePath is still set.
		return err
	}

	// Compute staleness BEFORE taking the write lock. The bd call must not
	// hold c.mu — it calls a subprocess and can take ~700ms.
	dirty := true
	confirmedMismatch := false
	var seedHash string
	c.mu.RLock()
	vcFn := c.vcStatusFunc
	c.mu.RUnlock()
	if manifest.BDCommitHash != "" && vcFn != nil {
		timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		currentHash, vcErr := vcFn(timeoutCtx)
		cancel()
		if vcErr == nil {
			// Anchor lastHash baseline in both match and mismatch cases so
			// the first RefreshIfChanged tick can detect FUTURE changes.
			seedHash = currentHash
			if currentHash == manifest.BDCommitHash {
				dirty = false
			} else {
				// Confirmed mismatch: persisted data is stale. Do NOT load
				// it into memory — start cold so stale per-ID entries are
				// never served. dashboardDirty stays true (safe default).
				confirmedMismatch = true
			}
		}
	}

	// When the repo is confirmed fresh (dirty=false) and the v2 header carries
	// a non-empty dashboardCache, restore dashboardCache directly from the header.
	// This is the candidate (b) fix: dashboardCache is served from the persisted
	// value without recomputing from the (potentially partial) memory snapshot.
	//
	// IMPORTANT: We deliberately do NOT fall back to pre-computing dashboardCache
	// from the loaded memory store. The old fallback (precompute from memory) was
	// the root cause of Defect #1 (fbea): a partial memory snapshot (click-trail)
	// produced a falsely "complete" small dashboard, and dashboardDirty=false
	// prevented the next Dashboard() call from fetching the real data from backing.
	//
	// When the v2Header is nil or its DashboardCache is empty (zero-value), we
	// leave dashboardDirty=true. The first Dashboard() call will fan out to
	// backing and populate the full dashboardCache. This is the correct behaviour
	// for a degenerate or newly-written file that has not yet performed a Dashboard
	// fetch.
	var hydratedDashboard repository.DashboardData
	var dashboardHydrated bool
	if !dirty && v2Header != nil && isDashboardDataNonEmpty(v2Header.DashboardCache) {
		// Restore directly from v2 header — this is the fast-paint path.
		// Only taken when the persisted dashboardCache is provably non-empty,
		// guaranteeing that the served dashboard is the real backing data.
		hydratedDashboard = v2Header.DashboardCache
		dashboardHydrated = true
	}
	// If !dashboardHydrated: dashboardDirty stays true (default from New),
	// and the next Dashboard() call fans out to backing.

	// Apply state under lock — brief swap only, no IO.
	//
	// On confirmed mismatch, c.memory is intentionally left as the fresh empty
	// memory.Repository created by New. Swapping in stale loaded data would
	// cause per-ID Issue cache hits to return session-A values for the remainder
	// of session B. The dashboardDirty flag already forces a Dashboard re-fetch;
	// setting lastHash=currentHash here ensures future ticks detect subsequent
	// external changes rather than treating the first tick as a baseline from "".
	c.mu.Lock()
	c.cacheFilePath = writePath
	if !confirmedMismatch {
		c.memory = loaded
	}
	if !dirty && dashboardHydrated {
		c.dashboardCache = hydratedDashboard
		c.dashboardDirty = false
		// Restore catalogsCache from v2 header if present. A nil CatalogsCache
		// in the header means no catalogs were persisted; leave c.catalogsCache
		// nil so the next Catalogs() call fetches from backing.
		if v2Header != nil && v2Header.CatalogsCache != nil {
			cats := *v2Header.CatalogsCache
			c.catalogsCache = &cats
			c.catalogsFetched = c.clock()
		}
	} else {
		c.dashboardDirty = true
	}
	if seedHash != "" {
		c.lastHash = seedHash
	}
	c.mu.Unlock()
	return nil
}

// isDashboardDataNonEmpty reports whether d contains any non-zero content.
// A zero-value DashboardData (all nil slices and zero ints) is treated as
// "no cached dashboard" — Hydrate falls back to the precompute path.
func isDashboardDataNonEmpty(d repository.DashboardData) bool {
	return d.ClosedTotal > 0 ||
		len(d.ReadyExplain.Ready) > 0 ||
		len(d.ReadyExplain.Blocked) > 0 ||
		len(d.InProgress) > 0 ||
		len(d.Closed) > 0 ||
		len(d.Blocked) > 0
}

// SaveNow writes the current in-memory cache state to the configured file path
// synchronously. No-op if cacheFilePath is empty. Returns any error from
// filestorage.SaveSnapshotWithHash; the caller decides whether to propagate or
// log.
//
// SaveNow reads vcStatusFunc (if set) to obtain the current bd commit hash,
// which is persisted in the manifest so a subsequent Hydrate can skip the
// backing Dashboard fan-out when the repo is unchanged. If vcStatusFunc is nil
// or returns an error, the hash is written as empty string; the save still
// proceeds.
//
// Locking: path, vcStatusFunc, and the memory snapshot are all captured under
// c.mu.RLock. The snapshot is taken while the lock is held so that a
// concurrent RefreshIfChanged — which holds c.mu.Lock before calling
// memory.Reset — cannot race between pointer capture and snapshot. The lock is
// released before calling vcStatusFunc and filestorage.SaveSnapshotWithHash;
// file IO must never hold c.mu.
//
// Lock order: c.mu → memory.mu (Snapshot acquires memory.mu.RLock internally).
// memory never acquires c.mu, so no deadlock cycle is possible.
func (c *CachingRepository) SaveNow() error {
	c.mu.RLock()
	path := c.cacheFilePath
	vcFn := c.vcStatusFunc
	// Snapshot and cache copies are taken under c.mu.RLock so that any
	// concurrent RefreshIfChanged (which holds c.mu.Lock around memory.Reset)
	// cannot interleave between pointer capture and snapshot. All returned
	// values are value-typed and safe to use after the lock is released.
	var snapshot []memory.SnapshotIssue
	var dashboardCache repository.DashboardData
	var catalogsCache *repository.Catalogs
	if path != "" {
		snapshot = c.memory.Snapshot()
		dashboardCache = c.dashboardCache
		if c.catalogsCache != nil {
			cats := *c.catalogsCache
			catalogsCache = &cats
		}
	}
	c.mu.RUnlock()

	if path == "" {
		return nil
	}

	var hash string
	if vcFn != nil {
		// Use a background ctx with a short timeout — SaveNow must not hang on
		// a stuck bd subprocess.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h, err := vcFn(ctx)
		if err == nil {
			hash = h
		}
		// On error, hash stays "". Save still proceeds.
	}
	return filestorage.SaveSnapshotV2WithHash(snapshot, dashboardCache, catalogsCache, path, hash)
}

// ---- read methods ----

// Dashboard implements repository.Repository.
//
// Returns the cached DashboardData when not dirty AND the requested ClosedLimit
// matches the limit used for the last successful fetch. On a cache miss (dirty
// flag set, or a different ClosedLimit is requested), fetches from backing,
// stores the result, and clears the dirty flag.
//
// When opts.ForceFresh is true the cache fast-path is skipped entirely and
// backing is called unconditionally. The result is stored in the cache and
// replaces any previously cached Dashboard. ForceFresh is a request modifier,
// not a cache key — it does not change the keying logic.
//
// Caching strategy for variable ClosedLimit (option a — re-fetch on change):
// A different requested ClosedLimit is treated as a cache miss, distinct from
// the dashboardDirty flag. This keeps ForceFresh orthogonal: we never silently
// slice a high-water cache to satisfy a smaller request, which would hide
// partial data and complicate the force-fresh interaction. The cost is one extra
// backing round-trip on a resize event, which is acceptable because resize
// events are rare compared to steady-state reads.
// See internal/repository/caching/doc.go for the full rationale.
//
// Caching strategy for ClosedOffset > 0 (pass-through):
// When opts.ClosedOffset > 0 the caller is requesting a deep page of closed
// issues that is not part of the first-page snapshot held in the cache. The
// cache fast-path is bypassed and backing is called unconditionally. On success
// the returned Closed slice is appended to the persisted dashboardCache snapshot
// (deduped by ID, latest version wins) and ClosedTotal is updated. This makes
// subsequent saves carry the full known closed set. The first-page cache
// (ClosedOffset == 0 path) is NOT modified by this call: its dirty flag and
// lastDashboardClosedLimit remain unchanged so callers using ClosedOffset == 0
// continue to receive cached first-page results as before.
func (c *CachingRepository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	if err := ctx.Err(); err != nil {
		return repository.DashboardData{}, err
	}

	// Pass-through path: deep pagination pages bypass the first-page cache.
	if opts.ClosedOffset > 0 {
		data, err := c.backing.Dashboard(ctx, opts)
		if err != nil {
			return repository.DashboardData{}, err
		}
		// Append the returned closed page into the persisted-cache snapshot so
		// that SaveNow carries the full known closed set. We do NOT touch
		// dashboardDirty or lastDashboardClosedLimit — the first-page cache
		// remains valid and unmodified.
		c.mu.Lock()
		c.dashboardCache = mergeClosedIntoCache(c.dashboardCache, data)
		c.mu.Unlock()
		return data, nil
	}

	if !opts.ForceFresh {
		// Fast path: serve from cache when not dirty and ClosedLimit is unchanged.
		c.mu.RLock()
		dirty := c.dashboardDirty
		limitChanged := opts.ClosedLimit != c.lastDashboardClosedLimit
		if !dirty && !limitChanged {
			data := c.dashboardCache
			c.mu.RUnlock()
			return data, nil
		}
		c.mu.RUnlock()
	}

	// Cache miss or ForceFresh: fetch from backing (no lock held).
	data, err := c.backing.Dashboard(ctx, opts)
	if err != nil {
		return repository.DashboardData{}, err
	}

	// Populate cache.
	c.mu.Lock()
	c.dashboardCache = data
	c.dashboardDirty = false
	c.lastDashboardClosedLimit = opts.ClosedLimit
	c.mu.Unlock()

	return data, nil
}

// mergeClosedIntoCache merges the Closed slice from page into the existing
// dashboardCache snapshot. Issues already in the cache (matched by ID) are
// replaced with the version from page (latest wins); new issues are appended.
// ClosedTotal in the returned value is taken from page (backing is authoritative
// for the total count). All other fields (ReadyExplain, InProgress, Blocked) are
// left unchanged from existing.
func mergeClosedIntoCache(existing repository.DashboardData, page repository.DashboardData) repository.DashboardData {
	if len(page.Closed) == 0 {
		// Nothing to merge; update ClosedTotal if the page is authoritative.
		if page.ClosedTotal > 0 {
			existing.ClosedTotal = page.ClosedTotal
		}
		return existing
	}

	// Build a set of IDs from the incoming page for fast lookup.
	pageIDs := make(map[string]struct{}, len(page.Closed))
	for _, issue := range page.Closed {
		pageIDs[issue.ID] = struct{}{}
	}

	// Retain existing entries that are NOT overridden by the incoming page.
	merged := make([]domain.IssueSummary, 0, len(existing.Closed)+len(page.Closed))
	for _, issue := range existing.Closed {
		if _, replaced := pageIDs[issue.ID]; !replaced {
			merged = append(merged, issue)
		}
	}
	// Append all entries from the incoming page (new + replacements).
	merged = append(merged, page.Closed...)

	existing.Closed = merged
	existing.ClosedTotal = page.ClosedTotal
	return existing
}

// Issue implements repository.Repository.
//
// Returns the cached IssueDetail when available. On a cache miss, fetches from
// backing, seeds the result into the in-memory store, and returns the backing's
// value directly. Cache misses on backing errors are NOT stored.
//
// Note: on a cache hit the returned value is projected from the in-memory store
// via memory.Repository.Issue, which re-resolves cross-issue references (Related,
// Blocks, ParentGroupBrowser) against whatever is currently in the memory store.
// This is safe in practice because those reference fields carry only IDs and
// lightweight metadata, but callers should be aware the hit-path projection may
// differ slightly from the backing's original value when the memory store has
// been partially hydrated.
func (c *CachingRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	if err := ctx.Err(); err != nil {
		return domain.IssueDetail{}, err
	}

	// Fast path: check in-memory cache.
	c.mu.RLock()
	detail, err := c.memory.Issue(ctx, id)
	c.mu.RUnlock()

	if err == nil {
		// Cache hit.
		return detail, nil
	}
	if !errors.Is(err, repository.ErrIssueNotFound) {
		// Unexpected error from in-memory store; propagate.
		return domain.IssueDetail{}, err
	}

	// Cache miss: fetch from backing (no lock held).
	detail, err = c.backing.Issue(ctx, id)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	// Seed into memory cache.
	c.mu.Lock()
	c.memory.SeedDetail(detail)
	c.mu.Unlock()

	return detail, nil
}

// Search implements repository.Repository.
//
// Always passes through to backing. Search results are not cached because
// queries are too varied to key effectively.
func (c *CachingRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return c.backing.Search(ctx, query)
}

// Catalogs implements repository.Repository.
//
// Returns cached catalogs when within TTL. On a cache miss (or first call),
// fetches from backing, stores the result with the current timestamp, and
// returns the value.
func (c *CachingRepository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	if err := ctx.Err(); err != nil {
		return repository.Catalogs{}, err
	}

	// Fast path: serve from cache when within TTL.
	c.mu.RLock()
	if c.catalogsCache != nil && c.clock().Sub(c.catalogsFetched) < c.catalogsTTL {
		cats := *c.catalogsCache
		c.mu.RUnlock()
		return cats, nil
	}
	c.mu.RUnlock()

	// Cache miss: fetch from backing (no lock held).
	cats, err := c.backing.Catalogs(ctx)
	if err != nil {
		return repository.Catalogs{}, err
	}

	// Populate cache.
	c.mu.Lock()
	c.catalogsCache = &cats
	c.catalogsFetched = c.clock()
	c.mu.Unlock()

	return cats, nil
}

// HealthCheck implements repository.Repository.
//
// Always passes through to backing. Cancellation is handled by the backing
// implementation; no additional check is needed here since no cache path is
// taken.
func (c *CachingRepository) HealthCheck(ctx context.Context) error {
	return c.backing.HealthCheck(ctx)
}

// ---- helpers ----

// invalidateCatalogsLocked clears the catalogsCache. Must be called with c.mu
// held for writing. Setting catalogsCache to nil is sufficient: the Catalogs()
// fast path checks `catalogsCache != nil` before reading catalogsFetched, so a
// nil pointer causes an unconditional cache miss on the next call.
//
// Called from every write method so that newly introduced labels or types are
// visible immediately without waiting for the TTL to expire.
func (c *CachingRepository) invalidateCatalogsLocked() {
	c.catalogsCache = nil
}

// ---- write methods ----

// CreateIssue implements repository.Repository.
//
// Calls backing first. On success, marks dashboardDirty so the next
// Dashboard() call re-fetches from backing and sees the new issue.
//
// We do not seed memory from the input — the backing's true record is
// fetched on the next Issue(id) call. This avoids fabricating
// Status/Type/Priority/CreatedAt defaults: the backing may assign
// different values (project-policy priority, server-stamped CreatedAt,
// normalized title) than any input-derived guess would produce.
func (c *CachingRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	result, err := c.backing.CreateIssue(ctx, input)
	if err != nil {
		return domain.CreateIssueResult{}, err
	}

	c.mu.Lock()
	c.dashboardDirty = true
	c.invalidateCatalogsLocked()
	c.mu.Unlock()

	return result, nil
}

// UpdateIssue implements repository.Repository.
//
// Calls backing first. On success, marks dashboardDirty and forgets the cached
// Issue(id) so the next Issue(id) call re-fetches the updated state from backing.
func (c *CachingRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	if err := c.backing.UpdateIssue(ctx, id, input); err != nil {
		return err
	}

	c.mu.Lock()
	c.dashboardDirty = true
	c.memory.Forget(id)
	c.invalidateCatalogsLocked()
	c.mu.Unlock()

	return nil
}

// CloseIssue implements repository.Repository.
//
// Calls backing first. On success, marks dashboardDirty and forgets the cached
// Issue(id).
func (c *CachingRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	if err := c.backing.CloseIssue(ctx, id, input); err != nil {
		return err
	}

	c.mu.Lock()
	c.dashboardDirty = true
	c.memory.Forget(id)
	c.invalidateCatalogsLocked()
	c.mu.Unlock()

	return nil
}

// AddComment implements repository.Repository.
//
// Calls backing first. On success, forgets the cached Issue(id) so the next
// Issue(id) call includes the new comment. Dashboard IS marked dirty because
// bd advances UpdatedAt on AddComment, and Dashboard slot summaries carry
// UpdatedAt and sort by it.
func (c *CachingRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if err := c.backing.AddComment(ctx, id, input); err != nil {
		return err
	}

	c.mu.Lock()
	c.memory.Forget(id)
	c.dashboardDirty = true
	c.invalidateCatalogsLocked()
	c.mu.Unlock()

	return nil
}
