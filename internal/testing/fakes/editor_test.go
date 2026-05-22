package fakes

import (
	"context"
	"errors"
	"testing"

	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
)

func TestFakeEditorPrepareDocumentReturnsConfiguredResult(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}

	prepared, err := fake.PrepareDocument(context.Background(), "bw-1")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}

	if prepared.IssueID != "bw-1" {
		t.Fatalf("expected IssueID bw-1, got %q", prepared.IssueID)
	}
	if prepared.TempPath == "" {
		t.Fatalf("expected non-empty TempPath")
	}

	if len(fake.Calls) != 1 || fake.Calls[0].IssueID != "bw-1" {
		t.Fatalf("unexpected recorded calls: %#v", fake.Calls)
	}
}

func TestFakeEditorPrepareDocumentReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("prepare failed")
	fake := &FakeEditor{PrepareErr: wantErr}

	_, err := fake.PrepareDocument(context.Background(), "bw-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}

func TestFakeEditorApplyEditsReturnsConfiguredResult(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}

	got, err := fake.ApplyEdits(context.Background(), "bw-1", launchereditor.Prepared{}.Issue, "fake-path")
	if err != nil {
		t.Fatalf("ApplyEdits returned error: %v", err)
	}

	if !got.Updated {
		t.Fatalf("expected updated result, got %#v", got)
	}
}

func TestFakeEditorApplyEditsReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("apply failed")
	fake := &FakeEditor{ApplyErr: wantErr}

	_, err := fake.ApplyEdits(context.Background(), "bw-1", launchereditor.Prepared{}.Issue, "fake-path")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}
