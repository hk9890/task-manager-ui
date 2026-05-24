package beads

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository/beads/contractcheck"
)

// capturingHandler is a slog.Handler that records all log records for inspection.
type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(name string) slog.Handler       { return h }

// warnMessages returns the messages from all Warn-level records.
func (h *capturingHandler) warnMessages() []string {
	var msgs []string
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			msgs = append(msgs, r.Message)
		}
	}
	return msgs
}

// warnAttrs returns a flat map of all key→value attrs from Warn-level records.
func (h *capturingHandler) warnAttrs() map[string]string {
	out := make(map[string]string)
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			out[a.Key] = a.Value.String()
			return true
		})
	}
	return out
}

// warnCount returns the number of Warn-level records.
func (h *capturingHandler) warnCount() int {
	count := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			count++
		}
	}
	return count
}

// newCapturingGateway wraps a mock inner with a validatingGateway backed by a
// capturing logger so tests can assert on logged warnings.
func newCapturingGateway(inner BeadsGateway) (*validatingGateway, *capturingHandler) {
	handler := &capturingHandler{}
	logger := slog.New(handler)
	vg := newValidatingGateway(inner, logger)
	return vg, handler
}

// ---- mock inner gateways for violation tests ----

// emptyIDGateway returns IssueSummary values with empty IDs from ListIssues.
type emptyIDGateway struct{ stubGateway }

func (g emptyIDGateway) ListIssues(_ context.Context, _ domain.IssueListQuery) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "", Title: "Missing ID issue", Status: "open", Type: "task"},
	}, nil
}

// closedInDefaultGateway returns a closed issue from a default (no-filter) ListIssues call.
type closedInDefaultGateway struct{ stubGateway }

func (g closedInDefaultGateway) ListIssues(_ context.Context, _ domain.IssueListQuery) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "x-1", Title: "Closed issue", Status: "closed", Type: "task"},
	}, nil
}

// emptyBlockedByGateway returns a BlockedIssueView with an empty BlockedBy slice.
type emptyBlockedByGateway struct{ stubGateway }

func (g emptyBlockedByGateway) BlockedIssues(_ context.Context, _ domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	return []domain.BlockedIssueView{
		{
			Issue:     domain.IssueSummary{ID: "x-1", Title: "Blocked issue", Status: "blocked", Type: "task"},
			BlockedBy: []domain.IssueReference{}, // VIOLATION: must be non-empty
		},
	}, nil
}

// overlapReadyBlockedGateway returns a ReadyExplainResult with an issue in both slices.
type overlapReadyBlockedGateway struct{ stubGateway }

func (g overlapReadyBlockedGateway) ReadyExplain(_ context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	sharedIssue := domain.IssueSummary{ID: "x-1", Title: "Overlap issue", Status: "open", Type: "task"}
	return domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{sharedIssue},
		Blocked: []domain.BlockedIssueView{
			{Issue: sharedIssue, BlockedBy: []domain.IssueReference{{ID: "x-0"}}},
		},
		TotalReady:   1,
		TotalBlocked: 1,
	}, nil
}

// mismatchedIDGateway returns a ShowIssue detail whose Summary.ID doesn't match the input.
type mismatchedIDGateway struct{ stubGateway }

func (g mismatchedIDGateway) ShowIssue(_ context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID: "wrong-id", Title: "Wrong issue", Status: "open", Type: "task",
		},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}

// nilCommentsGateway returns a ShowIssue detail with nil Comments.
type nilCommentsGateway struct{ stubGateway }

func (g nilCommentsGateway) ShowIssue(_ context.Context, _ domain.ShowIssueQuery) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "x-1", Title: "Issue", Status: "open", Type: "task"},
		Comments:  nil, // VIOLATION
		BlockedBy: []domain.IssueReference{},
	}, nil
}

// badSearchMetaGateway returns a SearchResultPage where ReturnedCount != len(Results).
type badSearchMetaGateway struct{ stubGateway }

func (g badSearchMetaGateway) SearchIssues(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{
		Results: []domain.SearchResult{
			{Issue: domain.IssueSummary{ID: "x-1", Title: "Issue", Status: "open", Type: "task"}},
		},
		Metadata: domain.SearchResultMetadata{
			ReturnedCount: 99, // VIOLATION: should be 1
		},
	}, nil
}

// countMismatchGateway returns an IssueCountResult where Total != sum(Groups).
type countMismatchGateway struct{ stubGateway }

func (g countMismatchGateway) CountIssues(_ context.Context, _ domain.IssueCountQuery) (domain.IssueCountResult, error) {
	return domain.IssueCountResult{
		Groups: []domain.IssueStatusCount{{Status: "open", Count: 3}},
		Total:  7, // VIOLATION: sum(groups)=3 != Total=7
	}, nil
}

