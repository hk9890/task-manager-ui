package beads

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

const (
	operationListIssues    = "list issues"
	operationReadyIssues   = "ready issues"
	operationBlockedIssues = "blocked issues"
	operationShowIssue     = "show issue"
	operationSearchIssues  = "search issues"
	operationStatuses      = "status catalog"
	operationTypes         = "type catalog"
	operationLabels        = "label catalog"
)

// Gateway is a beads gateway implementation backed by official bd commands.
type Gateway struct {
	runner *CommandRunner
}

// NewCLIGateway builds a CLI-backed beads gateway.
func NewCLIGateway(runner *CommandRunner) *Gateway {
	if runner == nil {
		runner = NewCommandRunner(RunnerConfig{})
	}

	return &Gateway{runner: runner}
}

// ListIssues returns issue summaries using `bd list --json`.
func (g *Gateway) ListIssues(ctx context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error) {
	args := []string{"list", "--json"}
	args = append(args, buildFilterArgs(issueFilterArgs{
		Statuses: query.Statuses,
		Types:    query.Types,
		Assignee: query.Assignee,
		Labels:   query.Labels,
	})...)

	if query.SortBy != "" {
		sortField := mapListSortField(query.SortBy)
		if sortField != "" {
			args = append(args, "--sort", sortField)
		}
	}

	if query.SortOrder == domain.SortDirectionDescending {
		args = append(args, "--reverse")
	}

	limit := withOffsetWindow(query.Limit, query.Offset)
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := g.decodeIssueArray(ctx, operationListIssues, args)
	if err != nil {
		return nil, err
	}

	return mapIssueSummaries(operationListIssues, items, query.Offset, query.Limit)
}

// ReadyIssues returns ready issue summaries using `bd ready --json`.
func (g *Gateway) ReadyIssues(ctx context.Context, query domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	args := []string{"ready", "--json"}

	if limit := withOffsetWindow(query.Limit, query.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := g.decodeIssueArray(ctx, operationReadyIssues, args)
	if err != nil {
		return nil, err
	}

	return mapIssueSummaries(operationReadyIssues, items, query.Offset, query.Limit)
}

// BlockedIssues returns blocked issue views using `bd blocked --json`.
func (g *Gateway) BlockedIssues(ctx context.Context, query domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	items, err := g.decodeIssueArray(ctx, operationBlockedIssues, []string{"blocked", "--json"})
	if err != nil {
		return nil, err
	}

	views := make([]domain.BlockedIssueView, 0, len(items))
	for _, item := range items {
		summary, mapErr := issueSummaryFromMap(operationBlockedIssues, item)
		if mapErr != nil {
			return nil, mapErr
		}

		blockedByIDs, listErr := stringListFromMap(item, "blocked_by")
		if listErr != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationBlockedIssues, "failed to decode command JSON output", listErr)
		}

		blockedBy := make([]domain.IssueReference, 0, len(blockedByIDs))
		for _, id := range blockedByIDs {
			blockedBy = append(blockedBy, domain.IssueReference{ID: id})
		}

		views = append(views, domain.BlockedIssueView{
			Issue:     summary,
			BlockedBy: blockedBy,
		})
	}

	return paginate(views, query.Offset, query.Limit), nil
}

