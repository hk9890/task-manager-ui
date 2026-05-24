package caching

import (
	"context"
	"time"
)

// Option configures a CachingRepository at construction time.
type Option func(*CachingRepository)

// WithCatalogsTTL sets the time-to-live for cached Catalogs results.
// After the TTL elapses, the next Catalogs call re-fetches from backing.
// Defaults to 5 minutes if not set.
func WithCatalogsTTL(d time.Duration) Option {
	return func(c *CachingRepository) {
		c.catalogsTTL = d
	}
}

// WithClock replaces the real-time clock used for TTL calculations. Useful in
// tests to advance time deterministically.
func WithClock(fn func() time.Time) Option {
	return func(c *CachingRepository) {
		c.clock = fn
	}
}

// WithRefreshInterval sets the polling interval for the background refresh
// goroutine started by Start. Defaults to 60 seconds if not set.
func WithRefreshInterval(d time.Duration) Option {
	return func(c *CachingRepository) {
		c.refreshInterval = d
	}
}

// WithVCStatusFunc injects the function used by the background goroutine to
// poll for VCS state changes. The function should return a hash string that
// changes whenever the backing store's issue data has changed (e.g. a Dolt
// commit hash from bd vc status).
//
// When nil (the default), Start is a no-op and no background polling occurs.
// Inject a real implementation (e.g. beads.VCStatusHash) via application
// wiring; inject a deterministic fake in tests.
func WithVCStatusFunc(fn func(ctx context.Context) (string, error)) Option {
	return func(c *CachingRepository) {
		c.vcStatusFunc = fn
	}
}

// WithSaveInterval sets the interval between periodic saves performed by the
// background goroutine. Defaults to 30 seconds if not set.
//
// Note: periodic save only fires when the background goroutine is running
// (i.e. Start has been called with a non-nil vcStatusFunc). Use SaveNow for
// explicit shutdown saves regardless of goroutine state.
func WithSaveInterval(d time.Duration) Option {
	return func(c *CachingRepository) {
		c.saveInterval = d
	}
}
