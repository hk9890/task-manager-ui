// Package storecatalog is the port for the central task-manager store registry:
// the machine-wide list of stores taskmgr-ui can browse.
//
// It is deliberately separate from repository.Repository, which reads and writes
// the issues of one already-open store. This package answers the question that
// comes before that one — which stores exist — and it is the seam that keeps the
// store picker testable without reading the registry of whoever runs the suite.
package storecatalog

import "context"

// Health classifies an entry's store directory. It carries the SDK's three
// cases rather than collapsing them into a boolean: the picker renders a broken
// entry differently from a dangling one, and hiding either would disagree with
// what `taskmgr store list` shows.
type Health string

const (
	// HealthOK marks a finished, usable store.
	HealthOK Health = "ok"
	// HealthDangling marks a registry entry whose store directory is absent.
	HealthDangling Health = "dangling"
	// HealthBroken marks a store directory holding no config.
	HealthBroken Health = "broken"
	// HealthUnknown marks a health value this build does not recognise, which
	// is what a newer SDK adding a fourth case looks like from here.
	HealthUnknown Health = "unknown"
)

// Usable reports whether an entry can be opened. Only HealthOK can.
func (h Health) Usable() bool { return h == HealthOK }

// Entry is one central-registry store.
type Entry struct {
	// Name is the registry name, unique across the machine.
	Name string
	// ProjectPath is the project directory the entry maps.
	ProjectPath string
	// StorePath is the store directory itself, under the central root.
	StorePath string
	Health    Health
}

// Catalog lists the stores this machine knows about.
//
// Local stores are absent by design: nothing indexes the .tasks directories on
// a machine, and this app does not walk the filesystem looking for them. A
// local store is reached by starting taskmgr-ui inside its project.
type Catalog interface {
	// Stores returns every central-registry entry, in the order the registry
	// reports them. An absent registry is an empty slice, not an error.
	//
	// The listing is all-or-nothing: the SDK abandons it when a single entry's
	// store directory cannot be stat'd, so one unreadable directory costs every
	// other store on the machine. That is the SDK's behaviour and this port
	// reports it rather than working around it (docs/OVERVIEW.md).
	Stores(ctx context.Context) ([]Entry, error)
}
