package fakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
)

// GatewayMethod identifies one BeadsGateway method for deterministic error injection.
type GatewayMethod string

const (
	MethodHealthCheck   GatewayMethod = "HealthCheck"
	MethodListIssues    GatewayMethod = "ListIssues"
	MethodReadyIssues   GatewayMethod = "ReadyIssues"
	MethodBlockedIssues GatewayMethod = "BlockedIssues"
	MethodShowIssue     GatewayMethod = "ShowIssue"
	MethodSearchIssues  GatewayMethod = "SearchIssues"

	MethodQuery        GatewayMethod = "Query"
	MethodReadyExplain GatewayMethod = "ReadyExplain"

	MethodCountIssues GatewayMethod = "CountIssues"

	MethodCreateIssue GatewayMethod = "CreateIssue"
	MethodUpdateIssue GatewayMethod = "UpdateIssue"
	MethodCloseIssue  GatewayMethod = "CloseIssue"
	MethodAddComment  GatewayMethod = "AddComment"

	MethodStatusCatalog GatewayMethod = "StatusCatalog"
	MethodTypeCatalog   GatewayMethod = "TypeCatalog"
	MethodLabelCatalog  GatewayMethod = "LabelCatalog"
)

// GatewayCall captures one invocation against the fake gateway.
type GatewayCall struct {
	Method GatewayMethod
	Input  any
}

type ListIssuesCall struct {
	Query domain.IssueListQuery
}

type ReadyIssuesCall struct {
	Query domain.ReadyIssuesQuery
}

type BlockedIssuesCall struct {
	Query domain.BlockedIssuesQuery
}

type ShowIssueCall struct {
	Query domain.ShowIssueQuery
}

type SearchIssuesCall struct {
	Query domain.SearchIssuesQuery
}

type QueryCall struct {
	Expr string
	Opts domain.QueryOptions
}

type ReadyExplainCall struct {
	Opts domain.ReadyExplainOptions
}

type CountIssuesCall struct {
	Query domain.IssueCountQuery
}

type CreateIssueCall struct {
	Input domain.CreateIssueInput
}

type UpdateIssueCall struct {
	IssueID string
	Input   domain.UpdateIssueInput
}

type CloseIssueCall struct {
	IssueID string
	Input   domain.CloseIssueInput
}

type AddCommentCall struct {
	IssueID string
	Input   domain.AddCommentInput
}

// FakeBeadsGateway is a deterministic BeadsGateway test double for UI tests.
//
// It supports:
//   - fixed return payloads for each gateway method,
//   - per-method error injection for error-path testing,
//   - call recording so tests can assert interactions.
//
// Write-state store: CreateIssue, UpdateIssue, CloseIssue, and AddComment mutate
// an internal in-memory issue store (issueStore). ShowIssue reads from that store
// as a secondary source, falling through to ShowIssuesByID (keyed overrides) and
// finally ShowIssueResponse (verbatim fallback for legacy UI tests). This means
// CreateIssue/UpdateIssue/CloseIssue/AddComment are observable through ShowIssue
// without any wrapper, satisfying RunWriteContract.
//
//   - CreateIssue validates that Title is non-empty (returns ErrorCodeCommandFailed
//     for an empty title, matching real bd behaviour) and assigns a unique
//     "tmp-<n>" ID, storing the issue in the write-state store.
//   - UpdateIssue returns ErrorCodeCommandFailed for an ID absent from the store.
//   - CloseIssue sets Status="closed" in the store; idempotent.
//   - AddComment appends the comment body to the store entry.
//   - CountIssues counts live from the write-state store when the store is
//     non-empty; otherwise falls back to CountIssuesResponse (static stub).
//
// ShowIssuesByID is an optional ID-keyed map for ShowIssue. When set, ShowIssue
// looks up the requested ID in the map and returns a domain.GatewayError with
// ErrorCodeCommandFailed when the ID is absent (matching real bd behaviour).
// When nil, ShowIssueResponse is returned for every ShowIssue call (legacy
// behaviour, preserved for UI tests).
//
// Priority for ShowIssue: MethodErrors → issueStore → ShowIssuesByID → ShowIssueResponse.
//
// SearchResultsByText is an optional text-keyed map for SearchIssues. When set,
// SearchIssues looks up by query.Text (trimmed) and returns the matching page —
// or an empty page if the key is absent. This lets contract tests verify
// text-filtered search without breaking UI tests that use SearchIssuesResponse
// as a verbatim stub.
type FakeBeadsGateway struct {
	mu           sync.Mutex
	issueStore   map[string]domain.IssueDetail
	issueCounter atomic.Int64

	ListIssuesResponse    []domain.IssueSummary
	ReadyIssuesResponse   []domain.IssueSummary
	BlockedIssuesResponse []domain.BlockedIssueView
	ShowIssueResponse     domain.IssueDetail
	ShowIssuesByID        map[string]domain.IssueDetail
	SearchIssuesResponse  domain.SearchResultPage
	SearchResultsByText   map[string]domain.SearchResultPage
	QueryResponse         []domain.IssueSummary
	// QueryResponsesByExpr is an optional expr-keyed map for Query. When set,
	// Query looks up the expr in this map and returns the matching slice. If
	// the key is absent the method falls back to QueryResponse (verbatim stub
	// behaviour). This lets contract tests verify expression-filtered queries
	// without breaking UI tests that use QueryResponse as a simple stub.
	QueryResponsesByExpr map[string][]domain.IssueSummary
	ReadyExplainResponse domain.ReadyExplainResult
	CountIssuesResponse  domain.IssueCountResult

	CreateIssueResponse domain.CreateIssueResult

	StatusCatalogResponse []domain.StatusOption
	TypeCatalogResponse   []domain.TypeOption
	LabelCatalogResponse  []domain.LabelOption

	MethodErrors map[GatewayMethod]error
	Calls        []GatewayCall
}

