package fakes

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestFakeBeadsGatewayReturnsConfiguredResponses(t *testing.T) {
	t.Parallel()

	fake := NewFakeBeadsGateway()
	fake.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "one"}}
	fake.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "detail"}}

	issues, err := fake.ListIssues(context.Background(), domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("ListIssues returned error: %v", err)
	}

	detail, err := fake.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-1"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if len(issues) != 1 || issues[0].ID != "bw-1" {
		t.Fatalf("unexpected issues response: %#v", issues)
	}

	if detail.Summary.ID != "bw-1" || detail.Summary.Title != "detail" {
		t.Fatalf("unexpected detail response: %#v", detail)
	}

	if len(fake.Calls) != 2 {
		t.Fatalf("expected two recorded calls, got %d", len(fake.Calls))
	}
}

func TestFakeBeadsGatewaySupportsPerMethodErrorInjection(t *testing.T) {
	t.Parallel()

	fake := NewFakeBeadsGateway()
	wantErr := errors.New("gateway down")
	fake.SetError(MethodSearchIssues, wantErr)

	_, err := fake.SearchIssues(context.Background(), domain.SearchIssuesQuery{Text: "bug"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected injected error, got %v", err)
	}

	_, err = fake.ListIssues(context.Background(), domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("expected no error on other methods, got %v", err)
	}
}

func TestFakeBeadsGatewayResetCalls(t *testing.T) {
	t.Parallel()

	fake := NewFakeBeadsGateway()
	_, _ = fake.TypeCatalog(context.Background())
	_, _ = fake.LabelCatalog(context.Background())
	if len(fake.Calls) != 2 {
		t.Fatalf("expected two calls before reset, got %d", len(fake.Calls))
	}

	fake.ResetCalls()
	if len(fake.Calls) != 0 {
		t.Fatalf("expected no calls after reset, got %d", len(fake.Calls))
	}
}
