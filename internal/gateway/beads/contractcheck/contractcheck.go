// Package contractcheck provides pure validator functions for BeadsGateway
// contract invariants. These functions are consumed by both the runtime
// validatingGateway decorator (for production tripwires) and the contract test
// suite (RunReadContract), ensuring the two share a single source of truth.
//
// All functions are pure: they take domain values and return Violation slices.
// They never call t.Error / t.Fatal and carry no testing.T dependency.
package contractcheck

import (
	"fmt"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// Violation describes a single contract invariant violation.
type Violation struct {
	// Method is the gateway method name where the violation was detected.
	Method string
	// Rule is the invariant name / description (e.g. "NonEmptyID").
	Rule string
	// Sample is a brief string representation of the first violating item.
	Sample string
}

func (v Violation) String() string {
	return fmt.Sprintf("[%s] %s: %s", v.Method, v.Rule, v.Sample)
}

// ValidateIssueSummaries checks structural invariants on a slice of IssueSummary
// values as returned by ListIssues, ReadyIssues, Query, and similar methods.
//
// Rules checked:
//   - NonEmptyID: each summary must have a non-empty ID.
//   - NonEmptyTitle: each summary must have a non-empty Title.
//   - NonEmptyStatus: each summary must have a non-empty Status.
//   - NonEmptyType: each summary must have a non-empty Type.
func ValidateIssueSummaries(method string, items []domain.IssueSummary) []Violation {
	var vs []Violation
	for i, item := range items {
		if item.ID == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyID",
				Sample: fmt.Sprintf("items[%d]: ID is empty (title=%q)", i, item.Title),
			})
		}
		if item.Title == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyTitle",
				Sample: fmt.Sprintf("items[%d]: Title is empty (id=%q)", i, item.ID),
			})
		}
		if item.Status == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyStatus",
				Sample: fmt.Sprintf("items[%d]: Status is empty (id=%q)", i, item.ID),
			})
		}
		if item.Type == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyType",
				Sample: fmt.Sprintf("items[%d]: Type is empty (id=%q)", i, item.ID),
			})
		}
	}
	return vs
}

// ValidateListIssuesStatusFilter checks that when a status filter was applied,
// all returned issues match that filter. This catches bd-side filter bugs.
func ValidateListIssuesStatusFilter(method string, items []domain.IssueSummary, requestedStatuses []string) []Violation {
	if len(requestedStatuses) == 0 {
		return nil
	}
	statusSet := make(map[string]struct{}, len(requestedStatuses))
	for _, s := range requestedStatuses {
		statusSet[s] = struct{}{}
	}
	var vs []Violation
	for i, item := range items {
		if _, ok := statusSet[item.Status]; !ok {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "StatusFilterRespected",
				Sample: fmt.Sprintf("items[%d]: id=%q has status=%q, not in requested %v", i, item.ID, item.Status, requestedStatuses),
			})
		}
	}
	return vs
}

// ValidateListIssuesClosedExcluded checks that closed issues are not returned
// when no status filter is applied (default bd list behavior excludes closed).
func ValidateListIssuesClosedExcluded(method string, items []domain.IssueSummary, requestedStatuses []string) []Violation {
	if len(requestedStatuses) > 0 {
		// Status filter was applied — caller opted in to specific statuses.
		return nil
	}
	var vs []Violation
	for i, item := range items {
		if item.Status == "closed" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "ClosedExcludedByDefault",
				Sample: fmt.Sprintf("items[%d]: closed issue id=%q returned in default query", i, item.ID),
			})
		}
	}
	return vs
}

// ValidateBlockedViews checks structural invariants on BlockedIssueView slices
// as returned by BlockedIssues.
//
// Rules checked:
//   - NonEmptyID: each view.Issue must have a non-empty ID.
//   - NonEmptyBlockedBySlice: each view must have at least one blocker.
//   - BlockerIDsNonEmpty: each blocker reference must have a non-empty ID.
func ValidateBlockedViews(method string, views []domain.BlockedIssueView) []Violation {
	var vs []Violation
	for i, view := range views {
		if view.Issue.ID == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyID",
				Sample: fmt.Sprintf("views[%d]: Issue.ID is empty (title=%q)", i, view.Issue.Title),
			})
		}
		if len(view.BlockedBy) == 0 {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyBlockedBySlice",
				Sample: fmt.Sprintf("views[%d]: id=%q has empty BlockedBy slice", i, view.Issue.ID),
			})
		}
		for j, ref := range view.BlockedBy {
			if ref.ID == "" {
				vs = append(vs, Violation{
					Method: method,
					Rule:   "BlockerIDsNonEmpty",
					Sample: fmt.Sprintf("views[%d]: id=%q BlockedBy[%d] has empty ID", i, view.Issue.ID, j),
				})
			}
		}
	}
	return vs
}

