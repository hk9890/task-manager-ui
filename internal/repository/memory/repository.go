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
//	g.Seed(memory.Issue{ID: "bd-1", Title: "...", Status: "open", DependsOn: []string{"bd-0"}})
//	g.SeedComments("bd-1", memory.Comment{Author: "alice", Body: "..."})
//	g.SeedCatalogs(memory.DefaultCatalogs())
//
// # Error codes
//
// Issue() returns repository.ErrIssueNotFound for unknown IDs — this is the
// local-state carve-out documented in the interface godoc. UpdateIssue,
// CloseIssue, and AddComment return domain.RepositoryError{Code:
// ErrorCodeCommandFailed} to match bd's observable behavior, as documented in
// the Repository interface.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
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
	labels      []string
	description string
	notes       string
	dependsOn   []string // IDs of issues this one is blocked by (BlockedBy)
	blocksIDs   []string // IDs of issues this one blocks (explicit override; empty = use reverse lookup)
	related     []string // IDs of related issues
	parentID    string   // parent group issue ID (empty if no parent)
	childrenIDs []string // sibling/children IDs in the parent group
	comments    []storedComment
	created     time.Time
	updated     time.Time
	closed      time.Time
	closeReason string

	// Full IssueReference metadata for cross-ref projection, stored at SeedDetail
	// time so that toDetailLocked can return them verbatim without re-resolving
	// against r.issues. These are nil when the issue was seeded via Seed (not
	// SeedDetail); toDetailLocked falls back to re-resolution in that case.
	// The nil-vs-non-nil distinction is the sentinel: non-nil (even empty slice)
	// means "use verbatim"; nil means "re-resolve from memory map".
	//
	// Persistence: these fields are serialized into SnapshotIssue by Snapshot()
	// and restored by SeedFromSnapshot(), so they survive a Save+Load cycle.
	// Old on-disk JSONLs without these fields decode them as nil, which correctly
	// falls back to re-resolution (backward-compatible).
	blockedByRefs []domain.IssueReference // corresponds to dependsOn
	relatedRefs   []domain.IssueReference // corresponds to related
	parentRef     *domain.IssueReference  // corresponds to parentID (nil = re-resolve)
	childrenRefs  []domain.IssueReference // corresponds to childrenIDs
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
	ChildrenIDs []string // sibling/children IDs in the parent group
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

	childrenIDs := make([]string, len(iss.ChildrenIDs))
	copy(childrenIDs, iss.ChildrenIDs)

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
		childrenIDs: childrenIDs,
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

// DefaultCatalogs returns the bd 1.0.4 standard statuses, core types, and an
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

// ---- Repository interface methods ----