// ShowIssue returns issue details using `bd show --json`.
func (g *Gateway) ShowIssue(ctx context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	if strings.TrimSpace(query.IssueID) == "" {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeValidationFailed, operationShowIssue, "issue id is required", nil)
	}

	items, err := g.decodeIssueArray(ctx, operationShowIssue, []string{"show", query.IssueID, "--json"})
	if err != nil {
		return domain.IssueDetail{}, err
	}

	if len(items) == 0 {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeNotFound, operationShowIssue, fmt.Sprintf("issue not found: %s", query.IssueID), nil)
	}

	primary := items[0]
	summary, err := issueSummaryFromMap(operationShowIssue, primary)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	description, err := stringFromMap(primary, "description")
	if err != nil {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeDecodeFailed, operationShowIssue, "failed to decode command JSON output", err)
	}

	notes, err := optionalStringFromMap(primary, "notes")
	if err != nil {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeDecodeFailed, operationShowIssue, "failed to decode command JSON output", err)
	}

	blockedBy, err := referencesFromMapArray(primary, "dependencies", operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	blocks, err := referencesFromMapArray(primary, "dependents", operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	related, err := referencesFromMapArray(primary, "related", operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	comments, err := commentsFromMapArray(primary, "comments", operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	return domain.IssueDetail{
		Summary:     summary,
		Description: description,
		Notes:       notes,
		BlockedBy:   blockedBy,
		Blocks:      blocks,
		Related:     related,
		Comments:    comments,
	}, nil
}

// SearchIssues returns issue search results using `bd search --json`.
func (g *Gateway) SearchIssues(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if query.PriorityMin != nil && query.PriorityMax != nil && *query.PriorityMin > *query.PriorityMax {
		return domain.SearchResultPage{}, newGatewayError(domain.ErrorCodeValidationFailed, operationSearchIssues, "priority_min cannot be greater than priority_max", nil)
	}

	if strings.TrimSpace(query.Text) == "" && query.WorkState == domain.WorkStateAny {
		return g.searchIssuesFromList(ctx, query)
	}

	switch query.WorkState {
	case domain.WorkStateReady:
		return g.searchIssuesFromReady(ctx, query)
	case domain.WorkStateBlocked:
		return g.searchIssuesFromBlocked(ctx, query)
	}

	args := []string{"search"}
	if text := strings.TrimSpace(query.Text); text != "" {
		args = append(args, text)
	}

	args = append(args, "--json")
	args = append(args, buildFilterArgs(issueFilterArgs{
		Statuses:    query.Statuses,
		Types:       query.Types,
		PriorityMin: query.PriorityMin,
		PriorityMax: query.PriorityMax,
		Assignee:    query.Assignee,
		Labels:      query.Labels,
	})...)

	if limit := withOffsetWindow(query.Limit, query.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := g.decodeIssueArray(ctx, operationSearchIssues, args)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	summaries, err := mapIssueSummaries(operationSearchIssues, items, query.Offset, query.Limit)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	results := toSearchResults(summaries)

	return domain.SearchResultPage{
		Results: results,
		Total:   len(items),
	}, nil
}

func (g *Gateway) searchIssuesFromList(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	args := []string{"list", "--json"}
	args = append(args, buildFilterArgs(issueFilterArgs{
		Statuses:    query.Statuses,
		Types:       query.Types,
		PriorityMin: query.PriorityMin,
		PriorityMax: query.PriorityMax,
		Assignee:    query.Assignee,
		Labels:      query.Labels,
	})...)

	if limit := withOffsetWindow(query.Limit, query.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := g.decodeIssueArray(ctx, operationSearchIssues, args)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	summaries, err := mapIssueSummaries(operationSearchIssues, items, query.Offset, query.Limit)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	results := toSearchResults(summaries)

	return domain.SearchResultPage{
		Results: results,
		Total:   len(items),
	}, nil
}

func (g *Gateway) searchIssuesFromReady(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	// `bd search` does not expose ready semantics. For ready filtering we route
	// through `bd ready --json` and apply the remaining structured filters in-memory.
	items, err := g.decodeIssueArray(ctx, operationSearchIssues, []string{"ready", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	return g.searchIssuePageFromRecords(items, query)
}

func (g *Gateway) searchIssuesFromBlocked(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	// `bd search` does not expose dependency-blocked semantics and `bd blocked`
	// has a narrow flag surface. We intentionally keep the gateway API rich and
	// complete filtering in-memory after loading blocked results.
	items, err := g.decodeIssueArray(ctx, operationSearchIssues, []string{"blocked", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	return g.searchIssuePageFromRecords(items, query)
}

func (g *Gateway) searchIssuePageFromRecords(items []map[string]any, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	summaries, err := mapIssueSummaries(operationSearchIssues, items, 0, 0)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	filtered := filterIssueSummariesForSearch(summaries, query)
	paged := paginate(filtered, query.Offset, query.Limit)
	results := toSearchResults(paged)

	return domain.SearchResultPage{
		Results: results,
		Total:   len(filtered),
	}, nil
}

func filterIssueSummariesForSearch(items []domain.IssueSummary, query domain.SearchIssuesQuery) []domain.IssueSummary {
	out := make([]domain.IssueSummary, 0, len(items))
	for _, item := range items {
		if !issueMatchesSearchQuery(item, query) {
			continue
		}

		out = append(out, item)
	}

	return out
}

func issueMatchesSearchQuery(item domain.IssueSummary, query domain.SearchIssuesQuery) bool {
	if text := strings.TrimSpace(query.Text); text != "" {
		text = strings.ToLower(text)
		id := strings.ToLower(item.ID)
		title := strings.ToLower(item.Title)
		if !strings.Contains(id, text) && !strings.Contains(title, text) {
			return false
		}
	}

	if len(query.Statuses) > 0 && !slices.Contains(query.Statuses, item.Status) {
		return false
	}

	if len(query.Types) > 0 && !slices.Contains(query.Types, item.Type) {
		return false
	}

	if query.Assignee != "" && item.Assignee != query.Assignee {
		return false
	}

	for _, label := range query.Labels {
		if strings.TrimSpace(label) == "" {
			continue
		}

		if !slices.Contains(item.Labels, label) {
			return false
		}
	}

	if query.PriorityMin != nil && item.Priority < *query.PriorityMin {
		return false
	}

	if query.PriorityMax != nil && item.Priority > *query.PriorityMax {
		return false
	}

	return true
}

type issueFilterArgs struct {
	Statuses    []string
	Types       []string
	PriorityMin *int
	PriorityMax *int
	Assignee    string
	Labels      []string
}

func buildFilterArgs(filter issueFilterArgs) []string {
	args := make([]string, 0)

	if len(filter.Statuses) > 0 {
		args = append(args, "--status", strings.Join(filter.Statuses, ","))
	}

	if len(filter.Types) > 0 {
		args = append(args, "--type", strings.Join(filter.Types, ","))
	}

	if filter.PriorityMin != nil {
		args = append(args, "--priority-min", strconv.Itoa(*filter.PriorityMin))
	}

	if filter.PriorityMax != nil {
		args = append(args, "--priority-max", strconv.Itoa(*filter.PriorityMax))
	}

	if filter.Assignee != "" {
		args = append(args, "--assignee", filter.Assignee)
	}

	for _, label := range filter.Labels {
		if strings.TrimSpace(label) == "" {
			continue
		}

		args = append(args, "--label", label)
	}

	return args
}

func toSearchResults(issues []domain.IssueSummary) []domain.SearchResult {
	results := make([]domain.SearchResult, 0, len(issues))
	for _, issue := range issues {
		results = append(results, domain.SearchResult{Issue: issue})
	}

	return results
}

// StatusCatalog returns available issue statuses using `bd statuses --json`.
func (g *Gateway) StatusCatalog(ctx context.Context) ([]domain.StatusOption, error) {
	payload, err := RunJSON[map[string]any](ctx, g.runner, CommandRequest{Operation: operationStatuses, Args: []string{"statuses", "--json"}})
	if err != nil {
		return nil, err
	}

	builtIn, err := mapArrayFromMap(payload, "built_in_statuses")
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationStatuses, "failed to decode command JSON output", err)
	}

	custom, err := mapArrayFromMap(payload, "custom_statuses")
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationStatuses, "failed to decode command JSON output", err)
	}

	all := append(builtIn, custom...)
	out := make([]domain.StatusOption, 0, len(all))
	for _, status := range all {
		name, err := stringFromMap(status, "name")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationStatuses, "failed to decode command JSON output", err)
		}

		description, err := stringFromMap(status, "description")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationStatuses, "failed to decode command JSON output", err)
		}

		out = append(out, domain.StatusOption{Name: name, Description: description})
	}

	return out, nil
}

// TypeCatalog returns available issue types using `bd types --json`.
func (g *Gateway) TypeCatalog(ctx context.Context) ([]domain.TypeOption, error) {
	payload, err := RunJSON[map[string]any](ctx, g.runner, CommandRequest{Operation: operationTypes, Args: []string{"types", "--json"}})
	if err != nil {
		return nil, err
	}

	core, err := mapArrayFromMap(payload, "core_types")
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationTypes, "failed to decode command JSON output", err)
	}

	custom, err := mapArrayFromMap(payload, "custom_types")
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationTypes, "failed to decode command JSON output", err)
	}

	all := append(core, custom...)
	out := make([]domain.TypeOption, 0, len(all))
	for _, typ := range all {
		name, err := stringFromMap(typ, "name")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationTypes, "failed to decode command JSON output", err)
		}

		description, err := stringFromMap(typ, "description")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operationTypes, "failed to decode command JSON output", err)
		}

		out = append(out, domain.TypeOption{Name: name, Description: description})
	}

	return out, nil
}

