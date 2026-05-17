package beads

import (
	"context"
	"errors"
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
		// Workaround for bd 1.0.4: `bd update --set-labels ""` exits 0 but
		// silently ignores the clear request, leaving labels unchanged (see [[ubav]]).
		// Instead, fetch the current labels via ShowIssue and emit
		// `bd update --remove-labels <csv>` enumerating each existing label.
		// If the issue has no labels, skip the bd update call entirely.
		detail, showErr := g.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
		if showErr != nil {
			return showErr
		}
		if len(detail.Summary.Labels) == 0 {
			// Nothing to remove; issue has no labels.
			return nil
		}
		args = append(args, "--remove-labels", strings.Join(detail.Summary.Labels, ","))
	}

	_, err = runner.Run(ctx, CommandRequest{
		Operation: opUpdateIssue,
		Args:      args,
		IsWrite:   true,
	})

	return err
}

// closeIssueNotFoundFragment is the bd 1.0.4 stderr substring emitted by
// `bd close <id>` when its internal UPDATE returns RowsAffected==0. The
// wording is distinct from the generic ID-resolver path ("resolving ID X:
// no issue found matching X") and is produced by close-specific code in
// internal/storage/issueops/close.go. See the CloseIssue idempotency note
// in interface.go and upstream gastownhall/beads#4025.
const closeIssueNotFoundFragment = "issue not found"

// CloseIssue closes an issue through `bd close`.
//
// Idempotency emulation over upstream bd bug — REMOVE WHEN FIXED.
//
// bd 1.0.4 `bd close <id>` infers "issue not found" from
// result.RowsAffected()==0. Because the schema's `closed_at`/`updated_at`
// are DATETIME (second resolution) and a re-close within the same second
// changes no columns (status/closed_at/updated_at/close_reason/closed_by_session
// are all already at their target values), MySQL/Dolt returns
// RowsAffected==0 and bd misreports the still-present issue as missing.
// Filed upstream as gastownhall/beads#4025 with suggested fix
// (SELECT status before/after UPDATE to disambiguate missing-row from
// no-op-update). When that ships and we bump the mise-pinned bd version,
// delete this recovery block (and the related unit tests in writes_test.go),
// revert the interface.go CloseIssue note to plain "idempotent (exit 0)",
// and let bd's native exit-0 carry the contract.
//
// Behavior while the upstream bug remains: on a close-specific not-found
// error, probe via ShowIssue and return nil iff the issue exists with
// status=closed. Truly missing issues and any non-closed end-state still
// surface the original error.
func (g *Gateway) CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error {
	if strings.TrimSpace(issueID) == "" {
		return newGatewayError(domain.ErrorCodeValidationFailed, opCloseIssue, "issue id is required", nil)
	}

	runner := g.runner

	args := []string{"close", issueID}

	if input.Reason != "" {
		args = append(args, "--reason", input.Reason)
	}

	_, err := runner.Run(ctx, CommandRequest{
		Operation: opCloseIssue,
		Args:      args,
		IsWrite:   true,
	})

	if err == nil {
		return nil
	}
	if !isCloseNotFound(err) {
		return err
	}
	// bd reported not-found; verify whether the issue is actually already
	// closed (bd quirk) vs truly missing.
	detail, showErr := g.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issueID})
	if showErr != nil {
		return err
	}
	if detail.Summary.Status == "closed" {
		return nil
	}
	return err
}

func isCloseNotFound(err error) bool {
	var gwErr domain.GatewayError
	if !errors.As(err, &gwErr) {
		return false
	}
	if gwErr.Code != domain.ErrorCodeCommandFailed {
		return false
	}
	return strings.Contains(gwErr.Message, closeIssueNotFoundFragment)
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
