package beads

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hk9890/beads-workbench/internal/domain"
)

const (
	operationHealthCheck     = "health check"
	operationListIssues      = "list issues"
	operationReadyIssues     = "ready issues"
	operationBlockedIssues   = "blocked issues"
	operationReadyExplain    = "ready explain"
	operationShowIssue       = "show issue"
	operationSearchIssues    = "search issues"
	operationCountIssues     = "count issues"
	operationQuery           = "query issues"
	operationStatuses        = "status catalog"
	operationTypes           = "type catalog"
	operationLabels          = "label catalog"
	searchNoticeMaybeMore    = "Results may be incomplete because the backend limit may have capped additional matches."
	searchNoticeNoTextFilter = "No text filter applied; returning entire ready/blocked queue."
)

// Gateway is a beads gateway implementation backed by official bd commands.
type Gateway struct {
	runner *CommandRunner

	// parentSiblingCacheMu guards parentSiblingCache.
	parentSiblingCacheMu sync.RWMutex
	// parentSiblingCache stores the children list for a given parent issue ID,
	// keyed by parent ID. Populated lazily by parentChildSiblings; reused across
	// ShowIssue calls within the same gateway instance so each unique parent is
	// fetched at most once.
	parentSiblingCache map[string][]domain.IssueReference
}

// NewCLIGateway builds a CLI-backed beads gateway.
func NewCLIGateway(runner *CommandRunner) *Gateway {
	if runner == nil {
		runner = NewCommandRunner(RunnerConfig{})
	}

	return &Gateway{
		runner:             runner,
		parentSiblingCache: make(map[string][]domain.IssueReference),
	}
}

// HealthCheck verifies that bd is reachable and a beads database exists in the
// working directory. Returns ErrorCodeCommandUnavailable if bd is not in PATH,
// ErrorCodeNoDatabaseFound if no database is present, nil on success.
func (g *Gateway) HealthCheck(ctx context.Context) error {
	_, err := g.runner.Run(ctx, CommandRequest{Operation: operationHealthCheck, Args: []string{"ping", "--json"}})
	return err
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

// Query returns issue summaries using `bd query "<expr>" --json` with the bd query DSL.
// The expr must be a non-empty, non-whitespace string.
func (g *Gateway) Query(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, newGatewayError(domain.ErrorCodeValidationFailed, operationQuery, "query expression is required", nil)
	}

	args := []string{"query", expr, "--json"}

	if opts.IncludeClosed {
		args = append(args, "-a")
	}

	if opts.SortBy != "" {
		if sortField := mapListSortField(opts.SortBy); sortField != "" {
			args = append(args, "--sort", sortField)
		}
	}

	if opts.SortOrder == domain.SortDirectionDescending {
		args = append(args, "--reverse")
	}

	if limit := withOffsetWindow(opts.Limit, opts.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := g.decodeIssueArray(ctx, operationQuery, args)
	if err != nil {
		return nil, err
	}

	return mapIssueSummaries(operationQuery, items, opts.Offset, opts.Limit)
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
		summary, mapErr := item.toIssueSummary(operationBlockedIssues)
		if mapErr != nil {
			return nil, mapErr
		}

		blockedBy := make([]domain.IssueReference, 0, len(item.BlockedBy))
		for _, id := range item.BlockedBy {
			blockedBy = append(blockedBy, domain.IssueReference{ID: id})
		}

		views = append(views, domain.BlockedIssueView{
			Issue:     summary,
			BlockedBy: blockedBy,
		})
	}

	return paginate(views, query.Offset, query.Limit), nil
}

