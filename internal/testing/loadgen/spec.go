// Package loadgen produces seeded beads repositories from a workload spec.
// It is used by the bwb-loadtest CLI and can be imported directly by
// benchmarks or integration tests.
//
// # Reproducibility
//
// Two Generate calls with the same Spec and Seed produce identical workload
// SHAPE: counts per status, issue-creation order, dependency edge structure.
// Timestamps in the resulting .beads/ directory will differ because they
// reflect wall-clock time of bd subprocess calls.
//
// # Dep density budget
//
// Blocked-status issues require ≥1 incoming blocker dep. Those mandatory
// edges count against the dep-density budget. If density × total_issues <
// blocked_count, mandatory edges exceed the budget; Generate emits a warning
// and proceeds without error.
//
// # Comments
//
// CommentsPer > 0 enables optional comment generation via bd subprocess (one
// call per comment). This is slow at scale — roughly O(N × CommentsPer) bd
// invocations. For large corpora, prefer CommentsPer=0 for the seeding phase
// and add comments in a post-process step.
package loadgen

// DefaultPriorities is the default priority distribution used when Spec.Priorities
// is nil or empty. P2 is dominant; P0 and P4 are rare.
var DefaultPriorities = map[int]float64{
	0: 0.05,
	1: 0.20,
	2: 0.60,
	3: 0.10,
	4: 0.05,
}

// Spec describes the workload shape to generate.
type Spec struct {
	// Counts maps status name to the number of issues with that status.
	// Valid keys: "open", "in_progress", "blocked", "closed".
	// Zero values are allowed (no issues with that status).
	Counts map[string]int

	// DepDensity is the average number of dependency edges per issue,
	// independent of mandatory blocker edges for blocked issues.
	// Fractional values are supported (e.g. 0.5 means ~1 edge per 2 issues).
	DepDensity float64

	// CommentsPer is the average number of comments per issue.
	// 0 disables comment generation entirely (recommended for large corpora).
	// Each comment requires a bd subprocess call; this is slow at scale.
	CommentsPer int

	// Priorities maps priority level (0–4) to a sampling weight.
	// Weights are normalized before sampling; need not sum to 1.
	// If nil or empty, DefaultPriorities is used.
	Priorities map[int]float64

	// Seed is mandatory. The same Seed + Spec produces identical workload shape.
	Seed int64
}

// TotalIssues returns the sum of all counts in the spec.
func (s Spec) TotalIssues() int {
	total := 0
	for _, n := range s.Counts {
		total += n
	}
	return total
}

// Manifest describes the actual generated workload.
type Manifest struct {
	// Spec is the spec that was used to generate this workload.
	Spec Spec `json:"spec"`

	// ActualCounts is the number of issues by status in the generated repo.
	ActualCounts map[string]int `json:"actual_counts"`

	// ActualEdges is the total number of dep edges created.
	ActualEdges int `json:"actual_edges"`

	// IssuesPath is the absolute path to the .beads/ directory.
	IssuesPath string `json:"issues_path"`

	// BdVersion is the output of `bd --version` at generation time.
	BdVersion string `json:"bd_version"`

	// Warnings holds non-fatal messages emitted during generation
	// (e.g. density budget exceeded by mandatory blocker edges).
	Warnings []string `json:"warnings,omitempty"`
}

// issuePlan is the internal plan for one issue before it is created in bd.
type issuePlan struct {
	title    string
	status   string
	priority int
}

// edgePlan is the internal plan for one dep edge before it is created in bd.
// blocker blocks blocked. Both indices reference the creation-order slice.
type edgePlan struct {
	blockerIdx int // index in issuePlans; blocker is created before blocked
	blockedIdx int
}

// plan is the complete pre-bd plan derived from a Spec.
// It contains no I/O state and is fully deterministic.
type plan struct {
	issues []issuePlan
	edges  []edgePlan
}
