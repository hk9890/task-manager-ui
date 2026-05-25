// Package loadgen produces seeded beads repositories from a workload spec
// and provides a measurement harness for exercising the bwb data layer.
//
// See spec.go for the workload model and generate.go for repository seeding.
//
// # Measurement
//
// Measure exercises Repository.Dashboard (cold + warm), Repository.Search across
// a representative query matrix, Repository.Issue for a sample of IDs, and
// the CachingRepository lifecycle (cold hydrate, hot read, force-fresh).
//
// "Cold" means a fresh CachingRepository instance (dashboardDirty=true; first
// Dashboard call always hits the backing store). "Warm" means the second call on
// the same instance (served from the populated in-memory cache).
//
// Statistics: p50, p95, p99 are computed via linear interpolation between
// adjacent order statistics (equivalent to NumPy's default interpolation,
// R type 7). For a sample [1,2,...,10], this yields p50=5.5 and p95=9.55.
// Times are in milliseconds (float64).
package loadgen

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	bdrunner "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
)

// MeasureOpts configures a Measure run.
type MeasureOpts struct {
	// SamplesCold is the number of samples to take for cold-path operations
	// (Dashboard cold, cache hydrate). Defaults to 5 if zero.
	SamplesCold int

	// SamplesWarm is the number of samples to take for warm-path operations
	// (Dashboard warm, search, issue detail, hot reads). Defaults to 20 if zero.
	SamplesWarm int

	// IssueDetailN is the number of distinct issue IDs to sample for
	// Repository.Issue measurements. Defaults to 10 if zero.
	IssueDetailN int
}

func (o MeasureOpts) withDefaults() MeasureOpts {
	if o.SamplesCold <= 0 {
		o.SamplesCold = 5
	}
	if o.SamplesWarm <= 0 {
		o.SamplesWarm = 20
	}
	if o.IssueDetailN <= 0 {
		o.IssueDetailN = 10
	}
	return o
}

// OperationStats holds timing statistics for one measured operation.
type OperationStats struct {
	// Operation is the stable name for this measurement (e.g. "dashboard.cold").
	Operation string `json:"operation"`
	// SampleCount is the number of timing samples taken.
	SampleCount int `json:"sample_count"`
	// Approximate, when true, indicates that SampleCount < 5 and percentile
	// values are approximate due to the small sample size.
	Approximate bool `json:"approximate,omitempty"`
	// P50Ms is the 50th-percentile latency in milliseconds.
	P50Ms float64 `json:"p50_ms"`
	// P95Ms is the 95th-percentile latency in milliseconds.
	P95Ms float64 `json:"p95_ms"`
	// P99Ms is the 99th-percentile latency in milliseconds.
	P99Ms float64 `json:"p99_ms"`
	// MaxMs is the maximum observed latency in milliseconds.
	MaxMs float64 `json:"max_ms"`
}

// ReportHeader captures metadata about the measurement run.
type ReportHeader struct {
	// GeneratedAt is the wall-clock time when the report was produced (RFC3339).
	GeneratedAt time.Time `json:"generated_at"`
	// BdVersion is the output of `bd --version` at measurement time.
	BdVersion string `json:"bd_version"`
	// Manifest describes the generated workload that was measured.
	Manifest *Manifest `json:"manifest,omitempty"`
	// IssueDetailIDs is the list of issue IDs sampled for detail measurements.
	IssueDetailIDs []string `json:"issue_detail_ids,omitempty"`
	// SamplesCold is the cold-path sample count used.
	SamplesCold int `json:"samples_cold"`
	// SamplesWarm is the warm-path sample count used.
	SamplesWarm int `json:"samples_warm"`
}

// Report is the top-level measurement report. Its JSON schema is stable: field
// names and ordering are fixed so a future epic can diff reports for regression
// detection.
type Report struct {
	// Header contains run metadata.
	Header ReportHeader `json:"header"`
	// Operations contains per-operation statistics, in measurement order.
	Operations []OperationStats `json:"operations"`
}

// searchCase is one entry in the representative search matrix.
type searchCase struct {
	name  string
	query domain.SearchIssuesQuery
}

