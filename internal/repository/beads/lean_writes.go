package beads

import (
	"context"
	"errors"
	"strconv"
	"strings"

	bdrunner "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
)

// CreateIssue creates a new issue via `bd create --json`.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	args := []string{"create", "--json", "--title", input.Title}

	if input.Description != "" {
		args = append(args, "--description", input.Description)
	}
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}
	if input.Priority != nil {
		args = append(args, "--priority", strconv.Itoa(*input.Priority))
	}
	if input.Assignee != "" {
		args = append(args, "--assignee", input.Assignee)
	}
	if len(input.Labels) > 0 {
		args = append(args, "--labels", strings.Join(input.Labels, ","))
	}

	payload, err := repoRunJSON[leanCreateResultPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpCreateIssue,
		Args:      args,
		IsWrite:   true,
	})
	if err != nil {
		return domain.CreateIssueResult{}, err
	}

	if strings.TrimSpace(payload.ID) == "" {
		return domain.CreateIssueResult{}, leanNewGWError(leanErrorCodeDecodeFailed, leanOpCreateIssue, "failed to decode create issue output", nil)
	}

	return domain.CreateIssueResult{IssueID: payload.ID}, nil
}

// UpdateIssue applies a partial update via `bd update <id>`.
//
// ClearLabels workaround: bd 1.0.4 `bd update --set-labels ""` silently ignores
// the clear request. The lean Repository replicates the repository workaround:
// fetch current labels via Issue (bd show), then emit
// `bd update --remove-label <csv>` (singular flag). If no labels exist, skip.
func (r *Repository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	if strings.TrimSpace(id) == "" {
		return leanNewGWError(leanErrorCodeValidation, leanOpUpdateIssue, "issue id is required", nil)
	}

	args := []string{"update", id}

	if input.Title != nil {
		args = append(args, "--title", *input.Title)
	}
	if input.Description != nil {
		args = append(args, "--description", *input.Description)
	}
	if input.Status != nil {
		args = append(args, "--status", *input.Status)
	}
	if input.Type != nil {
		args = append(args, "--type", *input.Type)
	}
	if input.Priority != nil {
		args = append(args, "--priority", strconv.Itoa(*input.Priority))
	}
	if input.Assignee != nil {
		args = append(args, "--assignee", *input.Assignee)
	}

	if len(input.Labels) > 0 {
		args = append(args, "--set-labels", strings.Join(input.Labels, ","))
	} else if input.ClearLabels {
		// bd 1.0.4 workaround: `--set-labels ""` is silently ignored.
		// Fetch current labels and enumerate each with --remove-label.
		detail, err := r.Issue(ctx, id)
		if err != nil {
			return err
		}
		if len(detail.Summary.Labels) == 0 {
			// Nothing to remove.
			return nil
		}
		args = append(args, "--remove-label", strings.Join(detail.Summary.Labels, ","))
	}

	_, err := r.run(ctx, bdrunner.CommandRequest{
		Operation: leanOpUpdateIssue,
		Args:      args,
		IsWrite:   true,
	})
	return err
}

// leanCloseNotFoundFragment is the bd 1.0.4 stderr substring for the close
// RowsAffected==0 false-positive. See CloseIssue comment and interface.go.
const leanCloseNotFoundFragment = "issue not found"

// CloseIssue closes an issue via `bd close <id>`.
//
// Idempotency workaround: bd 1.0.4 emits "issue not found" when RowsAffected==0
// on a re-close within the same second (schema DATETIME resolution means no
// columns change). The lean Repository replicates the repository workaround:
// on the close-specific not-found error, verify the issue exists with
// status=closed; return nil iff it does. Filed upstream as
// gastownhall/beads#4025.
func (r *Repository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	if strings.TrimSpace(id) == "" {
		return leanNewGWError(leanErrorCodeValidation, leanOpCloseIssue, "issue id is required", nil)
	}

	args := []string{"close", id}
	if input.Reason != "" {
		args = append(args, "--reason", input.Reason)
	}

	_, err := r.run(ctx, bdrunner.CommandRequest{
		Operation: leanOpCloseIssue,
		Args:      args,
		IsWrite:   true,
	})

	if err == nil {
		return nil
	}
	if !leanIsCloseNotFound(err) {
		return err
	}

	// bd misreported the issue as not-found. Verify it is actually closed.
	detail, showErr := r.Issue(ctx, id)
	if showErr != nil {
		return err // return original close error
	}
	if detail.Summary.Status == "closed" {
		return nil
	}
	return err
}

func leanIsCloseNotFound(err error) bool {
	var gwErr domain.RepositoryError
	if !errors.As(err, &gwErr) {
		return false
	}
	if gwErr.Code != domain.ErrorCodeCommandFailed {
		return false
	}
	return strings.Contains(gwErr.Message, leanCloseNotFoundFragment)
}

// AddComment adds a comment via `bd comments add <id> <body>`.
func (r *Repository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if strings.TrimSpace(id) == "" {
		return leanNewGWError(leanErrorCodeValidation, leanOpAddComment, "issue id is required", nil)
	}

	_, err := r.run(ctx, bdrunner.CommandRequest{
		Operation: leanOpAddComment,
		Args:      []string{"comments", "add", id, input.Body},
		IsWrite:   true,
	})
	return err
}
