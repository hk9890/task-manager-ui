package fakes

import (
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// TrackedRepository is a seedable repository that records every call.
//
// It composes the two halves a controller test almost always wants together: a
// memory.Repository to seed fixture data through the typed seeders, and an
// ErrorInjectingRepository wrapping it for call tracking and failure injection.
// It satisfies repository.Repository through the embedded wrapper.
//
// This bundle was hand-rolled in three packages under three names before it
// lived here, with a fourth package writing the composition inline.
type TrackedRepository struct {
	// Memory is the seeding half. Reach for it to call Seed, SeedComments,
	// SeedClosed and SeedCatalogs; drive the repository under test through the
	// embedded wrapper instead.
	Memory *memoryrepo.Repository

	*ErrorInjectingRepository
}

// NewTracked returns a TrackedRepository over an empty memory repository.
// Options are passed through to memory.New — WithClock and WithIDGenerator make
// timestamps and IDs deterministic.
func NewTracked(opts ...memoryrepo.Option) *TrackedRepository {
	repo := memoryrepo.New(opts...)
	return &TrackedRepository{
		Memory:                   repo,
		ErrorInjectingRepository: NewErrorInjecting(repo),
	}
}

// HasCall reports whether method appears anywhere in the recorded calls.
func (t *TrackedRepository) HasCall(method Method) bool {
	for _, c := range t.Calls() {
		if c.Method == method {
			return true
		}
	}
	return false
}

// HasCallSince reports whether method appears at or after index start. Take
// start from CallCount() before the action you want to measure.
func (t *TrackedRepository) HasCallSince(start int, method Method) bool {
	return t.CallCountSince(start, method) > 0
}

// CallCount is the number of calls recorded so far. It is the marker to pass to
// HasCallSince and CallCountSince.
func (t *TrackedRepository) CallCount() int { return len(t.Calls()) }

// CallCountSince counts calls to method recorded at or after index start.
func (t *TrackedRepository) CallCountSince(start int, method Method) int {
	all := t.Calls()
	if start < 0 {
		start = 0
	}
	n := 0
	for i := start; i < len(all); i++ {
		if all[i].Method == method {
			n++
		}
	}
	return n
}

// CallsFor returns every recorded call to method, in order. Read Call.Args for
// the operation's typed input — this is what replaced the per-package capture
// stubs.
func (t *TrackedRepository) CallsFor(method Method) []Call {
	var out []Call
	for _, c := range t.Calls() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// LastArgs returns the Args of the most recent call to method, and whether such
// a call exists.
func (t *TrackedRepository) LastArgs(method Method) (any, bool) {
	calls := t.CallsFor(method)
	if len(calls) == 0 {
		return nil, false
	}
	return calls[len(calls)-1].Args, true
}