var _ beads.BeadsGateway = (*FakeBeadsGateway)(nil)

// NewFakeBeadsGateway creates a fake gateway with deterministic defaults.
func NewFakeBeadsGateway() *FakeBeadsGateway {
	return &FakeBeadsGateway{
		MethodErrors: make(map[GatewayMethod]error),
		issueStore:   make(map[string]domain.IssueDetail),
	}
}

// SeedIssue inserts a pre-built IssueDetail directly into the write-state store.
// Use this when a test needs a pre-existing issue that write operations (UpdateIssue,
// CloseIssue, AddComment) can target without calling CreateIssue first.
// The issue is also visible via ShowIssue at the seeded Summary.ID.
func (f *FakeBeadsGateway) SeedIssue(detail domain.IssueDetail) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.issueStore[detail.Summary.ID] = detail
}

// SetError injects or clears the error returned by a given gateway method.
func (f *FakeBeadsGateway) SetError(method GatewayMethod, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err == nil {
		delete(f.MethodErrors, method)
		return
	}

	f.MethodErrors[method] = err
}

// ResetCalls clears recorded gateway calls while keeping configured responses.
func (f *FakeBeadsGateway) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = nil
}

// HasCall reports whether a method was called.
func (f *FakeBeadsGateway) HasCall(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, c := range f.Calls {
		if string(c.Method) == method {
			return true
		}
	}

	return false
}

func (f *FakeBeadsGateway) HealthCheck(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodHealthCheck})
	return f.MethodErrors[MethodHealthCheck]
}

func (f *FakeBeadsGateway) ListIssues(_ context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodListIssues, Input: ListIssuesCall{Query: query}})
	if err := f.MethodErrors[MethodListIssues]; err != nil {
		return nil, err
	}

	out := append([]domain.IssueSummary(nil), f.ListIssuesResponse...)

	// Mirror real bd list behaviour: filter by status when statuses are specified.
	if len(query.Statuses) > 0 {
		filtered := out[:0]
		for _, s := range out {
			for _, status := range query.Statuses {
				if s.Status == status {
					filtered = append(filtered, s)
					break
				}
			}
		}
		out = filtered
	}

	return out, nil
}

func (f *FakeBeadsGateway) ReadyIssues(_ context.Context, query domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodReadyIssues, Input: ReadyIssuesCall{Query: query}})
	if err := f.MethodErrors[MethodReadyIssues]; err != nil {
		return nil, err
	}

	return append([]domain.IssueSummary(nil), f.ReadyIssuesResponse...), nil
}

func (f *FakeBeadsGateway) BlockedIssues(_ context.Context, query domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodBlockedIssues, Input: BlockedIssuesCall{Query: query}})
	if err := f.MethodErrors[MethodBlockedIssues]; err != nil {
		return nil, err
	}

	return append([]domain.BlockedIssueView(nil), f.BlockedIssuesResponse...), nil
}

