package dashboard

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
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

// BuiltInProvider is a dashboard definition provider backed by built-in queue
// definitions mapped to supported gateway query contracts.
type BuiltInProvider struct {
}

var _ Provider = (*BuiltInProvider)(nil)

// NewBuiltInProvider creates a built-in dashboard provider.
func NewBuiltInProvider() *BuiltInProvider {
	return &BuiltInProvider{}
}

// Dashboards returns built-in dashboard definitions.
func (p *BuiltInProvider) Dashboards(_ context.Context) ([]Definition, error) {
	sections := []Section{
		notReadySection(),
		readySection(),
		inProgressSection(),
		doneSection(),
	}

	return []Definition{{
		ID:       builtInDashboardIDDefault,
		Title:    builtInDashboardTitleDefault,
		Sections: sections,
	}}, nil
}

func notReadySection() Section {
	return Section{
		ID:    builtInSectionIDNotReady,
		Title: builtInSectionTitleNotReady,
		Query: Query{
			Type:          QueryTypeBlockedIssues,
			BlockedIssues: domain.BlockedIssuesQuery{Limit: 0},
		},
	}
}

func readySection() Section {
	return Section{
		ID:    builtInSectionIDReady,
		Title: builtInSectionTitleReady,
		Query: Query{
			Type:        QueryTypeReadyIssues,
			ReadyIssues: domain.ReadyIssuesQuery{Limit: 0},
		},
	}
}

func inProgressSection() Section {
	return Section{
		ID:    builtInSectionIDInProgress,
		Title: builtInSectionTitleInProgress,
		Query: Query{
			Type: QueryTypeListIssues,
			ListIssues: domain.IssueListQuery{
				Statuses: []string{inProgressStatus},
				Limit:    0,
			},
		},
	}
}

func doneSection() Section {
	return Section{
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
	}
}
