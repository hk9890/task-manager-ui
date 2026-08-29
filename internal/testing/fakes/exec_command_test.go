package fakes

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// seedEditDocument writes a document at the path the editor subprocess would be
// handed, and returns that path.
func seedEditDocument(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "issue-edit.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed edit document: %v", err)
	}

	return path
}

// runThroughFactory drives the fake the way tea.Exec does: build the command,
// hand it to the factory, then Run the returned ExecCommand.
func runThroughFactory(f *FakeExecCommand, path string) error {
	cmd := exec.Command("vi", path)

	return f.Factory()(cmd).Run()
}

func TestFakeExecCommandWritesEditedContentToTheCapturedPath(t *testing.T) {
	t.Parallel()

	path := seedEditDocument(t, "original")
	f := &FakeExecCommand{EditedContent: "edited by the operator"}

	if err := runThroughFactory(f, path); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edit document back: %v", err)
	}
	if string(got) != "edited by the operator" {
		t.Fatalf("edit document = %q, want %q", got, "edited by the operator")
	}
	if f.RunCount() != 1 {
		t.Fatalf("RunCount = %d, want 1", f.RunCount())
	}
}

// The empty-EditedContent contract is what every editor-handoff test reads as
// "the operator saved without changing anything". A fake that blanked the
// document instead would make those tests assert the wrong flow.
func TestFakeExecCommandLeavesTheDocumentUnchangedWhenEditedContentIsEmpty(t *testing.T) {
	t.Parallel()

	path := seedEditDocument(t, "original")
	f := &FakeExecCommand{}

	if err := runThroughFactory(f, path); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edit document back: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("edit document = %q, want it unchanged at %q", got, "original")
	}
}

// Run without a factory call has no captured path. It must not write anywhere,
// and it must still report RunErr — the two halves the && in Run controls.
func TestFakeExecCommandWithoutACapturedPathWritesNothingAndReturnsRunErr(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wantErr := errors.New("editor exited non-zero")
	f := &FakeExecCommand{EditedContent: "would-be edit", RunErr: wantErr}

	if err := f.Run(); !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Run wrote %d file(s) with no captured path", len(entries))
	}
}

func TestFakeExecCommandRunCountTracksEveryRun(t *testing.T) {
	t.Parallel()

	path := seedEditDocument(t, "original")
	f := &FakeExecCommand{}

	if f.RunCount() != 0 {
		t.Fatalf("RunCount before any run = %d, want 0", f.RunCount())
	}
	for i := 1; i <= 3; i++ {
		if err := runThroughFactory(f, path); err != nil {
			t.Fatalf("Run %d returned error: %v", i, err)
		}
		if f.RunCount() != i {
			t.Fatalf("RunCount after %d run(s) = %d", i, f.RunCount())
		}
	}
}

func TestFakeExecCommandFactoryCapturesTheLastArgumentAsThePath(t *testing.T) {
	t.Parallel()

	// The document path is the last argument of the built editor command, so a
	// multi-flag editor command (`code --wait <path>`) must still be captured.
	path := seedEditDocument(t, "original")
	f := &FakeExecCommand{EditedContent: "edited"}

	cmd := exec.Command("code", "--wait", path)
	if err := f.Factory()(cmd).Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edit document back: %v", err)
	}
	if string(got) != "edited" {
		t.Fatalf("edit document = %q, want %q — the factory captured the wrong argument", got, "edited")
	}
}
