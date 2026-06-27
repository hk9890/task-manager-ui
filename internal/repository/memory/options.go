package memory

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Option configures a memory Repository at construction time.
type Option func(*Repository)

// WithClock replaces the real-time clock used for Created/Updated/Closed
// timestamps. Useful in tests to produce deterministic timestamps.
func WithClock(fn func() time.Time) Option {
	return func(r *Repository) {
		r.clock = fn
	}
}

// WithIDGenerator replaces the default ID generator. The supplied function
// must return a unique, non-empty string on each call. Useful in tests to
// produce deterministic IDs.
func WithIDGenerator(fn func() string) Option {
	return func(r *Repository) {
		r.idgen = fn
	}
}

// defaultIDGenerator returns a counter-based generator that produces IDs
// in the form "mem-1", "mem-2", ... using an atomic counter. Safe for
// concurrent use.
func defaultIDGenerator() func() string {
	var counter atomic.Int64
	return func() string {
		n := counter.Add(1)
		return fmt.Sprintf("mem-%d", n)
	}
}
