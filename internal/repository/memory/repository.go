// Package memory provides a standalone in-memory implementation of
// repository.Repository. It is the canonical local-state backend for tests and
// offline scenarios.
//
// # Concurrency
//
// Repository uses a sync.RWMutex. All read methods (Dashboard, Issue, Search,
// Catalogs, HealthCheck) acquire a shared read lock; all write methods
// (CreateIssue, UpdateIssue, CloseIssue, AddComment) acquire the exclusive
// write lock. This satisfies the Repository interface's concurrency contract.
//
// # Seeding
//
// Tests populate the store through the typed seeders rather than through
// interface methods:
//
//	g := memory.New(memory.WithClock(staticClock), memory.WithIDGenerator(seqIDs))
//	g.Seed(memory.Issue{ID: "taskmgr-1", Title: "...", Status: "open", DependsOn: []string{"taskmgr-0"}})
//	g.SeedComments("taskmgr-1", memory.Comment{Author: "alice", Body: "..."})
//	g.SeedCatalogs(memory.DefaultCatalogs())
//
// # Error codes
//
// Issue() returns repository.ErrIssueNotFound for unknown IDs — this is the
// local-state carve-out documented in the interface godoc. UpdateIssue,
// CloseIssue, and AddComment return domain.RepositoryError{Code:
// ErrorCodeCommandFailed} to match taskmgr's observable behavior, as documented in
// the Repository interface.
package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// storedIssue is the internal representation of an issue in the memory store.
// All domain types use string IDs; no numeric surrogates.
type storedIssue struct {
	id          string
	title       string
	status      string
	priority    int
	issueType   string
	assignee    string
	creator     string
	labels      []string
	description string
	notes       string
	dependsOn   []string // IDs of issues this one is blocked by (BlockedBy)
	blocksIDs   []string // IDs of issues this one blocks (explicit override; empty = use reverse lookup)
	related     []string // IDs of related issues
	parentID    string   // parent group issue ID (empty if no parent)
	comments    []storedComment
	created     time.Time
	updated     time.Time
	closed      time.Time
	closeReason string
}

// storedComment is a comment record inside a storedIssue.
type storedComment struct {
	id        string
	author    string
	body      string
	createdAt time.Time
}

// Issue is the seeder input type for Seed. It mirrors storedIssue fields but
// is exported so callers outside this package can build values conveniently.
type Issue struct {
	ID          string
	Title       string
	Status      string // defaults to "open" when empty
	Priority    int
	Type        string // defaults to "task" when empty
	Assignee    string
	Labels      []string
	Description string
	Notes       string
	DependsOn   []string // IDs of issues this one is blocked by (BlockedBy)
	BlocksIDs   []string // IDs of issues this one blocks (explicit; empty = use reverse lookup)
	Related     []string // IDs of related issues
	ParentID    string   // parent group issue ID (empty if no parent)
	Created     time.Time
	Updated     time.Time
}

// Comment is the seeder input type for SeedComments.
type Comment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
}

// Repository is the in-memory implementation of repository.Repository.
type Repository struct {
	mu       sync.RWMutex
	issues   map[string]*storedIssue
	catalogs repository.Catalogs
	clock    func() time.Time
	idgen    func() string
}

var _ repository.Repository = (*Repository)(nil)

