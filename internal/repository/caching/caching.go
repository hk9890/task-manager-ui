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
	memory  *memory.Repository

	dashboardCache repository.DashboardData
	dashboardDirty bool

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
// ctx is propagated to the vcStatusFunc call (with a 2-second sub-timeout) and
// to the Dashboard precompute call on the match path. Callers that have no
// meaningful context may pass context.Background().
//
// loadPath and writePath may differ — this is the primary use case: load from
// the most-recent prior session's file while writing to the current session's
// own file.  Both may be empty.
//
// Behavior matrix:
//   - loadPath == "": skip load; just set cacheFilePath = writePath
//   - loadPath file does not exist: cold start; set cacheFilePath = writePath
//   - filestorage.ErrSchemaMismatch on load: cold start; set cacheFilePath = writePath
//   - other read error on load: return error; still set cacheFilePath = writePath
//   - load success, hash matches: replace memory with loaded data, attempt to
//     pre-compute dashboardCache from the loaded memory store. On success, set
//     dashboardDirty=false so the first Dashboard() call is served entirely from
//     memory. On Dashboard precompute error (e.g. ctx cancellation, future
//     invariant check), skip dashboardCache assignment and set dashboardDirty=true
//     so the next Dashboard() call falls back to backing — Hydrate still returns
//     nil (non-fatal). Also seed lastHash so future ticks detect changes rather
//     than treating the first tick as a fresh baseline.
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
	loaded, manifest, err := filestorage.LoadWithManifest(loadPath)
	if err != nil {
		// Always set writePath even on load failure so future saves work.
		setWritePath()
		if errors.Is(err, repository.ErrSchemaMismatch) {
			// Schema moved forward: degrade to cold start; future saves use writePath.
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

	// When the repo is confirmed fresh (dirty=false), pre-compute the dashboard
	// from the loaded in-memory store so the first Dashboard() call is served
	// entirely from memory without touching the backing store. This happens
	// outside the lock because loaded.Dashboard() takes its own internal lock.
	// If precompute fails (e.g. ctx cancellation), dashboardDirty is left true
	// so the next Dashboard() call falls back to backing. Hydrate still returns
	// nil — a Dashboard fallback round-trip is acceptable.
	var precomputedDashboard repository.DashboardData
	var precomputeErr error
	if !dirty {
		precomputedDashboard, precomputeErr = loaded.Dashboard(ctx)
	}

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
	if !dirty && precomputeErr == nil {
		c.dashboardCache = precomputedDashboard
		c.dashboardDirty = false
	} else {
		c.dashboardDirty = true
	}
	if seedHash != "" {
		c.lastHash = seedHash
	}
	c.mu.Unlock()
	return nil
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
	// Snapshot is taken under c.mu.RLock so that any concurrent
	// RefreshIfChanged (which holds c.mu.Lock around memory.Reset) cannot
	// interleave between pointer capture and snapshot. The returned slice is
	// value-typed and safe to use after the lock is released.
	var snapshot []memory.SnapshotIssue
	if path != "" {
		snapshot = c.memory.Snapshot()
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
	return filestorage.SaveSnapshotWithHash(snapshot, path, hash)
}

// ---- read methods ----

// Dashboard implements repository.Repository.
//
// Returns the cached DashboardData when not dirty. On a cache miss (dirty flag
// set), fetches from backing, stores the result, and clears the dirty flag.
// The dirty flag is NOT cleared on backing error.
func (c *CachingRepository) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	if err := ctx.Err(); err != nil {
		return repository.DashboardData{}, err
	}

	// Fast path: serve from cache when not dirty.
	c.mu.RLock()
	dirty := c.dashboardDirty
	if !dirty {
		data := c.dashboardCache
		c.mu.RUnlock()
		return data, nil
	}
	c.mu.RUnlock()

	// Cache miss: fetch from backing (no lock held).
	data, err := c.backing.Dashboard(ctx)
	if err != nil {
		return repository.DashboardData{}, err
	}

	// Populate cache.
	c.mu.Lock()
	c.dashboardCache = data
	c.dashboardDirty = false
	c.mu.Unlock()

	return data, nil
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
// Issue(id) call includes the new comment. Dashboard is NOT marked dirty
// because comments do not affect the Dashboard projection.
func (c *CachingRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if err := c.backing.AddComment(ctx, id, input); err != nil {
		return err
	}

	c.mu.Lock()
	c.memory.Forget(id)
	c.invalidateCatalogsLocked()
	c.mu.Unlock()

	return nil
}
