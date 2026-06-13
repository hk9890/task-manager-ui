package fakes

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

func TestFakeLauncherRecordsActionAndIssue(t *testing.T) {
	t.Parallel()

	fake := &FakeLauncher{}
	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-9"}}

	err := fake.Launch(context.Background(), "open-editor", issue)
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if len(fake.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(fake.Calls))
	}

	if fake.Calls[0].Action != "open-editor" || fake.Calls[0].Issue.Summary.ID != "bw-9" {
		t.Fatalf("unexpected call payload: %#v", fake.Calls[0])
	}
}

func TestFakeLauncherReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("launch failed")
	fake := &FakeLauncher{Err: wantErr}

	err := fake.Launch(context.Background(), "open-editor", domain.IssueDetail{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}
