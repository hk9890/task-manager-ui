package beads

import (
	"fmt"
	"strings"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

type bdIssuePayload struct {
	ID           *string                 `json:"id"`
	Title        *string                 `json:"title"`
	Status       *string                 `json:"status"`
	IssueType    *string                 `json:"issue_type"`
	Priority     *int                    `json:"priority"`
	Assignee     *string                 `json:"assignee"`
	Owner        *string                 `json:"owner"`
	Labels       []string                `json:"labels"`
	CreatedAt    *string                 `json:"created_at"`
	UpdatedAt    *string                 `json:"updated_at"`
	Description  *string                 `json:"description"`
	Notes        *string                 `json:"notes"`
	ClosedAt     *string                 `json:"closed_at"`
	CloseReason  *string                 `json:"close_reason"`
	BlockedBy    []string                `json:"blocked_by"`
	Dependencies []bdIssueRefPayload     `json:"dependencies"`
	Dependents   []bdIssueRefPayload     `json:"dependents"`
	Related      []bdIssueRefPayload     `json:"related"`
	Comments     []bdIssueCommentPayload `json:"comments"`
}

type bdIssueRefPayload struct {
	ID             *string `json:"id"`
	Title          *string `json:"title"`
	IssueType      *string `json:"issue_type"`
	Priority       *int    `json:"priority"`
	Status         *string `json:"status"`
	DependencyType *string `json:"dependency_type"`
}

type bdIssueCommentPayload struct {
	ID        *string `json:"id"`
	Author    *string `json:"author"`
	Text      *string `json:"text"`
	CreatedAt *string `json:"created_at"`
}

type bdCatalogEntryPayload struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type bdStatusCatalogPayload struct {
	BuiltInStatuses []bdCatalogEntryPayload `json:"built_in_statuses"`
	CustomStatuses  []bdCatalogEntryPayload `json:"custom_statuses"`
}

type bdTypeCatalogPayload struct {
	CoreTypes []bdCatalogEntryPayload `json:"core_types"`
	// CustomTypes is a []string in bd 1.0.4 (bare names, no description).
	// Core types are objects with {name, description}; custom types are bare
	// strings because bd does not store descriptions for user-defined types.
	// See puy3 for the divergence discovered during the contract audit.
	CustomTypes []string `json:"custom_types"`
}

type bdLabelCatalogEntryPayload struct {
	Label *string `json:"label"`
	Count *int    `json:"count"`
}

type bdLabelListAllPayload []bdLabelCatalogEntryPayload

// bdBlockedByRefPayload is the object form of a blocked_by entry in bd ready --explain --json.
// Note: bd blocked --json represents blocked_by as []string (bare IDs);
// bd ready --explain --json represents blocked_by as []object with id/title/priority/status.
type bdBlockedByRefPayload struct {
	ID       *string `json:"id"`
	Title    *string `json:"title"`
	Priority *int    `json:"priority"`
	Status   *string `json:"status"`
}

// bdExplainBlockedItemPayload is the per-item shape in the "blocked" array of
// bd ready --explain --json. It mirrors bdIssuePayload for the summary fields
// but carries blocked_by as objects rather than bare string IDs.
type bdExplainBlockedItemPayload struct {
	ID        *string                 `json:"id"`
	Title     *string                 `json:"title"`
	Status    *string                 `json:"status"`
	IssueType *string                 `json:"issue_type"`
	Priority  *int                    `json:"priority"`
	Assignee  *string                 `json:"assignee"`
	Owner     *string                 `json:"owner"`
	Labels    []string                `json:"labels"`
	CreatedAt *string                 `json:"created_at"`
	UpdatedAt *string                 `json:"updated_at"`
	BlockedBy []bdBlockedByRefPayload `json:"blocked_by"`
}

type bdReadyExplainPayload struct {
	Ready   []bdIssuePayload              `json:"ready"`
	Blocked []bdExplainBlockedItemPayload `json:"blocked"`
	Summary bdReadyExplainSummary         `json:"summary"`
}

type bdReadyExplainSummary struct {
	TotalReady   int `json:"total_ready"`
	TotalBlocked int `json:"total_blocked"`
	CycleCount   int `json:"cycle_count"`
}

func (p bdExplainBlockedItemPayload) toIssueSummary(operation string) (domain.IssueSummary, error) {
	id, err := requiredString(p.ID, "id")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	title, err := requiredString(p.Title, "title")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	status, err := requiredString(p.Status, "status")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	issueType, err := requiredString(p.IssueType, "issue_type")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	priority, err := requiredInt(p.Priority, "priority")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	assignee := optionalString(p.Assignee)
	if assignee == "" {
		assignee = optionalString(p.Owner)
	}

	createdAt, err := requiredTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	updatedAt, err := requiredTimestamp(p.UpdatedAt, "updated_at")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
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

type bdCountGroupPayload struct {
	Group string `json:"group"`
	Count int    `json:"count"`
}

type bdCountByStatusPayload struct {
	Groups        []bdCountGroupPayload `json:"groups"`
	SchemaVersion int                   `json:"schema_version"`
	Total         int                   `json:"total"`
}

func (p bdIssuePayload) toIssueSummary(operation string) (domain.IssueSummary, error) {
	id, err := requiredString(p.ID, "id")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	title, err := requiredString(p.Title, "title")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	status, err := requiredString(p.Status, "status")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	issueType, err := requiredString(p.IssueType, "issue_type")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	priority, err := requiredInt(p.Priority, "priority")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	assignee := optionalString(p.Assignee)
	if assignee == "" {
		assignee = optionalString(p.Owner)
	}

	createdAt, err := requiredTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	updatedAt, err := requiredTimestamp(p.UpdatedAt, "updated_at")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
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

func (p bdIssueRefPayload) toIssueReference(operation string) (domain.IssueReference, error) {
	id, err := requiredString(p.ID, "id")
	if err != nil {
		return domain.IssueReference{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	title, err := requiredString(p.Title, "title")
	if err != nil {
		return domain.IssueReference{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return domain.IssueReference{
		ID:       id,
		Title:    title,
		Type:     optionalString(p.IssueType),
		Priority: optionalInt(p.Priority),
		Status:   optionalString(p.Status),
	}, nil
}

func (p bdIssueCommentPayload) toIssueComment(operation string) (domain.IssueComment, error) {
	id, err := requiredString(p.ID, "id")
	if err != nil {
		return domain.IssueComment{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	createdAt, err := requiredTimestamp(p.CreatedAt, "created_at")
	if err != nil {
		return domain.IssueComment{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return domain.IssueComment{
		ID:        id,
		Author:    optionalString(p.Author),
		Body:      optionalString(p.Text),
		CreatedAt: createdAt,
	}, nil
}

func (p bdCatalogEntryPayload) toStatusOption(operation string) (domain.StatusOption, error) {
	name, err := requiredString(p.Name, "name")
	if err != nil {
		return domain.StatusOption{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return domain.StatusOption{Name: name, Description: optionalString(p.Description)}, nil
}

func (p bdCatalogEntryPayload) toTypeOption(operation string) (domain.TypeOption, error) {
	name, err := requiredString(p.Name, "name")
	if err != nil {
		return domain.TypeOption{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return domain.TypeOption{Name: name, Description: optionalString(p.Description)}, nil
}

func (p bdLabelCatalogEntryPayload) toLabelOption(operation string) (domain.LabelOption, error) {
	label, err := requiredString(p.Label, "label")
	if err != nil {
		return domain.LabelOption{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return domain.LabelOption{Name: strings.TrimSpace(label)}, nil
}

func requiredString(v *string, field string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("missing field %q", field)
	}

	return *v, nil
}

func optionalString(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}

func requiredInt(v *int, field string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("missing field %q", field)
	}

	return *v, nil
}

func optionalInt(v *int) int {
	if v == nil {
		return 0
	}

	return *v
}

func requiredTimestamp(v *string, field string) (time.Time, error) {
	if v == nil {
		return time.Time{}, fmt.Errorf("missing field %q", field)
	}

	return parseTimestamp(*v, field)
}

func optionalTimestamp(v *string, field string) (time.Time, error) {
	if v == nil {
		return time.Time{}, nil
	}

	return parseTimestamp(*v, field)
}

func parseTimestamp(value string, field string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("field %q has invalid timestamp: %w", field, err)
	}

	return parsed, nil
}
