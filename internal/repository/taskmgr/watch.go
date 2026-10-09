package taskmgr

import (
	"context"

	"github.com/hk9890/task-manager-ui/internal/repository"
)

// WatchChanges reports every change to the store, from any process.
func (r *Repository) WatchChanges(ctx context.Context) (<-chan struct{}, error) {
	changes, err := r.store.Watch(ctx)
	if err != nil {
		return nil, mapReadErr("watch", err)
	}
	return changes, nil
}

var _ repository.ChangeWatcher = (*Repository)(nil)
