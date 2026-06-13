// Package taskmgr implements repository.Repository over an in-process
// task-manager store (github.com/hk9890/task-manager/sdk/tasks).
//
// It is the production backend for bwb: a thin adapter mapping the SDK's typed
// model onto bwb's domain types. The SDK serializes writes (in-process mutex +
// flock) and dedups cross-partition reads, so this wrapper holds no lock of its
// own; each method only pre-checks ctx cancellation before delegating.
//
// Error mapping mirrors the local-state contract used by the memory backend:
// Issue on an unknown ID returns repository.ErrIssueNotFound, while the write
// methods (UpdateIssue/CloseIssue/AddComment) return a domain.RepositoryError
// (by value, so errors.As matches) with domain.ErrorCodeCommandFailed to match
// the documented Repository interface behavior.
package taskmgr
