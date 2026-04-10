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

type fakeOpener struct {
	err   error
	calls []string
	edit  string
}

func (f *fakeOpener) Open(_ context.Context, path string) error {
	f.calls = append(f.calls, path)
	if f.edit != "" {
		if err := os.WriteFile(path, []byte(f.edit), 0o600); err != nil {
			return err
		}
	}
	return f.err
}

func TestIssueEditorAppliesGatewayUpdateFromEditedDocument(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{
		ID: "bw-7", Title: "Old", Status: "open", Type: "task", Priority: 2, Assignee: "hans", Labels: []string{"one"},
	}, Description: "old desc"}

	rendered := domain.RenderIssueEditDocument(gateway.ShowIssueResponse)
	edited := strings.Replace(rendered, "Old", "Updated title", 1)

	opener := &fakeOpener{edit: edited}
	service, err := launchereditor.NewIssueEditor(gateway, opener)
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	result, err := service.EditIssue(context.Background(), "bw-7")
	if err != nil {
		t.Fatalf("EditIssue returned error: %v", err)
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
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-8", Title: "Same", Status: "open", Type: "task", Priority: 2}, Description: "same"}

	rendered := domain.RenderIssueEditDocument(gateway.ShowIssueResponse)
	opener := &fakeOpener{edit: rendered}

	service, err := launchereditor.NewIssueEditor(gateway, opener)
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	result, err := service.EditIssue(context.Background(), "bw-8")
	if err != nil {
		t.Fatalf("EditIssue returned error: %v", err)
	}

	if result.Updated {
		t.Fatalf("expected updated=false")
	}

	if gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("did not expect UpdateIssue call, calls=%#v", gateway.Calls)
	}
}

func TestIssueEditorReturnsOpenerError(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-9", Title: "Issue", Status: "open", Type: "task", Priority: 2}}

	wantErr := errors.New("editor failed")
	opener := &fakeOpener{err: wantErr}

	service, err := launchereditor.NewIssueEditor(gateway, opener)
	if err != nil {
		t.Fatalf("NewIssueEditor returned error: %v", err)
	}

	_, err = service.EditIssue(context.Background(), "bw-9")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected opener error, got %v", err)
	}
}