// Dashboard implements repository.Repository.
//
// ReadyExplain.Ready: issues where status is not "closed" and all DependsOn
// IDs point to closed issues (or DependsOn is empty).
// ReadyExplain.Blocked: issues where status is not "closed" and at least one
// DependsOn ID points to a non-closed issue.
// DashboardData.Blocked: issues where status == "blocked" (stored status).
// DashboardData.InProgress: issues where status == "in_progress".
// DashboardData.Closed: all closed issues, sorted by ClosedAt DESC.
func (r *Repository) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	if err := ctx.Err(); err != nil {
		return repository.DashboardData{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var ready []domain.IssueSummary
	var blockedExplain []domain.BlockedIssueView
	var inProgress []domain.IssueSummary
	var closed []domain.IssueSummary
	var blocked []domain.IssueSummary

	for _, si := range r.issues {
		sum := r.toSummaryLocked(si)

		switch si.status {
		case "closed":
			closed = append(closed, sum)
		case "in_progress":
			inProgress = append(inProgress, sum)
		case "blocked":
			blocked = append(blocked, sum)
		}

		// ReadyExplain logic mirrors bd ready --explain semantics:
		// - Ready: stored status == "open" AND all dep-IDs are closed (or no deps).
		// - Blocked: has at least one open dep, AND status is not "closed" (any
		//   non-closed stored status can be dep-blocked, matching bd's behaviour
		//   with bwf-2 status=blocked/has-dep → appears in Blocked).
		if si.status == "closed" {
			continue
		}

		allDepsClosed, openDeps := r.depStateLocked(si.dependsOn)

		if allDepsClosed && si.status == "open" {
			ready = append(ready, sum)
		} else if !allDepsClosed {
			// Build blocker references from open deps.
			blockerRefs := make([]domain.IssueReference, 0, len(openDeps))
			for _, depID := range openDeps {
				dep, ok := r.issues[depID]
				if !ok {
					// Dep not in store — treat as a forward reference, include by ID.
					blockerRefs = append(blockerRefs, domain.IssueReference{ID: depID})
					continue
				}
				blockerRefs = append(blockerRefs, domain.IssueReference{
					ID:       dep.id,
					Title:    dep.title,
					Type:     dep.issueType,
					Priority: dep.priority,
					Status:   dep.status,
				})
			}
			blockedExplain = append(blockedExplain, domain.BlockedIssueView{
				Issue:     sum,
				BlockedBy: blockerRefs,
			})
		}
	}

	// Sort closed DESC by ClosedAt.
	sort.Slice(closed, func(i, j int) bool {
		// Closed issues with a zero closed time sort last.
		ti := r.issues[closed[i].ID].closed
		tj := r.issues[closed[j].ID].closed
		return ti.After(tj)
	})

	re := domain.ReadyExplainResult{
		Ready:        ready,
		Blocked:      blockedExplain,
		TotalReady:   len(ready),
		TotalBlocked: len(blockedExplain),
	}

	if re.Ready == nil {
		re.Ready = []domain.IssueSummary{}
	}
	if re.Blocked == nil {
		re.Blocked = []domain.BlockedIssueView{}
	}

	if inProgress == nil {
		inProgress = []domain.IssueSummary{}
	}
	if closed == nil {
		closed = []domain.IssueSummary{}
	}
	if blocked == nil {
		blocked = []domain.IssueSummary{}
	}

	return repository.DashboardData{
		ReadyExplain: re,
		InProgress:   inProgress,
		Closed:       closed,
		ClosedTotal:  len(closed),
		Blocked:      blocked,
	}, nil
}

// Issue implements repository.Repository.
//
// Returns repository.ErrIssueNotFound for unknown IDs (local-state carve-out
// as documented in the Repository interface). The domain.RepositoryError path is
// reserved for bd-backed implementations.
func (r *Repository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	if err := ctx.Err(); err != nil {
		return domain.IssueDetail{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.IssueDetail{}, repository.ErrIssueNotFound
	}

	return r.toDetailLocked(si), nil
}

// Search implements repository.Repository.
//
// Text is matched case-insensitively across Title, Description, and Notes.
// Statuses, Types, Labels, and Assignee are intersection filters (AND
// semantics across fields; OR semantics within Labels). PriorityMin/Max bound
// priority (nil = unbounded). WorkState=ready and WorkState=blocked derive
// from dep-closure state (not stored status). Limit and Offset apply after all
// filters.
//
// The returned Metadata.Completeness is always SearchResultCompletenessExact
// because memory always returns the full result set.
func (r *Repository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if err := ctx.Err(); err != nil {
		return domain.SearchResultPage{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []domain.SearchResult

	for _, si := range r.issues {
		if !r.matchesSearchLocked(si, query) {
			continue
		}

		snippet := r.buildSnippet(si, query.Text)
		results = append(results, domain.SearchResult{
			Issue:   r.toSummaryLocked(si),
			Snippet: snippet,
		})
	}

	// Sort consistently: by ID for determinism (mirrors typical list behavior).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Issue.ID < results[j].Issue.ID
	})

	// Apply offset and limit.
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	} else if query.Offset >= len(results) {
		results = nil
	}

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	if results == nil {
		results = []domain.SearchResult{}
	}

	return domain.SearchResultPage{
		Results: results,
		Metadata: domain.SearchResultMetadata{
			ReturnedCount:  len(results),
			RequestedLimit: query.Limit,
			Completeness:   domain.SearchResultCompletenessExact,
			Source:         domain.SearchResultSourceBDSearch,
		},
	}, nil
}

// CreateIssue implements repository.Repository.
//
// Returns domain.RepositoryError with ErrorCodeValidationFailed when Title is
// empty.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CreateIssueResult{}, err
	}

	if strings.TrimSpace(input.Title) == "" {
		return domain.CreateIssueResult{}, domain.RepositoryError{
			Code:      domain.ErrorCodeValidationFailed,
			Operation: "create issue",
			Message:   "title must not be empty",
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clock()
	id := r.idgen()

	issueType := input.Type
	if issueType == "" {
		issueType = "task"
	}

	priority := 0
	if input.Priority != nil {
		priority = *input.Priority
	}

	labels := make([]string, len(input.Labels))
	copy(labels, input.Labels)

	si := &storedIssue{
		id:          id,
		title:       input.Title,
		status:      "open",
		priority:    priority,
		issueType:   issueType,
		assignee:    input.Assignee,
		labels:      labels,
		description: input.Description,
		created:     now,
		updated:     now,
		comments:    []storedComment{},
	}

	r.issues[id] = si
	return domain.CreateIssueResult{IssueID: id}, nil
}

// UpdateIssue implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match bd's observable behavior, as documented in the Repository interface.
func (r *Repository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "update issue",
			Message:   fmt.Sprintf("command exited with code 1: Error resolving %q: no issue found", id),
		}
	}

	now := r.clock()

	if input.Title != nil {
		si.title = *input.Title
	}
	if input.Description != nil {
		si.description = *input.Description
	}
	if input.Status != nil {
		si.status = *input.Status
	}
	if input.Type != nil {
		si.issueType = *input.Type
	}
	if input.Priority != nil {
		si.priority = *input.Priority
	}
	if input.Assignee != nil {
		si.assignee = *input.Assignee
	}
	if input.ClearLabels {
		si.labels = []string{}
	} else if len(input.Labels) > 0 {
		si.labels = make([]string, len(input.Labels))
		copy(si.labels, input.Labels)
	}

	si.updated = now
	return nil
}

