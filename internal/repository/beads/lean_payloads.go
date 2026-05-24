package beads

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
)

// Operation names used in lean gateway errors.
const (
	leanOpHealthCheck  = "health check"
	leanOpDashboard    = "dashboard"
	leanOpShowIssue    = "show issue"
	leanOpSearchIssues = "search issues"
	leanOpStatuses     = "status catalog"
	leanOpTypes        = "type catalog"
	leanOpLabels       = "label catalog"
	leanOpReadyExplain = "ready explain"
	leanOpQuery        = "query issues"
	leanOpCountIssues  = "count issues"
	leanOpCreateIssue  = "create issue"
	leanOpUpdateIssue  = "update issue"
	leanOpCloseIssue   = "close issue"
	leanOpAddComment   = "add comment"
)

// Error code aliases (avoid importing domain.ErrorCode* names redundantly).
const (
	leanErrorCodeDecodeFailed  = domain.ErrorCodeDecodeFailed
	leanErrorCodeCommandFailed = domain.ErrorCodeCommandFailed
	leanErrorCodeNotFound      = domain.ErrorCodeNotFound
	leanErrorCodeValidation    = domain.ErrorCodeValidationFailed
)

func leanNewGWError(code domain.ErrorCode, op, msg string, cause error) error {
	return domain.GatewayError{Code: code, Operation: op, Message: msg, Cause: cause}
}

// -- JSON payload types --

// leanIssuePayload is the JSON shape for a single issue as returned by
// bd list/ready/blocked/show/search/query.
type leanIssuePayload struct {
	ID           *string               `json:"id"`
	Title        *string               `json:"title"`
	Status       *string               `json:"status"`
	IssueType    *string               `json:"issue_type"`
	Priority     *int                  `json:"priority"`
	Assignee     *string               `json:"assignee"`
	Owner        *string               `json:"owner"`
	Labels       []string              `json:"labels"`
	CreatedAt    *string               `json:"created_at"`
	UpdatedAt    *string               `json:"updated_at"`
	Description  *string               `json:"description"`
	Notes        *string               `json:"notes"`
	ClosedAt     *string               `json:"closed_at"`
	CloseReason  *string               `json:"close_reason"`
	BlockedBy    []string              `json:"blocked_by"`
	Dependencies []leanIssueRefPayload `json:"dependencies"`
	Dependents   []leanIssueRefPayload `json:"dependents"`
	Related      []leanIssueRefPayload `json:"related"`
	Comments     []leanCommentPayload  `json:"comments"`
}

type leanIssueRefPayload struct {
	ID             *string `json:"id"`
	Title          *string `json:"title"`
	IssueType      *string `json:"issue_type"`
	Priority       *int    `json:"priority"`
	Status         *string `json:"status"`
	DependencyType *string `json:"dependency_type"`
}

type leanCommentPayload struct {
	ID        *string `json:"id"`
	Author    *string `json:"author"`
	Text      *string `json:"text"`
	CreatedAt *string `json:"created_at"`
}

type leanStatusCatalogPayload struct {
	BuiltInStatuses []leanCatalogEntryPayload `json:"built_in_statuses"`
	CustomStatuses  []leanCatalogEntryPayload `json:"custom_statuses"`
}

type leanCatalogEntryPayload struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type leanTypeCatalogPayload struct {
	CoreTypes   []leanCatalogEntryPayload `json:"core_types"`
	CustomTypes []string                  `json:"custom_types"`
}

type leanLabelEntryPayload struct {
	Label *string `json:"label"`
	Count *int    `json:"count"`
}

// leanBlockedByRefPayload is the object form of a blocked_by entry in
// bd ready --explain --json (richer than the bare-string form in bd blocked).
type leanBlockedByRefPayload struct {
	ID       *string `json:"id"`
	Title    *string `json:"title"`
	Priority *int    `json:"priority"`
	Status   *string `json:"status"`
}

type leanExplainBlockedItemPayload struct {
	ID        *string                   `json:"id"`
	Title     *string                   `json:"title"`
	Status    *string                   `json:"status"`
	IssueType *string                   `json:"issue_type"`
	Priority  *int                      `json:"priority"`
	Assignee  *string                   `json:"assignee"`
	Owner     *string                   `json:"owner"`
	Labels    []string                  `json:"labels"`
	CreatedAt *string                   `json:"created_at"`
	UpdatedAt *string                   `json:"updated_at"`
	BlockedBy []leanBlockedByRefPayload `json:"blocked_by"`
}

