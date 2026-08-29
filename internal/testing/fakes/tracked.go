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
