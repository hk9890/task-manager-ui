package memory

import (
	"context"
	"slices"
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

	if err := validateIssueFields("create issue", r.catalogs, si); err != nil {
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
		return notFoundError("update issue", id)
	}
	// The SDK refuses every update of a closed issue but a reopen.
	if si.status == "closed" && (input.Status == nil || *input.Status == "closed") {
		return domain.RepositoryError{
			Code:      domain.ErrorCodeConflict,
			Operation: "update issue",
			Message:   "issue is closed; reopen it before editing",
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
		// Dedupe, as CreateIssue does and as the SDK does on every label write
		// (Store.applyLabels). Without it the fixture stored a duplicate set the
		// production backend would have collapsed.
		updated.labels = dedupeLabels(input.Labels)
	}
	if input.Status != nil {
		applyStatusTransition(&updated, *input.Status, now)
	}

	if err := validateIssueWrite("update issue", r.catalogs, si, &updated); err != nil {
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
		return notFoundError("close issue", id)
	}

	// Closing an already-closed issue is a successful no-op in the SDK
	// (Store.Close): no re-stamped close time, no updated bump, no hooks. The
	// fixture used to re-stamp both, so a test could not tell a second close
	// from a first.
	if si.status == "closed" {
		return nil
	}

	now := r.clock()

	closed := *si
	closed.status = "closed"
	closed.closed = now
	closed.updated = now

	// An empty reason leaves the stored reason alone, as Store.applyStatus does.
	// Writing the literal "Closed" for it put a reason on screen that nobody
	// typed and that the production backend never stores.
	if input.Reason != "" {
		closed.closeReason = input.Reason
	}

	if err := validateIssueWrite("close issue", r.catalogs, si, &closed); err != nil {
		return err
	}

	*si = closed
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
		return notFoundError("add comment", id)
	}
	if err := validateCommentBody("add comment", input.Body); err != nil {
		return err
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

// EditComment implements repository.Repository.
//
// The comment keeps its place and gets a new ID, the author and the edit time,
// as a revision does in the SDK. The issue's updated time does not change: the
// SDK does not rewrite the issue file for a comment.
func (r *Repository) EditComment(ctx context.Context, id, commentID string, input domain.EditCommentInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return notFoundError("edit comment", id)
	}
	index := commentIndex(si, commentID)
	if index < 0 {
		return validationError("edit comment", "comment_id", "not found")
	}
	if err := validateCommentBody("edit comment", input.Body); err != nil {
		return err
	}

	si.comments[index] = storedComment{
		id:        r.idgen(),
		author:    "memory-user",
		body:      input.Body,
		createdAt: r.clock(),
	}
	return nil
}

// DeleteComment implements repository.Repository.
func (r *Repository) DeleteComment(ctx context.Context, id, commentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	si, ok := r.issues[id]
	if !ok {
		return notFoundError("delete comment", id)
	}
	index := commentIndex(si, commentID)
	if index < 0 {
		return validationError("delete comment", "comment_id", "not found")
	}

	si.comments = slices.Delete(si.comments, index, index+1)
	return nil
}

func commentIndex(si *storedIssue, commentID string) int {
	return slices.IndexFunc(si.comments, func(c storedComment) bool { return c.id == commentID })
}

// validateCommentBody refuses an empty comment, as the SDK does.
func validateCommentBody(operation, body string) error {
	if strings.TrimSpace(body) == "" {
		return validationError(operation, "body", "comment must have a non-empty body")
	}
	return nil
}