// CloseIssue implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match bd's observable behavior, as documented in the Repository interface.
func (r *Repository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "close issue",
			Message:   fmt.Sprintf("command exited with code 1: Error resolving %q: no issue found", id),
		}
	}

	now := r.clock()

	si.status = "closed"
	si.closed = now
	si.updated = now

	if input.Reason != "" {
		si.closeReason = input.Reason
	} else {
		si.closeReason = "Closed"
	}

	return nil
}

// AddComment implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match bd's observable behavior, as documented in the Repository interface.
func (r *Repository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "add comment",
			Message:   fmt.Sprintf("command exited with code 1: unknown issue %q", id),
		}
	}

	now := r.clock()

	si.comments = append(si.comments, storedComment{
		id:        r.idgen(),
		author:    "memory-user",
		body:      input.Body,
		createdAt: now,
	})
	si.updated = now
	return nil
}

// HealthCheck implements repository.Repository.
//
// Always returns nil — in-memory stores are always healthy.
func (r *Repository) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Catalogs implements repository.Repository.
func (r *Repository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	if err := ctx.Err(); err != nil {
		return repository.Catalogs{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.catalogs, nil
}

// SnapshotIssue is a read-only exported view of a storedIssue for use by
// file persistence (Save). It preserves all fields, including timestamps and
// comments, so a round-trip through Save/Load is lossless.
//
// # Cross-reference metadata
//
// BlockedByRefs, RelatedRefs, ParentRef, and ChildrenRefs carry the full
// IssueReference metadata (Title, Status, Type, Priority) that was stored at
// SeedDetail time. These fields follow the same nil-vs-non-nil sentinel as
// storedIssue:
//
//   - nil → refs were not populated at SeedDetail time; Load should call Seed
//     (re-resolution path) so that toDetailLocked falls back to the memory map.
//   - non-nil (even empty slice) → refs are authoritative; Load should call
//     SeedFromSnapshot so toDetailLocked returns them verbatim.
//
// JSON encoding: nil slice → null (or absent) → decodes back to nil.
// Non-nil empty slice → [] → decodes back to non-nil empty slice.
// Do NOT add omitempty to the slice fields; that would silently flip
// authoritative-empty into re-resolution.
type SnapshotIssue struct {
	ID          string
	Title       string
	Status      string
	Priority    int
	Type        string
	Assignee    string
	Labels      []string
	Description string
	Notes       string
	DependsOn   []string
	Related     []string
	ParentID    string
	ChildrenIDs []string
	Comments    []SnapshotComment
	Created     time.Time
	Updated     time.Time
	Closed      time.Time
	CloseReason string

	// Cross-reference metadata. Nil means "use re-resolution path on Load".
	// See type-level doc for the nil-vs-non-nil sentinel semantics.
	BlockedByRefs []domain.IssueReference // corresponds to DependsOn
	RelatedRefs   []domain.IssueReference // corresponds to Related
	ParentRef     *domain.IssueReference  // corresponds to ParentID (nil = re-resolve)
	ChildrenRefs  []domain.IssueReference // corresponds to ChildrenIDs
}

// SnapshotComment is the exported view of a storedComment used by Snapshot.
type SnapshotComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
}

// Snapshot returns a read-only copy of all issues in the store. The slice is
// safe to use after the call without holding any lock. It is used by
// [repository.Save] to serialize the in-memory store; callers outside the
// persistence layer should prefer the normal Repository interface.
func (r *Repository) Snapshot() []SnapshotIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]SnapshotIssue, 0, len(r.issues))
	for _, si := range r.issues {
		labels := make([]string, len(si.labels))
		copy(labels, si.labels)

		deps := make([]string, len(si.dependsOn))
		copy(deps, si.dependsOn)

		related := make([]string, len(si.related))
		copy(related, si.related)

		childrenIDs := make([]string, len(si.childrenIDs))
		copy(childrenIDs, si.childrenIDs)

		comments := make([]SnapshotComment, len(si.comments))
		for i, c := range si.comments {
			comments[i] = SnapshotComment{
				ID:        c.id,
				Author:    c.author,
				Body:      c.body,
				CreatedAt: c.createdAt,
			}
		}

		// Copy the cross-reference metadata preserving the nil sentinel.
		// nil means "re-resolve on Load"; non-nil (even empty) means "verbatim".
		// We must NOT use make(..., 0) for a nil source — that would flip nil
		// into non-nil and incorrectly activate the verbatim path on Load.
		var blockedByRefs []domain.IssueReference
		if si.blockedByRefs != nil {
			blockedByRefs = make([]domain.IssueReference, len(si.blockedByRefs))
			copy(blockedByRefs, si.blockedByRefs)
		}

		var relatedRefs []domain.IssueReference
		if si.relatedRefs != nil {
			relatedRefs = make([]domain.IssueReference, len(si.relatedRefs))
			copy(relatedRefs, si.relatedRefs)
		}

		var parentRef *domain.IssueReference
		if si.parentRef != nil {
			ref := *si.parentRef
			parentRef = &ref
		}

		var childrenRefs []domain.IssueReference
		if si.childrenRefs != nil {
			childrenRefs = make([]domain.IssueReference, len(si.childrenRefs))
			copy(childrenRefs, si.childrenRefs)
		}

		out = append(out, SnapshotIssue{
			ID:            si.id,
			Title:         si.title,
			Status:        si.status,
			Priority:      si.priority,
			Type:          si.issueType,
			Assignee:      si.assignee,
			Labels:        labels,
			Description:   si.description,
			Notes:         si.notes,
			DependsOn:     deps,
			Related:       related,
			ParentID:      si.parentID,
			ChildrenIDs:   childrenIDs,
			Comments:      comments,
			Created:       si.created,
			Updated:       si.updated,
			Closed:        si.closed,
			CloseReason:   si.closeReason,
			BlockedByRefs: blockedByRefs,
			RelatedRefs:   relatedRefs,
			ParentRef:     parentRef,
			ChildrenRefs:  childrenRefs,
		})
	}
	return out
}

