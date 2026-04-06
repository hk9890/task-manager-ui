package domain

// SortDirection controls ascending vs descending sort behavior.
type SortDirection string

const (
	SortDirectionAscending  SortDirection = "asc"
	SortDirectionDescending SortDirection = "desc"
)

// SortField identifies supported issue sort fields.
type SortField string

const (
	SortFieldUpdatedAt SortField = "updated_at"
	SortFieldCreatedAt SortField = "created_at"
	SortFieldPriority  SortField = "priority"
	SortFieldID        SortField = "id"
)

// WorkStateFilter narrows search results to readiness/blocking queues.
type WorkStateFilter string

const (
	WorkStateAny     WorkStateFilter = ""
	WorkStateReady   WorkStateFilter = "ready"
	WorkStateBlocked WorkStateFilter = "blocked"
)

// IssueListQuery is the generic list query used by browse views.
type IssueListQuery struct {
	Statuses  []string
	Types     []string
	Assignee  string
	Labels    []string
	Limit     int
	Offset    int
	SortBy    SortField
	SortOrder SortDirection
}

// ReadyIssuesQuery requests ready-work queues.
type ReadyIssuesQuery struct {
	Limit  int
	Offset int
}

// BlockedIssuesQuery requests blocked-work queues.
type BlockedIssuesQuery struct {
	Limit  int
	Offset int
}

// ShowIssueQuery identifies a single issue to load.
type ShowIssueQuery struct {
	IssueID string
}

// SearchIssuesQuery requests text and structured search.
type SearchIssuesQuery struct {
	Text string

	Statuses []string
	Types    []string
	Labels   []string
	Assignee string

	// PriorityMin/PriorityMax align to the official `bd search` filter surface.
	// Use both set to the same value to request an exact priority.
	PriorityMin *int
	PriorityMax *int

	// WorkState maps to readiness/blocking narrowing used by browse/search flows.
	// Note: `bd search` does not currently expose ready/blocked flags directly.
	// Gateway implementations may route through `bd ready`/`bd blocked` and apply
	// additional filters in-memory to preserve a stable UI-facing API.
	WorkState WorkStateFilter

	Limit  int
	Offset int
}
