package repository

import "errors"

// ErrIssueNotFound is returned by local-state implementations (e.g. memory)
// when an issue ID is not present in the store. It is distinct from a
// *domain.RepositoryError wrapping domain.ErrorCodeCommandFailed, which is what
// taskmgr-backed implementations return for unknown identifiers.
var ErrIssueNotFound = errors.New("repository: issue not found")

// ErrSchemaMismatch is returned when a persisted file (snapshot, fixture, or
// on-disk cache) does not match the expected schema version. Callers should
// treat this as a permanent load failure for that file and report it to the
// operator.
var ErrSchemaMismatch = errors.New("repository: schema mismatch")