// ---- internal helpers ----

// toSummaryLocked projects a storedIssue to domain.IssueSummary.
// Caller must hold at least RLock.
func (r *Repository) toSummaryLocked(si *storedIssue) domain.IssueSummary {
	labels := make([]string, len(si.labels))
	copy(labels, si.labels)
	return domain.IssueSummary{
		ID:        si.id,
		Title:     si.title,
		Status:    si.status,
		Type:      si.issueType,
		Priority:  si.priority,
		Assignee:  si.assignee,
		Labels:    labels,
		CreatedAt: si.created,
		UpdatedAt: si.updated,
	}
}

// toDetailLocked projects a storedIssue to domain.IssueDetail with resolved
// dep references. Caller must hold at least RLock.
//
// When blockedByRefs/relatedRefs/parentRef/childrenRefs are non-nil (set by
// SeedDetail), they are returned verbatim without re-resolving against r.issues.
// When those fields are nil (set by Seed), the classic re-resolution path is
// used instead. The nil-vs-non-nil distinction is the sentinel.
func (r *Repository) toDetailLocked(si *storedIssue) domain.IssueDetail {
	sum := r.toSummaryLocked(si)

	// Resolve DependsOn as BlockedBy references.
	// Use stored refs verbatim when available (SeedDetail path); otherwise
	// re-resolve from the in-memory map (Seed path).
	var blockedBy []domain.IssueReference
	if si.blockedByRefs != nil {
		blockedBy = make([]domain.IssueReference, len(si.blockedByRefs))
		copy(blockedBy, si.blockedByRefs)
	} else {
		blockedBy = make([]domain.IssueReference, 0, len(si.dependsOn))
		for _, depID := range si.dependsOn {
			dep, ok := r.issues[depID]
			if !ok {
				blockedBy = append(blockedBy, domain.IssueReference{ID: depID})
				continue
			}
			blockedBy = append(blockedBy, domain.IssueReference{
				ID:       dep.id,
				Title:    dep.title,
				Type:     dep.issueType,
				Priority: dep.priority,
				Status:   dep.status,
			})
		}
	}

	// Resolve Blocks: if blocksIDs is explicitly set, use it; otherwise fall
	// back to reverse-lookup (find issues whose dependsOn contains si.id).
	blocks := make([]domain.IssueReference, 0)
	if len(si.blocksIDs) > 0 {
		for _, blockedID := range si.blocksIDs {
			other, ok := r.issues[blockedID]
			if !ok {
				blocks = append(blocks, domain.IssueReference{ID: blockedID})
				continue
			}
			blocks = append(blocks, domain.IssueReference{
				ID:       other.id,
				Title:    other.title,
				Type:     other.issueType,
				Priority: other.priority,
				Status:   other.status,
			})
		}
	} else {
		for _, other := range r.issues {
			if other.id == si.id {
				continue
			}
			for _, depID := range other.dependsOn {
				if depID == si.id {
					blocks = append(blocks, domain.IssueReference{
						ID:       other.id,
						Title:    other.title,
						Type:     other.issueType,
						Priority: other.priority,
						Status:   other.status,
					})
					break
				}
			}
		}
	}

	// Project comments.
	comments := make([]domain.IssueComment, len(si.comments))
	for i, c := range si.comments {
		comments[i] = domain.IssueComment{
			ID:        c.id,
			Author:    c.author,
			Body:      c.body,
			CreatedAt: c.createdAt,
		}
	}

	// Resolve Related references.
	// Use stored refs verbatim when available (SeedDetail path); otherwise re-resolve.
	var related []domain.IssueReference
	if si.relatedRefs != nil {
		related = make([]domain.IssueReference, len(si.relatedRefs))
		copy(related, si.relatedRefs)
	} else {
		related = make([]domain.IssueReference, 0, len(si.related))
		for _, relID := range si.related {
			rel, ok := r.issues[relID]
			if !ok {
				related = append(related, domain.IssueReference{ID: relID})
				continue
			}
			related = append(related, domain.IssueReference{
				ID:       rel.id,
				Title:    rel.title,
				Type:     rel.issueType,
				Priority: rel.priority,
				Status:   rel.status,
			})
		}
	}

	// Resolve ParentGroupBrowserContext.
	// Use stored refs verbatim when available (SeedDetail path); otherwise re-resolve.
	var parentGroupBrowser domain.ParentGroupBrowserContext
	if si.parentRef != nil {
		// SeedDetail path: parentRef was stored verbatim.
		parentGroupBrowser.Parent = *si.parentRef
	} else if si.parentID != "" {
		// Seed path: re-resolve from memory map.
		parent, ok := r.issues[si.parentID]
		if ok {
			parentGroupBrowser.Parent = domain.IssueReference{
				ID:       parent.id,
				Title:    parent.title,
				Type:     parent.issueType,
				Priority: parent.priority,
				Status:   parent.status,
			}
		} else {
			parentGroupBrowser.Parent = domain.IssueReference{ID: si.parentID}
		}
	}
	var children []domain.IssueReference
	if si.childrenRefs != nil {
		// SeedDetail path: childrenRefs were stored verbatim.
		children = make([]domain.IssueReference, len(si.childrenRefs))
		copy(children, si.childrenRefs)
	} else {
		// Seed path: re-resolve from memory map.
		children = make([]domain.IssueReference, 0, len(si.childrenIDs))
		for _, childID := range si.childrenIDs {
			child, ok := r.issues[childID]
			if !ok {
				children = append(children, domain.IssueReference{ID: childID})
				continue
			}
			children = append(children, domain.IssueReference{
				ID:       child.id,
				Title:    child.title,
				Type:     child.issueType,
				Priority: child.priority,
				Status:   child.status,
			})
		}
	}
	parentGroupBrowser.Children = children

	return domain.IssueDetail{
		Summary:            sum,
		Description:        si.description,
		Notes:              si.notes,
		ClosedAt:           si.closed,
		CloseReason:        si.closeReason,
		BlockedBy:          blockedBy,
		Blocks:             blocks,
		Comments:           comments,
		Related:            related,
		ParentGroupBrowser: parentGroupBrowser,
	}
}

