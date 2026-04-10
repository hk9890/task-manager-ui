package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
)

// Service defines the rich issue external-editor flow.
type Service interface {
	EditIssue(ctx context.Context, issueID string) (Result, error)
}

// Result reports whether an editor flow produced issue updates.
type Result struct {
	Updated bool
}

// Opener is the replaceable seam for launching the editor subprocess.
// Tests should use fakes so no interactive editor process is spawned.
type Opener interface {
	Open(ctx context.Context, path string) error
}

// IssueEditor applies rich editor updates through the official gateway.
type IssueEditor struct {
	gateway beads.BeadsGateway
	opener  Opener
	tempDir string
}

var _ Service = (*IssueEditor)(nil)

// NewIssueEditor builds the default issue editor flow.
func NewIssueEditor(gateway beads.BeadsGateway, opener Opener) (*IssueEditor, error) {
	if gateway == nil {
		return nil, fmt.Errorf("gateway is required")
	}

	if opener == nil {
		return nil, fmt.Errorf("editor opener is required")
	}

	return &IssueEditor{
		gateway: gateway,
		opener:  opener,
		tempDir: os.TempDir(),
	}, nil
}

// EditIssue runs the issue document round-trip and applies updates when changed.
func (e *IssueEditor) EditIssue(ctx context.Context, issueID string) (Result, error) {
	issue, err := e.gateway.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if err != nil {
		return Result{}, err
	}

	doc := domain.RenderIssueEditDocument(issue)

	path, err := e.writeTempDocument(issueID, doc)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		_ = os.Remove(path)
	}()

	if err := e.opener.Open(ctx, path); err != nil {
		return Result{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read edited document: %w", err)
	}

	edited, err := domain.ParseIssueEditDocument(string(content))
	if err != nil {
		return Result{}, err
	}

	input, changed := domain.BuildIssueUpdateInput(issue, edited)
	if !changed {
		return Result{Updated: false}, nil
	}

	if err := e.gateway.UpdateIssue(ctx, issueID, input); err != nil {
		return Result{}, err
	}

	return Result{Updated: true}, nil
}

func (e *IssueEditor) writeTempDocument(issueID, content string) (string, error) {
	if e.tempDir == "" {
		e.tempDir = os.TempDir()
	}

	pattern := fmt.Sprintf("bwb-issue-%s-*.md", issueID)
	file, err := os.CreateTemp(e.tempDir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temporary issue document: %w", err)
	}

	path := filepath.Clean(file.Name())
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temporary issue document: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temporary issue document: %w", err)
	}

	return path, nil
}
