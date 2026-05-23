package repository

import "github.com/hk9890/beads-workbench/internal/domain"

// DashboardData is the composite board snapshot returned by Repository.Dashboard.
// All per-issue fields use domain.* types directly; this struct is the only
// place the repository package aggregates them.
//
// Note: Blocked contains []domain.IssueSummary (compact projections), while
// domain.ReadyExplainResult.Blocked contains []domain.BlockedIssueView (with
// blocker references). Callers that need the full blocked-by graph should
// inspect ReadyExplain.Blocked directly.
type DashboardData struct {
	ReadyExplain domain.ReadyExplainResult
	InProgress   []domain.IssueSummary
	Closed       []domain.IssueSummary
	ClosedTotal  int
	Blocked      []domain.IssueSummary
}

// Catalogs holds the selectable option sets for create and edit forms.
// See Repository.Catalogs for freshness semantics.
type Catalogs struct {
	Statuses []domain.StatusOption
	Types    []domain.TypeOption
	Labels   []domain.LabelOption
}