// LabelCatalog returns available labels using `bd label list-all --json`.
func (g *Gateway) LabelCatalog(ctx context.Context) ([]domain.LabelOption, error) {
	labels, err := RunJSON[[]string](ctx, g.runner, CommandRequest{Operation: operationLabels, Args: []string{"label", "list-all", "--json"}})
	if err != nil {
		return nil, err
	}

	out := make([]domain.LabelOption, 0, len(labels))
	for _, label := range labels {
		if strings.TrimSpace(label) == "" {
			continue
		}

		out = append(out, domain.LabelOption{Name: label})
	}

	return out, nil
}

func (g *Gateway) decodeIssueArray(ctx context.Context, operation string, args []string) ([]map[string]any, error) {
	return RunJSON[[]map[string]any](ctx, g.runner, CommandRequest{Operation: operation, Args: args})
}

func mapIssueSummaries(operation string, records []map[string]any, offset int, limit int) ([]domain.IssueSummary, error) {
	out := make([]domain.IssueSummary, 0, len(records))
	for _, record := range records {
		summary, err := issueSummaryFromMap(operation, record)
		if err != nil {
			return nil, err
		}

		out = append(out, summary)
	}

	return paginate(out, offset, limit), nil
}

func issueSummaryFromMap(operation string, record map[string]any) (domain.IssueSummary, error) {
	id, err := stringFromMap(record, "id")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	title, err := stringFromMap(record, "title")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	status, err := stringFromMap(record, "status")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	issueType, err := stringFromMap(record, "issue_type")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	priority, err := intFromMap(record, "priority")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	assignee, err := optionalStringFromMap(record, "owner")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	if assignee == "" {
		assignee, err = optionalStringFromMap(record, "assignee")
		if err != nil {
			return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}
	}

	labels, err := stringListFromMap(record, "labels")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	createdAt, err := timestampFromMap(record, "created_at")
	if err != nil {
		return domain.IssueSummary{}, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	updatedAt, err := timestampFromMap(record, "updated_at")
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
		Labels:    labels,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func referencesFromMapArray(parent map[string]any, key string, operation string) ([]domain.IssueReference, error) {
	records, err := mapArrayFromMap(parent, key)
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	out := make([]domain.IssueReference, 0, len(records))
	for _, record := range records {
		id, err := stringFromMap(record, "id")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		title, err := stringFromMap(record, "title")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		out = append(out, domain.IssueReference{ID: id, Title: title})
	}

	return out, nil
}

func commentsFromMapArray(parent map[string]any, key string, operation string) ([]domain.IssueComment, error) {
	records, err := mapArrayFromMap(parent, key)
	if err != nil {
		return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	out := make([]domain.IssueComment, 0, len(records))
	for _, record := range records {
		id, err := stringFromMap(record, "id")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		author, err := optionalStringFromMap(record, "author")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		body, err := optionalStringFromMap(record, "text")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		createdAt, err := timestampFromMap(record, "created_at")
		if err != nil {
			return nil, newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
		}

		out = append(out, domain.IssueComment{
			ID:        id,
			Author:    author,
			Body:      body,
			CreatedAt: createdAt,
		})
	}

	return out, nil
}

func stringFromMap(record map[string]any, key string) (string, error) {
	v, ok := record[key]
	if !ok {
		return "", fmt.Errorf("missing field %q", key)
	}

	if v == nil {
		return "", nil
	}

	value, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string", key)
	}

	return value, nil
}

func optionalStringFromMap(record map[string]any, key string) (string, error) {
	v, ok := record[key]
	if !ok || v == nil {
		return "", nil
	}

	value, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string", key)
	}

	return value, nil
}

func intFromMap(record map[string]any, key string) (int, error) {
	v, ok := record[key]
	if !ok {
		return 0, fmt.Errorf("missing field %q", key)
	}

	number, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("field %q is not a number", key)
	}

	return int(number), nil
}

