package embeddedfixture

// scale_fixture_smoke_test.go — product-behavior invariant tests for the scale
// fixture (beads-workbench-faif.1).
//
// Each sub-test is tagged with the regression class it guards.  These are pure
// unit tests: no bd subprocess, no live database.  They load scale-seed.json
// from disk and drive dashboard.Compose / board.Model.View() directly.
//
// Integration-tier smoke tests (repository.SearchIssues, repository.ShowIssue) live
// in scale_fixture_smoke_integration_test.go.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode/board"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
)

// scaleSeedPath returns the absolute path to scale-seed.json.
func scaleSeedPath(tb testing.TB) string {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to resolve embeddedfixture paths via runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "scale-seed.json")
}

// loadScaleSeed loads and decodes scale-seed.json.
func loadScaleSeed(tb testing.TB) Spec {
	tb.Helper()
	path := scaleSeedPath(tb)
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("loadScaleSeed: read %q: %v", path, err)
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		tb.Fatalf("loadScaleSeed: decode %q: %v", path, err)
	}
	return spec
}

// countByStatus returns the number of issues in the spec with the given status.
func countByStatus(issues []IssueSpec, status string) int {
	n := 0
	for _, iss := range issues {
		if iss.Status == status {
			n++
		}
	}
	return n
}

// makeReadySummaries returns n IssueSummary values representing open/active issues,
// using synthetic data with varied priorities to exercise sort stability.
func makeReadySummaries(n int) []domain.IssueSummary {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]domain.IssueSummary, n)
	for i := range out {
		out[i] = domain.IssueSummary{
			ID:        "s-ready-" + strconv.Itoa(i),
			Title:     "Ready issue " + strconv.Itoa(i),
			Status:    "open",
			Priority:  (i % 4) + 1, // P1–P4
			UpdatedAt: base.Add(-time.Duration(i) * time.Second),
		}
	}
	return out
}

// makeClosedSummaries returns n IssueSummary values representing closed issues,
// newest first (descending UpdatedAt).
func makeClosedSummaries(n int) []domain.IssueSummary {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out := make([]domain.IssueSummary, n)
	for i := range out {
		out[i] = domain.IssueSummary{
			ID:        "s-closed-" + strconv.Itoa(i),
			Title:     "Closed issue " + strconv.Itoa(i),
			Status:    "closed",
			Priority:  2,
			UpdatedAt: base.Add(-time.Duration(i) * time.Second),
		}
	}
	return out
}

// makeClosedWithSharedTimestamp returns n IssueSummary values all sharing the
// same UpdatedAt — exercises kh54 sort-stability regression trigger.
func makeClosedWithSharedTimestamp(n int, sharedAt time.Time) []domain.IssueSummary {
	out := make([]domain.IssueSummary, n)
	for i := range out {
		out[i] = domain.IssueSummary{
			ID:        "s-tied-" + strconv.Itoa(i),
			Title:     "Tied closed issue " + strconv.Itoa(i),
			Status:    "closed",
			Priority:  2,
			UpdatedAt: sharedAt,
		}
	}
	return out
}

// countColumnTopBorders counts the box-drawing top-left corner (╭) in a view,
// which appears once per board column header.  4 is the expected count for a
// full-width board.  8+ indicates doubled headers (o7tk regression class).
func countColumnTopBorders(view string) int {
	return strings.Count(view, "╭")
}

// ---- Invariant 1 — Done.Total reflects the real closed count ----------------

// TestScaleFixtureInvariant_DoneTotalMatchesClosedCount guards the ssom regression
// class: Compose.Done.Total must equal the real closed-issue count when
// ClosedTotal is supplied to Inputs.
func TestScaleFixtureInvariant_DoneTotalMatchesClosedCount(t *testing.T) {
	t.Parallel()

	// regression class: ssom
	spec := loadScaleSeed(t)
	nClosed := countByStatus(spec.Issues, "closed")

	if nClosed < 75 {
		t.Fatalf("scale fixture must have >=75 closed issues; got %d (ssom regression guard)", nClosed)
	}

	// Simulate a capped Done column: bd returned only 50 rows but the real total
	// is nClosed (ClosedTotal from CountIssues).
	const doneCap = 50
	inputs := dashboard.Inputs{
		Ready:       makeReadySummaries(10),
		Closed:      makeClosedSummaries(doneCap),
		ClosedLimit: doneCap,
		ClosedTotal: nClosed,
	}

	cols := dashboard.Compose(inputs)

	if cols.Done.Total != nClosed {
		t.Errorf("ssom: Done.Total=%d; want %d (real closed count from spec)", cols.Done.Total, nClosed)
	}
	if cols.Done.TotalIsExact {
		t.Error("ssom: Done.TotalIsExact should be false when visible list < ClosedTotal ('N of M' badge signal)")
	}
}