type leanReadyExplainPayload struct {
	Ready   []leanIssuePayload              `json:"ready"`
	Blocked []leanExplainBlockedItemPayload `json:"blocked"`
	Summary leanReadyExplainSummary         `json:"summary"`
}

type leanReadyExplainSummary struct {
	TotalReady   int `json:"total_ready"`
	TotalBlocked int `json:"total_blocked"`
	CycleCount   int `json:"cycle_count"`
}

type leanCountGroupPayload struct {
	Group string `json:"group"`
	Count int    `json:"count"`
}

type leanCountByStatusPayload struct {
	Groups []leanCountGroupPayload `json:"groups"`
	Total  int                     `json:"total"`
}

type leanCreateResultPayload struct {
	ID string `json:"id"`
}

// -- Scalar helpers --

func leanReqStr(v *string, field string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("missing field %q", field)
	}
	return *v, nil
}

func leanOptStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func leanReqInt(v *int, field string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("missing field %q", field)
	}
	return *v, nil
}

func leanOptInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func leanReqTimestamp(v *string, field string) (time.Time, error) {
	if v == nil {
		return time.Time{}, fmt.Errorf("missing field %q", field)
	}
	return leanParseTimestamp(*v, field)
}

func leanOptTimestamp(v *string, field string) (time.Time, error) {
	if v == nil {
		return time.Time{}, nil
	}
	return leanParseTimestamp(*v, field)
}

func leanParseTimestamp(value, field string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("field %q has invalid timestamp: %w", field, err)
	}
	return t, nil
}

// -- Payload-to-domain converters --

