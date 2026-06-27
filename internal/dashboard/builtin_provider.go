package dashboard

import (
	"context"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

const (
	builtInDashboardIDDefault     = "default"
	builtInDashboardTitleDefault  = "Default"
	builtInSectionIDNotReady      = "not_ready"
	builtInSectionTitleNotReady   = "Not Ready"
	builtInSectionIDReady         = "ready"
	builtInSectionTitleReady      = "Ready"
	builtInSectionIDInProgress    = "in_progress"
	builtInSectionTitleInProgress = "In Progress"
	builtInSectionIDDone          = "done"
	builtInSectionTitleDone       = "Done"
	inProgressStatus              = "in_progress"
	doneStatus                    = "closed"
)

// BuiltInProvider is a metadata-only dashboard definition catalog.
// It supplies section IDs and titles; in v1 it also populates Section.Query
// for backward compatibility with the board model until the board-model
// query-routing migration moves repository query routing into the board model
// directly. Future
// providers should omit the Query field and rely on section ID and title only.
type BuiltInProvider struct {
}

var _ Provider = (*BuiltInProvider)(nil)

// NewBuiltInProvider creates a built-in dashboard provider.
func NewBuiltInProvider() *BuiltInProvider {
	return &BuiltInProvider{}
}

// Dashboards returns the built-in dashboard definition with four sections:
// not_ready, ready, in_progress, and done. Section IDs and titles are the
// stable contract. The Query field is populated only as a backward-compat shim
// for the current board model; it will be removed by the board-model migration.
func (p *BuiltInProvider) Dashboards(_ context.Context) ([]Definition, error) {
	sections := []Section{
		{
			ID:    builtInSectionIDNotReady,
			Title: builtInSectionTitleNotReady,
			Query: Query{Type: QueryTypeBlockedIssues, BlockedIssues: domain.BlockedIssuesQuery{Limit: 0}},
		},
		{
			ID:    builtInSectionIDReady,
			Title: builtInSectionTitleReady,
			Query: Query{Type: QueryTypeReadyIssues, ReadyIssues: domain.ReadyIssuesQuery{Limit: 0}},
		},
		{
			ID:    builtInSectionIDInProgress,
			Title: builtInSectionTitleInProgress,
			Query: Query{Type: QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{inProgressStatus}, Limit: 0}},
		},
		{
			ID:    builtInSectionIDDone,
			Title: builtInSectionTitleDone,
			Query: Query{
				Type: QueryTypeListIssues,
				ListIssues: domain.IssueListQuery{
					Statuses:  []string{doneStatus},
					SortBy:    domain.SortFieldClosedAt,
					SortOrder: domain.SortDirectionDescending,
					Limit:     0,
				},
			},
		},
	}

	return []Definition{{
		ID:       builtInDashboardIDDefault,
		Title:    builtInDashboardTitleDefault,
		Sections: sections,
	}}, nil
}
