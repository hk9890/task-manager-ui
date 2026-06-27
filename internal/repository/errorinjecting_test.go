package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// fakeInner is a minimal happy-path Repository stub used by decorator tests.
// It records the last method called so tests can confirm delegation happened.
type fakeInner struct {
	lastCall string
}

var _ repository.Repository = (*fakeInner)(nil)

func (f *fakeInner) Dashboard(_ context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
	f.lastCall = "Dashboard"
	return repository.DashboardData{}, nil
}

func (f *fakeInner) Issue(_ context.Context, _ string) (domain.IssueDetail, error) {
	f.lastCall = "Issue"
	return domain.IssueDetail{}, nil
}

func (f *fakeInner) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	f.lastCall = "Search"
	return domain.SearchResultPage{}, nil
}

func (f *fakeInner) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	f.lastCall = "CreateIssue"
	return domain.CreateIssueResult{}, nil
}

func (f *fakeInner) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	f.lastCall = "UpdateIssue"
	return nil
}

func (f *fakeInner) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	f.lastCall = "CloseIssue"
	return nil
}

func (f *fakeInner) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	f.lastCall = "AddComment"
	return nil
}

func (f *fakeInner) HealthCheck(_ context.Context) error {
	f.lastCall = "HealthCheck"
	return nil
}

func (f *fakeInner) Catalogs(_ context.Context) (repository.Catalogs, error) {
	f.lastCall = "Catalogs"
	return repository.Catalogs{}, nil
}

// TestErrorInjecting_Delegation verifies that when no error is configured all
// 9 methods delegate to the inner repository and call tracking records the
// invocation.
func TestErrorInjecting_Delegation(t *testing.T) {
	ctx := context.Background()

	methods := []struct {
		name string
		call func(r repository.Repository) error
	}{
		{"Dashboard", func(r repository.Repository) error {
			_, err := r.Dashboard(ctx, repository.DashboardOptions{})
			return err
		}},
		{"Issue", func(r repository.Repository) error {
			_, err := r.Issue(ctx, "x")
			return err
		}},
		{"Search", func(r repository.Repository) error {
			_, err := r.Search(ctx, domain.SearchIssuesQuery{})
			return err
		}},
		{"CreateIssue", func(r repository.Repository) error {
			_, err := r.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"})
			return err
		}},
		{"UpdateIssue", func(r repository.Repository) error {
			return r.UpdateIssue(ctx, "x", domain.UpdateIssueInput{})
		}},
		{"CloseIssue", func(r repository.Repository) error {
			return r.CloseIssue(ctx, "x", domain.CloseIssueInput{})
		}},
		{"AddComment", func(r repository.Repository) error {
			return r.AddComment(ctx, "x", domain.AddCommentInput{Body: "hi"})
		}},
		{"HealthCheck", func(r repository.Repository) error {
			return r.HealthCheck(ctx)
		}},
		{"Catalogs", func(r repository.Repository) error {
			_, err := r.Catalogs(ctx)
			return err
		}},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			inner := &fakeInner{}
			ei := repository.NewErrorInjecting(inner)

			if err := m.call(ei); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inner.lastCall != m.name {
				t.Errorf("expected inner.lastCall=%q, got %q", m.name, inner.lastCall)
			}

			calls := ei.Calls()
			if len(calls) != 1 {
				t.Fatalf("expected 1 call recorded, got %d", len(calls))
			}
			if string(calls[0].Method) != m.name {
				t.Errorf("expected recorded method=%q, got %q", m.name, calls[0].Method)
			}
		})
	}
}

