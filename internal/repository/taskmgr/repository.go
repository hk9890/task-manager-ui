package taskmgr

import (
	"time"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/repository"
)

// StoreLockTimeout is how long a write waits for the store's write lock while
// another writer holds it. Every store the app opens takes it through
// tasks.WithLockTimeout: when it runs out, the write fails as
// domain.ErrorCodeTimeout and the app does not freeze.
const StoreLockTimeout = 10 * time.Second

// Repository adapts a *tasks.Store to the repository.Repository interface.
type Repository struct {
	store  *tasks.Store
	author string // identity recorded as issue creator and comment author
}

// Option configures a Repository.
type Option func(*Repository)

// WithAuthor sets the identity recorded as the creator of new issues and the
// author of comments. Empty values are ignored (the default is kept).
func WithAuthor(author string) Option {
	return func(r *Repository) {
		if author != "" {
			r.author = author
		}
	}
}

// New wraps an already-open *tasks.Store. The caller owns the store's lifetime
// (Open/Init happen outside this package).
func New(store *tasks.Store, opts ...Option) *Repository {
	r := &Repository{store: store, author: "taskmgr-ui"}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

var _ repository.Repository = (*Repository)(nil)
