package repository_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// ============================================================
// Test harness
// ============================================================

// capturingHandler is a slog.Handler that records all Warn-level log records.
type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) warnCount() int {
	count := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			count++
		}
	}
	return count
}

// hasWarnWithRule returns true if any Warn record has attr rule=r.
func (h *capturingHandler) hasWarnWithRule(r string) bool {
	for _, rec := range h.records {
		if rec.Level != slog.LevelWarn {
			continue
		}
		found := false
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "rule" && a.Value.String() == r {
				found = true
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// warnAttrs returns a flat key→value map of attrs from the first Warn record.
func (h *capturingHandler) warnAttrs() map[string]string {
	out := make(map[string]string)
	for _, rec := range h.records {
		if rec.Level != slog.LevelWarn {
			continue
		}
		rec.Attrs(func(a slog.Attr) bool {
			out[a.Key] = a.Value.String()
			return true
		})
		return out // only first warn record
	}
	return out
}

// newCapturingRepo wraps inner with a validatingRepository backed by a capturing
// logger so tests can assert on logged warnings.
func newCapturingRepo(inner repository.Repository) (repository.Repository, *capturingHandler) {
	h := &capturingHandler{}
	logger := slog.New(h)
	return repository.NewValidating(inner, logger), h
}

// ============================================================
// Stub repository base
// ============================================================

// stubRepository satisfies repository.Repository with well-formed no-op returns.
// Embed and override individual methods for violation tests.
type stubRepository struct{}

var _ repository.Repository = stubRepository{}

func (stubRepository) HealthCheck(_ context.Context) error { return nil }

func (stubRepository) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready:        []domain.IssueSummary{{ID: "s-1", Title: "Ready", Status: "open", Type: "task"}},
			Blocked:      []domain.BlockedIssueView{},
			TotalReady:   1,
			TotalBlocked: 0,
		},
		InProgress:  []domain.IssueSummary{{ID: "s-2", Title: "In-progress", Status: "in_progress", Type: "task"}},
		Closed:      []domain.IssueSummary{{ID: "s-3", Title: "Closed", Status: "closed", Type: "task"}},
		ClosedTotal: 1,
		Blocked:     []domain.IssueSummary{{ID: "s-4", Title: "Blocked", Status: "blocked", Type: "task"}},
	}, nil
}

func (stubRepository) Issue(_ context.Context, id string) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: id, Title: "Stub issue", Status: "open", Type: "task"},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}

func (stubRepository) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	results := []domain.SearchResult{{Issue: domain.IssueSummary{ID: "s-1", Title: "Stub", Status: "open", Type: "task"}}}
	return domain.SearchResultPage{
		Results:  results,
		Metadata: domain.SearchResultMetadata{ReturnedCount: len(results)},
	}, nil
}

func (stubRepository) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{IssueID: "new-1"}, nil
}
func (stubRepository) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (stubRepository) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (stubRepository) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}
func (stubRepository) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{
		Statuses: []domain.StatusOption{{Name: "open"}, {Name: "closed"}},
		Types:    []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
		Labels:   []domain.LabelOption{{Name: "area:ui"}},
	}, nil
}

// ============================================================
// Positive test — no violations on clean data
// ============================================================

