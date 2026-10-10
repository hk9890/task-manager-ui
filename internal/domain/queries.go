package domain

// WorkStateFilter narrows search results to readiness/blocking queues.
type WorkStateFilter string

const (
	WorkStateAny     WorkStateFilter = ""
	WorkStateReady   WorkStateFilter = "ready"
	WorkStateBlocked WorkStateFilter = "blocked"
)

// ReadyExplainResult is the result of a ReadyExplain call. It combines ready
// and dependency-blocked issues with aggregate counts from a single taskmgr invocation.
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

	// IncludeClosed widens the search to the closed history. It defaults to
	// false: a store accumulates closed issues without bound (this project's own
	// runs ~880 closed against ~10 open), so including them by default buries
	// every live issue under years of finished work.
	IncludeClosed bool

	Statuses []string
	Types    []string
	Labels   []string
	Assignee string

	// PriorityMin/PriorityMax are the inclusive priority bounds.
	// Use both set to the same value to request an exact priority.
	PriorityMin *int
	PriorityMax *int

	// WorkState narrows the result to ready or blocked issues, in addition to
	// every other filter.
	WorkState WorkStateFilter

	Limit  int
	Offset int
}
