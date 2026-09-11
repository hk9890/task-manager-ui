// Package taskmgr implements storecatalog.Catalog over the task-manager SDK's
// central registry.
package taskmgr

import (
	"context"
	"fmt"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// Catalog reads the central registry through the SDK.
type Catalog struct{}

// New returns a Catalog. It holds no state: the SDK reads the registry from
// disk on every call, so a listing is never served from a snapshot taken before
// the operator ran `taskmgr init --central` in another terminal.
func New() Catalog { return Catalog{} }

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
