// Package repository defines the Repository interface and its associated
// composite types. Implementations live in sub-packages (beads/, memory/);
// this package contains only the interface, composite types, and error
// sentinels.
package repository

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// Repository is the central read/write abstraction for beads issue data.
// Callers use this interface without knowing whether the backing store is a
// live bd process, an in-memory fixture, or a file snapshot.
//
// # Concurrency safety
//
// All methods are safe for concurrent use by multiple Bubble Tea goroutines.
// Read methods (Dashboard, Issue, Search, Catalogs, HealthCheck) may execute
// concurrently with other reads. Write methods (CreateIssue, UpdateIssue,
// CloseIssue, AddComment) serialize against all other operations at the
// implementation level; callers need not add external locks.
//
// # Cancellation semantics
//
// Every method accepts a context.Context. When the context is cancelled before
// or during execution the method returns ctx.Err() promptly. Dashboard fan-out
// cancels any in-flight underlying calls as soon as one cancellation is
// observed.
//
// # Error code stability
//
// Implementations return errors as *domain.GatewayError where a typed code is
// meaningful. Documented error codes per method are part of the stable API;
// callers may switch on domain.ErrorCode* values. Unknown-ID methods (Issue,
// UpdateIssue, CloseIssue, AddComment) wrap domain.ErrorCodeCommandFailed to
// match bd's behavior. Missing database conditions return
// domain.ErrorCodeNoDatabaseFound. Sentinel errors ErrIssueNotFound and
// ErrSchemaMismatch are returned for the pure local-state cases they document.
type Repository interface {
	// Dashboard returns a composite snapshot of the board state: ready-explain
	// data, in-progress issues, recently closed issues, a closed-total count,
	// and currently blocked issues.
	//
	// Partial-failure shape: Dashboard fans out across multiple underlying
	// calls. If any of those calls fails the whole method returns an error;
	// no partial result is returned. Callers must treat Dashboard as atomic —
	// do not expect "best-effort" partial composition.
	//
	// On context cancellation the fan-out is abandoned and ctx.Err() is
	// returned promptly.
	Dashboard(ctx context.Context) (DashboardData, error)

	// Issue returns the full detail model for the issue identified by id.
	//
	// If id does not identify a known issue the implementation returns a
	// *domain.GatewayError with Code == domain.ErrorCodeCommandFailed, matching
	// bd's failure behavior for unknown identifiers.
	// ErrIssueNotFound is returned only by local-state paths (e.g. memory
	// implementations) that can distinguish "never existed" from a bd error.
	Issue(ctx context.Context, id string) (domain.IssueDetail, error)

	// Search executes a text and structured search against the issue store and
	// returns a paged result set.
	//
	// Unknown filter field values (e.g. unrecognised status strings) are
	// forwarded to the backing store without validation; the implementation
	// surfaces any resulting error as *domain.GatewayError.
	Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error)

	// CreateIssue creates a new issue and returns the assigned ID.
	//
	// Validation failures (e.g. empty title) are returned as
	// *domain.GatewayError with Code == domain.ErrorCodeValidationFailed.
	CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error)

	// UpdateIssue applies a partial update to the issue identified by id.
	// Only non-nil fields in input are applied; nil fields are left unchanged.
	//
	// If id is not found the implementation returns *domain.GatewayError with
	// Code == domain.ErrorCodeCommandFailed to match bd's behavior.
	UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error

	// CloseIssue closes the issue identified by id with the supplied reason.
	//
	// If id is not found the implementation returns *domain.GatewayError with
	// Code == domain.ErrorCodeCommandFailed to match bd's behavior.
	CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error

	// AddComment appends a comment to the issue identified by id.
	//
	// If id is not found the implementation returns *domain.GatewayError with
	// Code == domain.ErrorCodeCommandFailed to match bd's behavior.
	AddComment(ctx context.Context, id string, input domain.AddCommentInput) error

	// HealthCheck verifies that the backing store is reachable and usable.
	// A nil return means the store is healthy. A non-nil return wraps a
	// *domain.GatewayError; Code == domain.ErrorCodeNoDatabaseFound indicates
	// a missing or inaccessible database, Code == domain.ErrorCodeCommandUnavailable
	// indicates the bd CLI is not installed or not executable.
	HealthCheck(ctx context.Context) error

	// Catalogs returns the available status, type, and label options for use
	// in create/edit forms.
	//
	// Catalog freshness: results reflect the backing store at call time but may
	// be stale relative to labels or types added externally mid-session.
	// Callers should fetch Catalogs once at form-open time (or at startup) and
	// cache the result for the duration of the session. Do not call Catalogs
	// per-edit-form-open expecting fresh data — this is a documented limitation
	// that matches today's bd behavior.
	Catalogs(ctx context.Context) (Catalogs, error)
}