// ReadyExplain returns ready and dependency-blocked issues with aggregate counts
// using `bd ready --explain --json`. Unlike separate ReadyIssues and BlockedIssues
// calls, this is a single bd invocation that surfaces both queues together with
// summary totals and cycle detection metadata.
func (g *Gateway) ReadyExplain(ctx context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	args := []string{"ready", "--explain", "--json"}
	if opts.Limit > 0 {
		args = append(args, "--limit", strconv.Itoa(opts.Limit))
	}

	payload, err := RunJSON[bdReadyExplainPayload](ctx, g.runner, CommandRequest{Operation: operationReadyExplain, Args: args})
	if err != nil {
		return domain.ReadyExplainResult{}, err
	}

	ready, err := mapIssueSummaries(operationReadyExplain, payload.Ready, 0, 0)
	if err != nil {
		return domain.ReadyExplainResult{}, err
	}

	blocked := make([]domain.BlockedIssueView, 0, len(payload.Blocked))
	for _, item := range payload.Blocked {
		summary, mapErr := item.toIssueSummary(operationReadyExplain)
		if mapErr != nil {
			return domain.ReadyExplainResult{}, mapErr
		}

		blockedBy := make([]domain.IssueReference, 0, len(item.BlockedBy))
		for _, ref := range item.BlockedBy {
			blockedBy = append(blockedBy, domain.IssueReference{
				ID:       optionalString(ref.ID),
				Title:    optionalString(ref.Title),
				Priority: optionalInt(ref.Priority),
				Status:   optionalString(ref.Status),
			})
		}

		blocked = append(blocked, domain.BlockedIssueView{
			Issue:     summary,
			BlockedBy: blockedBy,
		})
	}

	return domain.ReadyExplainResult{
		Ready:        ready,
		Blocked:      blocked,
		TotalReady:   payload.Summary.TotalReady,
		TotalBlocked: payload.Summary.TotalBlocked,
		CycleCount:   payload.Summary.CycleCount,
	}, nil
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
	summary, err := primary.toIssueSummary(operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	description, err := requiredString(primary.Description, "description")
	if err != nil {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeDecodeFailed, operationShowIssue, "failed to decode command JSON output", err)
	}

	closedAt, err := optionalTimestamp(primary.ClosedAt, "closed_at")
	if err != nil {
		return domain.IssueDetail{}, newGatewayError(domain.ErrorCodeDecodeFailed, operationShowIssue, "failed to decode command JSON output", err)
	}

	blockedBy, relatedFromDependencies, parentIssue, hasParent, err := dependencyReferencesFromPayload(primary.Dependencies, operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	blocks, relatedFromDependents, err := dependentReferencesFromPayload(primary.Dependents, operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	related, err := referencesFromPayload(primary.Related, operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	related = mergeUniqueReferences(related, relatedFromDependencies, relatedFromDependents)

	comments, err := commentsFromPayload(primary.Comments, operationShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	parentGroupContext := domain.ParentGroupBrowserContext{}
	if hasParent {
		siblings, err := g.parentChildSiblings(ctx, parentIssue.ID)
		if err != nil {
			return domain.IssueDetail{}, err
		}

		parentGroupContext = domain.ParentGroupBrowserContext{
			Parent:   parentIssue,
			Children: siblings,
		}
	}

	return domain.IssueDetail{
		Summary:            summary,
		Creator:            optionalString(primary.Owner),
		Description:        description,
		Notes:              optionalString(primary.Notes),
		ClosedAt:           closedAt,
		CloseReason:        optionalString(primary.CloseReason),
		ParentGroupBrowser: parentGroupContext,
		BlockedBy:          blockedBy,
		Blocks:             blocks,
		Related:            related,
		Comments:           comments,
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

	noTextFilter := strings.TrimSpace(query.Text) == ""

	switch query.WorkState {
	case domain.WorkStateReady:
		page, err := g.searchIssuesFromReady(ctx, query)
		if err != nil {
			return domain.SearchResultPage{}, err
		}
		if noTextFilter {
			page.Metadata.Notice = searchNoticeNoTextFilter
		}
		return page, nil
	case domain.WorkStateBlocked:
		page, err := g.searchIssuesFromBlocked(ctx, query)
		if err != nil {
			return domain.SearchResultPage{}, err
		}
		if noTextFilter {
			page.Metadata.Notice = searchNoticeNoTextFilter
		}
		return page, nil
	}

	args := []string{"search"}
	if text := strings.TrimSpace(query.Text); text != "" {
		args = append(args, text)
	}

	args = append(args, "--json")
	filterStatuses := query.Statuses
	if len(filterStatuses) == 0 {
		filterStatuses = []string{"all"}
	}
	args = append(args, buildFilterArgs(issueFilterArgs{
		Statuses:    filterStatuses,
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
		Metadata: searchMetadataFromLimitedBackendResults(
			len(results),
			query.Limit,
			domain.SearchResultSourceBDSearch,
		),
	}, nil
}

// CountIssues returns issue counts by status using `bd count --by-status --json`.
// Zero-count groups are omitted by bd count; Groups in the result contains only
// entries with a non-zero count.
func (g *Gateway) CountIssues(ctx context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	args := []string{"count", "--by-status", "--json"}
	args = append(args, buildFilterArgs(issueFilterArgs{
		Statuses: query.Statuses,
		Types:    query.Types,
		Assignee: query.Assignee,
		Labels:   query.Labels,
	})...)

	payload, err := RunJSON[bdCountByStatusPayload](ctx, g.runner, CommandRequest{Operation: operationCountIssues, Args: args})
	if err != nil {
		return domain.IssueCountResult{}, err
	}

	groups := make([]domain.IssueStatusCount, 0, len(payload.Groups))
	for _, g := range payload.Groups {
		groups = append(groups, domain.IssueStatusCount{
			Status: g.Group,
			Count:  g.Count,
		})
	}

	return domain.IssueCountResult{
		Groups: groups,
		Total:  payload.Total,
	}, nil
}

func (g *Gateway) searchIssuesFromList(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	args := []string{"list", "--json"}
	if len(query.Statuses) == 0 {
		args = append(args, "--all")
	}
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
		Metadata: searchMetadataFromLimitedBackendResults(
			len(results),
			query.Limit,
			domain.SearchResultSourceBDListFallback,
		),
	}, nil
}

func (g *Gateway) searchIssuesFromReady(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	// `bd search` does not expose ready semantics. For ready filtering we route
	// through `bd ready --json` and apply the remaining structured filters in-memory.
	items, err := g.decodeIssueArray(ctx, operationSearchIssues, []string{"ready", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	return g.searchIssuePageFromRecords(items, query, domain.SearchResultSourceReadyFilter)
}

func (g *Gateway) searchIssuesFromBlocked(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	// `bd search` does not expose dependency-blocked semantics and `bd blocked`
	// has a narrow flag surface. We intentionally keep the gateway API rich and
	// complete filtering in-memory after loading blocked results.
	items, err := g.decodeIssueArray(ctx, operationSearchIssues, []string{"blocked", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	return g.searchIssuePageFromRecords(items, query, domain.SearchResultSourceBlockedFilter)
}

func (g *Gateway) searchIssuePageFromRecords(items []bdIssuePayload, query domain.SearchIssuesQuery, source domain.SearchResultSource) (domain.SearchResultPage, error) {
	summaries, err := mapIssueSummaries(operationSearchIssues, items, 0, 0)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	filtered := filterIssueSummariesForSearch(summaries, query)
	paged := paginate(filtered, query.Offset, query.Limit)
	results := toSearchResults(paged)

	return domain.SearchResultPage{
		Results: results,
		// Use limited-backend metadata rather than exact: bd ready/blocked have
		// their own backend caps so the upstream set may be incomplete even after
		// in-memory filtering. Notice is intentionally empty here; the caller
		// (SearchIssues) attaches the no-text-filter notice when appropriate.
		Metadata: searchMetadataFromCappedBackendRecords(len(results), query.Limit, source),
	}, nil
}

func searchMetadataFromLimitedBackendResults(returnedCount int, requestedLimit int, source domain.SearchResultSource) domain.SearchResultMetadata {
	metadata := domain.SearchResultMetadata{
		ReturnedCount:  returnedCount,
		RequestedLimit: requestedLimit,
		Completeness:   domain.SearchResultCompletenessMaybeMore,
		Source:         source,
		Notice:         searchNoticeMaybeMore,
	}

	if requestedLimit <= 0 || returnedCount < requestedLimit {
		metadata.Completeness = domain.SearchResultCompletenessPartial
	}

	return metadata
}

// searchMetadataFromCappedBackendRecords builds metadata for in-memory filtered
// results fetched from a capped backend (bd ready / bd blocked). Unlike
// searchMetadataFromLimitedBackendResults it does not attach the MaybeMore
// notice text — callers attach their own notice when appropriate.
func searchMetadataFromCappedBackendRecords(returnedCount int, requestedLimit int, source domain.SearchResultSource) domain.SearchResultMetadata {
	completeness := domain.SearchResultCompletenessMaybeMore
	if requestedLimit <= 0 || returnedCount < requestedLimit {
		completeness = domain.SearchResultCompletenessPartial
	}

	return domain.SearchResultMetadata{
		ReturnedCount:  returnedCount,
		RequestedLimit: requestedLimit,
		Completeness:   completeness,
		Source:         source,
	}
}

func searchMetadataFromExactFilter(returnedCount int, requestedLimit int, source domain.SearchResultSource) domain.SearchResultMetadata {
	return domain.SearchResultMetadata{
		ReturnedCount:  returnedCount,
		RequestedLimit: requestedLimit,
		Completeness:   domain.SearchResultCompletenessExact,
		Source:         source,
	}
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
	payload, err := RunJSON[bdStatusCatalogPayload](ctx, g.runner, CommandRequest{Operation: operationStatuses, Args: []string{"statuses", "--json"}})
	if err != nil {
		return nil, err
	}

	all := append(payload.BuiltInStatuses, payload.CustomStatuses...)
	out := make([]domain.StatusOption, 0, len(all))
	for _, status := range all {
		mapped, mapErr := status.toStatusOption(operationStatuses)
		if mapErr != nil {
			return nil, mapErr
		}

		out = append(out, mapped)
	}

	return out, nil
}

// TypeCatalog returns available issue types using `bd types --json`.
func (g *Gateway) TypeCatalog(ctx context.Context) ([]domain.TypeOption, error) {
	payload, err := RunJSON[bdTypeCatalogPayload](ctx, g.runner, CommandRequest{Operation: operationTypes, Args: []string{"types", "--json"}})
	if err != nil {
		return nil, err
	}

	all := append(payload.CoreTypes, payload.CustomTypes...)
	out := make([]domain.TypeOption, 0, len(all))
	for _, typ := range all {
		mapped, mapErr := typ.toTypeOption(operationTypes)
		if mapErr != nil {
			return nil, mapErr
		}

		out = append(out, mapped)
	}

	return out, nil
}

// LabelCatalog returns available labels using `bd label list-all --json`.
func (g *Gateway) LabelCatalog(ctx context.Context) ([]domain.LabelOption, error) {
	labels, err := RunJSON[bdLabelListAllPayload](ctx, g.runner, CommandRequest{Operation: operationLabels, Args: []string{"label", "list-all", "--json"}})
	if err != nil {
		return nil, err
	}

	out := make([]domain.LabelOption, 0, len(labels))
	for _, label := range labels {
		mapped, mapErr := label.toLabelOption(operationLabels)
		if mapErr != nil {
			return nil, mapErr
		}

		if mapped.Name == "" {
			continue
		}

		out = append(out, mapped)
	}

	return out, nil
}

func (g *Gateway) decodeIssueArray(ctx context.Context, operation string, args []string) ([]bdIssuePayload, error) {
	return RunJSON[[]bdIssuePayload](ctx, g.runner, CommandRequest{Operation: operation, Args: args})
}

func mapIssueSummaries(operation string, records []bdIssuePayload, offset int, limit int) ([]domain.IssueSummary, error) {
	out := make([]domain.IssueSummary, 0, len(records))
	for _, record := range records {
		summary, err := record.toIssueSummary(operation)
		if err != nil {
			return nil, err
		}

		out = append(out, summary)
	}

	return paginate(out, offset, limit), nil
}

func referencesFromPayload(records []bdIssueRefPayload, operation string) ([]domain.IssueReference, error) {
	out := make([]domain.IssueReference, 0, len(records))
	for _, record := range records {
		ref, err := record.toIssueReference(operation)
		if err != nil {
			return nil, err
		}

		out = append(out, ref)
	}

	return out, nil
}

func dependencyReferencesFromPayload(records []bdIssueRefPayload, operation string) ([]domain.IssueReference, []domain.IssueReference, domain.IssueReference, bool, error) {
	blockedBy := make([]domain.IssueReference, 0, len(records))
	related := make([]domain.IssueReference, 0)
	parentIssue := domain.IssueReference{}
	hasParent := false
	for _, record := range records {
		ref, err := record.toIssueReference(operation)
		if err != nil {
			return nil, nil, domain.IssueReference{}, false, err
		}

		dependencyType := optionalString(record.DependencyType)
		if dependencyType == "related" || dependencyType == "relates-to" {
			related = append(related, ref)
			continue
		}

		if dependencyType == "parent-child" {
			if !hasParent {
				parentIssue = ref
				hasParent = true
			}

			continue
		}

		blockedBy = append(blockedBy, ref)
	}

	return blockedBy, related, parentIssue, hasParent, nil
}

func dependentReferencesFromPayload(records []bdIssueRefPayload, operation string) ([]domain.IssueReference, []domain.IssueReference, error) {
	blocks := make([]domain.IssueReference, 0, len(records))
	related := make([]domain.IssueReference, 0)
	for _, record := range records {
		ref, err := record.toIssueReference(operation)
		if err != nil {
			return nil, nil, err
		}

		dependencyType := optionalString(record.DependencyType)
		if dependencyType == "related" || dependencyType == "relates-to" {
			related = append(related, ref)
			continue
		}

		blocks = append(blocks, ref)
	}

	return blocks, related, nil
}

func (g *Gateway) parentChildSiblings(ctx context.Context, parentID string) ([]domain.IssueReference, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, nil
	}

	// Check cache first to avoid a second bd show per ShowIssue call when the
	// same parent has already been fetched by a prior detail load.
	g.parentSiblingCacheMu.RLock()
	cached, hit := g.parentSiblingCache[parentID]
	g.parentSiblingCacheMu.RUnlock()
	if hit {
		return cached, nil
	}

	items, err := g.decodeIssueArray(ctx, operationShowIssue, []string{"show", parentID, "--json"})
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}

	dependents := items[0].Dependents
	out := make([]domain.IssueReference, 0, len(dependents))
	for _, dependent := range dependents {
		if optionalString(dependent.DependencyType) != "parent-child" {
			continue
		}

		ref, err := dependent.toIssueReference(operationShowIssue)
		if err != nil {
			return nil, err
		}

		out = append(out, ref)
	}

	// Store in cache so future ShowIssue calls for siblings of the same parent
	// do not re-fetch.
	g.parentSiblingCacheMu.Lock()
	g.parentSiblingCache[parentID] = out
	g.parentSiblingCacheMu.Unlock()

	return out, nil
}

func mergeUniqueReferences(groups ...[]domain.IssueReference) []domain.IssueReference {
	seen := make(map[string]struct{})
	out := make([]domain.IssueReference, 0)
	for _, group := range groups {
		for _, ref := range group {
			if _, ok := seen[ref.ID]; ok {
				continue
			}

			seen[ref.ID] = struct{}{}
			out = append(out, ref)
		}
	}

	return out
}

func commentsFromPayload(records []bdIssueCommentPayload, operation string) ([]domain.IssueComment, error) {
	out := make([]domain.IssueComment, 0, len(records))
	for _, record := range records {
		comment, err := record.toIssueComment(operation)
		if err != nil {
			return nil, err
		}

		out = append(out, comment)
	}

	return out, nil
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
	case domain.SortFieldClosedAt:
		return "closed"
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
