package loadgen

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// statusOrder defines the creation order of issues by status.
// All open issues are created first, then in_progress, blocked, closed.
// This order is deterministic and ensures blocked issues can always find
// an earlier-created issue to act as their blocker.
var statusOrder = []string{"open", "in_progress", "blocked", "closed"}

// buildPlan constructs the deterministic issue + edge plan from a Spec.
// This is a pure function: no I/O, same Spec+Seed → same plan every time.
func buildPlan(spec Spec) plan {
	rng := rand.New(rand.NewSource(spec.Seed)) //nolint:gosec // not crypto

	priorities := spec.Priorities
	if len(priorities) == 0 {
		priorities = DefaultPriorities
	}

	// Build a weighted priority sampler.
	type pw struct {
		priority int
		weight   float64
	}
	var pws []pw
	for p, w := range priorities {
		if w > 0 {
			pws = append(pws, pw{p, w})
		}
	}
	// Sort for determinism (map iteration order is random).
	sort.Slice(pws, func(i, j int) bool { return pws[i].priority < pws[j].priority })
	totalWeight := 0.0
	for _, pw := range pws {
		totalWeight += pw.weight
	}

	samplePriority := func() int {
		r := rng.Float64() * totalWeight
		cumulative := 0.0
		for _, pw := range pws {
			cumulative += pw.weight
			if r < cumulative {
				return pw.priority
			}
		}
		return pws[len(pws)-1].priority
	}

	// Build issue plans in deterministic order.
	var issues []issuePlan
	issueNum := 0
	for _, status := range statusOrder {
		count := spec.Counts[status]
		for i := 0; i < count; i++ {
			issueNum++
			issues = append(issues, issuePlan{
				title:    fmt.Sprintf("load-test issue %d", issueNum),
				status:   status,
				priority: samplePriority(),
			})
		}
	}

	total := len(issues)
	if total == 0 {
		return plan{issues: issues}
	}

	// Find indices of blocked issues.
	var blockedIdxs []int
	for i, iss := range issues {
		if iss.status == "blocked" {
			blockedIdxs = append(blockedIdxs, i)
		}
	}

	// Budget calculation.
	// Total edges = round(DepDensity * total)
	// Mandatory edges for blocked issues (one per blocked issue) come first.
	totalEdgeBudget := int(math.Round(spec.DepDensity * float64(total)))
	mandatoryEdges := len(blockedIdxs)

	var warnings []string
	if mandatoryEdges > totalEdgeBudget && spec.DepDensity > 0 {
		warnings = append(warnings,
			fmt.Sprintf("dep-density budget (%d edges) is less than mandatory blocker edges (%d); actual density will exceed spec",
				totalEdgeBudget, mandatoryEdges))
	}
	_ = warnings // returned via plan; we embed in generate

	// edgeSet tracks (blockerIdx, blockedIdx) pairs to avoid duplicates.
	type edgeKey struct{ blocker, blocked int }
	edgeSet := make(map[edgeKey]struct{})

	var edges []edgePlan

	// Mandatory edges: each blocked issue needs ≥1 incoming blocker dep.
	// A blocker must be created earlier (lower index) than the blocked issue.
	// Because statusOrder places open/in_progress before blocked, all blocked
	// issues have index ≥ open+in_progress count in normal usage.
	//
	// If open+in_progress == 0, the first blocked issue is at index 0 and has
	// no earlier issue to serve as its blocker. This is a spec-level constraint
	// violation; planWarnings surfaces it and Generate warns at call time.
	for _, blockedIdx := range blockedIdxs {
		if blockedIdx == 0 {
			// No earlier issues available; mandatory edge cannot be satisfied.
			// Warning is emitted by planWarnings.
			continue
		}
		blockerIdx := rng.Intn(blockedIdx)
		k := edgeKey{blockerIdx, blockedIdx}
		edgeSet[k] = struct{}{}
		edges = append(edges, edgePlan{blockerIdx: blockerIdx, blockedIdx: blockedIdx})
	}

	// Optional edges: fill up to budget with random (blocker→blocked) pairs
	// where blocker < blocked in creation order (no cycles by construction).
	extraBudget := totalEdgeBudget - mandatoryEdges
	if extraBudget < 0 {
		extraBudget = 0
	}

	maxAttempts := extraBudget * 10 // avoid infinite loops on dense/small graphs
	attempts := 0
	for len(edges)-mandatoryEdges < extraBudget && attempts < maxAttempts {
		attempts++
		if total < 2 {
			break
		}
		// blocked must be > 0 to have a valid blocker
		blockedIdx := 1 + rng.Intn(total-1)
		blockerIdx := rng.Intn(blockedIdx)
		k := edgeKey{blockerIdx, blockedIdx}
		if _, dup := edgeSet[k]; dup {
			continue
		}
		edgeSet[k] = struct{}{}
		edges = append(edges, edgePlan{blockerIdx: blockerIdx, blockedIdx: blockedIdx})
	}

	return plan{issues: issues, edges: edges}
}