func stringListFromMap(record map[string]any, key string) ([]string, error) {
	v, ok := record[key]
	if !ok || v == nil {
		return nil, nil
	}

	rawItems, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", key)
	}

	out := make([]string, 0, len(rawItems))
	for i, raw := range rawItems {
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("field %q element %d is not a string", key, i)
		}

		out = append(out, text)
	}

	return out, nil
}

func mapArrayFromMap(record map[string]any, key string) ([]map[string]any, error) {
	v, ok := record[key]
	if !ok || v == nil {
		return nil, nil
	}

	rawItems, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", key)
	}

	out := make([]map[string]any, 0, len(rawItems))
	for i, raw := range rawItems {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field %q element %d is not an object", key, i)
		}

		out = append(out, m)
	}

	return out, nil
}

func timestampFromMap(record map[string]any, key string) (time.Time, error) {
	v, ok := record[key]
	if !ok {
		return time.Time{}, fmt.Errorf("missing field %q", key)
	}

	text, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("field %q is not a string", key)
	}

	if strings.TrimSpace(text) == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("field %q has invalid timestamp: %w", key, err)
	}

	return parsed, nil
}

func mapListSortField(field domain.SortField) string {
	switch field {
	case domain.SortFieldUpdatedAt:
		return "updated"
	case domain.SortFieldCreatedAt:
		return "created"
	case domain.SortFieldPriority:
		return "priority"
	case domain.SortFieldID:
		return "id"
	default:
		return ""
	}
}

func withOffsetWindow(limit int, offset int) int {
	if limit <= 0 {
		return 0
	}

	if offset <= 0 {
		return limit
	}

	return limit + offset
}

func paginate[T any](items []T, offset int, limit int) []T {
	if offset < 0 {
		offset = 0
	}

	if offset >= len(items) {
		return []T{}
	}

	page := items[offset:]
	if limit <= 0 {
		return page
	}

	if len(page) <= limit {
		return page
	}

	return page[:limit]
}
