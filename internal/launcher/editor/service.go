package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// Service defines the rich issue external-editor flow split into a prepare and
// apply phase so that tea.Exec can sit between them.
type Service interface {
	// PrepareDocument fetches the issue, renders the edit document, and writes
	// it to a temp file. The returned Prepared value carries the temp path and
	// original issue; it must be passed back to ApplyEdits after the editor exits.
	PrepareDocument(ctx context.Context, issueID string) (Prepared, error)

	// ApplyEdits reads the edited temp file, parses it, and applies changes via
	// the repository when the document differs from the original issue. The temp
	// file is removed regardless of the outcome.
	ApplyEdits(ctx context.Context, issueID string, issue domain.IssueDetail, path string) (Result, error)

	// BuildEditorCmd constructs the *exec.Cmd that opens path in the configured
	// editor. The command is passed to tea.Exec by the model.
	BuildEditorCmd(path string) (*exec.Cmd, error)
}

// Prepared holds the intermediate state between PrepareDocument and ApplyEdits.
type Prepared struct {
	// IssueID is the issue being edited.
	IssueID string
	// Issue is the original issue fetched in PrepareDocument.
	Issue domain.IssueDetail
	// TempPath is the path to the temporary edit document.
	TempPath string
}

// Result reports whether an editor flow produced issue updates.
type Result struct {
	Updated bool
}

// IssueEditor applies rich editor updates through the repository.
type IssueEditor struct {
	repo          repository.Repository
	editorCommand string
	tempDir       string
}

var _ Service = (*IssueEditor)(nil)

// NewIssueEditor builds the default issue editor flow.
func NewIssueEditor(repo repository.Repository, editorCommand string) (*IssueEditor, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo is required")
	}

	return &IssueEditor{
		repo:          repo,
		editorCommand: editorCommand,
		tempDir:       os.TempDir(),
	}, nil
}

// PrepareDocument fetches the issue, renders its edit document, and writes a
// temp file. The caller must eventually pass TempPath to ApplyEdits (which
// removes the file).
func (e *IssueEditor) PrepareDocument(ctx context.Context, issueID string) (Prepared, error) {
	issue, err := e.repo.Issue(ctx, issueID)
	if err != nil {
		return Prepared{}, err
	}

	doc := domain.RenderIssueEditDocument(issue)

	path, err := e.writeTempDocument(issueID, doc)
	if err != nil {
		return Prepared{}, err
	}

	return Prepared{
		IssueID:  issueID,
		Issue:    issue,
		TempPath: path,
	}, nil
}

// ApplyEdits reads the edited temp file, parses it, diffs against the original,
// and calls UpdateIssue when changed. The temp file is removed on all paths.
func (e *IssueEditor) ApplyEdits(ctx context.Context, issueID string, issue domain.IssueDetail, path string) (Result, error) {
	defer func() {
		_ = os.Remove(path)
	}()

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

	if err := e.repo.UpdateIssue(ctx, issueID, input); err != nil {
		return Result{}, err
	}

	return Result{Updated: true}, nil
}

// BuildEditorCmd constructs the *exec.Cmd that opens path in the configured
// editor. The model passes this to tea.Exec for terminal handover.
func (e *IssueEditor) BuildEditorCmd(path string) (*exec.Cmd, error) {
	return buildEditorCmd(e.editorCommand, path)
}

// buildEditorCmd parses the editor command string and appends path.
func buildEditorCmd(editorCommand, path string) (*exec.Cmd, error) {
	command, args, err := splitEditorCommand(resolveEditorCommand(editorCommand))
	if err != nil {
		return nil, err
	}
	args = append(args, path)
	return exec.Command(command, args...), nil
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
