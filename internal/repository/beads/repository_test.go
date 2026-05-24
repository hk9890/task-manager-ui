package beads_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	repbeads "github.com/hk9890/beads-workbench/internal/repository/beads"
)

// stubGateway is a hand-rolled stub of gateway/beads.BeadsGateway. Each field
// is a function that overrides the default (error) behaviour for that method.
// Only the fields exercised by the test under examination need to be set; all
// other methods return a sentinel "not configured" error so a mis-wired test
// fails loudly rather than silently succeeding.
//
// This stub is intentionally NOT the existing FakeBeadsGateway; the translator
// tests must not couple to that type's implementation details.
type stubGateway struct {
	healthCheckFn   func(ctx context.Context) error
	listIssuesFn    func(ctx context.Context, q domain.IssueListQuery) ([]domain.IssueSummary, error)
	readyIssuesFn   func(ctx context.Context, q domain.ReadyIssuesQuery) ([]domain.IssueSummary, error)
	queryFn         func(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error)
	blockedIssuesFn func(ctx context.Context, q domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error)
	readyExplainFn  func(ctx context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error)
	showIssueFn     func(ctx context.Context, q domain.ShowIssueQuery) (domain.IssueDetail, error)
	searchIssuesFn  func(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error)
	countIssuesFn   func(ctx context.Context, q domain.IssueCountQuery) (domain.IssueCountResult, error)
	createIssueFn   func(ctx context.Context, i domain.CreateIssueInput) (domain.CreateIssueResult, error)
	updateIssueFn   func(ctx context.Context, id string, i domain.UpdateIssueInput) error
	closeIssueFn    func(ctx context.Context, id string, i domain.CloseIssueInput) error
	addCommentFn    func(ctx context.Context, id string, i domain.AddCommentInput) error
	statusCatalogFn func(ctx context.Context) ([]domain.StatusOption, error)
	typeCatalogFn   func(ctx context.Context) ([]domain.TypeOption, error)
	labelCatalogFn  func(ctx context.Context) ([]domain.LabelOption, error)
}

var errNotConfigured = errors.New("stub: method not configured for this test")

func (s *stubGateway) HealthCheck(ctx context.Context) error {
	if s.healthCheckFn != nil {
		return s.healthCheckFn(ctx)
	}
	return errNotConfigured
}

