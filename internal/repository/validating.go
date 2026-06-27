// Package repository — validating decorator.
//
// This file defines [ValidatingRepository], a decorator that wraps any
// [Repository] implementation and validates the structural invariants of each
// return value. On violation it emits a slog Warn and returns the original
// result unchanged (warn-only, never fails the call).
//
// # Package placement rationale
//
// Placed in internal/repository (parent package) so it:
//   - imports only domain + slog (no backend-specific types)
//   - wraps any Repository implementation (taskmgr, memory)
//
// # Rule migration
//
// Rules are reimplemented from specification (validating_repository.go +
// contractcheck/) over Repository domain types. The contractcheck package
// is intentionally NOT imported — see rule-mapping comment per method.
package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// validatingRepository wraps an inner Repository and logs contract violations
// as slog Warn messages. All calls delegate to the inner repository; the inner
// result is always returned unchanged whether or not a violation was detected.
//
// Thread safety: safe for concurrent use. The decorator holds no per-call state
// (the repository-level ssom map and mutex are gone — Dashboard bundles count and
// list in one atomic return so the invariant is checked in-call).
type validatingRepository struct {
	inner  Repository
	logger *slog.Logger
}

// Compile-time interface assertion.
var _ Repository = (*validatingRepository)(nil)

// NewValidating wraps inner with contract validation. If logger is nil,
// slog.Default() is used.
//
// Usage: wrap any Repository implementation before handing it to callers.
// Wire into production in E4.3; production wiring is outside this file's scope.
func NewValidating(inner Repository, logger *slog.Logger) Repository {
	if logger == nil {
		logger = slog.Default()
	}
	return &validatingRepository{inner: inner, logger: logger}
}

// warn emits one structured warning per violation. No-op when violations is empty.
func (v *validatingRepository) warn(violations []violation) {
	for _, vi := range violations {
		v.logger.Warn("repository contract violation",
			"method", vi.method,
			"rule", vi.rule,
			"sample", vi.sample,
		)
	}
}

// violation describes a single repository contract invariant violation.
type violation struct {
	method string
	rule   string
	sample string
}

// -- Repository methods --

// HealthCheck delegates to inner. No structural invariants beyond error class.
func (v *validatingRepository) HealthCheck(ctx context.Context) error {
	return v.inner.HealthCheck(ctx)
}