// ssomViolationGateway is an inner that returns count=0 but list=1 for status=closed.
// This mirrors the ssomBrokenGateway in contract/ssom_invariant_test.go but is
// placed here to test the runtime decorator path (validatingGateway tracks the
// ssom state and fires the warning).
type ssomViolationGateway struct{ stubGateway }

func (g ssomViolationGateway) CountIssues(_ context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	for _, s := range query.Statuses {
		if s == "closed" {
			return domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{}, // zero-count omitted
				Total:  0,                           // DELIBERATE BUG: should be ≥ list result
			}, nil
		}
	}
	return domain.IssueCountResult{
		Groups: []domain.IssueStatusCount{{Status: "open", Count: 1}},
		Total:  1,
	}, nil
}

func (g ssomViolationGateway) ListIssues(_ context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error) {
	for _, s := range query.Statuses {
		if s == "closed" {
			return []domain.IssueSummary{
				{ID: "x-closed-1", Title: "Closed issue", Status: "closed", Type: "task"},
			}, nil
		}
	}
	return []domain.IssueSummary{
		{ID: "x-open-1", Title: "Open issue", Status: "open", Type: "task"},
	}, nil
}

// highCardinalityGateway returns 5001 IssueSummary items from ListIssues.
// Used to verify the high-cardinality fallback path.
type highCardinalityGateway struct{ stubGateway }

func (g highCardinalityGateway) ListIssues(_ context.Context, _ domain.IssueListQuery) ([]domain.IssueSummary, error) {
	items := make([]domain.IssueSummary, 5001)
	for i := range items {
		items[i] = domain.IssueSummary{
			// All items are valid — no violation expected.
			ID:     "item-" + string(rune('a'+i%26)),
			Title:  "item",
			Status: "open",
			Type:   "task",
		}
	}
	// Introduce an empty-ID violation at index 1 (should be in the first-10 spot check).
	items[1].ID = ""
	return items, nil
}

// stubGateway is an embedding base that satisfies BeadsGateway with no-op methods.
type stubGateway struct{}

var _ BeadsGateway = stubGateway{}

func (stubGateway) HealthCheck(_ context.Context) error { return nil }
func (stubGateway) ListIssues(_ context.Context, _ domain.IssueListQuery) ([]domain.IssueSummary, error) {
	return nil, nil
}
func (stubGateway) Query(_ context.Context, _ string, _ domain.QueryOptions) ([]domain.IssueSummary, error) {
	return nil, nil
}
func (stubGateway) ReadyIssues(_ context.Context, _ domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	return nil, nil
}
func (stubGateway) BlockedIssues(_ context.Context, _ domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	return nil, nil
}
func (stubGateway) ReadyExplain(_ context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	return domain.ReadyExplainResult{}, nil
}
func (stubGateway) ShowIssue(_ context.Context, _ domain.ShowIssueQuery) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "stub-1"},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}
func (stubGateway) SearchIssues(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{Results: []domain.SearchResult{}}, nil
}
func (stubGateway) CountIssues(_ context.Context, _ domain.IssueCountQuery) (domain.IssueCountResult, error) {
	return domain.IssueCountResult{}, nil
}
func (stubGateway) StatusCatalog(_ context.Context) ([]domain.StatusOption, error) {
	return []domain.StatusOption{{Name: "open"}}, nil
}
func (stubGateway) TypeCatalog(_ context.Context) ([]domain.TypeOption, error) {
	return []domain.TypeOption{{Name: "task"}}, nil
}
func (stubGateway) LabelCatalog(_ context.Context) ([]domain.LabelOption, error) {
	return nil, nil
}
func (stubGateway) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (stubGateway) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (stubGateway) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (stubGateway) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}

// ---- Tests ----

func TestValidatingGateway_ListIssues_EmptyIDViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(emptyIDGateway{})
	items, err := vg.ListIssues(context.Background(), domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected items to be returned")
	}
	if h.warnCount() == 0 {
		t.Fatal("expected at least one warning for empty-ID violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "NonEmptyID" {
		t.Errorf("expected rule=NonEmptyID, got %q", attrs["rule"])
	}
	if attrs["method"] != "ListIssues" {
		t.Errorf("expected method=ListIssues, got %q", attrs["method"])
	}
}

func TestValidatingGateway_ListIssues_ClosedExcludedViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(closedInDefaultGateway{})
	_, err := vg.ListIssues(context.Background(), domain.IssueListQuery{}) // no status filter
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for closed-in-default violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "ClosedExcludedByDefault" {
		t.Errorf("expected rule=ClosedExcludedByDefault, got %q", attrs["rule"])
	}
}

