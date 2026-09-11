package fakes

import (
	"context"
	"fmt"
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

	// Opens maps a registry name to what Open returns for it. A name absent
	// here fails Open, the way an unregistered name fails the SDK.
	Opens   map[string]storecatalog.Opened
	OpenErr error

	calls  int
	opened []string
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

// Open returns the configured store for name and records the call.
func (f *FakeStoreCatalog) Open(_ context.Context, name string) (storecatalog.Opened, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.opened = append(f.opened, name)
	if f.OpenErr != nil {
		return storecatalog.Opened{}, f.OpenErr
	}
	opened, ok := f.Opens[name]
	if !ok {
		return storecatalog.Opened{}, fmt.Errorf("no central store is registered as %q", name)
	}
	return opened, nil
}

// Calls returns how many times Stores has been called. It takes the same lock
// the recording path does, so a test reading it while a command is still in
// flight cannot race.
func (f *FakeStoreCatalog) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// Opened returns the names Open was called with, in order.
func (f *FakeStoreCatalog) Opened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.opened...)
}