// Dashboard delegates to inner and validates the composite return value.
//
// Rules migrated from the legacy repository-shape validator:
//
//   - ValidateIssueSummaries applied to all summary slices (InProgress, Closed,
//     Blocked, ReadyExplain.Ready, ReadyExplain.Blocked[].Issue).
//   - Status-slot consistency (new at Repository boundary):
//     InProgress items must have status=="in_progress",
//     Closed items must have status=="closed",
//     Blocked items (the Not-Ready feed) must have status=="blocked" or
//     status=="deferred".
//   - ValidateReadyExplain rules (6 sub-rules from contractcheck).
//   - Ssom in-call invariant (replaces cross-method state): ClosedTotal >= len(Closed).
//
// Rules dropped:
//   - ValidateListIssuesClosedExcluded: no "default no-filter list" surface on
//     Repository. Dashboard.Closed intentionally fetches closed. InProgress and
//     Blocked are status-constrained, covered by slot consistency above.
//   - ValidateBlockedViews (repository BlockedIssues): Dashboard.Blocked is
//     []IssueSummary with no BlockedBy info; that rule has no mapping here.
//   - ValidateCountIssues (TotalEqualsSumOfGroups etc.): Dashboard.ClosedTotal
//     is a scalar int with no Groups; sum-of-groups check has no mapping here.
//   - ValidateSsomInvariant (cross-method state): replaced by ClosedTotal >= len(Closed)
//     checked in the same call, no mutex or per-instance state needed.
func (v *validatingRepository) Dashboard(ctx context.Context, opts DashboardOptions) (DashboardData, error) {
	data, err := v.inner.Dashboard(ctx, opts)
	if err != nil {
		return data, err
	}

	var vs []violation

	// Summary structural checks on each slice.
	vs = append(vs, validateSummaries("Dashboard", data.InProgress)...)
	vs = append(vs, validateSummaries("Dashboard", data.Closed)...)
	vs = append(vs, validateSummaries("Dashboard", data.Blocked)...)
	vs = append(vs, validateSummaries("Dashboard", data.ReadyExplain.Ready)...)
	for _, bv := range data.ReadyExplain.Blocked {
		vs = append(vs, validateSummary("Dashboard", bv.Issue)...)
	}

	// Status-slot consistency.
	vs = append(vs, validateSlotStatus("Dashboard", "DashboardInProgressStatusMatches", data.InProgress, "in_progress")...)
	vs = append(vs, validateSlotStatus("Dashboard", "DashboardClosedStatusMatches", data.Closed, "closed")...)
	// The Blocked slot is the board's "Not Ready" feed, which both backends
	// populate with stored-status "blocked" AND "deferred" issues (deferred is an
	// active, non-closed status that is neither ready nor in-progress). Accept
	// either so legitimate deferred issues are not flagged as contract violations.
	vs = append(vs, validateSlotStatusOneOf("Dashboard", "DashboardBlockedStatusMatches", data.Blocked, "blocked", "deferred")...)

	// ReadyExplain structural rules.
	vs = append(vs, validateReadyExplain("Dashboard", data.ReadyExplain, false)...)

	// Ssom in-call invariant.
	if data.ClosedTotal < len(data.Closed) {
		vs = append(vs, violation{
			method: "Dashboard",
			rule:   "SsomClosedTotalGEQLen",
			sample: fmt.Sprintf("ClosedTotal=%d < len(Closed)=%d — total must be >= list length", data.ClosedTotal, len(data.Closed)),
		})
	}

	v.warn(vs)
	return data, nil
}

// Issue delegates to inner and validates the returned IssueDetail.
//
// Rules migrated from ValidateShowIssue (contractcheck):
//   - ReturnedIDMatchesInput: detail.Summary.ID must match the requested id.
//   - CommentsNotNil: detail.Comments must be non-nil.
//   - BlockedByNotNil: detail.BlockedBy must be non-nil.
func (v *validatingRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	detail, err := v.inner.Issue(ctx, id)
	if err != nil {
		return detail, err
	}

	var vs []violation

	if id != "" && detail.Summary.ID != id {
		vs = append(vs, violation{
			method: "Issue",
			rule:   "ReturnedIDMatchesInput",
			sample: fmt.Sprintf("requested=%q, returned=%q", id, detail.Summary.ID),
		})
	}
	if detail.Comments == nil {
		vs = append(vs, violation{
			method: "Issue",
			rule:   "CommentsNotNil",
			sample: fmt.Sprintf("id=%q: Comments slice is nil", detail.Summary.ID),
		})
	}
	if detail.BlockedBy == nil {
		vs = append(vs, violation{
			method: "Issue",
			rule:   "BlockedByNotNil",
			sample: fmt.Sprintf("id=%q: BlockedBy slice is nil", detail.Summary.ID),
		})
	}

	v.warn(vs)
	return detail, nil
}