func TestValidatingGateway_BlockedIssues_EmptyBlockedByViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(emptyBlockedByGateway{})
	_, err := vg.BlockedIssues(context.Background(), domain.BlockedIssuesQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for empty-BlockedBy violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "NonEmptyBlockedBySlice" {
		t.Errorf("expected rule=NonEmptyBlockedBySlice, got %q", attrs["rule"])
	}
	if attrs["method"] != "BlockedIssues" {
		t.Errorf("expected method=BlockedIssues, got %q", attrs["method"])
	}
}

func TestValidatingGateway_ReadyExplain_OverlapViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(overlapReadyBlockedGateway{})
	_, err := vg.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for ready-and-blocked-disjoint violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "ReadyAndBlockedDisjoint" {
		t.Errorf("expected rule=ReadyAndBlockedDisjoint, got %q", attrs["rule"])
	}
}

func TestValidatingGateway_ShowIssue_IDMismatchViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(mismatchedIDGateway{})
	_, err := vg.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "requested-id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for ID-mismatch violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "ReturnedIDMatchesInput" {
		t.Errorf("expected rule=ReturnedIDMatchesInput, got %q", attrs["rule"])
	}
}

func TestValidatingGateway_ShowIssue_NilCommentsViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(nilCommentsGateway{})
	_, err := vg.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "x-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for nil-Comments violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "CommentsNotNil" {
		t.Errorf("expected rule=CommentsNotNil, got %q", attrs["rule"])
	}
}

func TestValidatingGateway_SearchIssues_ReturnedCountMismatchViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(badSearchMetaGateway{})
	_, err := vg.SearchIssues(context.Background(), domain.SearchIssuesQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for ReturnedCount mismatch violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "ReturnedCountMatchesLen" {
		t.Errorf("expected rule=ReturnedCountMatchesLen, got %q", attrs["rule"])
	}
}

func TestValidatingGateway_CountIssues_TotalMismatchViolation(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(countMismatchGateway{})
	_, err := vg.CountIssues(context.Background(), domain.IssueCountQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.warnCount() == 0 {
		t.Fatal("expected warning for Total-vs-Groups mismatch violation, got none")
	}
	attrs := h.warnAttrs()
	if attrs["rule"] != "TotalEqualsSumOfGroups" {
		t.Errorf("expected rule=TotalEqualsSumOfGroups, got %q", attrs["rule"])
	}
}

// TestValidatingGateway_SsomViolation verifies that the ssom cross-method
// invariant fires when CountIssues(closed).Total < len(ListIssues(closed)).
// The decorator must track the count result in state and compare it against
// a subsequent list call for the same status.
func TestValidatingGateway_SsomViolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	vg, h := newCapturingGateway(ssomViolationGateway{})

	// Step 1: call CountIssues(closed) — decorator caches Total=0.
	_, err := vg.CountIssues(ctx, domain.IssueCountQuery{Statuses: []string{"closed"}})
	if err != nil {
		t.Fatalf("CountIssues error: %v", err)
	}
	// Count violation fires (Total=0 != sum(groups)=0 is OK; but Total=0 != sum is fine here).
	// Clear warnings before the ssom-specific assertion.
	h.records = nil

	// Step 2: call ListIssues(closed) — returns 1 item; ssom invariant must fire.
	_, err = vg.ListIssues(ctx, domain.IssueListQuery{Statuses: []string{"closed"}})
	if err != nil {
		t.Fatalf("ListIssues error: %v", err)
	}

	if h.warnCount() == 0 {
		t.Fatal("expected ssom invariant warning after ListIssues with stale low count, got none")
	}

	// Verify the ssom rule name is in the warnings.
	found := false
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "rule" && a.Value.String() == "SsomCountGreaterThanOrEqualToListSize" {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Errorf("expected warning with rule=SsomCountGreaterThanOrEqualToListSize, warnings=%v", h.records)
	}
}

