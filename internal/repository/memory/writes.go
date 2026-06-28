package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// CreateIssue implements repository.Repository.
//
// Returns domain.RepositoryError with ErrorCodeValidationFailed when Title is
// empty.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CreateIssueResult{}, err
	}

	if strings.TrimSpace(input.Title) == "" {
		return domain.CreateIssueResult{}, domain.RepositoryError{
			Code:      domain.ErrorCodeValidationFailed,
			Operation: "create issue",
			Message:   "title must not be empty",
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clock()
	id := r.idgen()

	issueType := input.Type
	if issueType == "" {
		issueType = "task"
	}

	priority := 0
	if input.Priority != nil {
		priority = *input.Priority
	}

	labels := make([]string, len(input.Labels))
	copy(labels, input.Labels)

	si := &storedIssue{
		id:          id,
		title:       input.Title,
		status:      "open",
		priority:    priority,
		issueType:   issueType,
		assignee:    input.Assignee,
		labels:      labels,
		description: input.Description,
		created:     now,
		updated:     now,
		comments:    []storedComment{},
	}

	r.issues[id] = si
	return domain.CreateIssueResult{IssueID: id}, nil
}

// UpdateIssue implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match taskmgr's observable behavior, as documented in the Repository interface.
func (r *Repository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "update issue",
			Message:   fmt.Sprintf("command exited with code 1: Error resolving %q: no issue found", id),
		}
	}

	now := r.clock()

	if input.Title != nil {
		si.title = *input.Title
	}
	if input.Description != nil {
		si.description = *input.Description
	}
	if input.Status != nil {
		si.status = *input.Status
	}
	if input.Type != nil {
		si.issueType = *input.Type
	}
	if input.Priority != nil {
		si.priority = *input.Priority
	}
	if input.Assignee != nil {
		si.assignee = *input.Assignee
	}
	if input.ClearLabels {
		si.labels = []string{}
	} else if len(input.Labels) > 0 {
		si.labels = make([]string, len(input.Labels))
		copy(si.labels, input.Labels)
	}

	si.updated = now
	return nil
}

// CloseIssue implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match taskmgr's observable behavior, as documented in the Repository interface.
func (r *Repository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "close issue",
			Message:   fmt.Sprintf("command exited with code 1: Error resolving %q: no issue found", id),
		}
	}

	now := r.clock()

	si.status = "closed"
	si.closed = now
	si.updated = now

	if input.Reason != "" {
		si.closeReason = input.Reason
	} else {
		si.closeReason = "Closed"
	}

	return nil
}

// AddComment implements repository.Repository.
//
// Returns domain.RepositoryError{Code: ErrorCodeCommandFailed} for unknown IDs
// to match taskmgr's observable behavior, as documented in the Repository interface.
func (r *Repository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeCommandFailed,
			Operation: "add comment",
			Message:   fmt.Sprintf("command exited with code 1: unknown issue %q", id),
		}
	}

	now := r.clock()

	si.comments = append(si.comments, storedComment{
		id:        r.idgen(),
		author:    "memory-user",
		body:      input.Body,
		createdAt: now,
	})
	si.updated = now
	return nil
}