// Search delegates to inner and validates the returned SearchResultPage.
//
// Rules migrated from ValidateSearchPage (contractcheck):
//   - ResultsNotNil: Results must be a non-nil slice.
//   - NonEmptyIDs: each result.Issue must have a non-empty ID.
//   - ReturnedCountMatchesLen: Metadata.ReturnedCount must equal len(Results).
//
// Additional rule at Repository boundary:
//   - SearchStatusFilterRespected: when query.Statuses is non-empty, all returned
//     results must match one of the requested statuses. Migrated from
//     ValidateListIssuesStatusFilter (contractcheck), now applied to Search results.
func (v *validatingRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	page, err := v.inner.Search(ctx, query)
	if err != nil {
		return page, err
	}

	var vs []violation

	if page.Results == nil {
		vs = append(vs, violation{
			method: "Search",
			rule:   "ResultsNotNil",
			sample: "Results slice is nil",
		})
		v.warn(vs)
		return page, nil // no point iterating nil slice
	}

	for i, r := range page.Results {
		if r.Issue.ID == "" {
			vs = append(vs, violation{
				method: "Search",
				rule:   "NonEmptyIDs",
				sample: fmt.Sprintf("Results[%d]: Issue.ID is empty (title=%q)", i, r.Issue.Title),
			})
		}
	}

	if page.Metadata.ReturnedCount != len(page.Results) {
		vs = append(vs, violation{
			method: "Search",
			rule:   "ReturnedCountMatchesLen",
			sample: fmt.Sprintf("Metadata.ReturnedCount=%d != len(Results)=%d", page.Metadata.ReturnedCount, len(page.Results)),
		})
	}

	if len(query.Statuses) > 0 {
		statusSet := make(map[string]struct{}, len(query.Statuses))
		for _, s := range query.Statuses {
			statusSet[s] = struct{}{}
		}
		for i, r := range page.Results {
			if _, ok := statusSet[r.Issue.Status]; !ok {
				vs = append(vs, violation{
					method: "Search",
					rule:   "SearchStatusFilterRespected",
					sample: fmt.Sprintf("Results[%d]: id=%q has status=%q, not in requested %v", i, r.Issue.ID, r.Issue.Status, query.Statuses),
				})
			}
		}
	}

	v.warn(vs)
	return page, nil
}

// CreateIssue delegates directly — no validation on write responses.
// Matches today's behaviour (write-side contract tracked separately).
func (v *validatingRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return v.inner.CreateIssue(ctx, input)
}

// UpdateIssue delegates directly — no validation on write responses.
func (v *validatingRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	return v.inner.UpdateIssue(ctx, id, input)
}

// CloseIssue delegates directly — no validation on write responses.
func (v *validatingRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	return v.inner.CloseIssue(ctx, id, input)
}

// AddComment delegates directly — no validation on write responses.
func (v *validatingRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	return v.inner.AddComment(ctx, id, input)
}

// Catalogs delegates to inner and validates the returned Catalogs.
//
// Rules migrated from ValidateStatusCatalog, ValidateTypeCatalog, ValidateLabelCatalog
// (contractcheck):
//   - CatalogsStatusNonEmpty: Statuses must not be empty.
//   - CatalogsStatusAllNamesNonEmpty: every StatusOption.Name must be non-empty.
//   - CatalogsTypeNonEmpty: Types must not be empty.
//   - CatalogsTypeAllNamesNonEmpty: every TypeOption.Name must be non-empty.
//   - CatalogsLabelAllNamesNonEmpty: every LabelOption.Name must be non-empty.
func (v *validatingRepository) Catalogs(ctx context.Context) (Catalogs, error) {
	cats, err := v.inner.Catalogs(ctx)
	if err != nil {
		return cats, err
	}

	var vs []violation

	if len(cats.Statuses) == 0 {
		vs = append(vs, violation{
			method: "Catalogs",
			rule:   "CatalogsStatusNonEmpty",
			sample: "Catalogs.Statuses is empty",
		})
	}
	for i, o := range cats.Statuses {
		if o.Name == "" {
			vs = append(vs, violation{
				method: "Catalogs",
				rule:   "CatalogsStatusAllNamesNonEmpty",
				sample: fmt.Sprintf("Statuses[%d]: Name is empty", i),
			})
		}
	}

	if len(cats.Types) == 0 {
		vs = append(vs, violation{
			method: "Catalogs",
			rule:   "CatalogsTypeNonEmpty",
			sample: "Catalogs.Types is empty",
		})
	}
	for i, o := range cats.Types {
		if o.Name == "" {
			vs = append(vs, violation{
				method: "Catalogs",
				rule:   "CatalogsTypeAllNamesNonEmpty",
				sample: fmt.Sprintf("Types[%d]: Name is empty", i),
			})
		}
	}

	for i, o := range cats.Labels {
		if o.Name == "" {
			vs = append(vs, violation{
				method: "Catalogs",
				rule:   "CatalogsLabelAllNamesNonEmpty",
				sample: fmt.Sprintf("Labels[%d]: Name is empty", i),
			})
		}
	}

	v.warn(vs)
	return cats, nil
}

