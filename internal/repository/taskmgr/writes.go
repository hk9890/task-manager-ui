package taskmgr

import (
	"context"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// CreateIssue creates a new issue. The configured author is recorded as the
// creator. Validation failures (e.g. empty title) surface as
// domain.ErrorCodeValidationFailed.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CreateIssueResult{}, err
	}
	iss, err := r.store.Create(tasks.CreateInput{
		Title:       input.Title,
		Description: input.Description,
		Type:        tasks.Type(input.Type),
		Priority:    input.Priority,
		Assignee:    input.Assignee,
		Creator:     r.author,
		Labels:      input.Labels,
	})
	if err != nil {
		return domain.CreateIssueResult{}, mapWriteErr("create issue", err)
	}
	return domain.CreateIssueResult{IssueID: iss.ID}, nil
}

// UpdateIssue applies a partial update. Status transitions (close/reopen) are
// handled by the SDK's Update, which lands the issue on the requested status —
// callers never dispatch Close/Reopen for a status change.
func (r *Repository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	in := tasks.UpdateInput{
		Title:       input.Title,
		Description: input.Description,
		Priority:    input.Priority,
		Assignee:    input.Assignee,
		ClearLabels: input.ClearLabels,
	}
	if input.Status != nil {
		s := tasks.Status(*input.Status)
		in.Status = &s
	}
	if input.Type != nil {
		t := tasks.Type(*input.Type)
		in.Type = &t
	}
	if !input.ClearLabels && len(input.Labels) > 0 {
		in.SetLabels = input.Labels
	}
	if _, err := r.store.Update(id, in); err != nil {
		return mapWriteErr("update issue", err)
	}
	return nil
}

// CloseIssue closes the issue with the supplied reason. Close is idempotent in
// the SDK.
func (r *Repository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.store.Close(id, input.Reason); err != nil {
		return mapWriteErr("close issue", err)
	}
	return nil
}

// AddComment appends a comment authored by the configured identity.
func (r *Repository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.store.AddComment(id, r.author, input.Body); err != nil {
		return mapWriteErr("add comment", err)
	}
	return nil
}
