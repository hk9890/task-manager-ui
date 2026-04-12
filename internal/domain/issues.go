package domain

import "time"

// IssueSummary is the compact issue projection used in list and queue views.
type IssueSummary struct {
	ID        string
	Title     string
	Status    string
	Type      string
	Priority  int
	Assignee  string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueReference identifies a related issue.
type IssueReference struct {
	ID       string
	Title    string
	Type     string
	Priority int
	Status   string
}

// ParentGroupBrowserContext is the normalized parent-group relationship
// projection used by the issue-details left browser panel.
type ParentGroupBrowserContext struct {
	Parent   IssueReference
	Children []IssueReference
}

// IssueComment is a normalized issue comment representation.
type IssueComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
}

// IssueDetail is the full issue read model for details and editing flows.
type IssueDetail struct {
	Summary            IssueSummary
	Creator            string
	Description        string
	Notes              string
	ClosedAt           time.Time
	CloseReason        string
	ParentGroupBrowser ParentGroupBrowserContext
	BlockedBy          []IssueReference
	Blocks             []IssueReference
	Related            []IssueReference
	Comments           []IssueComment
}

// BlockedIssueView is the blocked-work projection used by blocked dashboards.
type BlockedIssueView struct {
	Issue     IssueSummary
	BlockedBy []IssueReference
}

// SearchResult is a single matched issue for search responses.
type SearchResult struct {
	Issue   IssueSummary
	Snippet string
}

// SearchResultPage represents a paged search response from the gateway.
type SearchResultPage struct {
	Results []SearchResult
	Total   int
}