// depStateLocked checks whether all dependency IDs point to closed issues.
// Returns (true, nil) when all are closed; (false, openDeps) listing IDs of
// non-closed or unknown deps. Caller must hold at least RLock.
func (r *Repository) depStateLocked(dependsOn []string) (allClosed bool, openDeps []string) {
	if len(dependsOn) == 0 {
		return true, nil
	}

	for _, depID := range dependsOn {
		dep, ok := r.issues[depID]
		if !ok || dep.status != "closed" {
			openDeps = append(openDeps, depID)
		}
	}

	return len(openDeps) == 0, openDeps
}

// matchesSearchLocked reports whether si matches the given SearchIssuesQuery.
// Caller must hold at least RLock.
func (r *Repository) matchesSearchLocked(si *storedIssue, q domain.SearchIssuesQuery) bool {
	// Text filter: case-insensitive substring across Title, Description, Notes.
	if q.Text != "" {
		needle := strings.ToLower(q.Text)
		if !strings.Contains(strings.ToLower(si.title), needle) &&
			!strings.Contains(strings.ToLower(si.description), needle) &&
			!strings.Contains(strings.ToLower(si.notes), needle) {
			return false
		}
	}

	// Statuses filter: OR semantics within the list.
	if len(q.Statuses) > 0 {
		matched := false
		for _, s := range q.Statuses {
			if si.status == s {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Types filter: OR semantics within the list.
	if len(q.Types) > 0 {
		matched := false
		for _, t := range q.Types {
			if si.issueType == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Labels filter: issue must contain all requested labels.
	if len(q.Labels) > 0 {
		labelSet := make(map[string]struct{}, len(si.labels))
		for _, l := range si.labels {
			labelSet[l] = struct{}{}
		}
		for _, want := range q.Labels {
			if _, ok := labelSet[want]; !ok {
				return false
			}
		}
	}

	// Assignee filter: exact match.
	if q.Assignee != "" && si.assignee != q.Assignee {
		return false
	}

	// Priority bounds.
	if q.PriorityMin != nil && si.priority < *q.PriorityMin {
		return false
	}
	if q.PriorityMax != nil && si.priority > *q.PriorityMax {
		return false
	}

	// WorkState filter mirrors bd ready / bd blocked semantics:
	// - WorkStateReady: stored status == "open" AND all deps closed (matches bd ready).
	// - WorkStateBlocked: has at least one open dep AND status not "closed" (matches
	//   bd blocked, which returns dep-blocked issues regardless of stored status).
	if q.WorkState != domain.WorkStateAny {
		allDepsClosed, _ := r.depStateLocked(si.dependsOn)

		switch q.WorkState {
		case domain.WorkStateReady:
			// Ready requires literal status=open (bd ready excludes blocked, deferred, etc.)
			// AND no open dependency blockers.
			if si.status != "open" || !allDepsClosed {
				return false
			}
		case domain.WorkStateBlocked:
			// Blocked (dep-closure sense): has open deps AND status is not "closed".
			if si.status == "closed" || allDepsClosed {
				return false
			}
		}
	}

	return true
}

// buildSnippet produces a short snippet from the matched field. Returns the
// field where the needle was found; empty string when no match (shouldn't
// happen after matchesSearch returns true, but guards edge cases).
func (r *Repository) buildSnippet(si *storedIssue, text string) string {
	if text == "" {
		return ""
	}
	needle := strings.ToLower(text)
	if strings.Contains(strings.ToLower(si.title), needle) {
		return si.title
	}
	if strings.Contains(strings.ToLower(si.description), needle) {
		return si.description
	}
	if strings.Contains(strings.ToLower(si.notes), needle) {
		return si.notes
	}
	return ""
}