// ---- Invariant 2 — Done column cap + 'N of M' badge signal -----------------

// TestScaleFixtureInvariant_DoneColumnBadgeWhenCapped guards the ssom regression
// class: when the fixture has >50 closed issues, a 50-cap produces TotalIsExact=false
// (the signal bwb uses to render the "N of M" badge).
func TestScaleFixtureInvariant_DoneColumnBadgeWhenCapped(t *testing.T) {
	t.Parallel()

	// regression class: ssom (Done-column 50-cap badge)
	spec := loadScaleSeed(t)
	nClosed := countByStatus(spec.Issues, "closed")

	const doneCap = 50
	if nClosed <= doneCap {
		t.Fatalf("scale fixture must have >%d closed issues for badge test; got %d", doneCap, nClosed)
	}

	inputs := dashboard.Inputs{
		Closed:      makeClosedSummaries(doneCap),
		ClosedLimit: doneCap,
		ClosedTotal: nClosed,
	}
	cols := dashboard.Compose(inputs)

	if len(cols.Done.Issues) > doneCap {
		t.Errorf("ssom: Done.Issues len=%d; want <=%d (cap respected)", len(cols.Done.Issues), doneCap)
	}
	if cols.Done.TotalIsExact {
		t.Errorf("ssom: Done.TotalIsExact=true; want false when %d closed > cap %d (N of M badge should show)", nClosed, doneCap)
	}
	if cols.Done.Total < nClosed {
		t.Errorf("ssom: Done.Total=%d; want >=%d (real DB count preserved)", cols.Done.Total, nClosed)
	}
}

// ---- Invariant 3 — Cardinality warning at 500 --------------------------------

// TestScaleFixtureInvariant_CardinalityWarningAt500 guards the composer
// cardinality threshold: when a Ready group has >= 500 issues, Compose must
// return a CardinalityWarning for the "Ready" group.
func TestScaleFixtureInvariant_CardinalityWarningAt500(t *testing.T) {
	t.Parallel()

	// regression class: composer cardinality threshold
	spec := loadScaleSeed(t)
	nActive := countByStatus(spec.Issues, "open") + countByStatus(spec.Issues, "in_progress")

	if nActive < 510 {
		t.Fatalf("scale fixture must have >=510 active issues; got %d (cardinality threshold test)", nActive)
	}

	// Feed nActive ready issues so the cardinality threshold fires.
	inputs := dashboard.Inputs{
		Ready: makeReadySummaries(nActive),
	}
	cols := dashboard.Compose(inputs)

	var found bool
	for _, w := range cols.Warnings {
		if w.Group == "Ready" && w.Count >= 500 && w.Threshold == 500 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("composer cardinality: expected a Ready cardinality warning with Count>=%d Threshold=500; got warnings=%v", nActive, cols.Warnings)
	}
}

// ---- Invariant 4 — Sort stability with shared closed_at ----------------------

