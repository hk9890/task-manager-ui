// Package taskmgr implements storecatalog.Catalog over the task-manager SDK's
// central registry.
package taskmgr

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hk9890/task-manager/sdk/tasks"

	repositorytaskmgr "github.com/hk9890/task-manager-ui/internal/repository/taskmgr"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// Catalog reads and opens stores through the SDK.
type Catalog struct {
	// author is recorded as the creator of issues and the author of comments
	// written through a store this catalog opens, as it is for the store
	// resolved at startup.
	author string
}

// New returns a Catalog whose opened stores write as author. It holds no other
// state: the SDK reads the registry from disk on every call, so a listing is
// never served from a snapshot taken before the operator ran
// `taskmgr init --central` in another terminal.
func New(author string) Catalog { return Catalog{author: author} }

// Stores implements storecatalog.Catalog.
//
// tasks.Stores takes no context — it is a synchronous read of one small YAML
// file — so cancellation is honoured by checking before the call rather than
// during it.
func (c Catalog) Stores(ctx context.Context) ([]storecatalog.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := tasks.Stores()
	if err != nil {
		return nil, fmt.Errorf("failed to read the central store registry: %w", err)
	}

	out := make([]storecatalog.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, storecatalog.Entry{
			Name:        entry.Store,
			ProjectPath: entry.Path,
			StorePath:   entry.StorePath,
			Health:      convertHealth(entry.Health),
		})
	}
	return out, nil
}

// Open implements storecatalog.Catalog. It resolves by registry name the way
// --store-name does at startup, so a store opened from the picker and the same
// store named on the command line are the same store.
func (c Catalog) Open(ctx context.Context, name string) (storecatalog.Opened, error) {
	if err := ctx.Err(); err != nil {
		return storecatalog.Opened{}, err
	}

	store, info, err := tasks.Resolve(tasks.ResolveOptions{StoreName: name})
	if err != nil {
		return storecatalog.Opened{}, fmt.Errorf("failed to open central task-manager store %q: %w", name, err)
	}
	return c.opened(store, info), nil
}

func (c Catalog) opened(store *tasks.Store, info tasks.ResolveInfo) storecatalog.Opened {
	return storecatalog.Opened{
		Repo:        repositorytaskmgr.New(store, repositorytaskmgr.WithAuthor(c.author)),
		Name:        StoreName(info),
		ProjectPath: info.ProjectPath,
		StorePath:   info.StorePath,
	}
}

// StoreName is what the header calls a resolved store. A central store is its
// registry name, which is the name of its directory under the central root; a
// local store's directory is always .tasks, so it is named after its project.
func StoreName(info tasks.ResolveInfo) string {
	if info.Kind == tasks.ResolvedLocal {
		return filepath.Base(info.ProjectPath)
	}
	return filepath.Base(info.StorePath)
}

func convertHealth(health tasks.StoreHealth) storecatalog.Health {
	switch health {
	case tasks.StoreOK:
		return storecatalog.HealthOK
	case tasks.StoreDangling:
		return storecatalog.HealthDangling
	case tasks.StoreBroken:
		return storecatalog.HealthBroken
	default:
		return storecatalog.HealthUnknown
	}
}
