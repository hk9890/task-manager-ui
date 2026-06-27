package dashboard

import (
	"context"
	"fmt"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// Provider loads dashboard definitions independently from board rendering.
type Provider interface {
	Dashboards(ctx context.Context) ([]Definition, error)
}

// Definition describes one dashboard and its ordered sections.
type Definition struct {
	ID       string
	Title    string
	Sections []Section
}

// Section describes one queue shown on a dashboard.
// Providers supply IDs and titles only; the board model owns repository query
// routing for each section.
type Section struct {
	ID    string
	Title string

	// Query is a backward-compatibility shim retained until the board-model
	// query-routing migration moves repository calls into the board model. New providers should
	// leave this field at its zero value. It will be removed once the board model
	// no longer reads it.
	Query Query
}

// QueryType identifies which supported repository query contract backs a section.
// This type is a backward-compatibility shim retained until the board-model
// query-routing migration. New code should not reference
// QueryType from dashboard definitions.
type QueryType string

const (
	QueryTypeListIssues    QueryType = "list_issues"
	QueryTypeReadyIssues   QueryType = "ready_issues"
	QueryTypeBlockedIssues QueryType = "blocked_issues"
)

// Query describes the repository query that backed a board section in the
// legacy architecture. It is a backward-compatibility shim retained until the
// board-model query-routing migration moves repository query routing into the
// board model directly. New providers should not set this field.
type Query struct {
	Type          QueryType
	ListIssues    domain.IssueListQuery
	ReadyIssues   domain.ReadyIssuesQuery
	BlockedIssues domain.BlockedIssuesQuery
}

// ValidateDefinitions validates provider output before the board consumes it.
// Validation checks IDs, titles, and non-empty sections. Query payload
// validation is no longer enforced here; the board model owns query routing.
func ValidateDefinitions(defs []Definition) error {
	if len(defs) == 0 {
		return fmt.Errorf("dashboard provider returned zero definitions")
	}

	for dashboardIndex, def := range defs {
		if strings.TrimSpace(def.ID) == "" {
			return fmt.Errorf("dashboard[%d]: id is required", dashboardIndex)
		}
		if strings.TrimSpace(def.Title) == "" {
			return fmt.Errorf("dashboard[%d]: title is required", dashboardIndex)
		}
		if len(def.Sections) == 0 {
			return fmt.Errorf("dashboard[%d]: at least one section is required", dashboardIndex)
		}

		for sectionIndex, section := range def.Sections {
			if strings.TrimSpace(section.ID) == "" {
				return fmt.Errorf("dashboard[%d] section[%d]: id is required", dashboardIndex, sectionIndex)
			}
			if strings.TrimSpace(section.Title) == "" {
				return fmt.Errorf("dashboard[%d] section[%d]: title is required", dashboardIndex, sectionIndex)
			}
		}
	}

	return nil
}