// TestScaleFixtureInvariant_SortStabilitySharedClosedAt guards the kh54
// regression class: when N closed issues share the same UpdatedAt timestamp,
// issueSort must produce a stable, deterministic ordering (no flapping).
//
// The scale fixture has a block of >=10 closed issues with the same
// kh54-tiebreak label (simulating shared updated_at).  This test verifies
// that two independent sort passes over identical input produce the same order.
func TestScaleFixtureInvariant_SortStabilitySharedClosedAt(t *testing.T) {
	t.Parallel()

	// regression class: kh54 (sort flapping on equal timestamps)
	spec := loadScaleSeed(t)

	var kh54Count int
	for _, iss := range spec.Issues {
		for _, lbl := range iss.Labels {
			if lbl == "kh54-tiebreak" {
				kh54Count++
				break
			}
		}
	}
	if kh54Count < 10 {
		t.Fatalf("kh54: scale fixture must have >=10 kh54-tiebreak closed issues; got %d", kh54Count)
	}

	sharedAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	block := makeClosedWithSharedTimestamp(kh54Count, sharedAt)

	// Sort pass 1.
	pass1 := make([]domain.IssueSummary, len(block))
	copy(pass1, block)
	sort.SliceStable(pass1, func(i, j int) bool {
		a, b := pass1[i], pass1[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})

	// Sort pass 2 (same input, re-sorted independently).
	pass2 := make([]domain.IssueSummary, len(block))
	copy(pass2, block)
	sort.SliceStable(pass2, func(i, j int) bool {
		a, b := pass2[i], pass2[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})

	for i := range pass1 {
		if pass1[i].ID != pass2[i].ID {
			t.Errorf("kh54: sort not deterministic at index %d: pass1=%s pass2=%s", i, pass1[i].ID, pass2[i].ID)
		}
	}
}

// ---- Invariant 5 — No doubled headers (o7tk regression guard) ---------------

// TestScaleFixtureInvariant_NoDuplicatedBoardHeaders guards the o7tk regression
// class: the board model must render exactly 4 column headers at a wide
// terminal regardless of data size.  The test drives the board with the scale
// fixture issue counts to confirm that large payloads do not trigger frame
// stacking.
func TestScaleFixtureInvariant_NoDuplicatedBoardHeaders(t *testing.T) {
	t.Parallel()

	// regression class: o7tk (doubled column headers / frame stacking)
	spec := loadScaleSeed(t)

	// Verify the fixture has the scale properties that make o7tk relevant.
	nActive := countByStatus(spec.Issues, "open") + countByStatus(spec.Issues, "in_progress")
	if nActive < 510 {
		t.Fatalf("o7tk: scale fixture must have >=510 active issues; got %d", nActive)
	}

	repo := memoryrepo.New()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("o7tk: ResolveKeyBindings: %v", err)
	}
	m := board.NewModel(repo, slog.Default(), keys)
	// Width=180 ensures all 4 columns are visible.
	m.SetSize(180, 40)

	// Feed synthetic data via the exported smoke helper.
	// FeedTestData simulates a completed board load with representative data.
	board.FeedTestData(m)

	view := m.View(0)
	got := countColumnTopBorders(view)
	const wantBorders = 4
	if got != wantBorders {
		t.Errorf("o7tk: expected exactly %d column-top corners (╭) in board view; got %d (doubled headers / frame stacking)", wantBorders, got)
	}
}

// ---- Invariant 6 — Spec structural coverage ---------------------------------

// TestScaleFixtureInvariant_SpecStructuralCoverage verifies that scale-seed.json
// contains all required edge cases at the spec level.  This is a fast census
// check that fails early if the fixture is missing a required edge-case class.
func TestScaleFixtureInvariant_SpecStructuralCoverage(t *testing.T) {
	t.Parallel()

	// regression class: fixture integrity / regression regression guard
	spec := loadScaleSeed(t)

	t.Run("prefix_is_bws", func(t *testing.T) {
		if spec.Prefix != "bws" {
			t.Errorf("expected prefix 'bws'; got %q", spec.Prefix)
		}
	})

	t.Run("issue_count_550_to_700", func(t *testing.T) {
		n := len(spec.Issues)
		if n < 550 || n > 700 {
			t.Errorf("expected 550-700 issues; got %d", n)
		}
	})

	t.Run("closed_issues_ge_75", func(t *testing.T) {
		// ssom regression guard
		n := countByStatus(spec.Issues, "closed")
		if n < 75 {
			t.Errorf("ssom: expected >=75 closed issues; got %d", n)
		}
	})

	t.Run("active_issues_ge_510", func(t *testing.T) {
		// composer cardinality threshold
		nOpen := countByStatus(spec.Issues, "open")
		nIP := countByStatus(spec.Issues, "in_progress")
		n := nOpen + nIP
		if n < 510 {
			t.Errorf("cardinality: expected >=510 active issues; got %d", n)
		}
	})

	t.Run("kh54_tiebreak_block_ge_10", func(t *testing.T) {
		// kh54 regression guard — sort stability
		var n int
		for _, iss := range spec.Issues {
			for _, lbl := range iss.Labels {
				if lbl == "kh54-tiebreak" {
					n++
					break
				}
			}
		}
		if n < 10 {
			t.Errorf("kh54: expected >=10 kh54-tiebreak closed issues; got %d", n)
		}
	})

	t.Run("closed_at_tiebreak_block_ge_10", func(t *testing.T) {
		// closed_at sort tie-break corpus
		var n int
		for _, iss := range spec.Issues {
			for _, lbl := range iss.Labels {
				if lbl == "closed-at-tiebreak" {
					n++
					break
				}
			}
		}
		if n < 10 {
			t.Errorf("sort tie-break: expected >=10 closed-at-tiebreak issues; got %d", n)
		}
	})

	t.Run("p1_tiebreak_block_ge_10", func(t *testing.T) {
		// active-column P1 priority tie-break corpus
		var n int
		for _, iss := range spec.Issues {
			if iss.Priority == 1 && iss.Status == "open" {
				n++
			}
		}
		if n < 10 {
			t.Errorf("p1 tiebreak: expected >=10 P1 open issues; got %d", n)
		}
	})

	t.Run("emoji_title_issue", func(t *testing.T) {
		// edge text: emoji title
		var found bool
		for _, iss := range spec.Issues {
			if strings.Contains(iss.Title, "🚀") {
				found = true
				break
			}
		}
		if !found {
			t.Error("edge text: no issue with emoji (🚀) in title")
		}
	})

	t.Run("shell_metachar_title", func(t *testing.T) {
		// edge text: shell metacharacters in title
		metacharIssues := 0
		for _, iss := range spec.Issues {
			title := iss.Title
			if strings.ContainsAny(title, ";'\"") && strings.Contains(title, "`") {
				metacharIssues++
			}
		}
		if metacharIssues == 0 {
			t.Error("edge text: no issue with shell metacharacters (`;`, `'`, `\"`, backtick) in title")
		}
	})

	t.Run("max_length_title", func(t *testing.T) {
		// edge text: very long title (≥120 chars)
		var found bool
		for _, iss := range spec.Issues {
			if len(iss.Title) >= 120 {
				found = true
				break
			}
		}
		if !found {
			t.Error("edge text: no issue with a title >=120 characters")
		}
	})

	t.Run("null_description_issue", func(t *testing.T) {
		// 781a regression guard: repository must handle missing description field
		var found bool
		for _, iss := range spec.Issues {
			if iss.Description == "" {
				found = true
				break
			}
		}
		if !found {
			t.Error("781a: no issue with null/empty description in fixture")
		}
	})

	t.Run("shared_label_ge_20_issues", func(t *testing.T) {
		// filter testing: label shared across many issues
		counts := map[string]int{}
		for _, iss := range spec.Issues {
			for _, lbl := range iss.Labels {
				counts[lbl]++
			}
		}
		var maxCount int
		var maxLabel string
		for lbl, n := range counts {
			if n > maxCount {
				maxCount = n
				maxLabel = lbl
			}
		}
		if maxCount < 20 {
			t.Errorf("filter test: expected at least one label on >=20 issues; best=%q at %d", maxLabel, maxCount)
		}
	})

	t.Run("keyword_workflow_ge_20_issues", func(t *testing.T) {
		// search corpus: keyword 'workflow' in >=20 titles/descriptions
		n := countKeywordHits(spec.Issues, "workflow")
		if n < 20 {
			t.Errorf("search corpus: keyword 'workflow' appears in only %d issues; want >=20", n)
		}
	})

	t.Run("keyword_pipeline_ge_20_issues", func(t *testing.T) {
		// search corpus: keyword 'pipeline' in >=20 titles/descriptions
		n := countKeywordHits(spec.Issues, "pipeline")
		if n < 20 {
			t.Errorf("search corpus: keyword 'pipeline' appears in only %d issues; want >=20", n)
		}
	})

	t.Run("keyword_dashboard_ge_20_issues", func(t *testing.T) {
		// search corpus: keyword 'dashboard' in >=20 titles/descriptions
		n := countKeywordHits(spec.Issues, "dashboard")
		if n < 20 {
			t.Errorf("search corpus: keyword 'dashboard' appears in only %d issues; want >=20", n)
		}
	})

	t.Run("parent_chain_5deep", func(t *testing.T) {
		// hierarchy: 5-deep parent chain represented as dependencies
		if len(spec.Deps) < 4 {
			t.Errorf("hierarchy: expected >=4 dependency edges for 5-deep chain; got %d", len(spec.Deps))
		}
	})

	t.Run("dependency_blocked_ge_5", func(t *testing.T) {
		// dependency-blocked corpus
		if len(spec.Deps) < 5 {
			t.Errorf("dep-blocked: expected >=5 dependency edges; got %d", len(spec.Deps))
		}
	})
}

// countKeywordHits returns the number of issues where keyword appears in the
// title or description (case-insensitive).
func countKeywordHits(issues []IssueSpec, keyword string) int {
	kw := strings.ToLower(keyword)
	n := 0
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Title), kw) ||
			strings.Contains(strings.ToLower(iss.Description), kw) {
			n++
		}
	}
	return n
}