// -- pure validator helpers --

// validateSummaries applies structural checks to each item in a slice.
// It uses spot-checking for high-cardinality slices (>5000 items).
func validateSummaries(method string, items []domain.IssueSummary) []violation {
	sample := spotCheckSummaries(items)
	var vs []violation
	for _, item := range sample {
		vs = append(vs, validateSummary(method, item)...)
	}
	return vs
}

// validateSummary checks structural invariants on a single IssueSummary.
//
// Rules migrated from ValidateIssueSummaries (contractcheck):
//   - NonEmptyID: summary must have a non-empty ID.
//   - NonEmptyTitle: summary must have a non-empty Title.
//   - NonEmptyStatus: summary must have a non-empty Status.
//   - NonEmptyType: summary must have a non-empty Type.
func validateSummary(method string, item domain.IssueSummary) []violation {
	var vs []violation
	if item.ID == "" {
		vs = append(vs, violation{
			method: method,
			rule:   "NonEmptyID",
			sample: fmt.Sprintf("IssueSummary ID is empty (title=%q)", item.Title),
		})
	}
	if item.Title == "" {
		vs = append(vs, violation{
			method: method,
			rule:   "NonEmptyTitle",
			sample: fmt.Sprintf("IssueSummary Title is empty (id=%q)", item.ID),
		})
	}
	if item.Status == "" {
		vs = append(vs, violation{
			method: method,
			rule:   "NonEmptyStatus",
			sample: fmt.Sprintf("IssueSummary Status is empty (id=%q)", item.ID),
		})
	}
	if item.Type == "" {
		vs = append(vs, violation{
			method: method,
			rule:   "NonEmptyType",
			sample: fmt.Sprintf("IssueSummary Type is empty (id=%q)", item.ID),
		})
	}
	return vs
}

// validateSlotStatus checks that every issue in items has the expected status.
// This is the Repository-level equivalent of ValidateListIssuesStatusFilter for
// Dashboard slots where the status is implied by the slot (e.g. InProgress → "in_progress").
func validateSlotStatus(method, rule string, items []domain.IssueSummary, expected string) []violation {
	var vs []violation
	for i, item := range items {
		if item.Status != expected {
			vs = append(vs, violation{
				method: method,
				rule:   rule,
				sample: fmt.Sprintf("items[%d]: id=%q has status=%q, expected %q for this slot", i, item.ID, item.Status, expected),
			})
		}
	}
	return vs
}

// validateSlotStatusOneOf is the multi-status variant of validateSlotStatus for
// slots whose membership is defined by a set of stored statuses (e.g. the
// Not-Ready/Blocked slot, which holds both "blocked" and "deferred" issues).
func validateSlotStatusOneOf(method, rule string, items []domain.IssueSummary, expected ...string) []violation {
	allowed := make(map[string]struct{}, len(expected))
	for _, s := range expected {
		allowed[s] = struct{}{}
	}
	var vs []violation
	for i, item := range items {
		if _, ok := allowed[item.Status]; !ok {
			vs = append(vs, violation{
				method: method,
				rule:   rule,
				sample: fmt.Sprintf("items[%d]: id=%q has status=%q, expected one of %v for this slot", i, item.ID, item.Status, expected),
			})
		}
	}
	return vs
}