// ValidateReadyExplain checks structural invariants on ReadyExplainResult.
//
// Rules checked:
//   - NonEmptyReadyIDs: every ready issue must have a non-empty ID.
//   - NonEmptyBlockedIDs: every blocked view must have a non-empty Issue.ID.
//   - ReadyAndBlockedDisjoint: an issue cannot appear in both Ready and Blocked.
//   - TotalReadyMatchesLenReady: TotalReady must equal len(Ready) when no limit is applied.
func ValidateReadyExplain(method string, result domain.ReadyExplainResult, limitApplied bool) []Violation {
	var vs []Violation

	for i, issue := range result.Ready {
		if issue.ID == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyReadyIDs",
				Sample: fmt.Sprintf("Ready[%d]: ID is empty (title=%q)", i, issue.Title),
			})
		}
	}

	for i, view := range result.Blocked {
		if view.Issue.ID == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyBlockedIDs",
				Sample: fmt.Sprintf("Blocked[%d]: Issue.ID is empty (title=%q)", i, view.Issue.Title),
			})
		}
	}

	readyIDs := make(map[string]struct{}, len(result.Ready))
	for _, issue := range result.Ready {
		readyIDs[issue.ID] = struct{}{}
	}
	for i, view := range result.Blocked {
		if _, ok := readyIDs[view.Issue.ID]; ok {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "ReadyAndBlockedDisjoint",
				Sample: fmt.Sprintf("Blocked[%d]: id=%q appears in both Ready and Blocked", i, view.Issue.ID),
			})
		}
	}

	if !limitApplied && result.TotalReady != len(result.Ready) {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "TotalReadyMatchesLenReady",
			Sample: fmt.Sprintf("TotalReady=%d != len(Ready)=%d (no limit applied)", result.TotalReady, len(result.Ready)),
		})
	}

	return vs
}

// ValidateShowIssue checks structural invariants on IssueDetail as returned by ShowIssue.
//
// Rules checked:
//   - ReturnedIDMatchesInput: detail.Summary.ID must match the requested ID.
//   - CommentsNotNil: detail.Comments must be non-nil.
//   - BlockedByNotNil: detail.BlockedBy must be non-nil.
func ValidateShowIssue(method string, detail domain.IssueDetail, requestedID string) []Violation {
	var vs []Violation

	if requestedID != "" && detail.Summary.ID != requestedID {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "ReturnedIDMatchesInput",
			Sample: fmt.Sprintf("requested=%q, returned=%q", requestedID, detail.Summary.ID),
		})
	}

	if detail.Comments == nil {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "CommentsNotNil",
			Sample: fmt.Sprintf("id=%q: Comments slice is nil", detail.Summary.ID),
		})
	}

	if detail.BlockedBy == nil {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "BlockedByNotNil",
			Sample: fmt.Sprintf("id=%q: BlockedBy slice is nil", detail.Summary.ID),
		})
	}

	return vs
}

// ValidateSearchPage checks structural invariants on SearchResultPage as returned
// by SearchIssues.
//
// Rules checked:
//   - ResultsNotNil: Results must be a non-nil slice.
//   - NonEmptyIDs: each result.Issue must have a non-empty ID.
//   - ReturnedCountMatchesLen: Metadata.ReturnedCount must equal len(Results).
func ValidateSearchPage(method string, page domain.SearchResultPage) []Violation {
	var vs []Violation

	if page.Results == nil {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "ResultsNotNil",
			Sample: "Results slice is nil",
		})
		// No point iterating nil slice.
		return vs
	}

	for i, r := range page.Results {
		if r.Issue.ID == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "NonEmptyIDs",
				Sample: fmt.Sprintf("Results[%d]: Issue.ID is empty (title=%q)", i, r.Issue.Title),
			})
		}
	}

	if page.Metadata.ReturnedCount != len(page.Results) {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "ReturnedCountMatchesLen",
			Sample: fmt.Sprintf("Metadata.ReturnedCount=%d != len(Results)=%d", page.Metadata.ReturnedCount, len(page.Results)),
		})
	}

	return vs
}

// ValidateCountIssues checks structural invariants on IssueCountResult as returned
// by CountIssues.
//
// Rules checked:
//   - TotalEqualsSumOfGroups: Total must equal the arithmetic sum of all group counts.
//   - GroupStatusNonEmpty: each group must have a non-empty Status.
func ValidateCountIssues(method string, result domain.IssueCountResult) []Violation {
	var vs []Violation

	sum := 0
	for i, g := range result.Groups {
		sum += g.Count
		if g.Status == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "GroupStatusNonEmpty",
				Sample: fmt.Sprintf("Groups[%d]: Status is empty", i),
			})
		}
	}

	if result.Total != sum {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "TotalEqualsSumOfGroups",
			Sample: fmt.Sprintf("Total=%d != sum(Groups)=%d", result.Total, sum),
		})
	}

	return vs
}

