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

	// Created is what CreateLocal and CreateCentral return; CreateErr fails
	// them instead.
	Created   storecatalog.Opened
	CreateErr error

	calls   int
	opened  []string
	creates []StoreCreateCall
}

// StoreCreateCall records one CreateLocal or CreateCentral call. Name is empty
// for a local store.
type StoreCreateCall struct {
	Central bool
	Dir     string
	Name    string
	Prefix  string
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

// CreateLocal records the call and returns the configured result.
func (f *FakeStoreCatalog) CreateLocal(_ context.Context, dir, prefix string) (storecatalog.Opened, error) {
	return f.create(StoreCreateCall{Dir: dir, Prefix: prefix})
}

// CreateCentral records the call and returns the configured result.
func (f *FakeStoreCatalog) CreateCentral(_ context.Context, dir, name, prefix string) (storecatalog.Opened, error) {
	return f.create(StoreCreateCall{Central: true, Dir: dir, Name: name, Prefix: prefix})
}

func (f *FakeStoreCatalog) create(call StoreCreateCall) (storecatalog.Opened, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.creates = append(f.creates, call)
	if f.CreateErr != nil {
		return storecatalog.Opened{}, f.CreateErr
	}
	return f.Created, nil
}

// DerivePrefix returns "pfx", so a test can tell a prefilled prefix from one
// the operator typed.
func (f *FakeStoreCatalog) DerivePrefix(string) string { return "pfx" }

// Creates returns the create calls, in order.
func (f *FakeStoreCatalog) Creates() []StoreCreateCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]StoreCreateCall(nil), f.creates...)
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
