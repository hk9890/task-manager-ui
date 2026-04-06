package fakes

import (
	"context"
	"errors"
	"testing"

	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
)

func TestFakeEditorReturnsConfiguredResult(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{Result: launchereditor.Result{Updated: true}}

	got, err := fake.EditIssue(context.Background(), "bw-1")
	if err != nil {
		t.Fatalf("EditIssue returned error: %v", err)
	}

	if !got.Updated {
		t.Fatalf("expected updated result, got %#v", got)
	}

	if len(fake.Calls) != 1 || fake.Calls[0].IssueID != "bw-1" {
		t.Fatalf("unexpected recorded calls: %#v", fake.Calls)
	}
}

func TestFakeEditorReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("editor failed")
	fake := &FakeEditor{Err: wantErr}

	_, err := fake.EditIssue(context.Background(), "bw-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}
