package fakes

import (
	"context"
	"strings"
	"sync"

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
// ShowIssuesByID is an optional ID-keyed map for ShowIssue. When set, ShowIssue
// looks up the requested ID in the map and returns a domain.GatewayError with
// ErrorCodeCommandFailed when the ID is absent (matching real bd behaviour).
// When nil, ShowIssueResponse is returned for every ShowIssue call (legacy
// behaviour, preserved for UI tests).
//
// SearchResultsByText is an optional text-keyed map for SearchIssues. When set,
// SearchIssues looks up by query.Text (trimmed) and returns the matching page —
// or an empty page if the key is absent. This lets contract tests verify
// text-filtered search without breaking UI tests that use SearchIssuesResponse
// as a verbatim stub.
type FakeBeadsGateway struct {
	mu sync.Mutex

	ListIssuesResponse    []domain.IssueSummary
	ReadyIssuesResponse   []domain.IssueSummary
	BlockedIssuesResponse []domain.BlockedIssueView
	ShowIssueResponse     domain.IssueDetail
	ShowIssuesByID        map[string]domain.IssueDetail
	SearchIssuesResponse  domain.SearchResultPage
	SearchResultsByText   map[string]domain.SearchResultPage
	QueryResponse         []domain.IssueSummary
	ReadyExplainResponse  domain.ReadyExplainResult
	CountIssuesResponse   domain.IssueCountResult

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
	}
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

	return f.CreateIssueResponse, nil
}

func (f *FakeBeadsGateway) UpdateIssue(_ context.Context, issueID string, input domain.UpdateIssueInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodUpdateIssue, Input: UpdateIssueCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodUpdateIssue]; err != nil {
		return err
	}

	return nil
}

func (f *FakeBeadsGateway) CloseIssue(_ context.Context, issueID string, input domain.CloseIssueInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodCloseIssue, Input: CloseIssueCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodCloseIssue]; err != nil {
		return err
	}

	return nil
}

func (f *FakeBeadsGateway) AddComment(_ context.Context, issueID string, input domain.AddCommentInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, GatewayCall{Method: MethodAddComment, Input: AddCommentCall{IssueID: issueID, Input: input}})
	if err := f.MethodErrors[MethodAddComment]; err != nil {
		return err
	}

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