func leanToIssueSummary(p leanIssuePayload, op string) (domain.IssueSummary, error) {
	id, err := leanReqStr(p.ID, "id")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	title, err := leanReqStr(p.Title, "title")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	status, err := leanReqStr(p.Status, "status")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	issueType, err := leanReqStr(p.IssueType, "issue_type")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	priority, err := leanReqInt(p.Priority, "priority")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	assignee := leanOptStr(p.Assignee)
	if assignee == "" {
		assignee = leanOptStr(p.Owner)
	}
	createdAt, err := leanReqTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	updatedAt, err := leanReqTimestamp(p.UpdatedAt, "updated_at")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	return domain.IssueSummary{
		ID:        id,
		Title:     title,
		Status:    status,
		Type:      issueType,
		Priority:  priority,
		Assignee:  assignee,
		Labels:    p.Labels,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func leanExplainItemToSummary(p leanExplainBlockedItemPayload, op string) (domain.IssueSummary, error) {
	id, err := leanReqStr(p.ID, "id")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	title, err := leanReqStr(p.Title, "title")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	status, err := leanReqStr(p.Status, "status")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	issueType, err := leanReqStr(p.IssueType, "issue_type")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	priority, err := leanReqInt(p.Priority, "priority")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	assignee := leanOptStr(p.Assignee)
	if assignee == "" {
		assignee = leanOptStr(p.Owner)
	}
	createdAt, err := leanReqTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	updatedAt, err := leanReqTimestamp(p.UpdatedAt, "updated_at")
	if err != nil {
		return domain.IssueSummary{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	return domain.IssueSummary{
		ID:        id,
		Title:     title,
		Status:    status,
		Type:      issueType,
		Priority:  priority,
		Assignee:  assignee,
		Labels:    p.Labels,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func leanToIssueRef(p leanIssueRefPayload, op string) (domain.IssueReference, error) {
	id, err := leanReqStr(p.ID, "id")
	if err != nil {
		return domain.IssueReference{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	title, err := leanReqStr(p.Title, "title")
	if err != nil {
		return domain.IssueReference{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	return domain.IssueReference{
		ID:       id,
		Title:    title,
		Type:     leanOptStr(p.IssueType),
		Priority: leanOptInt(p.Priority),
		Status:   leanOptStr(p.Status),
	}, nil
}

func leanToComment(p leanCommentPayload, op string) (domain.IssueComment, error) {
	id, err := leanReqStr(p.ID, "id")
	if err != nil {
		return domain.IssueComment{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	createdAt, err := leanReqTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueComment{}, leanNewGWError(leanErrorCodeDecodeFailed, op, "failed to decode command JSON output", err)
	}
	return domain.IssueComment{
		ID:        id,
		Author:    leanOptStr(p.Author),
		Body:      leanOptStr(p.Text),
		CreatedAt: createdAt,
	}, nil
}

// leanMapIssueSummaries converts a slice of leanIssuePayload to domain.IssueSummary,
// then applies pagination.
func leanMapIssueSummaries(op string, records []leanIssuePayload, offset, limit int) ([]domain.IssueSummary, error) {
	out := make([]domain.IssueSummary, 0, len(records))
	for _, p := range records {
		s, err := leanToIssueSummary(p, op)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return leanPaginate(out, offset, limit), nil
}

// leanDecodeIssueArray runs argv through run and decodes the JSON array into
// []leanIssuePayload. Callers pass r.run (the Repository's execution
// chokepoint) so the call is interceptable by [WithCommandHook].
func leanDecodeIssueArray(ctx context.Context, run runFn, op string, args []string) ([]leanIssuePayload, error) {
	req := bdrunner.CommandRequest{Operation: op, Args: args}
	out, err := run(ctx, req)
	if err != nil {
		return nil, err
	}
	var items []leanIssuePayload
	if err := bdrunner.DecodeJSONInto(op, out, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// leanBuildFilterArgs constructs bd flag args for common filter fields.
func leanBuildFilterArgs(statuses, types []string, priorityMin, priorityMax *int, assignee string, labels []string) []string {
	var args []string
	if len(statuses) > 0 {
		args = append(args, "--status", strings.Join(statuses, ","))
	}
	if len(types) > 0 {
		args = append(args, "--type", strings.Join(types, ","))
	}
	if priorityMin != nil {
		args = append(args, "--priority-min", fmt.Sprintf("%d", *priorityMin))
	}
	if priorityMax != nil {
		args = append(args, "--priority-max", fmt.Sprintf("%d", *priorityMax))
	}
	if assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	for _, label := range labels {
		if strings.TrimSpace(label) == "" {
			continue
		}
		args = append(args, "--label", label)
	}
	return args
}

// leanMapSortField translates a domain.SortField to the bd --sort argument.
func leanMapSortField(field domain.SortField) string {
	switch field {
	case domain.SortFieldUpdatedAt:
		return "updated"
	case domain.SortFieldCreatedAt:
		return "created"
	case domain.SortFieldPriority:
		return "priority"
	case domain.SortFieldID:
		return "id"
	case domain.SortFieldClosedAt:
		return "closed"
	default:
		return ""
	}
}

// leanDepsFromPayload splits a dependencies slice into blockedBy, related, and
// (if present) the parent issue reference.
func leanDepsFromPayload(records []leanIssueRefPayload, op string) (blockedBy []domain.IssueReference, related []domain.IssueReference, parentIssue domain.IssueReference, hasParent bool, err error) {
	for _, r := range records {
		ref, refErr := leanToIssueRef(r, op)
		if refErr != nil {
			return nil, nil, domain.IssueReference{}, false, refErr
		}
		depType := leanOptStr(r.DependencyType)
		switch depType {
		case "related", "relates-to":
			related = append(related, ref)
		case "parent-child":
			if !hasParent {
				parentIssue = ref
				hasParent = true
			}
		default:
			blockedBy = append(blockedBy, ref)
		}
	}
	return
}

// leanDependentsFromPayload splits a dependents slice into blocks and related.
func leanDependentsFromPayload(records []leanIssueRefPayload, op string) (blocks []domain.IssueReference, related []domain.IssueReference, err error) {
	for _, r := range records {
		ref, refErr := leanToIssueRef(r, op)
		if refErr != nil {
			return nil, nil, refErr
		}
		depType := leanOptStr(r.DependencyType)
		if depType == "related" || depType == "relates-to" {
			related = append(related, ref)
		} else {
			blocks = append(blocks, ref)
		}
	}
	return
}

// leanRefsFromPayload converts a slice of leanIssueRefPayload to []domain.IssueReference.
func leanRefsFromPayload(records []leanIssueRefPayload, op string) ([]domain.IssueReference, error) {
	out := make([]domain.IssueReference, 0, len(records))
	for _, r := range records {
		ref, err := leanToIssueRef(r, op)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

// leanCommentsFromPayload converts a slice of leanCommentPayload to []domain.IssueComment.
func leanCommentsFromPayload(records []leanCommentPayload, op string) ([]domain.IssueComment, error) {
	out := make([]domain.IssueComment, 0, len(records))
	for _, r := range records {
		c, err := leanToComment(r, op)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
