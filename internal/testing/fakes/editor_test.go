package fakes

import (
	"context"
	"errors"
	"testing"

	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
)

func TestFakeEditorPrepareDocumentReturnsConfiguredResult(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}

	prepared, err := fake.PrepareDocument(context.Background(), "tm-1")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}

	if prepared.IssueID != "tm-1" {
		t.Fatalf("expected IssueID tm-1, got %q", prepared.IssueID)
	}
	if prepared.TempPath == "" {
		t.Fatalf("expected non-empty TempPath")
	}

	if len(fake.Calls()) != 1 || fake.Calls()[0].IssueID != "tm-1" {
		t.Fatalf("unexpected recorded calls: %#v", fake.Calls())
	}
}

func TestFakeEditorPrepareDocumentReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("prepare failed")
	fake := &FakeEditor{PrepareErr: wantErr}

	_, err := fake.PrepareDocument(context.Background(), "tm-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}

func TestFakeEditorApplyEditsReturnsConfiguredResult(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}

	got, err := fake.ApplyEdits(context.Background(), "tm-1", launchereditor.Prepared{}.Issue, "fake-path")
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

	_, err := fake.ApplyEdits(context.Background(), "tm-1", launchereditor.Prepared{}.Issue, "fake-path")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}

// TestFakeEditorBuildEditorCmdReturnsARunnableNoOp pins the fake's documented
// contract: a real, runnable *exec.Cmd that does nothing. Returning nil left
// the whole repository green, because the seam is consumed from internal/app
// and no test there runs the command it is handed — so the nil would surface
// later, as a dereference inside the app, looking like an app bug rather than
// a harness one.
func TestFakeEditorBuildEditorCmdReturnsARunnableNoOp(t *testing.T) {
	t.Parallel()

	fake := &FakeEditor{}

	cmd, err := fake.BuildEditorCmd("/tmp/issue-edit.md")
	if err != nil {
		t.Fatalf("BuildEditorCmd returned error: %v", err)
	}
	if cmd == nil {
		t.Fatal("BuildEditorCmd returned a nil command: the caller would dereference it through tea.Exec")
	}
	if cmd.Path == "" {
		t.Error("command has no resolved path")
	}
	// It must actually run, and do nothing: that is what makes it safe for a
	// test to execute the editor handoff end to end.
	if err := cmd.Run(); err != nil {
		t.Errorf("the no-op editor command failed to run: %v", err)
	}
}
