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

	Statuses []string
	Types    []string
	Labels   []string
	Assignee string

	// PriorityMin/PriorityMax align to the official `taskmgr search` filter surface.
	// Use both set to the same value to request an exact priority.
	PriorityMin *int
	PriorityMax *int

	// WorkState maps to readiness/blocking narrowing used by browse/search flows.
	// Note: `taskmgr search` does not currently expose ready/blocked flags directly.
	// Repository implementations may route through `taskmgr ready`/`taskmgr blocked` and apply
	// additional filters in-memory to preserve a stable UI-facing API.
	WorkState WorkStateFilter

	Limit  int
	Offset int
}