// validateReadyExplain checks structural invariants on ReadyExplainResult.
//
// Rules migrated from ValidateReadyExplain (contractcheck):
//   - NonEmptyReadyIDs: every ready issue must have a non-empty ID.
//   - NonEmptyBlockedIDs: every blocked view must have a non-empty Issue.ID.
//   - ReadyAndBlockedDisjoint: an issue cannot appear in both Ready and Blocked.
//   - TotalReadyMatchesLenReady: TotalReady must equal len(Ready) when no limit is applied.
//   - TotalBlockedMatchesLenBlocked: TotalBlocked must equal len(Blocked) when no limit is applied.
//   - BlockedByEnriched: every BlockedBy entry in Blocked must have non-empty Title and Status
//     (ReadyExplain returns enriched blocker objects, not bare ID-only references).
func validateReadyExplain(method string, result domain.ReadyExplainResult, limitApplied bool) []violation {
	var vs []violation

	for i, issue := range result.Ready {
		if issue.ID == "" {
			vs = append(vs, violation{
				method: method,
				rule:   "NonEmptyReadyIDs",
				sample: fmt.Sprintf("Ready[%d]: ID is empty (title=%q)", i, issue.Title),
			})
		}
	}

	for i, bv := range result.Blocked {
		if bv.Issue.ID == "" {
			vs = append(vs, violation{
				method: method,
				rule:   "NonEmptyBlockedIDs",
				sample: fmt.Sprintf("Blocked[%d]: Issue.ID is empty (title=%q)", i, bv.Issue.Title),
			})
		}
		for j, ref := range bv.BlockedBy {
			if ref.Title == "" {
				vs = append(vs, violation{
					method: method,
					rule:   "BlockedByEnriched",
					sample: fmt.Sprintf("Blocked[%d].BlockedBy[%d]: Title is empty (id=%q) — ReadyExplain blockers must be enriched", i, j, ref.ID),
				})
			}
			if ref.Status == "" {
				vs = append(vs, violation{
					method: method,
					rule:   "BlockedByEnriched",
					sample: fmt.Sprintf("Blocked[%d].BlockedBy[%d]: Status is empty (id=%q) — ReadyExplain blockers must be enriched", i, j, ref.ID),
				})
			}
		}
	}

	readyIDs := make(map[string]struct{}, len(result.Ready))
	for _, issue := range result.Ready {
		readyIDs[issue.ID] = struct{}{}
	}
	for i, bv := range result.Blocked {
		if _, ok := readyIDs[bv.Issue.ID]; ok {
			vs = append(vs, violation{
				method: method,
				rule:   "ReadyAndBlockedDisjoint",
				sample: fmt.Sprintf("Blocked[%d]: id=%q appears in both Ready and Blocked", i, bv.Issue.ID),
			})
		}
	}

	if !limitApplied && result.TotalReady != len(result.Ready) {
		vs = append(vs, violation{
			method: method,
			rule:   "TotalReadyMatchesLenReady",
			sample: fmt.Sprintf("TotalReady=%d != len(Ready)=%d (no limit applied)", result.TotalReady, len(result.Ready)),
		})
	}

	if !limitApplied && result.TotalBlocked != len(result.Blocked) {
		vs = append(vs, violation{
			method: method,
			rule:   "TotalBlockedMatchesLenBlocked",
			sample: fmt.Sprintf("TotalBlocked=%d != len(Blocked)=%d (no limit applied)", result.TotalBlocked, len(result.Blocked)),
		})
	}

	return vs
}

// -- high-cardinality spot-check helpers --

// spotCheckThreshold is the length above which only head+tail items are checked.
// Rationale (same as contractcheck): above 5000 items, full validation costs >1ms.
// First+last spotCheckN items catch structural drift (e.g. a taskmgr schema change
// that affects all items, not random items).
const (
	spotCheckThreshold = 5000
	spotCheckN         = 10
)

// spotCheckSummaries returns the items to validate. Returns items unchanged when
// len(items) <= spotCheckThreshold, otherwise returns head+tail spotCheckN items.
func spotCheckSummaries(items []domain.IssueSummary) []domain.IssueSummary {
	if len(items) <= spotCheckThreshold {
		return items
	}
	seen := make(map[int]struct{})
	out := make([]domain.IssueSummary, 0, spotCheckN*2)
	for i := 0; i < spotCheckN && i < len(items); i++ {
		seen[i] = struct{}{}
		out = append(out, items[i])
	}
	for i := len(items) - spotCheckN; i < len(items); i++ {
		if i < 0 {
			continue
		}
		if _, ok := seen[i]; ok {
			continue
		}
		out = append(out, items[i])
	}
	return out
}