func TestValidatingRepository_NoViolationsOnCleanData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, h := newCapturingRepo(stubRepository{})

	if err := repo.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if _, err := repo.Dashboard(ctx); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if _, err := repo.Issue(ctx, "s-1"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := repo.Search(ctx, domain.SearchIssuesQuery{}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if _, err := repo.Catalogs(ctx); err != nil {
		t.Fatalf("Catalogs: %v", err)
	}
	if _, err := repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := repo.UpdateIssue(ctx, "s-1", domain.UpdateIssueInput{}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if err := repo.CloseIssue(ctx, "s-1", domain.CloseIssueInput{}); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if err := repo.AddComment(ctx, "s-1", domain.AddCommentInput{Body: "hi"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if h.warnCount() != 0 {
		t.Errorf("expected zero warnings for clean data, got %d", h.warnCount())
	}
}

// ============================================================
// Dashboard — NonEmptyID violation in InProgress slot
// ============================================================

type emptyIDDashboardRepo struct{ stubRepository }

func (emptyIDDashboardRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{TotalReady: 0, TotalBlocked: 0},
		InProgress: []domain.IssueSummary{
			{ID: "", Title: "Missing ID", Status: "in_progress", Type: "task"}, // VIOLATION
		},
		Closed:      []domain.IssueSummary{},
		ClosedTotal: 0,
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_NonEmptyIDViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(emptyIDDashboardRepo{})
	data, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.InProgress) == 0 {
		t.Fatal("expected data to be returned unchanged")
	}
	if !h.hasWarnWithRule("NonEmptyID") {
		t.Errorf("expected warn with rule=NonEmptyID; records=%v", h.records)
	}
	attrs := h.warnAttrs()
	if attrs["method"] != "Dashboard" {
		t.Errorf("expected method=Dashboard, got %q", attrs["method"])
	}
}

// ============================================================
// Dashboard — slot status mismatch
// ============================================================

type wrongSlotStatusRepo struct{ stubRepository }

func (wrongSlotStatusRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{TotalReady: 0, TotalBlocked: 0},
		InProgress: []domain.IssueSummary{
			{ID: "x-1", Title: "Wrong status", Status: "closed", Type: "task"}, // VIOLATION: wrong slot
		},
		Closed:      []domain.IssueSummary{},
		ClosedTotal: 0,
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_SlotStatusMismatch(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(wrongSlotStatusRepo{})
	_, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("DashboardInProgressStatusMatches") {
		t.Errorf("expected warn with rule=DashboardInProgressStatusMatches; records=%v", h.records)
	}
}

// ============================================================
// Dashboard — ReadyAndBlockedDisjoint violation
// ============================================================

type overlapReadyBlockedRepo struct{ stubRepository }

func (overlapReadyBlockedRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	sharedIssue := domain.IssueSummary{ID: "overlap-1", Title: "Overlap", Status: "open", Type: "task"}
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{sharedIssue},
			Blocked: []domain.BlockedIssueView{
				{
					Issue:     sharedIssue,
					BlockedBy: []domain.IssueReference{{ID: "x-0", Title: "Blocker", Status: "closed"}},
				},
			},
			TotalReady:   1,
			TotalBlocked: 1,
		},
		InProgress:  []domain.IssueSummary{},
		Closed:      []domain.IssueSummary{},
		ClosedTotal: 0,
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_ReadyAndBlockedDisjointViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(overlapReadyBlockedRepo{})
	_, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("ReadyAndBlockedDisjoint") {
		t.Errorf("expected warn with rule=ReadyAndBlockedDisjoint; records=%v", h.records)
	}
}

// ============================================================
// Dashboard — TotalReadyMatchesLenReady violation
// ============================================================

type readyTotalMismatchRepo struct{ stubRepository }

func (readyTotalMismatchRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready:        []domain.IssueSummary{{ID: "r-1", Title: "Ready", Status: "open", Type: "task"}},
			Blocked:      []domain.BlockedIssueView{},
			TotalReady:   99, // VIOLATION: should be 1
			TotalBlocked: 0,
		},
		InProgress:  []domain.IssueSummary{},
		Closed:      []domain.IssueSummary{},
		ClosedTotal: 0,
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_TotalReadyMismatch(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(readyTotalMismatchRepo{})
	_, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("TotalReadyMatchesLenReady") {
		t.Errorf("expected warn with rule=TotalReadyMatchesLenReady; records=%v", h.records)
	}
}

// ============================================================
// Dashboard — BlockedByEnriched violation
// ============================================================

type unenrichedBlockedByRepo struct{ stubRepository }

func (unenrichedBlockedByRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{},
			Blocked: []domain.BlockedIssueView{
				{
					Issue: domain.IssueSummary{ID: "b-1", Title: "Blocked", Status: "open", Type: "task"},
					BlockedBy: []domain.IssueReference{
						{ID: "x-0", Title: "", Status: ""}, // VIOLATION: not enriched
					},
				},
			},
			TotalReady:   0,
			TotalBlocked: 1,
		},
		InProgress:  []domain.IssueSummary{},
		Closed:      []domain.IssueSummary{},
		ClosedTotal: 0,
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_BlockedByEnrichedViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(unenrichedBlockedByRepo{})
	_, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("BlockedByEnriched") {
		t.Errorf("expected warn with rule=BlockedByEnriched; records=%v", h.records)
	}
}

// ============================================================
// Dashboard — SsomClosedTotalGEQLen violation
// ============================================================

type ssomViolationRepo struct{ stubRepository }

func (ssomViolationRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{TotalReady: 0, TotalBlocked: 0},
		InProgress:   []domain.IssueSummary{},
		Closed: []domain.IssueSummary{
			{ID: "c-1", Title: "Closed", Status: "closed", Type: "task"},
			{ID: "c-2", Title: "Closed2", Status: "closed", Type: "task"},
		},
		ClosedTotal: 1, // VIOLATION: ClosedTotal=1 < len(Closed)=2
		Blocked:     []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_SsomViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(ssomViolationRepo{})
	data, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Return value must be the inner's value unchanged.
	if len(data.Closed) != 2 {
		t.Errorf("expected inner data returned unchanged, got %d Closed items", len(data.Closed))
	}
	if !h.hasWarnWithRule("SsomClosedTotalGEQLen") {
		t.Errorf("expected warn with rule=SsomClosedTotalGEQLen; records=%v", h.records)
	}
}

// ============================================================
// Issue — ReturnedIDMatchesInput violation
// ============================================================

type mismatchedIDRepo struct{ stubRepository }

func (mismatchedIDRepo) Issue(_ context.Context, _ string) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "wrong-id", Title: "Wrong", Status: "open", Type: "task"},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}

