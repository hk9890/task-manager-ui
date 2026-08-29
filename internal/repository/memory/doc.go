// Package memory provides a standalone in-memory implementation of
// repository.Repository. It is the canonical local-state backend for tests and
// offline scenarios.
//
// # Concurrency
//
// Repository uses a sync.RWMutex. All read methods (Dashboard, Issue, Search,
// Catalogs, HealthCheck) acquire a shared read lock; all write methods
// (CreateIssue, UpdateIssue, CloseIssue, AddComment) acquire the exclusive
// write lock. That is strictly stronger than the Repository interface
// guarantees — the taskmgr backend's reads are lock-free — so a test relying on
// read/write serialisation here proves nothing about production.
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
// Issue() returns repository.ErrIssueNotFound for unknown IDs; UpdateIssue,
// CloseIssue, and AddComment return domain.RepositoryError{Code:
// ErrorCodeCommandFailed}. Both match the Repository interface and the taskmgr
// backend.
package memory
