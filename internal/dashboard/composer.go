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
type Inputs struct {
	Ready       []domain.IssueSummary
	Blocked     []domain.BlockedIssueView
	InProgress  []domain.IssueSummary
	Closed      []domain.IssueSummary
	ClosedLimit int // the cap that was sent to bd; used to compute "N+" badge
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
// For the Done column, Threshold is -1 (sentinel) and signals that the
// backend's expected closed_at-desc ordering appears to be violated.
// The caller should log a "backend sort assumption broken" alert when it
// observes Threshold == -1.
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

	// --- defensive sort check for Done (backend closed_at-desc assumption) ---
	// IssueSummary does not carry ClosedAt, so UpdatedAt is used as a proxy for
	// the backend's closed_at sort. When the first two items are not in
	// descending order the assumption is flagged.
	//
	// Threshold -1 is a sentinel: it signals a broken sort assumption, not a
	// population-size concern. The caller should log "backend sort assumption
	// broken" when it observes Threshold == -1.
	if len(in.Closed) >= 2 {
		first, second := in.Closed[0], in.Closed[1]
		// "non-descending" means first.UpdatedAt <= second.UpdatedAt,
		// i.e. the later item is >= the earlier one — backend order broken.
		if !first.UpdatedAt.After(second.UpdatedAt) {
			warnings = append(warnings, CardinalityWarning{
				Group:     "Closed",
				Count:     len(in.Closed),
				Threshold: -1, // sentinel: broken backend sort assumption
			})
		}
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
	// TotalIsExact is false when the returned slice hit the cap, signalling
	// that there may be more closed items beyond what bd returned.
	totalIsExact := in.ClosedLimit <= 0 || len(closedIssues) < in.ClosedLimit
	done := ColumnData{
		Issues:       closedIssues,
		Total:        len(closedIssues),
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