func TestValidatingRepository_Issue_IDMismatchViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(mismatchedIDRepo{})
	detail, err := repo.Issue(context.Background(), "requested-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Return value preserved.
	if detail.Summary.ID != "wrong-id" {
		t.Errorf("expected inner value returned unchanged, got %q", detail.Summary.ID)
	}
	if !h.hasWarnWithRule("ReturnedIDMatchesInput") {
		t.Errorf("expected warn with rule=ReturnedIDMatchesInput; records=%v", h.records)
	}
	attrs := h.warnAttrs()
	if attrs["method"] != "Issue" {
		t.Errorf("expected method=Issue, got %q", attrs["method"])
	}
}

// ============================================================
// Issue — CommentsNotNil violation
// ============================================================

type nilCommentsRepo struct{ stubRepository }

func (nilCommentsRepo) Issue(_ context.Context, id string) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: id, Title: "Issue", Status: "open", Type: "task"},
		Comments:  nil, // VIOLATION
		BlockedBy: []domain.IssueReference{},
	}, nil
}

func TestValidatingRepository_Issue_NilCommentsViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(nilCommentsRepo{})
	_, err := repo.Issue(context.Background(), "x-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("CommentsNotNil") {
		t.Errorf("expected warn with rule=CommentsNotNil; records=%v", h.records)
	}
}

// ============================================================
// Issue — BlockedByNotNil violation
// ============================================================

type nilBlockedByRepo struct{ stubRepository }

func (nilBlockedByRepo) Issue(_ context.Context, id string) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: id, Title: "Issue", Status: "open", Type: "task"},
		Comments:  []domain.IssueComment{},
		BlockedBy: nil, // VIOLATION
	}, nil
}

func TestValidatingRepository_Issue_NilBlockedByViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(nilBlockedByRepo{})
	_, err := repo.Issue(context.Background(), "x-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("BlockedByNotNil") {
		t.Errorf("expected warn with rule=BlockedByNotNil; records=%v", h.records)
	}
}

// ============================================================
// Search — ResultsNotNil violation
// ============================================================

type nilResultsRepo struct{ stubRepository }

func (nilResultsRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{
		Results:  nil, // VIOLATION
		Metadata: domain.SearchResultMetadata{ReturnedCount: 0},
	}, nil
}

func TestValidatingRepository_Search_NilResultsViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(nilResultsRepo{})
	_, err := repo.Search(context.Background(), domain.SearchIssuesQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("ResultsNotNil") {
		t.Errorf("expected warn with rule=ResultsNotNil; records=%v", h.records)
	}
}

// ============================================================
// Search — ReturnedCountMatchesLen violation
// ============================================================

type countMismatchSearchRepo struct{ stubRepository }

func (countMismatchSearchRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{
		Results: []domain.SearchResult{
			{Issue: domain.IssueSummary{ID: "s-1", Title: "Issue", Status: "open", Type: "task"}},
		},
		Metadata: domain.SearchResultMetadata{ReturnedCount: 99}, // VIOLATION: should be 1
	}, nil
}

func TestValidatingRepository_Search_ReturnedCountMismatchViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(countMismatchSearchRepo{})
	page, err := repo.Search(context.Background(), domain.SearchIssuesQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Return value must be the inner's value unchanged.
	if len(page.Results) != 1 {
		t.Errorf("expected inner data returned unchanged, got %d results", len(page.Results))
	}
	if !h.hasWarnWithRule("ReturnedCountMatchesLen") {
		t.Errorf("expected warn with rule=ReturnedCountMatchesLen; records=%v", h.records)
	}
	attrs := h.warnAttrs()
	if attrs["method"] != "Search" {
		t.Errorf("expected method=Search, got %q", attrs["method"])
	}
}

// ============================================================
// Search — SearchStatusFilterRespected violation
// ============================================================

type statusFilterViolationRepo struct{ stubRepository }

func (statusFilterViolationRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	results := []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "s-1", Title: "Open issue", Status: "open", Type: "task"}},     // matches
		{Issue: domain.IssueSummary{ID: "s-2", Title: "Closed issue", Status: "closed", Type: "task"}}, // VIOLATION
	}
	return domain.SearchResultPage{
		Results:  results,
		Metadata: domain.SearchResultMetadata{ReturnedCount: len(results)},
	}, nil
}