// searchMatrix is the representative set of search queries exercised during
// measurement. Chosen to cover the main bd routing paths:
//   - empty text → bd list --all (list fallback)
//   - text present → bd search <text> (text search path)
//   - status filter → bd search + --status
//   - workstate=ready → bd ready (ready path)
//   - workstate=blocked → bd blocked (blocked path)
var searchMatrix = []searchCase{
	{
		name:  "empty",
		query: domain.SearchIssuesQuery{Limit: 50},
	},
	{
		name:  "text=load-test",
		query: domain.SearchIssuesQuery{Text: "load-test", Limit: 50},
	},
	{
		name:  "text=issue",
		query: domain.SearchIssuesQuery{Text: "issue", Limit: 50},
	},
	{
		name:  "status=open",
		query: domain.SearchIssuesQuery{Statuses: []string{"open"}, Limit: 50},
	},
	{
		name:  "workstate=ready",
		query: domain.SearchIssuesQuery{WorkState: domain.WorkStateReady, Limit: 50},
	},
	{
		name:  "workstate=blocked",
		query: domain.SearchIssuesQuery{WorkState: domain.WorkStateBlocked, Limit: 50},
	},
}

// newBackingRepo constructs the raw beads-backed Repository against the
// directory containing .beads/. This mirrors the production wiring in
// cmd/bwb/main.go (constructRepository, "beads" path), but deliberately
// omits repository.NewValidating (which adds constant overhead) and caching
// so the measurement harness can layer those independently and attribute
// latency to each layer separately.
func newBackingRepo(repoDir string) repository.Repository {
	runner := bdrunner.NewCommandRunner(bdrunner.RunnerConfig{
		WorkDir: repoDir,
	})
	return repobeads.New(runner)
}

// newCachingRepo wraps a backing repository with a fresh CachingRepository.
// The returned instance has dashboardDirty=true (cold state by construction).
// No vcStatusFunc is set, so background polling is disabled — appropriate for
// a measurement harness where we control cache state explicitly.
func newCachingRepo(backing repository.Repository) *caching.CachingRepository {
	return caching.New(backing)
}

