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
	// SortFieldClosedAt sorts by issue close date. Use only with status=closed filters;
	// behavior for non-closed issues is undefined.
	SortFieldClosedAt SortField = "closed_at"
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
	// Limit is the maximum number of issues to return. A value of 0 means the
	// caller (e.g. the board model) will set an appropriate display limit before
	// dispatching. Custom dashboard providers must not assume Limit > 0.
	Limit     int
	Offset    int
	SortBy    SortField
	SortOrder SortDirection
}

// ReadyIssuesQuery requests ready-work queues.
type ReadyIssuesQuery struct {
	// Limit is the maximum number of issues to return. A value of 0 means the
	// caller (e.g. the board model) will set an appropriate display limit before
	// dispatching. Custom dashboard providers must not assume Limit > 0.
	Limit  int
	Offset int
}

// BlockedIssuesQuery requests blocked-work queues.
type BlockedIssuesQuery struct {
	// Limit is the maximum number of issues to return. A value of 0 means the
	// caller (e.g. the board model) will set an appropriate display limit before
	// dispatching. Custom dashboard providers must not assume Limit > 0.
	Limit  int
	Offset int
}

// ShowIssueQuery identifies a single issue to load.
type ShowIssueQuery struct {
	IssueID string
}

// IssueCountQuery requests issue counts. Statuses/Types/Assignee/Labels narrow the
// counted population. Empty fields = count all. Pass Statuses:[]string{"closed"} to
// count only closed issues.
type IssueCountQuery struct {
	Statuses []string
	Types    []string
	Assignee string
	Labels   []string
}

// IssueStatusCount holds the count for a single issue status group.
type IssueStatusCount struct {
	Status string
	Count  int
}

// IssueCountResult is the result of a CountIssues call.
// Groups contains only non-zero status entries (zero-count groups are omitted by bd count).
// Total is the sum of all group counts.
type IssueCountResult struct {
	Groups []IssueStatusCount
	Total  int
}

// QueryOptions controls filtering and pagination for the generic Query gateway method.
type QueryOptions struct {
	// Limit is the maximum number of issues to return. A value of 0 means uncapped.
	Limit int
	// Offset is used with Limit for paginated callers (bd receives Limit+Offset, caller receives page).
	Offset int
	// IncludeClosed includes closed issues in results (maps to bd's -a flag).
	IncludeClosed bool
	// SortBy identifies the sort field. An empty/zero value means no sort override.
	SortBy SortField
	// SortOrder controls ascending vs descending. Descending maps to bd's -r flag.
	SortOrder SortDirection
}

// ReadyExplainOptions controls the ReadyExplain gateway call.
type ReadyExplainOptions struct {
	// Limit is the maximum number of issues to return per section. A value of 0 means uncapped.
	Limit int
}

// ReadyExplainResult is the result of a ReadyExplain call. It combines ready
// and dependency-blocked issues with aggregate counts from a single bd invocation.
type ReadyExplainResult struct {
	Ready        []IssueSummary
	Blocked      []BlockedIssueView
	TotalReady   int
	TotalBlocked int
	CycleCount   int
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