// planWarnings returns any warnings that apply to a plan given its spec.
func planWarnings(spec Spec, p plan) []string {
	total := len(p.issues)
	if total == 0 {
		return nil
	}

	var warnings []string

	blockedCount := spec.Counts["blocked"]
	totalEdgeBudget := int(math.Round(spec.DepDensity * float64(total)))
	if blockedCount > totalEdgeBudget && spec.DepDensity > 0 {
		warnings = append(warnings,
			fmt.Sprintf("dep-density budget (%d edges) is less than mandatory blocker edges (%d); actual density will exceed spec",
				totalEdgeBudget, blockedCount))
	}

	// If open+in_progress == 0 and blocked > 0, the first blocked issue(s)
	// have no earlier issues available to serve as blockers. The invariant
	// (every blocked issue has ≥1 incoming blocker dep) cannot be satisfied
	// for those issues. Warn and proceed; the dep edge is simply omitted.
	preBlockedCount := spec.Counts["open"] + spec.Counts["in_progress"]
	if blockedCount > 0 && preBlockedCount == 0 {
		warnings = append(warnings,
			fmt.Sprintf("blocked issues with no preceding open/in_progress issues: "+
				"the first blocked issue (index 0) cannot satisfy the ≥1 blocker invariant; "+
				"it will be created with status=blocked but no incoming dep edge"))
	}

	return warnings
}

// bdCommander is the seam for all bd subprocess calls made by Generate.
// Tests inject a fake to avoid real subprocess forks.
type bdCommander interface {
	// version returns the bd version string (output of `bd --version`).
	version() (string, error)
	// init initialises a new beads repository in dir.
	init(dir string) error
	// create creates a new issue and returns its assigned ID.
	create(dir, title string, priority int) (string, error)
	// run executes an arbitrary bd subcommand in dir.
	run(dir string, args ...string) error
}

// realBdCommander implements bdCommander using exec.Command and the package-level
// subprocess helpers (bdVersionString, bdRun, bdCreate).
type realBdCommander struct{}

func (realBdCommander) version() (string, error) { return bdVersionString() }
func (realBdCommander) init(dir string) error {
	return bdRun(dir, "init", "--prefix", "lt", "--non-interactive")
}
func (realBdCommander) create(dir, title string, priority int) (string, error) {
	return bdCreate(dir, title, priority)
}
func (realBdCommander) run(dir string, args ...string) error { return bdRun(dir, args...) }

// Generate creates a seeded beads repository in outDir from the given spec.
// outDir must exist. bd must be on PATH.
//
// Generate is idempotent only in the sense that successive calls with the same
// spec produce the same structural shape; each call runs bd subprocess chains
// that take real time proportional to TotalIssues × CommentsPer.
func Generate(spec Spec, outDir string) (*Manifest, error) {
	// Verify bd is available.
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return nil, fmt.Errorf("loadgen: bd not found on PATH — install beads and ensure bd is executable: %w", err)
	}
	_ = bdPath

	return generateWith(spec, outDir, realBdCommander{})
}

// generateWith is the injectable core of Generate. It accepts a bdCommander
// seam so unit tests can substitute a fake without forking real bd subprocesses.
func generateWith(spec Spec, outDir string, cmdr bdCommander) (*Manifest, error) {
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return nil, fmt.Errorf("loadgen: resolve outDir: %w", err)
	}
	if info, err := os.Stat(absOut); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("loadgen: outDir %q must be an existing directory: %w", absOut, err)
	}

	// Capture bd version.
	bdVersion, err := cmdr.version()
	if err != nil {
		return nil, fmt.Errorf("loadgen: bd --version: %w", err)
	}

	// Build the deterministic plan.
	p := buildPlan(spec)
	warnings := planWarnings(spec, p)

	// Initialize the beads repo.
	if err := cmdr.init(absOut); err != nil {
		return nil, fmt.Errorf("loadgen: bd init: %w", err)
	}

	// Create issues in plan order, collecting their IDs.
	ids := make([]string, len(p.issues))
	actualCounts := make(map[string]int)
	for i, iss := range p.issues {
		id, err := cmdr.create(absOut, iss.title, iss.priority)
		if err != nil {
			return nil, fmt.Errorf("loadgen: bd create issue %d: %w", i, err)
		}
		ids[i] = id

		// Set non-open status.
		if iss.status != "open" {
			if err := cmdr.run(absOut, "update", id, "--status", iss.status); err != nil {
				return nil, fmt.Errorf("loadgen: bd update status for %s: %w", id, err)
			}
		}
		actualCounts[iss.status]++

		// Add comments if requested.
		for c := 0; c < spec.CommentsPer; c++ {
			comment := fmt.Sprintf("load-test comment %d", c+1)
			if err := cmdr.run(absOut, "comment", id, comment); err != nil {
				return nil, fmt.Errorf("loadgen: bd comment %s: %w", id, err)
			}
		}
	}

	// Wire dependency edges.
	for _, edge := range p.edges {
		blockerID := ids[edge.blockerIdx]
		blockedID := ids[edge.blockedIdx]
		// bd dep add <blocked> <blocker> (blocked depends on blocker)
		if err := cmdr.run(absOut, "dep", "add", blockedID, blockerID); err != nil {
			return nil, fmt.Errorf("loadgen: bd dep add %s → %s: %w", blockerID, blockedID, err)
		}
	}

	beadsPath := filepath.Join(absOut, ".beads")

	return &Manifest{
		Spec:         spec,
		ActualCounts: actualCounts,
		ActualEdges:  len(p.edges),
		IssuesPath:   beadsPath,
		BdVersion:    bdVersion,
		Warnings:     warnings,
	}, nil
}

// ── bd subprocess helpers ────────────────────────────────────────────────────

func bdVersionString() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// bdRun runs a bd subcommand in dir with the given args.
func bdRun(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s: %w\noutput: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// bdCreate creates an issue and returns its ID.
func bdCreate(dir, title string, priority int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "create",
		"--title", title,
		"--priority", fmt.Sprintf("%d", priority),
		"--silent",
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return "", fmt.Errorf("bd create %q: %w\nstderr: %s", title, err, stderr)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("bd create %q: empty ID in output", title)
	}
	return id, nil
}