func (f *FakeBeadsGateway) ShowIssue(_ context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodShowIssue, Input: ShowIssueCall{Query: query}})
	if err := f.MethodErrors[MethodShowIssue]; err != nil {
		return domain.IssueDetail{}, err
	}

	// Priority 1: write-state store — populated by CreateIssue/UpdateIssue/CloseIssue/AddComment.
	if detail, ok := f.issueStore[query.IssueID]; ok {
		return detail, nil
	}

	// Priority 2: ShowIssuesByID keyed overrides — used by contract tests that seed
	// canned issue states without going through the write path.
	if f.ShowIssuesByID != nil {
		detail, ok := f.ShowIssuesByID[query.IssueID]
		if !ok {
			// Mirror real bd behaviour: bd show <unknown-id> exits non-zero, which
			// the real gateway maps to ErrorCodeCommandFailed (not ErrorCodeNotFound).
			return domain.IssueDetail{}, domain.GatewayError{
				Code:      domain.ErrorCodeCommandFailed,
				Operation: "show issue",
				Message:   "command exited with code 1: no issue found matching \"" + query.IssueID + "\"",
			}
		}

		return detail, nil
	}

	// Priority 3: verbatim fallback for UI tests that use ShowIssueResponse.
	return f.ShowIssueResponse, nil
}

func (f *FakeBeadsGateway) SearchIssues(_ context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodSearchIssues, Input: SearchIssuesCall{Query: query}})
	if err := f.MethodErrors[MethodSearchIssues]; err != nil {
		return domain.SearchResultPage{}, err
	}

	// SearchResultsByText opt-in: when set, look up by exact (trimmed) text key.
	// This mirrors real gateway text-filter behaviour for contract tests without
	// breaking UI tests that use SearchIssuesResponse as a verbatim stub.
	if f.SearchResultsByText != nil {
		text := strings.TrimSpace(query.Text)
		page, ok := f.SearchResultsByText[text]
		if !ok {
			return domain.SearchResultPage{
				Results:  nil,
				Metadata: domain.SearchResultMetadata{ReturnedCount: 0},
			}, nil
		}
		resultsCopy := append([]domain.SearchResult(nil), page.Results...)
		metadata := page.Metadata
		metadata.ReturnedCount = len(resultsCopy)
		return domain.SearchResultPage{Results: resultsCopy, Metadata: metadata}, nil
	}

	resultsCopy := append([]domain.SearchResult(nil), f.SearchIssuesResponse.Results...)
	return domain.SearchResultPage{Results: resultsCopy, Metadata: f.SearchIssuesResponse.Metadata}, nil
}

func (f *FakeBeadsGateway) Query(_ context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodQuery, Input: QueryCall{Expr: expr, Opts: opts}})
	if err := f.MethodErrors[MethodQuery]; err != nil {
		return nil, err
	}

	// QueryResponsesByExpr opt-in: when set, look up by exact expr string.
	// Falls back to QueryResponse for callers that don't need per-expr fidelity.
	if f.QueryResponsesByExpr != nil {
		if results, ok := f.QueryResponsesByExpr[expr]; ok {
			return append([]domain.IssueSummary(nil), results...), nil
		}
		// Expr not found in map — return empty slice (mirrors real bd returning
		// no results for an unrecognised/unseeded expression).
		return []domain.IssueSummary{}, nil
	}

	return append([]domain.IssueSummary(nil), f.QueryResponse...), nil
}

func (f *FakeBeadsGateway) ReadyExplain(_ context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodReadyExplain, Input: ReadyExplainCall{Opts: opts}})
	if err := f.MethodErrors[MethodReadyExplain]; err != nil {
		return domain.ReadyExplainResult{}, err
	}

	readyCopy := append([]domain.IssueSummary(nil), f.ReadyExplainResponse.Ready...)
	blockedCopy := append([]domain.BlockedIssueView(nil), f.ReadyExplainResponse.Blocked...)
	return domain.ReadyExplainResult{
		Ready:        readyCopy,
		Blocked:      blockedCopy,
		TotalReady:   f.ReadyExplainResponse.TotalReady,
		TotalBlocked: f.ReadyExplainResponse.TotalBlocked,
		CycleCount:   f.ReadyExplainResponse.CycleCount,
	}, nil
}

func (f *FakeBeadsGateway) CountIssues(_ context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodCountIssues, Input: CountIssuesCall{Query: query}})
	if err := f.MethodErrors[MethodCountIssues]; err != nil {
		return domain.IssueCountResult{}, err
	}

	// When the write-state store has entries, count live from it so that
	// CountIncrementInvariant passes: CreateIssue increments the count.
	// When the store is empty, fall back to CountIssuesResponse for tests
	// that use it as a static stub (they never call CreateIssue).
	if len(f.issueStore) > 0 {
		statusCounts := make(map[string]int)
		for _, detail := range f.issueStore {
			statusCounts[detail.Summary.Status]++
		}

		filterSet := make(map[string]struct{}, len(query.Statuses))
		for _, s := range query.Statuses {
			filterSet[s] = struct{}{}
		}

		groups := make([]domain.IssueStatusCount, 0, len(statusCounts))
		total := 0
		for status, count := range statusCounts {
			if len(filterSet) > 0 {
				if _, ok := filterSet[status]; !ok {
					continue
				}
			}
			groups = append(groups, domain.IssueStatusCount{Status: status, Count: count})
			total += count
		}
		return domain.IssueCountResult{Groups: groups, Total: total}, nil
	}

	groupsCopy := append([]domain.IssueStatusCount(nil), f.CountIssuesResponse.Groups...)
	return domain.IssueCountResult{Groups: groupsCopy, Total: f.CountIssuesResponse.Total}, nil
}