// Measure runs the measurement harness against the repository rooted at
// repoDir (the directory containing .beads/), using the given options.
//
// manifest may be nil if the caller does not have a Manifest from a prior
// Generate call (e.g. when measuring a pre-existing repo). When non-nil it is
// embedded in the report header.
//
// The returned Report contains per-operation statistics and is safe to
// JSON-marshal to a stable schema.
func Measure(repoDir string, manifest *Manifest, opts MeasureOpts) (*Report, error) {
	opts = opts.withDefaults()
	ctx := context.Background()

	// Capture bd version for the report header.
	bdVersion, err := bdVersionString()
	if err != nil {
		return nil, fmt.Errorf("measure: bd --version: %w", err)
	}

	backing := newBackingRepo(repoDir)

	// ── Collect issue IDs for detail measurements ──────────────────────────
	// One setup call (not measured) to collect issue IDs from the repo.
	detailIDs, err := collectIssueIDs(ctx, backing, opts.IssueDetailN)
	if err != nil {
		return nil, fmt.Errorf("measure: collect issue IDs: %w", err)
	}

	var ops []OperationStats

	// ── Dashboard cold ─────────────────────────────────────────────────────
	// Each cold sample: construct a fresh CachingRepository (dirty flag = true)
	// and call Dashboard once. The fresh instance guarantees the backing store
	// is always queried.
	coldSamples := make([]time.Duration, 0, opts.SamplesCold)
	for i := 0; i < opts.SamplesCold; i++ {
		cache := newCachingRepo(backing)
		start := time.Now()
		if _, err := cache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
			return nil, fmt.Errorf("measure: dashboard cold sample %d: %w", i, err)
		}
		coldSamples = append(coldSamples, time.Since(start))
	}
	ops = append(ops, computeStats("dashboard.cold", coldSamples))

	// ── Dashboard warm ─────────────────────────────────────────────────────
	// Reuse one CachingRepository instance; the first call populates the cache,
	// subsequent calls are served from memory. The first call is setup (not
	// measured); all opts.SamplesWarm subsequent calls are measured.
	warmCache := newCachingRepo(backing)
	if _, err := warmCache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
		return nil, fmt.Errorf("measure: dashboard warm setup: %w", err)
	}
	warmSamples := make([]time.Duration, 0, opts.SamplesWarm)
	for i := 0; i < opts.SamplesWarm; i++ {
		start := time.Now()
		if _, err := warmCache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
			return nil, fmt.Errorf("measure: dashboard warm sample %d: %w", i, err)
		}
		warmSamples = append(warmSamples, time.Since(start))
	}
	ops = append(ops, computeStats("dashboard.warm", warmSamples))

	// ── Cache lifecycle: cold hydrate ──────────────────────────────────────
	// Cold hydrate = first Dashboard call on a fresh CachingRepository,
	// equivalent to dashboard.cold but framed as a lifecycle measurement.
	// We re-run a dedicated set of cold samples under the "cache.hydrate" name
	// so both exist in the report with distinct names.
	hydrSamples := make([]time.Duration, 0, opts.SamplesCold)
	for i := 0; i < opts.SamplesCold; i++ {
		cache := newCachingRepo(backing)
		start := time.Now()
		if _, err := cache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
			return nil, fmt.Errorf("measure: cache.hydrate sample %d: %w", i, err)
		}
		hydrSamples = append(hydrSamples, time.Since(start))
	}
	ops = append(ops, computeStats("cache.hydrate", hydrSamples))

	// ── Cache lifecycle: hot read ──────────────────────────────────────────
	// Hot read = repeated Dashboard calls on a pre-warmed instance (same as
	// dashboard.warm but framed as a caching-lifecycle measurement). Reuse the
	// warmCache that was already populated above.
	hotSamples := make([]time.Duration, 0, opts.SamplesWarm)
	for i := 0; i < opts.SamplesWarm; i++ {
		start := time.Now()
		if _, err := warmCache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
			return nil, fmt.Errorf("measure: cache.hot_read sample %d: %w", i, err)
		}
		hotSamples = append(hotSamples, time.Since(start))
	}
	ops = append(ops, computeStats("cache.hot_read", hotSamples))

	// ── Cache lifecycle: force-fresh ───────────────────────────────────────
	// Force-fresh = Dashboard on a fresh CachingRepository (dirty state = true).
	// Semantically equivalent to dashboard.cold; named separately in the report
	// to make the caching-lifecycle triplet explicit.
	ffSamples := make([]time.Duration, 0, opts.SamplesCold)
	for i := 0; i < opts.SamplesCold; i++ {
		cache := newCachingRepo(backing)
		start := time.Now()
		if _, err := cache.Dashboard(ctx, repository.DashboardOptions{}); err != nil {
			return nil, fmt.Errorf("measure: cache.force_fresh sample %d: %w", i, err)
		}
		ffSamples = append(ffSamples, time.Since(start))
	}
	ops = append(ops, computeStats("cache.force_fresh", ffSamples))

	// ── Search matrix ──────────────────────────────────────────────────────
	for _, sc := range searchMatrix {
		samples := make([]time.Duration, 0, opts.SamplesWarm)
		for i := 0; i < opts.SamplesWarm; i++ {
			start := time.Now()
			if _, err := backing.Search(ctx, sc.query); err != nil {
				return nil, fmt.Errorf("measure: search %q sample %d: %w", sc.name, i, err)
			}
			samples = append(samples, time.Since(start))
		}
		ops = append(ops, computeStats("search."+sc.name, samples))
	}

	// ── Issue detail ───────────────────────────────────────────────────────
	if len(detailIDs) > 0 {
		detailCache := newCachingRepo(backing)
		// Warm the cache for all IDs first (setup, not measured).
		for _, id := range detailIDs {
			if _, err := detailCache.Issue(ctx, id); err != nil {
				return nil, fmt.Errorf("measure: issue detail warmup %q: %w", id, err)
			}
		}

		// Measure warm Issue reads (served from in-memory cache).
		warmDetailSamples := make([]time.Duration, 0, opts.SamplesWarm*len(detailIDs))
		for i := 0; i < opts.SamplesWarm; i++ {
			for _, id := range detailIDs {
				start := time.Now()
				if _, err := detailCache.Issue(ctx, id); err != nil {
					return nil, fmt.Errorf("measure: issue detail warm %q sample %d: %w", id, i, err)
				}
				warmDetailSamples = append(warmDetailSamples, time.Since(start))
			}
		}
		ops = append(ops, computeStats("issue.detail.warm", warmDetailSamples))

		// Measure cold Issue reads (fresh backing store call per ID per sample).
		coldDetailSamples := make([]time.Duration, 0, opts.SamplesCold*len(detailIDs))
		for i := 0; i < opts.SamplesCold; i++ {
			freshCache := newCachingRepo(backing)
			for _, id := range detailIDs {
				start := time.Now()
				if _, err := freshCache.Issue(ctx, id); err != nil {
					return nil, fmt.Errorf("measure: issue detail cold %q sample %d: %w", id, i, err)
				}
				coldDetailSamples = append(coldDetailSamples, time.Since(start))
			}
		}
		ops = append(ops, computeStats("issue.detail.cold", coldDetailSamples))
	}

	report := &Report{
		Header: ReportHeader{
			GeneratedAt:    time.Now().UTC(),
			BdVersion:      bdVersion,
			Manifest:       manifest,
			IssueDetailIDs: detailIDs,
			SamplesCold:    opts.SamplesCold,
			SamplesWarm:    opts.SamplesWarm,
		},
		Operations: ops,
	}

	return report, nil
}

