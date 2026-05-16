package dashboard

import (
	"sort"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// cardinalityThreshold is the population size above which a CardinanlityWarning
// is emitted for active groups (Ready, Blocked, InProgress).
const cardinalityThreshold = 500

// Inputs carries the four data groups returned by the dashboard fetch plan.
// ClosedLimit is the cap that was sent to bd; it is used to compute the "N+"
// badge on the Done column.
// ClosedTotal is the real DB population count of closed issues (from CountIssues).
// When > 0 it overrides len(Closed) as Done.Total so the header shows the
// actual count rather than the capped slice size.
type Inputs struct {
	Ready       []domain.IssueSummary
	Blocked     []domain.BlockedIssueView
	InProgress  []domain.IssueSummary
	Closed      []domain.IssueSummary
	ClosedLimit int // the cap that was sent to bd; used to compute "N+" badge
	ClosedTotal int // real DB count of closed issues; 0 means unset (falls back to len(Closed))
}

// Columns is the typed column data the board renderer consumes.
type Columns struct {
	NotReady   ColumnData // from Blocked
	Ready      ColumnData
	InProgress ColumnData
	Done       ColumnData
	Warnings   []CardinalityWarning
}

// ColumnData holds the issues and totals for a single board column.
type ColumnData struct {
	Issues       []domain.IssueSummary
	Total        int
	TotalIsExact bool // false when "N+" should be rendered (closed only)
}

// CardinalityWarning is a data-level signal returned to the caller so that
// logging decisions are not made inside the pure composer function.
//
// For active groups (Ready, Blocked, InProgress), Threshold is 500.
// Backend sort ordering for the Done column is no longer checked here;
// it is verified by the sort-parity test (izds) against real bd data.
type CardinalityWarning struct {
	Group     string // "Ready", "Blocked", "InProgress", "Closed"
	Count     int
	Threshold int
}

// issueSort sorts a slice of IssueSummary in-place using the standard active-
// column ordering: Priority ascending, UpdatedAt descending, ID ascending.
// A stable sort is used so that equal items preserve their original relative
// order within each tie-break group.
func issueSort(issues []domain.IssueSummary) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
}

// mapBlockedToSummaries extracts the IssueSummary from each BlockedIssueView
// so the NotReady column rows are uniform with other active columns.
func mapBlockedToSummaries(blocked []domain.BlockedIssueView) []domain.IssueSummary {
	if len(blocked) == 0 {
		return nil
	}
	out := make([]domain.IssueSummary, len(blocked))
	for i, b := range blocked {
		out[i] = b.Issue
	}
	return out
}

// Compose is a pure function — no I/O, no globals, no logging — that combines
// the four data groups into the typed Columns value consumed by the board
// renderer.
func Compose(in Inputs) Columns {
	var warnings []CardinalityWarning

	// --- cardinality warnings for active groups ---
	if len(in.Ready) > cardinalityThreshold {
		warnings = append(warnings, CardinalityWarning{
			Group:     "Ready",
			Count:     len(in.Ready),
			Threshold: cardinalityThreshold,
		})
	}
	if len(in.Blocked) > cardinalityThreshold {
		warnings = append(warnings, CardinalityWarning{
			Group:     "Blocked",
			Count:     len(in.Blocked),
			Threshold: cardinalityThreshold,
		})
	}
	if len(in.InProgress) > cardinalityThreshold {
		warnings = append(warnings, CardinalityWarning{
			Group:     "InProgress",
			Count:     len(in.InProgress),
			Threshold: cardinalityThreshold,
		})
	}

	// --- build NotReady column (from Blocked) ---
	notReadyIssues := mapBlockedToSummaries(in.Blocked)
	issueSort(notReadyIssues)
	notReady := ColumnData{
		Issues:       notReadyIssues,
		Total:        len(notReadyIssues),
		TotalIsExact: true,
	}

	// --- build Ready column ---
	readyIssues := make([]domain.IssueSummary, len(in.Ready))
	copy(readyIssues, in.Ready)
	issueSort(readyIssues)
	ready := ColumnData{
		Issues:       readyIssues,
		Total:        len(readyIssues),
		TotalIsExact: true,
	}

	// --- build InProgress column ---
	inProgressIssues := make([]domain.IssueSummary, len(in.InProgress))
	copy(inProgressIssues, in.InProgress)
	issueSort(inProgressIssues)
	inProgress := ColumnData{
		Issues:       inProgressIssues,
		Total:        len(inProgressIssues),
		TotalIsExact: true,
	}

	// --- build Done column ---
	// Preserve backend order; do not re-sort.
	closedIssues := make([]domain.IssueSummary, len(in.Closed))
	copy(closedIssues, in.Closed)

	// Use the real DB population count when available; fall back to len(closedIssues).
	doneTotal := len(closedIssues)
	if in.ClosedTotal > doneTotal {
		doneTotal = in.ClosedTotal
	}

	// TotalIsExact is true when the visible list covers the entire population
	// (i.e. no items are hidden beyond the rendered cap). When ClosedTotal is
	// set, exact = visible list reaches or matches the real count.
	// When ClosedTotal is unset (0), fall back to the old ClosedLimit heuristic.
	var totalIsExact bool
	if in.ClosedTotal > 0 {
		totalIsExact = len(closedIssues) >= in.ClosedTotal
	} else {
		totalIsExact = in.ClosedLimit <= 0 || len(closedIssues) < in.ClosedLimit
	}

	done := ColumnData{
		Issues:       closedIssues,
		Total:        doneTotal,
		TotalIsExact: totalIsExact,
	}

	return Columns{
		NotReady:   notReady,
		Ready:      ready,
		InProgress: inProgress,
		Done:       done,
		Warnings:   warnings,
	}
}