// New creates a new empty Repository. Apply Option values to customise clock
// and ID generation.
func New(opts ...Option) *Repository {
	r := &Repository{
		issues:   make(map[string]*storedIssue),
		catalogs: DefaultCatalogs(),
		clock:    time.Now,
		idgen:    defaultIDGenerator(),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Seed inserts or replaces an issue in the store. It is intended for test
// setup; in production code, use CreateIssue. Calling Seed after the
// repository is being used concurrently is safe but the caller is responsible
// for memory ordering.
func (r *Repository) Seed(iss Issue) {
	now := r.clock()

	status := iss.Status
	if status == "" {
		status = "open"
	}

	issueType := iss.Type
	if issueType == "" {
		issueType = "task"
	}

	created := iss.Created
	if created.IsZero() {
		created = now
	}

	updated := iss.Updated
	if updated.IsZero() {
		updated = created
	}

	labels := make([]string, len(iss.Labels))
	copy(labels, iss.Labels)

	deps := make([]string, len(iss.DependsOn))
	copy(deps, iss.DependsOn)

	related := make([]string, len(iss.Related))
	copy(related, iss.Related)

	blocksIDs := make([]string, len(iss.BlocksIDs))
	copy(blocksIDs, iss.BlocksIDs)

	si := &storedIssue{
		id:          iss.ID,
		title:       iss.Title,
		status:      status,
		priority:    iss.Priority,
		issueType:   issueType,
		assignee:    iss.Assignee,
		labels:      labels,
		description: iss.Description,
		notes:       iss.Notes,
		dependsOn:   deps,
		blocksIDs:   blocksIDs,
		related:     related,
		parentID:    iss.ParentID,
		created:     created,
		updated:     updated,
	}

	r.mu.Lock()
	r.issues[iss.ID] = si
	r.mu.Unlock()
}

// SeedComments appends one or more comments to an already-seeded issue.
// Panics if the issue does not exist.
func (r *Repository) SeedComments(issueID string, comments ...Comment) {
	now := r.clock()

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[issueID]
	if !ok {
		panic(fmt.Sprintf("memory.Repository.SeedComments: issue %q not found", issueID))
	}

	for _, c := range comments {
		ts := c.CreatedAt
		if ts.IsZero() {
			ts = now
		}
		id := c.ID
		if id == "" {
			id = r.idgen()
		}
		si.comments = append(si.comments, storedComment{
			id:        id,
			author:    c.Author,
			body:      c.Body,
			createdAt: ts,
		})
	}
}

// SeedClosed sets the closed timestamp and close reason on an already-seeded
// issue. It is used by [repository.Load] to restore closed state that Seed
// does not accept directly. Panics if the issue does not exist.
func (r *Repository) SeedClosed(issueID string, closedAt time.Time, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	si, ok := r.issues[issueID]
	if !ok {
		panic("memory.Repository.SeedClosed: issue " + issueID + " not found")
	}
	si.closed = closedAt
	if reason != "" {
		si.closeReason = reason
	}
}

// SeedCatalogs replaces the catalog data returned by Catalogs(). If not
// called, DefaultCatalogs() is used.
func (r *Repository) SeedCatalogs(c repository.Catalogs) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.catalogs = c
}

// DefaultCatalogs returns the taskmgr 1.0.4 standard statuses, core types, and an
// empty label list. Tests that need labels should call SeedCatalogs with
// additional LabelOption values.
func DefaultCatalogs() repository.Catalogs {
	return repository.Catalogs{
		Statuses: []domain.StatusOption{
			{Name: "open", Description: "Available to work (default)"},
			{Name: "in_progress", Description: "Actively being worked on"},
			{Name: "blocked", Description: "Blocked by a dependency"},
			{Name: "deferred", Description: "Deliberately put on ice for later"},
			{Name: "closed", Description: "Completed"},
			{Name: "pinned", Description: "Pinned for visibility"},
			{Name: "hooked", Description: "Hooked — waiting on an external trigger"},
		},
		Types: []domain.TypeOption{
			{Name: "task", Description: "General work item (default)"},
			{Name: "bug", Description: "Bug report or defect"},
			{Name: "feature", Description: "New feature or enhancement"},
			{Name: "chore", Description: "Maintenance or housekeeping"},
			{Name: "epic", Description: "Large body of work spanning multiple issues"},
			{Name: "decision", Description: "Architectural or design decision"},
			{Name: "spike", Description: "Time-boxed research or investigation"},
			{Name: "story", Description: "User story"},
			{Name: "milestone", Description: "Project milestone"},
		},
		Labels: []domain.LabelOption{},
	}
}
