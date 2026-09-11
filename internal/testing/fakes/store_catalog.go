package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// FakeStoreCatalog is a deterministic storecatalog.Catalog test seam. Every
// test touching the store picker takes one: the real catalog reads the central
// registry of whoever runs the suite, so an assertion against it would depend
// on the machine.
type FakeStoreCatalog struct {
	mu sync.Mutex

	Entries []storecatalog.Entry
	Err     error

	calls int
}

var _ storecatalog.Catalog = (*FakeStoreCatalog)(nil)

// Stores returns the configured entries, or the configured error.
func (f *FakeStoreCatalog) Stores(_ context.Context) ([]storecatalog.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]storecatalog.Entry, len(f.Entries))
	copy(out, f.Entries)
	return out, nil
}

// Calls returns how many times Stores has been called. It takes the same lock
// the recording path does, so a test reading it while a command is still in
// flight cannot race.
func (f *FakeStoreCatalog) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}