func (f *FakeBeadsGateway) CreateIssue(_ context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodCreateIssue, Input: CreateIssueCall{Input: input}})
	if err := f.MethodErrors[MethodCreateIssue]; err != nil {
		return domain.CreateIssueResult{}, err
	}

	// Validate: empty title → ErrorCodeCommandFailed (mirrors real bd).
	if input.Title == "" {
		return domain.CreateIssueResult{}, domain.GatewayError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "create issue",
			Message:   `command exited with code 1: {"error":"title required"}`,
		}
	}

	id := fmt.Sprintf("tmp-%d", f.issueCounter.Add(1))

	typ := input.Type
	if typ == "" {
		typ = "task"
	}

	f.issueStore[id] = domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     id,
			Title:  input.Title,
			Status: "open",
			Type:   typ,
		},
		Description: input.Description,
		Comments:    []domain.IssueComment{},
		BlockedBy:   []domain.IssueReference{},
	}

	return domain.CreateIssueResult{IssueID: id}, nil
}

func (f *FakeBeadsGateway) UpdateIssue(_ context.Context, issueID string, input domain.UpdateIssueInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodUpdateIssue, Input: UpdateIssueCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodUpdateIssue]; err != nil {
		return err
	}

	// Validate: unknown ID → ErrorCodeCommandFailed (mirrors real bd).
	existing, ok := f.issueStore[issueID]
	if !ok {
		return domain.GatewayError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "update issue",
			Message:   fmt.Sprintf(`command exited with code 1: Error resolving %q: no issue found`, issueID),
		}
	}

	if input.Title != nil {
		existing.Summary.Title = *input.Title
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Status != nil {
		existing.Summary.Status = *input.Status
	}

	f.issueStore[issueID] = existing
	return nil
}

func (f *FakeBeadsGateway) CloseIssue(_ context.Context, issueID string, input domain.CloseIssueInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodCloseIssue, Input: CloseIssueCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodCloseIssue]; err != nil {
		return err
	}

	// Validate: unknown ID → ErrorCodeCommandFailed (mirrors real bd).
	existing, ok := f.issueStore[issueID]
	if !ok {
		return domain.GatewayError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "close issue",
			Message:   fmt.Sprintf(`command exited with code 1: Error resolving %q: no issue found`, issueID),
		}
	}

	// Idempotent: already closed → still return nil.
	existing.Summary.Status = "closed"
	f.issueStore[issueID] = existing
	return nil
}

func (f *FakeBeadsGateway) AddComment(_ context.Context, issueID string, input domain.AddCommentInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodAddComment, Input: AddCommentCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodAddComment]; err != nil {
		return err
	}

	// Validate: unknown ID → ErrorCodeCommandFailed (mirrors real bd).
	existing, ok := f.issueStore[issueID]
	if !ok {
		return domain.GatewayError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "add comment",
			Message:   fmt.Sprintf(`command exited with code 1: unknown issue %q`, issueID),
		}
	}

	existing.Comments = append(existing.Comments, domain.IssueComment{
		Body: input.Body,
	})
	f.issueStore[issueID] = existing
	return nil
}

func (f *FakeBeadsGateway) StatusCatalog(_ context.Context) ([]domain.StatusOption, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodStatusCatalog})
	if err := f.MethodErrors[MethodStatusCatalog]; err != nil {
		return nil, err
	}

	return append([]domain.StatusOption(nil), f.StatusCatalogResponse...), nil
}

func (f *FakeBeadsGateway) TypeCatalog(_ context.Context) ([]domain.TypeOption, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodTypeCatalog})
	if err := f.MethodErrors[MethodTypeCatalog]; err != nil {
		return nil, err
	}

	return append([]domain.TypeOption(nil), f.TypeCatalogResponse...), nil
}

func (f *FakeBeadsGateway) LabelCatalog(_ context.Context) ([]domain.LabelOption, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodLabelCatalog})
	if err := f.MethodErrors[MethodLabelCatalog]; err != nil {
		return nil, err
	}

	return append([]domain.LabelOption(nil), f.LabelCatalogResponse...), nil
}
