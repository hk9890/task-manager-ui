package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Method identifies a Repository interface method by name. Used with
// ErrorInjectingRepository.SetError to configure per-method injection.
type Method string

const (
	MethodDashboard   Method = "Dashboard"
	MethodIssue       Method = "Issue"
	MethodSearch      Method = "Search"
	MethodCreateIssue Method = "CreateIssue"
	MethodUpdateIssue Method = "UpdateIssue"
	MethodCloseIssue  Method = "CloseIssue"
	MethodAddComment  Method = "AddComment"
	MethodHealthCheck Method = "HealthCheck"
	MethodCatalogs    Method = "Catalogs"
)

// Call records a single invocation of a Repository method.
type Call struct {
	Method Method

	// IssueID is the issue the call targeted, or "" for methods that take no
	// issue ID (Dashboard, Search, CreateIssue, HealthCheck, Catalogs).
	IssueID string

	// Args is the operation's typed input - repository.DashboardOptions,
	// domain.SearchIssuesQuery, domain.CreateIssueInput, domain.UpdateIssueInput,
	// domain.CloseIssueInput, domain.AddCommentInput - or nil for the methods
	// that take none.
	//
	// Recording arguments is what stops each package writing its own capture
	// stub: four of them existed because this type carried only the method name.
	Args any
}

// ErrorInjectingRepository wraps any repository.Repository and allows tests to
// inject errors on a per-method basis. When an error is configured for a method
// that method returns the configured error immediately without delegating to the
// inner repository. When no error is configured the call is forwarded to inner
// unchanged.
//
// All methods are concurrency-safe (a single mutex guards both errs and calls).
//
// It lives in the fakes package so the failure-path test seam is shared across
// the app/board/search test suites without shipping in any product binary.
type ErrorInjectingRepository struct {
	mu    sync.Mutex
	inner repository.Repository
	errs  map[Method]error
	calls []Call
}

// NewErrorInjecting returns an ErrorInjectingRepository wrapping inner with no
// errors configured. Calls to inner are tracked; retrieve them with Calls.
func NewErrorInjecting(inner repository.Repository) *ErrorInjectingRepository {
	return &ErrorInjectingRepository{
		inner: inner,
		errs:  make(map[Method]error),
	}
}

// SetError configures method to return err on all future calls. Pass nil to
// clear a previously set error (restoring delegation to inner).
func (e *ErrorInjectingRepository) SetError(method Method, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil {
		delete(e.errs, method)
	} else {
		e.errs[method] = err
	}
}

// Calls returns a snapshot of all calls recorded so far, in order. Each
// element records only the method name; argument values are not captured.
func (e *ErrorInjectingRepository) Calls() []Call {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Call, len(e.calls))
	copy(out, e.calls)
	return out
}

func (e *ErrorInjectingRepository) record(m Method) {
	e.recordCall(Call{Method: m})
}

// recordArgs records a call together with its issue ID and typed input.
func (e *ErrorInjectingRepository) recordArgs(m Method, issueID string, args any) {
	e.recordCall(Call{Method: m, IssueID: issueID, Args: args})
}

func (e *ErrorInjectingRepository) recordCall(c Call) {
	e.calls = append(e.calls, c)
}

func (e *ErrorInjectingRepository) injected(m Method) error {
	return e.errs[m]
}

// Dashboard implements repository.Repository.
func (e *ErrorInjectingRepository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	e.mu.Lock()
	e.recordArgs(MethodDashboard, "", opts)
	err := e.injected(MethodDashboard)
	e.mu.Unlock()
	if err != nil {
		return repository.DashboardData{}, err
	}
	return e.inner.Dashboard(ctx, opts)
}

// Issue implements repository.Repository.
func (e *ErrorInjectingRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	e.mu.Lock()
	e.recordArgs(MethodIssue, id, nil)
	err := e.injected(MethodIssue)
	e.mu.Unlock()
	if err != nil {
		return domain.IssueDetail{}, err
	}
	return e.inner.Issue(ctx, id)
}

// Search implements repository.Repository.
func (e *ErrorInjectingRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	e.mu.Lock()
	e.recordArgs(MethodSearch, "", query)
	err := e.injected(MethodSearch)
	e.mu.Unlock()
	if err != nil {
		return domain.SearchResultPage{}, err
	}
	return e.inner.Search(ctx, query)
}

// CreateIssue implements repository.Repository.
func (e *ErrorInjectingRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	e.mu.Lock()
	e.recordArgs(MethodCreateIssue, "", input)
	err := e.injected(MethodCreateIssue)
	e.mu.Unlock()
	if err != nil {
		return domain.CreateIssueResult{}, err
	}
	return e.inner.CreateIssue(ctx, input)
}

// UpdateIssue implements repository.Repository.
func (e *ErrorInjectingRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	e.mu.Lock()
	e.recordArgs(MethodUpdateIssue, id, input)
	err := e.injected(MethodUpdateIssue)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	return e.inner.UpdateIssue(ctx, id, input)
}

// CloseIssue implements repository.Repository.
func (e *ErrorInjectingRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	e.mu.Lock()
	e.recordArgs(MethodCloseIssue, id, input)
	err := e.injected(MethodCloseIssue)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	return e.inner.CloseIssue(ctx, id, input)
}

// AddComment implements repository.Repository.
func (e *ErrorInjectingRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	e.mu.Lock()
	e.recordArgs(MethodAddComment, id, input)
	err := e.injected(MethodAddComment)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	return e.inner.AddComment(ctx, id, input)
}

// HealthCheck implements repository.Repository.
func (e *ErrorInjectingRepository) HealthCheck(ctx context.Context) error {
	e.mu.Lock()
	e.record(MethodHealthCheck)
	err := e.injected(MethodHealthCheck)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	return e.inner.HealthCheck(ctx)
}

// Catalogs implements repository.Repository.
func (e *ErrorInjectingRepository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	e.mu.Lock()
	e.record(MethodCatalogs)
	err := e.injected(MethodCatalogs)
	e.mu.Unlock()
	if err != nil {
		return repository.Catalogs{}, err
	}
	return e.inner.Catalogs(ctx)
}

// HasCall reports whether method appears anywhere in the recorded calls.
func (e *ErrorInjectingRepository) HasCall(method Method) bool {
	for _, c := range e.Calls() {
		if c.Method == method {
			return true
		}
	}
	return false
}

// HasCallSince reports whether method appears at or after index start. Take
// start from CallCount() before the action you want to measure.
func (e *ErrorInjectingRepository) HasCallSince(start int, method Method) bool {
	return e.CallCountSince(start, method) > 0
}

// CallCount is the number of calls recorded so far. It is the marker to pass to
// HasCallSince and CallCountSince.
func (e *ErrorInjectingRepository) CallCount() int { return len(e.Calls()) }

// CallCountSince counts calls to method recorded at or after index start.
func (e *ErrorInjectingRepository) CallCountSince(start int, method Method) int {
	all := e.Calls()
	if start < 0 {
		start = 0
	}
	n := 0
	for i := start; i < len(all); i++ {
		if all[i].Method == method {
			n++
		}
	}
	return n
}

// CallsFor returns every recorded call to method, in order. Read Call.Args for
// the operation's typed input — this is what replaced the per-package capture
// stubs.
func (e *ErrorInjectingRepository) CallsFor(method Method) []Call {
	var out []Call
	for _, c := range e.Calls() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// ErrorInjectingRepository is a repository.Repository. DelayingRepository asserts
// the same thing; this one relied on its users failing to compile instead.
var _ repository.Repository = (*ErrorInjectingRepository)(nil)