// TestValidatingGateway_HighCardinalityFastPath verifies that when a response
// has > 5000 items, the validator only checks a subset (first 10 + last 10)
// and still catches violations within that subset.
func TestValidatingGateway_HighCardinalityFastPath(t *testing.T) {
	t.Parallel()

	vg, h := newCapturingGateway(highCardinalityGateway{})
	items, err := vg.ListIssues(context.Background(), domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 5001 {
		t.Fatalf("expected 5001 items from mock, got %d", len(items))
	}

	// Verify the high-cardinality fallback activates.
	indices := contractcheck.SpotCheckIndices(5001)
	if indices == nil {
		t.Fatal("SpotCheckIndices(5001) returned nil — expected non-nil for high-cardinality input")
	}

	// Item at index 1 has empty ID — it is in the first-10 spot, so a warning must fire.
	if h.warnCount() == 0 {
		t.Fatal("expected violation warning for empty-ID at index 1 (within first-10 spot), got none")
	}
}

// TestValidatingGateway_NoViolationsOnCleanData verifies that a well-formed mock
// produces zero warnings. This is the complement of the end-to-end fixture test.
func TestValidatingGateway_NoViolationsOnCleanData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cleanInner := cleanMockGateway{}
	vg, h := newCapturingGateway(cleanInner)

	if err := vg.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if _, err := vg.ListIssues(ctx, domain.IssueListQuery{}); err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if _, err := vg.ReadyIssues(ctx, domain.ReadyIssuesQuery{}); err != nil {
		t.Fatalf("ReadyIssues: %v", err)
	}
	if _, err := vg.BlockedIssues(ctx, domain.BlockedIssuesQuery{}); err != nil {
		t.Fatalf("BlockedIssues: %v", err)
	}
	if _, err := vg.ReadyExplain(ctx, domain.ReadyExplainOptions{}); err != nil {
		t.Fatalf("ReadyExplain: %v", err)
	}
	if _, err := vg.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: "clean-1"}); err != nil {
		t.Fatalf("ShowIssue: %v", err)
	}
	if _, err := vg.SearchIssues(ctx, domain.SearchIssuesQuery{}); err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if _, err := vg.CountIssues(ctx, domain.IssueCountQuery{}); err != nil {
		t.Fatalf("CountIssues: %v", err)
	}
	if _, err := vg.StatusCatalog(ctx); err != nil {
		t.Fatalf("StatusCatalog: %v", err)
	}
	if _, err := vg.TypeCatalog(ctx); err != nil {
		t.Fatalf("TypeCatalog: %v", err)
	}
	if _, err := vg.LabelCatalog(ctx); err != nil {
		t.Fatalf("LabelCatalog: %v", err)
	}

	if h.warnCount() != 0 {
		t.Errorf("expected zero warnings for clean data, got %d: %v", h.warnCount(), h.records)
	}
}

// cleanMockGateway returns structurally valid data for all read methods.
type cleanMockGateway struct{}

var _ BeadsGateway = cleanMockGateway{}

func (cleanMockGateway) HealthCheck(_ context.Context) error { return nil }
func (cleanMockGateway) ListIssues(_ context.Context, _ domain.IssueListQuery) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "clean-1", Title: "Clean issue", Status: "open", Type: "task"},
	}, nil
}
func (cleanMockGateway) Query(_ context.Context, _ string, _ domain.QueryOptions) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "clean-1", Title: "Clean issue", Status: "open", Type: "task"},
	}, nil
}
func (cleanMockGateway) ReadyIssues(_ context.Context, _ domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "clean-1", Title: "Clean issue", Status: "open", Type: "task"},
	}, nil
}
func (cleanMockGateway) BlockedIssues(_ context.Context, _ domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	return []domain.BlockedIssueView{
		{
			Issue:     domain.IssueSummary{ID: "clean-2", Title: "Blocked", Status: "blocked", Type: "task"},
			BlockedBy: []domain.IssueReference{{ID: "clean-1"}},
		},
	}, nil
}
func (cleanMockGateway) ReadyExplain(_ context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	return domain.ReadyExplainResult{
		Ready:        []domain.IssueSummary{{ID: "clean-1", Title: "Clean issue", Status: "open", Type: "task"}},
		Blocked:      []domain.BlockedIssueView{},
		TotalReady:   1,
		TotalBlocked: 0,
	}, nil
}
func (cleanMockGateway) ShowIssue(_ context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: query.IssueID, Title: "Clean issue", Status: "open", Type: "task"},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}
func (cleanMockGateway) SearchIssues(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	results := []domain.SearchResult{{Issue: domain.IssueSummary{ID: "clean-1", Title: "Clean issue", Status: "open", Type: "task"}}}
	return domain.SearchResultPage{
		Results:  results,
		Metadata: domain.SearchResultMetadata{ReturnedCount: len(results)},
	}, nil
}
func (cleanMockGateway) CountIssues(_ context.Context, _ domain.IssueCountQuery) (domain.IssueCountResult, error) {
	return domain.IssueCountResult{
		Groups: []domain.IssueStatusCount{{Status: "open", Count: 1}},
		Total:  1,
	}, nil
}
func (cleanMockGateway) StatusCatalog(_ context.Context) ([]domain.StatusOption, error) {
	return []domain.StatusOption{{Name: "open"}, {Name: "closed"}}, nil
}
func (cleanMockGateway) TypeCatalog(_ context.Context) ([]domain.TypeOption, error) {
	return []domain.TypeOption{{Name: "task"}, {Name: "bug"}}, nil
}
func (cleanMockGateway) LabelCatalog(_ context.Context) ([]domain.LabelOption, error) {
	return []domain.LabelOption{{Name: "fixture"}}, nil
}
func (cleanMockGateway) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (cleanMockGateway) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (cleanMockGateway) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (cleanMockGateway) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}
