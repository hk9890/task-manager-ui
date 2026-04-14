package dashboard

import (
	"context"
	"fmt"
	"strings"

	"github.com/hk9890/beads-workbench/internal/domain"
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
// The query contract is intentionally narrow: Type must be one of the
// supported query kinds used by the board renderer.
//
// Payload fields are value types, so this contract cannot reliably distinguish
// "unset" from "zero value" payload shapes. Validation therefore enforces the
// supported Type and treats non-selected payload fields as ignored.
type Query struct {
	Type          QueryType
	ListIssues    domain.IssueListQuery
	ReadyIssues   domain.ReadyIssuesQuery
	BlockedIssues domain.BlockedIssuesQuery
}

// ValidateDefinitions validates provider output before the board consumes it.
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
			if err := ValidateQuery(section.Query); err != nil {
				return fmt.Errorf("dashboard[%d] section[%d]: %w", dashboardIndex, sectionIndex, err)
			}
		}
	}

	return nil
}

// ValidateQuery ensures the section query uses a supported query type.
func ValidateQuery(query Query) error {
	if !isSupportedQueryType(query.Type) {
		return fmt.Errorf("unsupported query type %q", query.Type)
	}

	return nil
}

func isSupportedQueryType(queryType QueryType) bool {
	switch queryType {
	case QueryTypeListIssues, QueryTypeReadyIssues, QueryTypeBlockedIssues:
		return true
	default:
		return false
	}
}
