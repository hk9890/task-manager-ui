package beads

import (
	"context"
	"strconv"
	"strings"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// createIssuePayload is the JSON response shape for `bd create --json`.
type createIssuePayload struct {
	ID string `json:"id"`
}

const (
	opCreateIssue = "create issue"
	opUpdateIssue = "update issue"
	opCloseIssue  = "close issue"
	opAddComment  = "add comment"
)

// CreateIssue creates a new issue through `bd create`.
func (g *Gateway) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	runner := g.runner

	// Use --json so the response is a structured payload rather than a bare
	// issue ID on stdout. This is safer than --silent + TrimSpace: structured
	// decode rejects unexpected trailing content (diagnostic chatter, NDJSON)
	// and the id field is unambiguously identified regardless of output format
	// changes in future bd releases.
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

	payload, err := RunJSON[createIssuePayload](ctx, runner, CommandRequest{
		Operation: opCreateIssue,
		Args:      args,
		IsWrite:   true,
	})
	if err != nil {
		return domain.CreateIssueResult{}, err
	}

	if strings.TrimSpace(payload.ID) == "" {
		return domain.CreateIssueResult{}, newGatewayError(domain.ErrorCodeDecodeFailed, opCreateIssue, "failed to decode create issue output", nil)
	}

	return domain.CreateIssueResult{IssueID: payload.ID}, nil
}

// UpdateIssue updates issue fields through `bd update`.
func (g *Gateway) UpdateIssue(ctx context.Context, issueID string, input domain.UpdateIssueInput) error {
	if strings.TrimSpace(issueID) == "" {
		return newGatewayError(domain.ErrorCodeValidationFailed, opUpdateIssue, "issue id is required", nil)
	}

	runner := g.runner
	var err error

	args := []string{"update", issueID}

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
		args = append(args, "--set-labels", "")
	}

	_, err = runner.Run(ctx, CommandRequest{
		Operation: opUpdateIssue,
		Args:      args,
		IsWrite:   true,
	})

	return err
}

// CloseIssue closes an issue through `bd close`.
func (g *Gateway) CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error {
	if strings.TrimSpace(issueID) == "" {
		return newGatewayError(domain.ErrorCodeValidationFailed, opCloseIssue, "issue id is required", nil)
	}

	runner := g.runner
	var err error

	args := []string{"close", issueID}

	if input.Reason != "" {
		args = append(args, "--reason", input.Reason)
	}

	_, err = runner.Run(ctx, CommandRequest{
		Operation: opCloseIssue,
		Args:      args,
		IsWrite:   true,
	})

	return err
}

// AddComment adds an issue comment through `bd comments add`.
func (g *Gateway) AddComment(ctx context.Context, issueID string, input domain.AddCommentInput) error {
	if strings.TrimSpace(issueID) == "" {
		return newGatewayError(domain.ErrorCodeValidationFailed, opAddComment, "issue id is required", nil)
	}

	runner := g.runner
	var err error

	_, err = runner.Run(ctx, CommandRequest{
		Operation: opAddComment,
		Args:      []string{"comments", "add", issueID, input.Body},
		IsWrite:   true,
	})

	return err
}
