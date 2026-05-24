package beads

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository/beads/contractcheck"
)

// validatingGateway is a BeadsGateway decorator that delegates every call to an
// inner gateway and then validates the response against the contract invariants
// defined in the contractcheck package. Invariant violations are logged at
// slog.Warn level and never cause the request to fail — the inner gateway's
// result is always returned unchanged.
//
// Purpose: production tripwire. When bd's behavior drifts (schema change, edge
// case, version skew), violations surface in ~/.local/state/bwb/bwb-<session_id>.log
// immediately, before users notice corrupted UI.
//
// Thread safety: validatingGateway is safe for concurrent use. The ssom state
// (lastCountByStatus) is guarded by mu.
type validatingGateway struct {
	inner  BeadsGateway
	logger *slog.Logger

	// mu guards lastCountByStatus.
	mu sync.Mutex
	// lastCountByStatus caches the most recent CountIssues result keyed by
	// a canonical sorted status string (e.g. "closed" or "open,blocked").
	// Used by the ssom cross-method invariant: when a ListIssues call
	// returns N items for the same status set, we assert lastCount >= N.
	// State is per-gateway-instance and ephemeral.
	lastCountByStatus map[string]int
}

// newValidatingGateway wraps inner with contract validation. If logger is nil,
// slog.Default() is used.
func newValidatingGateway(inner BeadsGateway, logger *slog.Logger) *validatingGateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &validatingGateway{
		inner:             inner,
		logger:            logger,
		lastCountByStatus: make(map[string]int),
	}
}

// warn emits a structured warning for each violation. It is a no-op when
// violations is empty.
func (g *validatingGateway) warn(violations []contractcheck.Violation) {
	for _, v := range violations {
		g.logger.Warn("gateway contract violation",
			"method", v.Method,
			"rule", v.Rule,
			"sample", v.Sample,
		)
	}
}

// statusKey returns a canonical cache key for a status slice.
// Statuses are joined sorted so that [closed] and [closed] always match.
func statusKey(statuses []string) string {
	if len(statuses) == 0 {
		return ""
	}
	// Sort a copy to avoid mutating the caller's slice.
	sorted := make([]string, len(statuses))
	copy(sorted, statuses)
	// Simple insertion sort — status slices are tiny (< 10 elements).
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return strings.Join(sorted, ",")
}

// HealthCheck delegates to inner. No structural invariants beyond error class.
func (g *validatingGateway) HealthCheck(ctx context.Context) error {
	return g.inner.HealthCheck(ctx)
}

// ListIssues delegates to inner and validates the result.
func (g *validatingGateway) ListIssues(ctx context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error) {
	items, err := g.inner.ListIssues(ctx, query)
	if err != nil {
		return items, err
	}

	// High-cardinality fallback: validate only first+last spotCheckCount items.
	sample := contractcheck.SelectIssueSummaries(items, contractcheck.SpotCheckIndices(len(items)))

	g.warn(contractcheck.ValidateIssueSummaries("ListIssues", sample))
	g.warn(contractcheck.ValidateListIssuesStatusFilter("ListIssues", sample, query.Statuses))
	g.warn(contractcheck.ValidateListIssuesClosedExcluded("ListIssues", sample, query.Statuses))

	// ssom cross-method invariant: if we've seen a CountIssues result for the
	// same status set, the list length must not exceed the count.
	if len(query.Statuses) > 0 {
		key := statusKey(query.Statuses)
		g.mu.Lock()
		lastCount, ok := g.lastCountByStatus[key]
		g.mu.Unlock()
		if ok {
			g.warn(contractcheck.ValidateSsomInvariant("ListIssues", query.Statuses, lastCount, len(items)))
		}
	}

	return items, nil
}

// Query delegates to inner and validates the result.
func (g *validatingGateway) Query(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
	items, err := g.inner.Query(ctx, expr, opts)
	if err != nil {
		return items, err
	}

	sample := contractcheck.SelectIssueSummaries(items, contractcheck.SpotCheckIndices(len(items)))
	g.warn(contractcheck.ValidateIssueSummaries("Query", sample))
	return items, nil
}

