package dashboard

import (
	"sort"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// cardinalityThreshold is the population size above which a CardinanlityWarning
// is emitted for active groups (Ready, Blocked, InProgress).
const cardinalityThreshold = 500

// Inputs carries the five data groups returned by the dashboard fetch plan.
// ClosedLimit is the cap that was sent to bd; it is used to compute the "N+"
// badge on the Done column.
// ClosedTotal is the real DB population count of closed issues (from CountIssues).
// When > 0 it overrides len(Closed) as Done.Total so the header shows the
// actual count rather than the capped slice size.
//
// StoredBlocked holds issues whose stored status == "blocked" (from
// Query("status=blocked")). These are distinct from Blocked (dependency-blocked
// issues from ReadyExplain). StoredBlocked issues that have no dependency
// blocker are silently excluded from ReadyExplain.Blocked; without this field
// they would fall through all four board columns and become invisible.
// Compose deduplicates the union of Blocked and StoredBlocked into the
// NotReady column so both populations are visible.
type Inputs struct {
	Ready         []domain.IssueSummary
	Blocked       []domain.BlockedIssueView
	StoredBlocked []domain.IssueSummary
	InProgress    []domain.IssueSummary
	Closed        []domain.IssueSummary
	ClosedLimit   int // the cap that was sent to bd; used to compute "N+" badge
	ClosedTotal   int // real DB count of closed issues; 0 means unset (falls back to len(Closed))
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

// mergeNotReadyIssues returns the union of dep-blocked issues (from ReadyExplain)
// and stored-blocked issues (from Query("status=blocked")), cross-deduplicated by
// ID so that an issue present in BOTH populations appears exactly once.
//
// Within each source slice, duplicates are preserved unchanged (consistent with
// the pre-existing behaviour for dep-blocked issues). Cross-source dedup uses
// the dep-blocked source as authoritative so its richer BlockedBy fields are
// retained when a conflict occurs.
func mergeNotReadyIssues(blocked []domain.BlockedIssueView, storedBlocked []domain.IssueSummary) []domain.IssueSummary {
	if len(storedBlocked) == 0 {
		return mapBlockedToSummaries(blocked)
	}
	if len(blocked) == 0 {
		out := make([]domain.IssueSummary, len(storedBlocked))
		copy(out, storedBlocked)
		return out
	}

	// Build a set of IDs already covered by the dep-blocked list.
	depBlockedIDs := make(map[string]struct{}, len(blocked))
	for _, b := range blocked {
		depBlockedIDs[b.Issue.ID] = struct{}{}
	}

	// Start with all dep-blocked issues (preserving any intra-slice duplicates).
	out := make([]domain.IssueSummary, 0, len(blocked)+len(storedBlocked))
	for _, b := range blocked {
		out = append(out, b.Issue)
	}
	// Append stored-blocked issues whose IDs are not already covered.
	for _, s := range storedBlocked {
		if _, covered := depBlockedIDs[s.ID]; !covered {
			out = append(out, s)
		}
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

	// --- build NotReady column (union of dep-blocked and stored-blocked) ---
	// mergeNotReadyIssues deduplicates by ID so an issue present in both
	// populations appears exactly once.
	notReadyIssues := mergeNotReadyIssues(in.Blocked, in.StoredBlocked)
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