func TestValidatingRepository_Search_StatusFilterViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(statusFilterViolationRepo{})
	_, err := repo.Search(context.Background(), domain.SearchIssuesQuery{Statuses: []string{"open"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("SearchStatusFilterRespected") {
		t.Errorf("expected warn with rule=SearchStatusFilterRespected; records=%v", h.records)
	}
}

// ============================================================
// Catalogs — CatalogsStatusNonEmpty violation
// ============================================================

type emptyStatusCatalogRepo struct{ stubRepository }

func (emptyStatusCatalogRepo) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{
		Statuses: []domain.StatusOption{}, // VIOLATION: must be non-empty
		Types:    []domain.TypeOption{{Name: "task"}},
		Labels:   nil,
	}, nil
}

func TestValidatingRepository_Catalogs_EmptyStatusesViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(emptyStatusCatalogRepo{})
	_, err := repo.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("CatalogsStatusNonEmpty") {
		t.Errorf("expected warn with rule=CatalogsStatusNonEmpty; records=%v", h.records)
	}
	attrs := h.warnAttrs()
	if attrs["method"] != "Catalogs" {
		t.Errorf("expected method=Catalogs, got %q", attrs["method"])
	}
}

// ============================================================
// Catalogs — CatalogsTypeNonEmpty violation
// ============================================================

type emptyTypesCatalogRepo struct{ stubRepository }

func (emptyTypesCatalogRepo) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{
		Statuses: []domain.StatusOption{{Name: "open"}},
		Types:    []domain.TypeOption{}, // VIOLATION: must be non-empty
		Labels:   nil,
	}, nil
}

func TestValidatingRepository_Catalogs_EmptyTypesViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(emptyTypesCatalogRepo{})
	_, err := repo.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("CatalogsTypeNonEmpty") {
		t.Errorf("expected warn with rule=CatalogsTypeNonEmpty; records=%v", h.records)
	}
}

// ============================================================
// Catalogs — CatalogsLabelAllNamesNonEmpty violation
// ============================================================

type emptyLabelNameRepo struct{ stubRepository }

func (emptyLabelNameRepo) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{
		Statuses: []domain.StatusOption{{Name: "open"}},
		Types:    []domain.TypeOption{{Name: "task"}},
		Labels:   []domain.LabelOption{{Name: ""}}, // VIOLATION
	}, nil
}

func TestValidatingRepository_Catalogs_EmptyLabelNameViolation(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(emptyLabelNameRepo{})
	_, err := repo.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.hasWarnWithRule("CatalogsLabelAllNamesNonEmpty") {
		t.Errorf("expected warn with rule=CatalogsLabelAllNamesNonEmpty; records=%v", h.records)
	}
}

// ============================================================
// High-cardinality Dashboard spot-check
// ============================================================

type highCardDashboardRepo struct{ stubRepository }

func (highCardDashboardRepo) Dashboard(_ context.Context) (repository.DashboardData, error) {
	// Build 5001 InProgress items; item[1] has empty ID — in the first-10 spot.
	items := make([]domain.IssueSummary, 5001)
	for i := range items {
		items[i] = domain.IssueSummary{
			ID:     "item-x",
			Title:  "item",
			Status: "in_progress",
			Type:   "task",
		}
	}
	items[1].ID = "" // VIOLATION at index 1 — within first-10 spot

	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{TotalReady: 0, TotalBlocked: 0},
		InProgress:   items,
		Closed:       []domain.IssueSummary{},
		ClosedTotal:  0,
		Blocked:      []domain.IssueSummary{},
	}, nil
}

func TestValidatingRepository_Dashboard_HighCardinalitySpotCheck(t *testing.T) {
	t.Parallel()
	repo, h := newCapturingRepo(highCardDashboardRepo{})
	data, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.InProgress) != 5001 {
		t.Fatalf("expected 5001 InProgress items unchanged, got %d", len(data.InProgress))
	}
	// Violation at index 1 must be caught by spot-check.
	if !h.hasWarnWithRule("NonEmptyID") {
		t.Errorf("expected warn with rule=NonEmptyID for spot-check at index 1; records=%v", h.records)
	}
}

// ============================================================
// Write methods — pure delegation, no validation
// ============================================================

func TestValidatingRepository_WritesDelegate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, h := newCapturingRepo(stubRepository{})

	result, err := repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if result.IssueID != "new-1" {
		t.Errorf("expected IssueID=new-1, got %q", result.IssueID)
	}
	if err := repo.UpdateIssue(ctx, "s-1", domain.UpdateIssueInput{}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if err := repo.CloseIssue(ctx, "s-1", domain.CloseIssueInput{}); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if err := repo.AddComment(ctx, "s-1", domain.AddCommentInput{Body: "hi"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if h.warnCount() != 0 {
		t.Errorf("expected zero warnings for write calls, got %d", h.warnCount())
	}
}
