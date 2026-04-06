package dashboard

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// DashboardDefinitionProvider loads dashboard definitions independently from
// board rendering.
type DashboardDefinitionProvider interface {
	Dashboards(ctx context.Context) ([]Definition, error)
}

// Definition describes one dashboard and its ordered sections.
type Definition struct {
	ID       string
	Title    string
	Sections []Section
}

// Section describes one queue shown on a dashboard.
type Section struct {
	ID    string
	Title string
	Query Query
}

// QueryType identifies which supported gateway query contract backs a section.
type QueryType string

const (
	QueryTypeListIssues    QueryType = "list_issues"
	QueryTypeReadyIssues   QueryType = "ready_issues"
	QueryTypeBlockedIssues QueryType = "blocked_issues"
)

// Query describes the supported gateway query that backs a board section.
//
// Exactly one concrete query payload should be populated based on Type.
type Query struct {
	Type          QueryType
	ListIssues    domain.IssueListQuery
	ReadyIssues   domain.ReadyIssuesQuery
	BlockedIssues domain.BlockedIssuesQuery
}