// ValidateStatusCatalog checks structural invariants on []StatusOption as returned
// by StatusCatalog.
//
// Rules checked:
//   - NonEmpty: the result must contain at least one option.
//   - AllNamesNonEmpty: each option must have a non-empty Name.
func ValidateStatusCatalog(method string, opts []domain.StatusOption) []Violation {
	var vs []Violation

	if len(opts) == 0 {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "NonEmpty",
			Sample: "StatusCatalog returned empty slice",
		})
		return vs
	}

	for i, o := range opts {
		if o.Name == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "AllNamesNonEmpty",
				Sample: fmt.Sprintf("opts[%d]: Name is empty", i),
			})
		}
	}

	return vs
}

// ValidateTypeCatalog checks structural invariants on []TypeOption as returned
// by TypeCatalog.
//
// Rules checked:
//   - NonEmpty: the result must contain at least one option.
//   - AllNamesNonEmpty: each option must have a non-empty Name.
func ValidateTypeCatalog(method string, opts []domain.TypeOption) []Violation {
	var vs []Violation

	if len(opts) == 0 {
		vs = append(vs, Violation{
			Method: method,
			Rule:   "NonEmpty",
			Sample: "TypeCatalog returned empty slice",
		})
		return vs
	}

	for i, o := range opts {
		if o.Name == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "AllNamesNonEmpty",
				Sample: fmt.Sprintf("opts[%d]: Name is empty", i),
			})
		}
	}

	return vs
}

// ValidateLabelCatalog checks structural invariants on []LabelOption as returned
// by LabelCatalog.
//
// Rules checked:
//   - AllNamesNonEmpty: each option must have a non-empty Name.
func ValidateLabelCatalog(method string, opts []domain.LabelOption) []Violation {
	var vs []Violation

	for i, o := range opts {
		if o.Name == "" {
			vs = append(vs, Violation{
				Method: method,
				Rule:   "AllNamesNonEmpty",
				Sample: fmt.Sprintf("opts[%d]: Name is empty (label field must not be blank)", i),
			})
		}
	}

	return vs
}

// ValidateSsomInvariant checks the cross-method ssom invariant:
// CountIssues(statuses).Total must be >= len(ListIssues(statuses, limit=N))
// when the list query requested a specific status set.
//
// This invariant catches the regression where CountIssues under-counts relative
// to what ListIssues actually returns for the same status filter.
//
// Parameters:
//   - statuses: the status filter used for both calls (must be non-empty).
//   - countTotal: result of CountIssues(statuses).Total.
//   - listLen: len of the ListIssues(statuses, limit=N) result.
func ValidateSsomInvariant(method string, statuses []string, countTotal int, listLen int) []Violation {
	if len(statuses) == 0 {
		// ssom invariant only applies when both calls used the same explicit status filter.
		return nil
	}
	if countTotal < listLen {
		return []Violation{{
			Method: method,
			Rule:   "SsomCountGreaterThanOrEqualToListSize",
			Sample: fmt.Sprintf("CountIssues(statuses=%v).Total=%d < len(ListIssues(statuses=%v))=%d — count must be >= list length", statuses, countTotal, statuses, listLen),
		}}
	}
	return nil
}

// spotCheckBounds returns the indices to validate for a high-cardinality slice.
// When n <= highCardinalityThreshold, returns nil (meaning: validate all items).
// When n > highCardinalityThreshold, returns the first spotCheckCount and last
// spotCheckCount indices.
//
// Threshold rationale: 5000 items at ~200ns per item = ~1ms overhead on a cold
// path. Above 5000, spot-checking keeps overhead bounded to < 1ms regardless
// of database size. The spot check of 10 head + 10 tail catches structural drift
// (empty IDs, empty status) introduced by a bd schema change, which tends to
// affect all items, not random items.
const (
	highCardinalityThreshold = 5000
	spotCheckCount           = 10
)

// SpotCheckIndices returns the indices to validate for a slice of length n.
// Returns nil when n <= highCardinalityThreshold (meaning: validate all).
// Returns sorted head+tail indices otherwise.
func SpotCheckIndices(n int) []int {
	if n <= highCardinalityThreshold {
		return nil
	}
	seen := make(map[int]struct{})
	indices := make([]int, 0, spotCheckCount*2)
	for i := 0; i < spotCheckCount && i < n; i++ {
		if _, ok := seen[i]; !ok {
			seen[i] = struct{}{}
			indices = append(indices, i)
		}
	}
	for i := n - spotCheckCount; i < n; i++ {
		if i < 0 {
			continue
		}
		if _, ok := seen[i]; !ok {
			seen[i] = struct{}{}
			indices = append(indices, i)
		}
	}
	return indices
}

// SelectIssueSummaries returns a sub-slice of items at the given indices.
// When indices is nil, returns items unchanged (validate all).
func SelectIssueSummaries(items []domain.IssueSummary, indices []int) []domain.IssueSummary {
	if indices == nil {
		return items
	}
	out := make([]domain.IssueSummary, 0, len(indices))
	for _, i := range indices {
		if i >= 0 && i < len(items) {
			out = append(out, items[i])
		}
	}
	return out
}
