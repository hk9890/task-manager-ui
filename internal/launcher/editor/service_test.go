package editor_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

func TestIssueEditorAppliesGatewayUpdateFromEditedDocument(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID: "bw-7", Title: "Old", Status: "open", Type: "task", Priority: 2, Assignee: "hans", Labels: []string{"one"},
	}, Description: "old desc"}
	// Seed into both the legacy verbatim field (for ShowIssueResponse fallback callers
	// that may still read it directly in test helpers) and the write-state store so that
	// UpdateIssue can find and mutate the issue.
	gateway.ShowIssueResponse = issue
	gateway.SeedIssue(issue)

	service, err := launchereditor.NewIssueEditor(gateway, "")
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	// Phase 1: prepare.
	prepared, err := service.PrepareDocument(context.Background(), "bw-7")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}
	defer func() { _ = os.Remove(prepared.TempPath) }()

	// Simulate editor: write an edited document to the temp file.
	rendered := domain.RenderIssueEditDocument(issue)
	edited := strings.Replace(rendered, "Old", "Updated title", 1)
	if err := os.WriteFile(prepared.TempPath, []byte(edited), 0o600); err != nil {
		t.Fatalf("WriteFile (simulate editor): %v", err)
	}

	// Phase 2: apply.
	result, err := service.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)
	if err != nil {
		t.Fatalf("ApplyEdits returned error: %v", err)
	}

	if !result.Updated {
		t.Fatalf("expected updated=true")
	}

	if !gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("expected UpdateIssue call, calls=%#v", gateway.Calls)
	}
}

func TestIssueEditorNoChangesSkipsGatewayUpdate(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-8", Title: "Same", Status: "open", Type: "task", Priority: 2}, Description: "same"}
	gateway.ShowIssueResponse = issue

	service, err := launchereditor.NewIssueEditor(gateway, "")
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	// Phase 1: prepare.
	prepared, err := service.PrepareDocument(context.Background(), "bw-8")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}
	defer func() { _ = os.Remove(prepared.TempPath) }()

	// Simulate editor: write back the document unchanged.
	rendered := domain.RenderIssueEditDocument(issue)
	if err := os.WriteFile(prepared.TempPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("WriteFile (simulate no-change): %v", err)
	}

	// Phase 2: apply.
	result, err := service.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)
	if err != nil {
		t.Fatalf("ApplyEdits returned error: %v", err)
	}

	if result.Updated {
		t.Fatalf("expected updated=false")
	}

	if gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("did not expect UpdateIssue call, calls=%#v", gateway.Calls)
	}
}

func TestIssueEditorApplyEditsReturnsParseError(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-9", Title: "Issue", Status: "open", Type: "task", Priority: 2}}
	gateway.ShowIssueResponse = issue

	service, err := launchereditor.NewIssueEditor(gateway, "")
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	// Phase 1: prepare.
	prepared, err := service.PrepareDocument(context.Background(), "bw-9")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}
	defer func() { _ = os.Remove(prepared.TempPath) }()

	// Simulate editor: write invalid content (missing markers).
	if err := os.WriteFile(prepared.TempPath, []byte("# invalid content no markers"), 0o600); err != nil {
		t.Fatalf("WriteFile (simulate bad edit): %v", err)
	}

	// Phase 2: apply — should return a parse error.
	_, err = service.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)
	if err == nil {
		t.Fatalf("expected parse error from ApplyEdits, got nil")
	}
}

func TestIssueEditorTempFileRemovedAfterApplyEdits(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-10", Title: "File", Status: "open", Type: "task", Priority: 2}, Description: "desc"}
	gateway.ShowIssueResponse = issue

	service, err := launchereditor.NewIssueEditor(gateway, "")
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	prepared, err := service.PrepareDocument(context.Background(), "bw-10")
	if err != nil {
		t.Fatalf("PrepareDocument returned error: %v", err)
	}

	// The temp file must exist before apply.
	if _, statErr := os.Stat(prepared.TempPath); errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected temp file to exist before ApplyEdits: %s", prepared.TempPath)
	}

	// Write back unchanged content so ApplyEdits doesn't need UpdateIssue.
	rendered := domain.RenderIssueEditDocument(issue)
	if err := os.WriteFile(prepared.TempPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("WriteFile (simulate no-change): %v", err)
	}

	_, _ = service.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)

	// The temp file must be removed after apply.
	if _, statErr := os.Stat(prepared.TempPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected temp file to be removed after ApplyEdits: %s", prepared.TempPath)
	}
}

func TestIssueEditorBuildEditorCmdReturnsCmd(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	service, err := launchereditor.NewIssueEditor(gateway, "nano -w")
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	cmd, err := service.BuildEditorCmd("/tmp/test-file.md")
	if err != nil {
		t.Fatalf("BuildEditorCmd returned error: %v", err)
	}

	if cmd == nil {
		t.Fatalf("expected non-nil *exec.Cmd")
	}

	// Check the command and last arg.
	if cmd.Path == "" {
		t.Fatalf("expected non-empty cmd.Path")
	}
	if len(cmd.Args) < 1 {
		t.Fatalf("expected at least one arg")
	}
	lastArg := cmd.Args[len(cmd.Args)-1]
	if lastArg != "/tmp/test-file.md" {
		t.Fatalf("expected last arg to be the file path, got %q", lastArg)
	}
}