func (s *stubGateway) ListIssues(ctx context.Context, q domain.IssueListQuery) ([]domain.IssueSummary, error) {
	if s.listIssuesFn != nil {
		return s.listIssuesFn(ctx, q)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) ReadyIssues(ctx context.Context, q domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	if s.readyIssuesFn != nil {
		return s.readyIssuesFn(ctx, q)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) Query(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, expr, opts)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) BlockedIssues(ctx context.Context, q domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	if s.blockedIssuesFn != nil {
		return s.blockedIssuesFn(ctx, q)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) ReadyExplain(ctx context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	if s.readyExplainFn != nil {
		return s.readyExplainFn(ctx, opts)
	}
	return domain.ReadyExplainResult{}, errNotConfigured
}

func (s *stubGateway) ShowIssue(ctx context.Context, q domain.ShowIssueQuery) (domain.IssueDetail, error) {
	if s.showIssueFn != nil {
		return s.showIssueFn(ctx, q)
	}
	return domain.IssueDetail{}, errNotConfigured
}

func (s *stubGateway) SearchIssues(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if s.searchIssuesFn != nil {
		return s.searchIssuesFn(ctx, q)
	}
	return domain.SearchResultPage{}, errNotConfigured
}

func (s *stubGateway) CountIssues(ctx context.Context, q domain.IssueCountQuery) (domain.IssueCountResult, error) {
	if s.countIssuesFn != nil {
		return s.countIssuesFn(ctx, q)
	}
	return domain.IssueCountResult{}, errNotConfigured
}

func (s *stubGateway) CreateIssue(ctx context.Context, i domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	if s.createIssueFn != nil {
		return s.createIssueFn(ctx, i)
	}
	return domain.CreateIssueResult{}, errNotConfigured
}

func (s *stubGateway) UpdateIssue(ctx context.Context, id string, i domain.UpdateIssueInput) error {
	if s.updateIssueFn != nil {
		return s.updateIssueFn(ctx, id, i)
	}
	return errNotConfigured
}

func (s *stubGateway) CloseIssue(ctx context.Context, id string, i domain.CloseIssueInput) error {
	if s.closeIssueFn != nil {
		return s.closeIssueFn(ctx, id, i)
	}
	return errNotConfigured
}

func (s *stubGateway) AddComment(ctx context.Context, id string, i domain.AddCommentInput) error {
	if s.addCommentFn != nil {
		return s.addCommentFn(ctx, id, i)
	}
	return errNotConfigured
}

func (s *stubGateway) StatusCatalog(ctx context.Context) ([]domain.StatusOption, error) {
	if s.statusCatalogFn != nil {
		return s.statusCatalogFn(ctx)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) TypeCatalog(ctx context.Context) ([]domain.TypeOption, error) {
	if s.typeCatalogFn != nil {
		return s.typeCatalogFn(ctx)
	}
	return nil, errNotConfigured
}

func (s *stubGateway) LabelCatalog(ctx context.Context) ([]domain.LabelOption, error) {
	if s.labelCatalogFn != nil {
		return s.labelCatalogFn(ctx)
	}
	return nil, errNotConfigured
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck_Delegates(t *testing.T) {
	stub := &stubGateway{
		healthCheckFn: func(_ context.Context) error { return nil },
	}
	repo := repbeads.NewFromGateway(stub)
	if err := repo.HealthCheck(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealthCheck_BubblesError(t *testing.T) {
	gwErr := domain.GatewayError{Code: domain.ErrorCodeCommandUnavailable}
	stub := &stubGateway{
		healthCheckFn: func(_ context.Context) error { return gwErr },
	}
	repo := repbeads.NewFromGateway(stub)
	err := repo.HealthCheck(context.Background())
	var got domain.GatewayError
	if !errors.As(err, &got) {
		t.Fatalf("expected GatewayError, got %T: %v", err, err)
	}
	if got.Code != domain.ErrorCodeCommandUnavailable {
		t.Errorf("expected code %q, got %q", domain.ErrorCodeCommandUnavailable, got.Code)
	}
}

// ---------------------------------------------------------------------------
// Issue
// ---------------------------------------------------------------------------

func TestIssue_Delegates(t *testing.T) {
	want := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "abc-1", Title: "The issue"},
	}
	stub := &stubGateway{
		showIssueFn: func(_ context.Context, q domain.ShowIssueQuery) (domain.IssueDetail, error) {
			if q.IssueID != "abc-1" {
				t.Errorf("expected IssueID=abc-1, got %q", q.IssueID)
			}
			return want, nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	got, err := repo.Issue(context.Background(), "abc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.ID != want.Summary.ID {
		t.Errorf("got ID %q, want %q", got.Summary.ID, want.Summary.ID)
	}
}

func TestIssue_BubblesOriginalErrorCode(t *testing.T) {
	gwErr := domain.GatewayError{Code: domain.ErrorCodeCommandFailed, Operation: "show issue"}
	stub := &stubGateway{
		showIssueFn: func(_ context.Context, _ domain.ShowIssueQuery) (domain.IssueDetail, error) {
			return domain.IssueDetail{}, gwErr
		},
	}
	repo := repbeads.NewFromGateway(stub)
	_, err := repo.Issue(context.Background(), "unknown-id")
	var got domain.GatewayError
	if !errors.As(err, &got) {
		t.Fatalf("expected GatewayError, got %T: %v", err, err)
	}
	if got.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("expected code %q, got %q", domain.ErrorCodeCommandFailed, got.Code)
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch_Delegates(t *testing.T) {
	want := domain.SearchResultPage{
		Results: []domain.SearchResult{
			{Issue: domain.IssueSummary{ID: "s1"}},
		},
	}
	stub := &stubGateway{
		searchIssuesFn: func(_ context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
			return want, nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	got, err := repo.Search(context.Background(), domain.SearchIssuesQuery{Text: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Issue.ID != "s1" {
		t.Errorf("unexpected results: %+v", got.Results)
	}
}

// ---------------------------------------------------------------------------
// CreateIssue
// ---------------------------------------------------------------------------

func TestCreateIssue_Delegates(t *testing.T) {
	want := domain.CreateIssueResult{IssueID: "new-1"}
	stub := &stubGateway{
		createIssueFn: func(_ context.Context, i domain.CreateIssueInput) (domain.CreateIssueResult, error) {
			if i.Title != "New issue" {
				t.Errorf("expected Title=%q, got %q", "New issue", i.Title)
			}
			return want, nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	got, err := repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "New issue"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IssueID != "new-1" {
		t.Errorf("got IssueID %q, want new-1", got.IssueID)
	}
}

// ---------------------------------------------------------------------------
// UpdateIssue
// ---------------------------------------------------------------------------

func TestUpdateIssue_Delegates(t *testing.T) {
	called := false
	stub := &stubGateway{
		updateIssueFn: func(_ context.Context, id string, _ domain.UpdateIssueInput) error {
			if id != "abc-1" {
				t.Errorf("expected id=abc-1, got %q", id)
			}
			called = true
			return nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	if err := repo.UpdateIssue(context.Background(), "abc-1", domain.UpdateIssueInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected UpdateIssue to be called on gateway")
	}
}

// ---------------------------------------------------------------------------
// CloseIssue
// ---------------------------------------------------------------------------

func TestCloseIssue_Delegates(t *testing.T) {
	called := false
	stub := &stubGateway{
		closeIssueFn: func(_ context.Context, id string, i domain.CloseIssueInput) error {
			if id != "abc-2" {
				t.Errorf("expected id=abc-2, got %q", id)
			}
			if i.Reason != "done" {
				t.Errorf("expected Reason=done, got %q", i.Reason)
			}
			called = true
			return nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	if err := repo.CloseIssue(context.Background(), "abc-2", domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected CloseIssue to be called on gateway")
	}
}

// ---------------------------------------------------------------------------
// AddComment
// ---------------------------------------------------------------------------

func TestAddComment_Delegates(t *testing.T) {
	called := false
	stub := &stubGateway{
		addCommentFn: func(_ context.Context, id string, i domain.AddCommentInput) error {
			if id != "abc-3" {
				t.Errorf("expected id=abc-3, got %q", id)
			}
			if i.Body != "hello" {
				t.Errorf("expected Body=hello, got %q", i.Body)
			}
			called = true
			return nil
		},
	}
	repo := repbeads.NewFromGateway(stub)
	if err := repo.AddComment(context.Background(), "abc-3", domain.AddCommentInput{Body: "hello"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected AddComment to be called on gateway")
	}
}

// ---------------------------------------------------------------------------
// Dashboard — success path
// ---------------------------------------------------------------------------

func TestDashboard_SuccessPath(t *testing.T) {
	readyResult := domain.ReadyExplainResult{TotalReady: 3}
	inProgressIssues := []domain.IssueSummary{{ID: "ip-1"}}
	closedIssues := []domain.IssueSummary{{ID: "cl-1"}}
	blockedIssues := []domain.IssueSummary{{ID: "bl-1"}}

	stub := &stubGateway{
		readyExplainFn: func(_ context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
			if opts.Limit != 0 {
				t.Errorf("expected ReadyExplain Limit=0, got %d", opts.Limit)
			}
			return readyResult, nil
		},
		queryFn: func(_ context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
			switch expr {
			case "status=in_progress":
				if opts.Limit != 0 {
					t.Errorf("in_progress: expected Limit=0, got %d", opts.Limit)
				}
				return inProgressIssues, nil
			case "status=closed":
				if !opts.IncludeClosed {
					t.Error("closed query: expected IncludeClosed=true")
				}
				if opts.SortBy != domain.SortFieldClosedAt {
					t.Errorf("closed query: expected SortBy=closed_at, got %q", opts.SortBy)
				}
				if opts.SortOrder != domain.SortDirectionDescending {
					t.Errorf("closed query: expected SortOrder=desc, got %q", opts.SortOrder)
				}
				if opts.Limit != 50 {
					t.Errorf("closed query: expected Limit=50, got %d", opts.Limit)
				}
				return closedIssues, nil
			case "status=blocked":
				if opts.Limit != 0 {
					t.Errorf("blocked: expected Limit=0, got %d", opts.Limit)
				}
				return blockedIssues, nil
			default:
				t.Errorf("unexpected Query expr=%q", expr)
				return nil, errNotConfigured
			}
		},
		countIssuesFn: func(_ context.Context, q domain.IssueCountQuery) (domain.IssueCountResult, error) {
			if len(q.Statuses) != 1 || q.Statuses[0] != "closed" {
				t.Errorf("expected Statuses=[closed], got %v", q.Statuses)
			}
			return domain.IssueCountResult{Total: 42}, nil
		},
	}

	repo := repbeads.NewFromGateway(stub)
	data, err := repo.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.ReadyExplain.TotalReady != 3 {
		t.Errorf("ReadyExplain.TotalReady = %d, want 3", data.ReadyExplain.TotalReady)
	}
	if len(data.InProgress) != 1 || data.InProgress[0].ID != "ip-1" {
		t.Errorf("InProgress = %+v, want [{ip-1}]", data.InProgress)
	}
	if len(data.Closed) != 1 || data.Closed[0].ID != "cl-1" {
		t.Errorf("Closed = %+v, want [{cl-1}]", data.Closed)
	}
	if data.ClosedTotal != 42 {
		t.Errorf("ClosedTotal = %d, want 42", data.ClosedTotal)
	}
	if len(data.Blocked) != 1 || data.Blocked[0].ID != "bl-1" {
		t.Errorf("Blocked = %+v, want [{bl-1}]", data.Blocked)
	}
}

// ---------------------------------------------------------------------------
// Dashboard — partial-failure: one call returns an error → Dashboard fails
// ---------------------------------------------------------------------------

func TestDashboard_PartialFailure_ReturnsError(t *testing.T) {
	gwErr := domain.GatewayError{Code: domain.ErrorCodeCommandFailed, Operation: "query in_progress"}

	stub := &stubGateway{
		readyExplainFn: func(_ context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
			return domain.ReadyExplainResult{TotalReady: 1}, nil
		},
		queryFn: func(_ context.Context, expr string, _ domain.QueryOptions) ([]domain.IssueSummary, error) {
			if expr == "status=in_progress" {
				return nil, gwErr
			}
			return nil, nil
		},
		countIssuesFn: func(_ context.Context, _ domain.IssueCountQuery) (domain.IssueCountResult, error) {
			return domain.IssueCountResult{Total: 0}, nil
		},
	}

	repo := repbeads.NewFromGateway(stub)
	data, err := repo.Dashboard(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Result must be the zero value (no partial result returned).
	if data.ClosedTotal != 0 || data.InProgress != nil || data.Closed != nil {
		t.Errorf("expected zero DashboardData on error, got %+v", data)
	}
	// Error code must be preserved (no double-wrap).
	var got domain.GatewayError
	if !errors.As(err, &got) {
		t.Fatalf("expected GatewayError, got %T: %v", err, err)
	}
	if got.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("expected code %q, got %q", domain.ErrorCodeCommandFailed, got.Code)
	}
}

// ---------------------------------------------------------------------------
// Dashboard — context cancellation
// ---------------------------------------------------------------------------

func TestDashboard_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// All gateway calls should observe the cancelled context.
	stub := &stubGateway{
		readyExplainFn: func(ctx context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
			return domain.ReadyExplainResult{}, ctx.Err()
		},
		queryFn: func(ctx context.Context, _ string, _ domain.QueryOptions) ([]domain.IssueSummary, error) {
			return nil, ctx.Err()
		},
		countIssuesFn: func(ctx context.Context, _ domain.IssueCountQuery) (domain.IssueCountResult, error) {
			return domain.IssueCountResult{}, ctx.Err()
		},
	}

	repo := repbeads.NewFromGateway(stub)
	_, err := repo.Dashboard(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Catalogs — success path
// ---------------------------------------------------------------------------

func TestCatalogs_SuccessPath(t *testing.T) {
	statuses := []domain.StatusOption{{Name: "open"}, {Name: "closed"}}
	types := []domain.TypeOption{{Name: "task"}, {Name: "bug"}}
	labels := []domain.LabelOption{{Name: "urgent"}}

	stub := &stubGateway{
		statusCatalogFn: func(_ context.Context) ([]domain.StatusOption, error) {
			return statuses, nil
		},
		typeCatalogFn: func(_ context.Context) ([]domain.TypeOption, error) {
			return types, nil
		},
		labelCatalogFn: func(_ context.Context) ([]domain.LabelOption, error) {
			return labels, nil
		},
	}

	repo := repbeads.NewFromGateway(stub)
	cats, err := repo.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats.Statuses) != 2 {
		t.Errorf("Statuses: got %d, want 2", len(cats.Statuses))
	}
	if len(cats.Types) != 2 {
		t.Errorf("Types: got %d, want 2", len(cats.Types))
	}
	if len(cats.Labels) != 1 || cats.Labels[0].Name != "urgent" {
		t.Errorf("Labels: got %+v, want [{urgent}]", cats.Labels)
	}
}

// ---------------------------------------------------------------------------
// Catalogs — partial-failure
// ---------------------------------------------------------------------------

func TestCatalogs_PartialFailure_ReturnsError(t *testing.T) {
	gwErr := domain.GatewayError{Code: domain.ErrorCodeDecodeFailed, Operation: "type catalog"}

	stub := &stubGateway{
		statusCatalogFn: func(_ context.Context) ([]domain.StatusOption, error) {
			return []domain.StatusOption{{Name: "open"}}, nil
		},
		typeCatalogFn: func(_ context.Context) ([]domain.TypeOption, error) {
			return nil, gwErr
		},
		labelCatalogFn: func(_ context.Context) ([]domain.LabelOption, error) {
			return []domain.LabelOption{{Name: "p1"}}, nil
		},
	}

	repo := repbeads.NewFromGateway(stub)
	cats, err := repo.Catalogs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Result must be the zero value.
	if cats.Statuses != nil || cats.Types != nil || cats.Labels != nil {
		t.Errorf("expected zero Catalogs on error, got %+v", cats)
	}
	// Original code preserved.
	var got domain.GatewayError
	if !errors.As(err, &got) {
		t.Fatalf("expected GatewayError, got %T: %v", err, err)
	}
	if got.Code != domain.ErrorCodeDecodeFailed {
		t.Errorf("expected code %q, got %q", domain.ErrorCodeDecodeFailed, got.Code)
	}
}

// ---------------------------------------------------------------------------
// Catalogs — context cancellation
// ---------------------------------------------------------------------------

func TestCatalogs_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stub := &stubGateway{
		statusCatalogFn: func(ctx context.Context) ([]domain.StatusOption, error) {
			return nil, ctx.Err()
		},
		typeCatalogFn: func(ctx context.Context) ([]domain.TypeOption, error) {
			return nil, ctx.Err()
		},
		labelCatalogFn: func(ctx context.Context) ([]domain.LabelOption, error) {
			return nil, ctx.Err()
		},
	}

	repo := repbeads.NewFromGateway(stub)
	_, err := repo.Catalogs(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Error code preservation — no double-wrap
// ---------------------------------------------------------------------------

// TestErrorPreservation_NoDoubleWrap verifies that errors returned from the
// gateway are surfaced unchanged without additional wrapping. Callers must be
// able to errors.As to *domain.GatewayError with the original Code.
func TestErrorPreservation_NoDoubleWrap(t *testing.T) {
	cases := []struct {
		name   string
		code   domain.ErrorCode
		callFn func(repo repository.Repository) error
		stubFn func(code domain.ErrorCode) *stubGateway
	}{
		{
			name: "HealthCheck",
			code: domain.ErrorCodeCommandUnavailable,
			callFn: func(repo repository.Repository) error {
				return repo.HealthCheck(context.Background())
			},
			stubFn: func(code domain.ErrorCode) *stubGateway {
				return &stubGateway{
					healthCheckFn: func(_ context.Context) error {
						return domain.GatewayError{Code: code}
					},
				}
			},
		},
		{
			name: "Issue",
			code: domain.ErrorCodeCommandFailed,
			callFn: func(repo repository.Repository) error {
				_, err := repo.Issue(context.Background(), "x")
				return err
			},
			stubFn: func(code domain.ErrorCode) *stubGateway {
				return &stubGateway{
					showIssueFn: func(_ context.Context, _ domain.ShowIssueQuery) (domain.IssueDetail, error) {
						return domain.IssueDetail{}, domain.GatewayError{Code: code}
					},
				}
			},
		},
		{
			name: "Search",
			code: domain.ErrorCodeCommandFailed,
			callFn: func(repo repository.Repository) error {
				_, err := repo.Search(context.Background(), domain.SearchIssuesQuery{})
				return err
			},
			stubFn: func(code domain.ErrorCode) *stubGateway {
				return &stubGateway{
					searchIssuesFn: func(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
						return domain.SearchResultPage{}, domain.GatewayError{Code: code}
					},
				}
			},
		},
		{
			name: "CreateIssue",
			code: domain.ErrorCodeCommandFailed,
			callFn: func(repo repository.Repository) error {
				_, err := repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "t"})
				return err
			},
			stubFn: func(code domain.ErrorCode) *stubGateway {
				return &stubGateway{
					createIssueFn: func(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
						return domain.CreateIssueResult{}, domain.GatewayError{Code: code}
					},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stubFn(tc.code)
			repo := repbeads.NewFromGateway(stub)
			err := tc.callFn(repo)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var gwErr domain.GatewayError
			if !errors.As(err, &gwErr) {
				t.Fatalf("expected *domain.GatewayError, got %T: %v", err, err)
			}
			if gwErr.Code != tc.code {
				t.Errorf("code = %q, want %q (error was double-wrapped)", gwErr.Code, tc.code)
			}
		})
	}
}