// TestErrorInjecting_InjectionPerMethod verifies that SetError causes the
// named method to return the injected error without calling inner.
func TestErrorInjecting_InjectionPerMethod(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("injected")

	methodCases := []struct {
		method repository.Method
		call   func(r repository.Repository) error
	}{
		{repository.MethodDashboard, func(r repository.Repository) error {
			_, err := r.Dashboard(ctx, repository.DashboardOptions{})
			return err
		}},
		{repository.MethodIssue, func(r repository.Repository) error {
			_, err := r.Issue(ctx, "x")
			return err
		}},
		{repository.MethodSearch, func(r repository.Repository) error {
			_, err := r.Search(ctx, domain.SearchIssuesQuery{})
			return err
		}},
		{repository.MethodCreateIssue, func(r repository.Repository) error {
			_, err := r.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"})
			return err
		}},
		{repository.MethodUpdateIssue, func(r repository.Repository) error {
			return r.UpdateIssue(ctx, "x", domain.UpdateIssueInput{})
		}},
		{repository.MethodCloseIssue, func(r repository.Repository) error {
			return r.CloseIssue(ctx, "x", domain.CloseIssueInput{})
		}},
		{repository.MethodAddComment, func(r repository.Repository) error {
			return r.AddComment(ctx, "x", domain.AddCommentInput{Body: "hi"})
		}},
		{repository.MethodHealthCheck, func(r repository.Repository) error {
			return r.HealthCheck(ctx)
		}},
		{repository.MethodCatalogs, func(r repository.Repository) error {
			_, err := r.Catalogs(ctx)
			return err
		}},
	}

	for _, tc := range methodCases {
		t.Run(string(tc.method), func(t *testing.T) {
			inner := &fakeInner{}
			ei := repository.NewErrorInjecting(inner)
			ei.SetError(tc.method, sentinel)

			err := tc.call(ei)
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected sentinel error, got %v", err)
			}
			// Inner must not have been called.
			if inner.lastCall != "" {
				t.Errorf("inner was called (%q) despite injected error", inner.lastCall)
			}
		})
	}
}

// TestErrorInjecting_ClearError verifies that passing nil to SetError restores
// delegation behaviour.
func TestErrorInjecting_ClearError(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("injected")

	inner := &fakeInner{}
	ei := repository.NewErrorInjecting(inner)

	// Inject an error, confirm it fires.
	ei.SetError(repository.MethodHealthCheck, sentinel)
	if err := ei.HealthCheck(ctx); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}

	// Clear the error; next call should delegate.
	ei.SetError(repository.MethodHealthCheck, nil)
	if err := ei.HealthCheck(ctx); err != nil {
		t.Fatalf("unexpected error after clear: %v", err)
	}
	if inner.lastCall != "HealthCheck" {
		t.Errorf("expected delegation after clear, inner.lastCall=%q", inner.lastCall)
	}
}

// TestErrorInjecting_CallTracking verifies that multiple calls across methods
// are recorded in order.
func TestErrorInjecting_CallTracking(t *testing.T) {
	ctx := context.Background()
	inner := &fakeInner{}
	ei := repository.NewErrorInjecting(inner)

	_ = ei.HealthCheck(ctx)
	_, _ = ei.Dashboard(ctx, repository.DashboardOptions{})
	_, _ = ei.Catalogs(ctx)

	calls := ei.Calls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(calls))
	}
	want := []repository.Method{
		repository.MethodHealthCheck,
		repository.MethodDashboard,
		repository.MethodCatalogs,
	}
	for i, w := range want {
		if calls[i].Method != w {
			t.Errorf("calls[%d].Method = %q, want %q", i, calls[i].Method, w)
		}
	}
}

// TestErrorInjecting_Snapshot verifies that Calls returns a copy — mutations
// to the returned slice do not affect subsequent calls.
func TestErrorInjecting_Snapshot(t *testing.T) {
	ctx := context.Background()
	inner := &fakeInner{}
	ei := repository.NewErrorInjecting(inner)

	_ = ei.HealthCheck(ctx)
	snap1 := ei.Calls()
	snap1[0].Method = "mutated"

	_ = ei.HealthCheck(ctx)
	snap2 := ei.Calls()

	if snap2[0].Method != repository.MethodHealthCheck {
		t.Errorf("snapshot mutation leaked into internal state: got %q", snap2[0].Method)
	}
	if len(snap2) != 2 {
		t.Errorf("expected 2 calls in second snapshot, got %d", len(snap2))
	}
}
