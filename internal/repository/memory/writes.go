package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// CreateIssue implements repository.Repository.
//
// Field validation, the default priority and the label dedupe mirror the
// task-manager SDK (see validate.go), so a write this backend accepts is a
// write the production backend accepts.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CreateIssueResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clock()

	issueType := input.Type
	if issueType == "" {
		issueType = "task"
	}

	priority := priorityDefault
	if input.Priority != nil {
		priority = *input.Priority
	}

	si := &storedIssue{
		title:       strings.TrimSpace(input.Title),
		status:      "open",
		priority:    priority,
		issueType:   issueType,
		assignee:    input.Assignee,
		labels:      dedupeLabels(input.Labels),
		description: input.Description,
		created:     now,
		updated:     now,
		comments:    []storedComment{},
	}

	if err := validateIssueFields("create issue", si); err != nil {
		return domain.CreateIssueResult{}, err
	}

	id, err := r.nextUnusedIssueID()
	if err != nil {
		return domain.CreateIssueResult{}, err
	}
	si.id = id

	r.issues[id] = si
	return domain.CreateIssueResult{IssueID: id}, nil
}

// nextUnusedIssueID draws from the ID generator until it yields an ID no issue
// already holds. A loaded snapshot seeds issues with IDs the default counter has
// not reached ("mem-1", "mem-2"), so without this the first created issue
// replaces a loaded one outright and the store silently loses it.
//
// Caller must hold the write lock.
func (r *Repository) nextUnusedIssueID() (string, error) {
	const maxAttempts = 1000
	for range maxAttempts {
		id := r.idgen()
		if id == "" {
			break
		}
		if _, exists := r.issues[id]; !exists {
			return id, nil
		}
	}
	return "", domain.RepositoryError{
		Code:      domain.ErrorCodeCommandFailed,
		Operation: "create issue",
		Message:   "id generator produced no unused issue id",
	}
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

	// Apply to a copy so a rejected update leaves the stored issue untouched.
	updated := *si

	if input.Title != nil {
		updated.title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		updated.description = *input.Description
	}
	if input.Type != nil {
		updated.issueType = *input.Type
	}
	if input.Priority != nil {
		updated.priority = *input.Priority
	}
	if input.Assignee != nil {
		updated.assignee = *input.Assignee
	}
	if input.ClearLabels {
		updated.labels = []string{}
	} else if len(input.Labels) > 0 {
		updated.labels = make([]string, len(input.Labels))
		copy(updated.labels, input.Labels)
	}
	if input.Status != nil {
		applyStatusTransition(&updated, *input.Status, now)
	}

	if err := validateIssueFields("update issue", &updated); err != nil {
		return err
	}

	updated.updated = now
	*si = updated
	return nil
}

// applyStatusTransition mirrors the SDK's applyStatus: closing stamps the close
// time, and reopening clears both the close time and the reason. Assigning the
// status field alone leaves a reopened issue carrying a closedAt and a close
// reason, which renders as a live issue that says it was closed.
func applyStatusTransition(si *storedIssue, status string, now time.Time) {
	previous := si.status
	si.status = status
	switch {
	case status == "closed" && previous != "closed":
		si.closed = now
	case status != "closed" && previous == "closed":
		si.closed = time.Time{}
		si.closeReason = ""
	}
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