// WriteReport writes the report as indented JSON to outPath.
func WriteReport(report *Report, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("measure: create report file %q: %w", outPath, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	encErr := enc.Encode(report)
	closeErr := f.Close()
	if encErr != nil {
		return fmt.Errorf("measure: encode report: %w", encErr)
	}
	if closeErr != nil {
		return fmt.Errorf("measure: close report file: %w", closeErr)
	}
	return nil
}

// PrintSummaryTable writes a human-readable summary of the report to w.
func PrintSummaryTable(report *Report, w *os.File) {
	_, _ = fmt.Fprintf(w, "\n=== bwb load-test measurement report ===\n")
	_, _ = fmt.Fprintf(w, "Generated:    %s\n", report.Header.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "bd version:   %s\n", report.Header.BdVersion)
	_, _ = fmt.Fprintf(w, "Cold samples: %d\n", report.Header.SamplesCold)
	_, _ = fmt.Fprintf(w, "Warm samples: %d\n", report.Header.SamplesWarm)
	if report.Header.Manifest != nil {
		m := report.Header.Manifest
		_, _ = fmt.Fprintf(w, "Workload:     %d issues (open=%d in_progress=%d blocked=%d closed=%d) edges=%d\n",
			m.Spec.TotalIssues(),
			m.ActualCounts["open"], m.ActualCounts["in_progress"],
			m.ActualCounts["blocked"], m.ActualCounts["closed"],
			m.ActualEdges,
		)
	}
	_, _ = fmt.Fprintf(w, "\n%-30s %6s %8s %8s %8s %8s\n",
		"operation", "N", "p50(ms)", "p95(ms)", "p99(ms)", "max(ms)")
	_, _ = fmt.Fprintf(w, "%s\n", "---------------------------------------------------------------------------------------------"[:79])
	for _, op := range report.Operations {
		approxMark := ""
		if op.Approximate {
			approxMark = "*"
		}
		_, _ = fmt.Fprintf(w, "%-30s %5d%s %8.2f %8.2f %8.2f %8.2f\n",
			op.Operation, op.SampleCount, approxMark,
			op.P50Ms, op.P95Ms, op.P99Ms, op.MaxMs)
	}
	if hasApprox(report.Operations) {
		_, _ = fmt.Fprintf(w, "\n* approximate: sample count < 5\n")
	}
	_, _ = fmt.Fprintln(w)
}

// hasApprox returns true if any operation has Approximate set.
func hasApprox(ops []OperationStats) bool {
	for _, op := range ops {
		if op.Approximate {
			return true
		}
	}
	return false
}

// collectIssueIDs performs a single (unmeasured) Search call to retrieve up to
// n issue IDs from the repository. Used to populate the detail measurement loop.
func collectIssueIDs(ctx context.Context, repo repository.Repository, n int) ([]string, error) {
	page, err := repo.Search(ctx, domain.SearchIssuesQuery{Limit: n})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Results))
	for _, r := range page.Results {
		ids = append(ids, r.Issue.ID)
	}
	return ids, nil
}

// ── Statistics ─────────────────────────────────────────────────────────────

// computeStats builds an OperationStats from a slice of duration samples.
// Samples must be non-empty; if empty the result has zero values.
func computeStats(operation string, samples []time.Duration) OperationStats {
	n := len(samples)
	if n == 0 {
		return OperationStats{Operation: operation}
	}

	ms := durationsToMs(samples)
	sort.Float64s(ms)

	stats := OperationStats{
		Operation:   operation,
		SampleCount: n,
		Approximate: n < 5,
		P50Ms:       percentile(ms, 50),
		P95Ms:       percentile(ms, 95),
		P99Ms:       percentile(ms, 99),
		MaxMs:       ms[n-1],
	}
	return stats
}

// durationsToMs converts a slice of time.Duration to millisecond float64 values.
func durationsToMs(durations []time.Duration) []float64 {
	ms := make([]float64, len(durations))
	for i, d := range durations {
		ms[i] = float64(d.Nanoseconds()) / 1e6
	}
	return ms
}

// percentile computes the p-th percentile (0–100) of a sorted float64 slice
// using linear interpolation between adjacent order statistics (NumPy default,
// R type 7).
//
// Convention: for a sorted sample of n values v[0]..v[n-1], the percentile p
// is computed as:
//
//	h = (p/100) * (n-1)
//	result = v[floor(h)] + (h - floor(h)) * (v[floor(h)+1] - v[floor(h)])
//
// For [1,2,3,4,5,6,7,8,9,10]:
//   - p50: h=4.5 → 5 + 0.5*(6-5) = 5.5
//   - p95: h=8.55 → 9 + 0.55*(10-9) = 9.55
//
// sorted must be sorted in ascending order and non-empty.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}

	h := (p / 100.0) * float64(n-1)
	lo := int(math.Floor(h))
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := h - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}
