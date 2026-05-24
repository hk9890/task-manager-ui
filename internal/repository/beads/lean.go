// Package beads — lean Repository implementation.
//
// This file defines the lean [Repository] struct and its constructor. The lean
// Repository implements [repository.Repository] directly on
// [bdrunner.CommandRunner], with no intermediate BeadsGateway or Gateway type.
//
// File layout:
//
//   - lean.go           — type, New, parent-sibling cache helper, small utilities
//   - lean_reads.go     — Dashboard, Issue, Search, HealthCheck, Catalogs
//   - lean_writes.go    — CreateIssue, UpdateIssue, CloseIssue, AddComment
//   - lean_payloads.go  — package-private JSON DTOs and scalar helpers
//
// The legacy gateway-backed adapter (repository.go, NewFromGateway) is preserved
// until E4.3 switches app wiring and E4.4 deletes the legacy code.
package beads

import (
	"context"
	"strings"
	"sync"

	"github.com/hk9890/beads-workbench/internal/domain"
	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// runFn is the function signature used by the internal run chokepoint and
// helpers that need to be testable via [WithCommandHook].
type runFn func(ctx context.Context, req bdrunner.CommandRequest) ([]byte, error)

// Repository is the lean, CommandRunner-backed implementation of
// [repository.Repository]. Construct with [New]; do not create a zero value
// directly.
//
// The lean implementation is introduced alongside the legacy gateway-backed
// adapter (NewFromGateway) so both compile while the app wiring switch happens
// in E4.3.
type Repository struct {
	runner *bdrunner.CommandRunner

	// hook is an optional test-only command interception function installed via
	// [WithCommandHook]. When non-nil every call to r.run goes through hook
	// instead of r.runner.Run. Production callers never set this field.
	hook runFn

	// parentSiblingCacheMu guards parentSiblingCache.
	parentSiblingCacheMu sync.RWMutex
	// parentSiblingCache stores the children list for a given parent issue ID,
	// keyed by parent ID. Populated lazily by parentChildSiblings; reused
	// across Issue calls within the same Repository instance so each unique
	// parent is fetched at most once.
	parentSiblingCache map[string][]domain.IssueReference
}

// Compile-time interface assertion.
var _ repository.Repository = (*Repository)(nil)

// New returns a lean Repository backed directly by runner.
// runner must be non-nil; passing nil will panic at the first method call.
// Optional [Option] values (e.g. [WithCommandHook]) customise the instance;
// see each option's documentation for details.
func New(runner *bdrunner.CommandRunner, opts ...Option) *Repository {
	r := &Repository{
		runner:             runner,
		parentSiblingCache: make(map[string][]domain.IssueReference),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// run is the single execution chokepoint for all bd command calls. When a
// [WithCommandHook] has been installed it delegates to the hook so tests can
// intercept or fail specific requests. In the default case (hook == nil) it
// calls r.runner.Run directly, preserving the runner's RW-lock and semaphore
// semantics unchanged.
func (r *Repository) run(ctx context.Context, req bdrunner.CommandRequest) ([]byte, error) {
	if r.hook != nil {
		return r.hook(ctx, req)
	}
	return r.runner.Run(ctx, req)
}

// repoRunJSON executes a command through the Repository's run chokepoint and
// decodes the JSON response into T. It is the Repository-local analogue of
// [bdrunner.RunJSON] but routes through r.run so that [WithCommandHook] is
// honoured.
func repoRunJSON[T any](ctx context.Context, r *Repository, req bdrunner.CommandRequest) (T, error) {
	var value T
	out, err := r.run(ctx, req)
	if err != nil {
		return value, err
	}
	if err := bdrunner.DecodeJSONInto(req.Operation, out, &value); err != nil {
		return value, err
	}
	return value, nil
}

// parentChildSiblings returns the children of parentID by running
// `bd show <parentID> --json` and filtering the dependents for
// dependency_type == "parent-child". Results are cached per Repository
// instance so each unique parent is fetched at most once.
func (r *Repository) parentChildSiblings(ctx context.Context, parentID string) ([]domain.IssueReference, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, nil
	}

	r.parentSiblingCacheMu.RLock()
	cached, hit := r.parentSiblingCache[parentID]
	r.parentSiblingCacheMu.RUnlock()
	if hit {
		return cached, nil
	}

	items, err := leanDecodeIssueArray(ctx, r.run, leanOpShowIssue, []string{"show", parentID, "--json"})
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}

	dependents := items[0].Dependents
	out := make([]domain.IssueReference, 0, len(dependents))
	for _, d := range dependents {
		if leanOptStr(d.DependencyType) != "parent-child" {
			continue
		}

		ref, err := leanToIssueRef(d, leanOpShowIssue)
		if err != nil {
			return nil, err
		}

		out = append(out, ref)
	}

	r.parentSiblingCacheMu.Lock()
	r.parentSiblingCache[parentID] = out
	r.parentSiblingCacheMu.Unlock()

	return out, nil
}

// leanMergeUniqueRefs merges reference slices, deduplicating by ID.
func leanMergeUniqueRefs(groups ...[]domain.IssueReference) []domain.IssueReference {
	seen := make(map[string]struct{})
	out := make([]domain.IssueReference, 0)
	for _, group := range groups {
		for _, ref := range group {
			if _, ok := seen[ref.ID]; ok {
				continue
			}
			seen[ref.ID] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}

// leanPaginate slices items[offset:offset+limit]. Both values are clamped to
// safe ranges; limit==0 means no cap.
func leanPaginate[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	page := items[offset:]
	if limit <= 0 {
		return page
	}
	if len(page) <= limit {
		return page
	}
	return page[:limit]
}

// leanWithOffsetWindow computes the bd --limit value needed to satisfy a
// (limit, offset) window: fetch limit+offset from bd, then slice off offset.
func leanWithOffsetWindow(limit, offset int) int {
	if limit <= 0 {
		return 0
	}
	if offset <= 0 {
		return limit
	}
	return limit + offset
}