// ReadyIssues delegates to inner and validates the result.
func (g *validatingGateway) ReadyIssues(ctx context.Context, query domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	items, err := g.inner.ReadyIssues(ctx, query)
	if err != nil {
		return items, err
	}

	sample := contractcheck.SelectIssueSummaries(items, contractcheck.SpotCheckIndices(len(items)))
	g.warn(contractcheck.ValidateIssueSummaries("ReadyIssues", sample))
	// Closed issues must never appear in the ready list.
	g.warn(contractcheck.ValidateListIssuesClosedExcluded("ReadyIssues", sample, nil /* treat as no-filter; always exclude closed */))
	return items, nil
}

// BlockedIssues delegates to inner and validates the result.
func (g *validatingGateway) BlockedIssues(ctx context.Context, query domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	views, err := g.inner.BlockedIssues(ctx, query)
	if err != nil {
		return views, err
	}

	g.warn(contractcheck.ValidateBlockedViews("BlockedIssues", views))
	return views, nil
}

// ReadyExplain delegates to inner and validates the result.
func (g *validatingGateway) ReadyExplain(ctx context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	result, err := g.inner.ReadyExplain(ctx, opts)
	if err != nil {
		return result, err
	}

	g.warn(contractcheck.ValidateReadyExplain("ReadyExplain", result, opts.Limit > 0))
	return result, nil
}

// ShowIssue delegates to inner and validates the result.
func (g *validatingGateway) ShowIssue(ctx context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	detail, err := g.inner.ShowIssue(ctx, query)
	if err != nil {
		return detail, err
	}

	g.warn(contractcheck.ValidateShowIssue("ShowIssue", detail, query.IssueID))
	return detail, nil
}

// SearchIssues delegates to inner and validates the result.
func (g *validatingGateway) SearchIssues(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	page, err := g.inner.SearchIssues(ctx, query)
	if err != nil {
		return page, err
	}

	g.warn(contractcheck.ValidateSearchPage("SearchIssues", page))
	return page, nil
}

// CountIssues delegates to inner, validates the result, and caches the count
// for use by the ssom cross-method invariant in subsequent ListIssues calls.
func (g *validatingGateway) CountIssues(ctx context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	result, err := g.inner.CountIssues(ctx, query)
	if err != nil {
		return result, err
	}

	g.warn(contractcheck.ValidateCountIssues("CountIssues", result))

	// Cache count for ssom invariant. Only cache when a status filter was
	// applied — an unfiltered count covers all statuses and should not be
	// compared against a status-filtered list query.
	if len(query.Statuses) > 0 {
		key := statusKey(query.Statuses)
		g.mu.Lock()
		g.lastCountByStatus[key] = result.Total
		g.mu.Unlock()
	}

	return result, nil
}

// StatusCatalog delegates to inner and validates the result.
func (g *validatingGateway) StatusCatalog(ctx context.Context) ([]domain.StatusOption, error) {
	opts, err := g.inner.StatusCatalog(ctx)
	if err != nil {
		return opts, err
	}

	g.warn(contractcheck.ValidateStatusCatalog("StatusCatalog", opts))
	return opts, nil
}

// TypeCatalog delegates to inner and validates the result.
func (g *validatingGateway) TypeCatalog(ctx context.Context) ([]domain.TypeOption, error) {
	opts, err := g.inner.TypeCatalog(ctx)
	if err != nil {
		return opts, err
	}

	g.warn(contractcheck.ValidateTypeCatalog("TypeCatalog", opts))
	return opts, nil
}

// LabelCatalog delegates to inner and validates the result.
func (g *validatingGateway) LabelCatalog(ctx context.Context) ([]domain.LabelOption, error) {
	opts, err := g.inner.LabelCatalog(ctx)
	if err != nil {
		return opts, err
	}

	g.warn(contractcheck.ValidateLabelCatalog("LabelCatalog", opts))
	return opts, nil
}

// Write-side methods: delegate directly — no invariant validation on write
// responses (write-side contract is tracked in epic beads-workbench-9x70).

func (g *validatingGateway) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return g.inner.CreateIssue(ctx, input)
}

func (g *validatingGateway) UpdateIssue(ctx context.Context, issueID string, input domain.UpdateIssueInput) error {
	return g.inner.UpdateIssue(ctx, issueID, input)
}

func (g *validatingGateway) CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error {
	return g.inner.CloseIssue(ctx, issueID, input)
}

func (g *validatingGateway) AddComment(ctx context.Context, issueID string, input domain.AddCommentInput) error {
	return g.inner.AddComment(ctx, issueID, input)
}
