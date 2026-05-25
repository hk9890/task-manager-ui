package beads

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	bdrunner "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// defaultLeanClosedLimit is the cap sent to the "status=closed" Query call
// inside Dashboard. Matches the floor from board.closedLimit().
const defaultLeanClosedLimit = 50

// searchNoticeMaybeMore is attached to search result metadata when the backend
// limit may have capped additional matches.
const searchNoticeMaybeMore = "Results may be incomplete because the backend limit may have capped additional matches."

// searchNoticeNoTextFilter is attached to search result metadata when a
// ready/blocked work-state search is performed without a text filter.
const searchNoticeNoTextFilter = "No text filter applied; returning entire ready/blocked queue."

// -- Read methods --

// HealthCheck verifies bd is reachable and a beads database exists.
func (r *Repository) HealthCheck(ctx context.Context) error {
	_, err := r.run(ctx, bdrunner.CommandRequest{
		Operation: leanOpHealthCheck,
		Args:      []string{"ping", "--json"},
	})
	return err
}

// Dashboard fans out five bd calls in parallel and assembles repository.DashboardData.
// Any single failure cancels remaining in-flight calls; no partial result is returned.
func (r *Repository) Dashboard(ctx context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
	g, gCtx := errgroup.WithContext(ctx)

	var readyExplain domain.ReadyExplainResult
	var inProgress []domain.IssueSummary
	var closed []domain.IssueSummary
	var closedCount domain.IssueCountResult
	var blocked []domain.IssueSummary

	g.Go(func() error {
		var err error
		readyExplain, err = r.readyExplain(gCtx, 0)
		return err
	})

	g.Go(func() error {
		var err error
		inProgress, err = r.query(gCtx, "status=in_progress", domain.QueryOptions{Limit: 0})
		return err
	})

	g.Go(func() error {
		var err error
		closed, err = r.query(gCtx, "status=closed", domain.QueryOptions{
			IncludeClosed: true,
			SortBy:        domain.SortFieldClosedAt,
			SortOrder:     domain.SortDirectionDescending,
			Limit:         defaultLeanClosedLimit,
		})
		return err
	})

	g.Go(func() error {
		var err error
		closedCount, err = r.countIssues(gCtx, domain.IssueCountQuery{Statuses: []string{"closed"}})
		return err
	})

	g.Go(func() error {
		var err error
		blocked, err = r.query(gCtx, "status=blocked", domain.QueryOptions{Limit: 0})
		return err
	})

	if err := g.Wait(); err != nil {
		return repository.DashboardData{}, err
	}

	return repository.DashboardData{
		ReadyExplain: readyExplain,
		InProgress:   inProgress,
		Closed:       closed,
		ClosedTotal:  closedCount.Total,
		Blocked:      blocked,
	}, nil
}

// Issue returns full detail for the issue identified by id.
func (r *Repository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	if strings.TrimSpace(id) == "" {
		return domain.IssueDetail{}, leanNewGWError(leanErrorCodeValidation, leanOpShowIssue, "issue id is required", nil)
	}

	items, err := leanDecodeIssueArray(ctx, r.run, leanOpShowIssue, []string{"show", id, "--json"})
	if err != nil {
		return domain.IssueDetail{}, err
	}

	if len(items) == 0 {
		return domain.IssueDetail{}, leanNewGWError(leanErrorCodeNotFound, leanOpShowIssue, fmt.Sprintf("issue not found: %s", id), nil)
	}

	primary := items[0]
	summary, err := leanToIssueSummary(primary, leanOpShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	closedAt, err := leanOptTimestamp(primary.ClosedAt, "closed_at")
	if err != nil {
		return domain.IssueDetail{}, leanNewGWError(leanErrorCodeDecodeFailed, leanOpShowIssue, "failed to decode command JSON output", err)
	}

	blockedBy, relatedFromDeps, parentRef, hasParent, err := leanDepsFromPayload(primary.Dependencies, leanOpShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	blocks, relatedFromDependents, children, err := leanDependentsFromPayload(primary.Dependents, leanOpShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	relatedTop, err := leanRefsFromPayload(primary.Related, leanOpShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	related := leanMergeUniqueRefs(relatedTop, relatedFromDeps, relatedFromDependents)

	comments, err := leanCommentsFromPayload(primary.Comments, leanOpShowIssue)
	if err != nil {
		return domain.IssueDetail{}, err
	}

	parentGroupContext := domain.ParentGroupBrowserContext{}
	if hasParent {
		siblings, err := r.parentChildSiblings(ctx, parentRef.ID)
		if err != nil {
			return domain.IssueDetail{}, err
		}
		parentGroupContext = domain.ParentGroupBrowserContext{
			Parent:   parentRef,
			Children: siblings,
		}
	}

	return domain.IssueDetail{
		Summary:            summary,
		Creator:            leanOptStr(primary.Owner),
		Description:        leanOptStr(primary.Description),
		Notes:              leanOptStr(primary.Notes),
		ClosedAt:           closedAt,
		CloseReason:        leanOptStr(primary.CloseReason),
		ParentGroupBrowser: parentGroupContext,
		BlockedBy:          blockedBy,
		Blocks:             blocks,
		Related:            related,
		Children:           children,
		Comments:           comments,
	}, nil
}

// Search delegates to the appropriate bd command depending on query shape.
//
// Routing strategy:
//   - Empty text + WorkStateAny  → bd list --all (searchable list fallback)
//   - WorkStateReady             → bd ready --json + in-memory filter
//   - WorkStateBlocked           → bd blocked --json + in-memory filter
//   - Non-empty text             → bd search <text> --json
func (r *Repository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if query.PriorityMin != nil && query.PriorityMax != nil && *query.PriorityMin > *query.PriorityMax {
		return domain.SearchResultPage{}, leanNewGWError(leanErrorCodeValidation, leanOpSearchIssues, "priority_min cannot be greater than priority_max", nil)
	}

	if strings.TrimSpace(query.Text) == "" && query.WorkState == domain.WorkStateAny {
		return r.searchFromList(ctx, query)
	}

	noText := strings.TrimSpace(query.Text) == ""

	switch query.WorkState {
	case domain.WorkStateReady:
		page, err := r.searchFromReady(ctx, query)
		if err != nil {
			return domain.SearchResultPage{}, err
		}
		if noText {
			page.Metadata.Notice = searchNoticeNoTextFilter
		}
		return page, nil

	case domain.WorkStateBlocked:
		page, err := r.searchFromBlocked(ctx, query)
		if err != nil {
			return domain.SearchResultPage{}, err
		}
		if noText {
			page.Metadata.Notice = searchNoticeNoTextFilter
		}
		return page, nil
	}

	// bd search path.
	args := []string{"search"}
	if text := strings.TrimSpace(query.Text); text != "" {
		args = append(args, text)
	}
	args = append(args, "--json")

	// When no explicit statuses are requested, pass query.Statuses as-is (empty
	// slice). leanBuildFilterArgs omits --status entirely when the slice is
	// empty, letting bd search use its own default which excludes closed issues.
	// Callers who want closed issues must set query.Statuses = []string{"closed"}
	// or []string{"all"} explicitly.
	args = append(args, leanBuildFilterArgs(query.Statuses, query.Types, query.PriorityMin, query.PriorityMax, query.Assignee, query.Labels)...)

	if limit := leanWithOffsetWindow(query.Limit, query.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := leanDecodeIssueArray(ctx, r.run, leanOpSearchIssues, args)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	summaries, err := leanMapIssueSummaries(leanOpSearchIssues, items, query.Offset, query.Limit)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	results := leanToSearchResults(summaries)
	return domain.SearchResultPage{
		Results:  results,
		Metadata: leanSearchMetadataFromBackend(len(results), query.Limit, domain.SearchResultSourceBDSearch),
	}, nil
}

// Catalogs fans out three bd calls in parallel and returns available options.
func (r *Repository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	g, gCtx := errgroup.WithContext(ctx)

	var statuses []domain.StatusOption
	var types []domain.TypeOption
	var labels []domain.LabelOption

	g.Go(func() error {
		var err error
		statuses, err = r.statusCatalog(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		types, err = r.typeCatalog(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		labels, err = r.labelCatalog(gCtx)
		return err
	})

	if err := g.Wait(); err != nil {
		return repository.Catalogs{}, err
	}

	return repository.Catalogs{
		Statuses: statuses,
		Types:    types,
		Labels:   labels,
	}, nil
}

// -- Private helpers --

// readyExplain fetches `bd ready --explain --json [--limit N]`.
func (r *Repository) readyExplain(ctx context.Context, limit int) (domain.ReadyExplainResult, error) {
	args := []string{"ready", "--explain", "--json"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	payload, err := repoRunJSON[leanReadyExplainPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpReadyExplain,
		Args:      args,
	})
	if err != nil {
		return domain.ReadyExplainResult{}, err
	}

	ready, err := leanMapIssueSummaries(leanOpReadyExplain, payload.Ready, 0, 0)
	if err != nil {
		return domain.ReadyExplainResult{}, err
	}

	blockedViews := make([]domain.BlockedIssueView, 0, len(payload.Blocked))
	for _, item := range payload.Blocked {
		summary, err := leanExplainItemToSummary(item, leanOpReadyExplain)
		if err != nil {
			return domain.ReadyExplainResult{}, err
		}

		blockedBy := make([]domain.IssueReference, 0, len(item.BlockedBy))
		for _, ref := range item.BlockedBy {
			blockedBy = append(blockedBy, domain.IssueReference{
				ID:       leanOptStr(ref.ID),
				Title:    leanOptStr(ref.Title),
				Priority: leanOptInt(ref.Priority),
				Status:   leanOptStr(ref.Status),
			})
		}

		blockedViews = append(blockedViews, domain.BlockedIssueView{
			Issue:     summary,
			BlockedBy: blockedBy,
		})
	}

	return domain.ReadyExplainResult{
		Ready:        ready,
		Blocked:      blockedViews,
		TotalReady:   payload.Summary.TotalReady,
		TotalBlocked: payload.Summary.TotalBlocked,
		CycleCount:   payload.Summary.CycleCount,
	}, nil
}

// query fetches `bd query "<expr>" --json` with optional flags.
func (r *Repository) query(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error) {
	args := []string{"query", expr, "--json"}

	if opts.IncludeClosed {
		args = append(args, "-a")
	}

	if opts.SortBy != "" {
		if sf := leanMapSortField(opts.SortBy); sf != "" {
			args = append(args, "--sort", sf)
		}
	}

	if opts.SortOrder == domain.SortDirectionAscending {
		args = append(args, "--reverse")
	}

	if limit := leanWithOffsetWindow(opts.Limit, opts.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := leanDecodeIssueArray(ctx, r.run, leanOpQuery, args)
	if err != nil {
		return nil, err
	}

	return leanMapIssueSummaries(leanOpQuery, items, opts.Offset, opts.Limit)
}

// countIssues fetches `bd count --by-status --json`.
//
// Multi-status bd 1.0.4 quirk: bd count --status open,in_progress treats the CSV
// as a literal status name. When len(query.Statuses) > 1 we fetch all groups
// and filter in-memory.
func (r *Repository) countIssues(ctx context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	args := []string{"count", "--by-status", "--json"}

	// Omit --status when multiple statuses are requested; filter in-memory.
	filterStatuses := query.Statuses
	if len(filterStatuses) > 1 {
		filterStatuses = nil
	}
	args = append(args, leanBuildFilterArgs(filterStatuses, query.Types, nil, nil, query.Assignee, query.Labels)...)

	payload, err := repoRunJSON[leanCountByStatusPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpCountIssues,
		Args:      args,
	})
	if err != nil {
		return domain.IssueCountResult{}, err
	}

	groups := make([]domain.IssueStatusCount, 0, len(payload.Groups))
	total := 0
	for _, g := range payload.Groups {
		if len(query.Statuses) > 1 && !slices.Contains(query.Statuses, g.Group) {
			continue
		}
		groups = append(groups, domain.IssueStatusCount{Status: g.Group, Count: g.Count})
		total += g.Count
	}

	if len(query.Statuses) == 0 {
		total = payload.Total
	}

	return domain.IssueCountResult{Groups: groups, Total: total}, nil
}

// statusCatalog fetches `bd statuses --json`.
func (r *Repository) statusCatalog(ctx context.Context) ([]domain.StatusOption, error) {
	payload, err := repoRunJSON[leanStatusCatalogPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpStatuses,
		Args:      []string{"statuses", "--json"},
	})
	if err != nil {
		return nil, err
	}

	all := append(payload.BuiltInStatuses, payload.CustomStatuses...)
	out := make([]domain.StatusOption, 0, len(all))
	for _, entry := range all {
		name, err := leanReqStr(entry.Name, "name")
		if err != nil {
			return nil, leanNewGWError(leanErrorCodeDecodeFailed, leanOpStatuses, "failed to decode command JSON output", err)
		}
		out = append(out, domain.StatusOption{Name: name, Description: leanOptStr(entry.Description)})
	}
	return out, nil
}

// typeCatalog fetches `bd types --json`.
func (r *Repository) typeCatalog(ctx context.Context) ([]domain.TypeOption, error) {
	payload, err := repoRunJSON[leanTypeCatalogPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpTypes,
		Args:      []string{"types", "--json"},
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.TypeOption, 0, len(payload.CoreTypes)+len(payload.CustomTypes))
	for _, entry := range payload.CoreTypes {
		name, err := leanReqStr(entry.Name, "name")
		if err != nil {
			return nil, leanNewGWError(leanErrorCodeDecodeFailed, leanOpTypes, "failed to decode command JSON output", err)
		}
		out = append(out, domain.TypeOption{Name: name, Description: leanOptStr(entry.Description)})
	}
	// bd 1.0.4 returns custom_types as bare strings.
	for _, name := range payload.CustomTypes {
		out = append(out, domain.TypeOption{Name: name})
	}
	return out, nil
}

// labelCatalog fetches `bd label list-all --json`.
func (r *Repository) labelCatalog(ctx context.Context) ([]domain.LabelOption, error) {
	labels, err := repoRunJSON[[]leanLabelEntryPayload](ctx, r, bdrunner.CommandRequest{
		Operation: leanOpLabels,
		Args:      []string{"label", "list-all", "--json"},
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.LabelOption, 0, len(labels))
	for _, entry := range labels {
		name, err := leanReqStr(entry.Label, "label")
		if err != nil {
			return nil, leanNewGWError(leanErrorCodeDecodeFailed, leanOpLabels, "failed to decode command JSON output", err)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, domain.LabelOption{Name: name})
	}
	return out, nil
}

// searchFromList handles empty-text + WorkStateAny searches via bd list --all.
func (r *Repository) searchFromList(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	args := []string{"list", "--json"}
	if len(query.Statuses) == 0 {
		args = append(args, "--all")
	}
	args = append(args, leanBuildFilterArgs(query.Statuses, query.Types, query.PriorityMin, query.PriorityMax, query.Assignee, query.Labels)...)

	if limit := leanWithOffsetWindow(query.Limit, query.Offset); limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	items, err := leanDecodeIssueArray(ctx, r.run, leanOpSearchIssues, args)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	summaries, err := leanMapIssueSummaries(leanOpSearchIssues, items, query.Offset, query.Limit)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	results := leanToSearchResults(summaries)
	return domain.SearchResultPage{
		Results:  results,
		Metadata: leanSearchMetadataFromBackend(len(results), query.Limit, domain.SearchResultSourceBDListFallback),
	}, nil
}

// searchFromReady handles WorkStateReady search via bd ready --json + in-memory filter.
func (r *Repository) searchFromReady(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	items, err := leanDecodeIssueArray(ctx, r.run, leanOpSearchIssues, []string{"ready", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}
	return r.searchPageFromRecords(items, query, domain.SearchResultSourceReadyFilter)
}

// searchFromBlocked handles WorkStateBlocked search via bd blocked --json + in-memory filter.
func (r *Repository) searchFromBlocked(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	items, err := leanDecodeIssueArray(ctx, r.run, leanOpSearchIssues, []string{"blocked", "--json"})
	if err != nil {
		return domain.SearchResultPage{}, err
	}
	return r.searchPageFromRecords(items, query, domain.SearchResultSourceBlockedFilter)
}

// searchPageFromRecords applies in-memory filters and pagination on pre-fetched records.
func (r *Repository) searchPageFromRecords(items []leanIssuePayload, query domain.SearchIssuesQuery, source domain.SearchResultSource) (domain.SearchResultPage, error) {
	summaries, err := leanMapIssueSummaries(leanOpSearchIssues, items, 0, 0)
	if err != nil {
		return domain.SearchResultPage{}, err
	}

	filtered := leanFilterForSearch(summaries, query)
	paged := leanPaginate(filtered, query.Offset, query.Limit)
	results := leanToSearchResults(paged)

	completeness := domain.SearchResultCompletenessExact
	if query.Limit > 0 && len(results) >= query.Limit {
		completeness = domain.SearchResultCompletenessPartial
	}

	return domain.SearchResultPage{
		Results: results,
		Metadata: domain.SearchResultMetadata{
			ReturnedCount:  len(results),
			RequestedLimit: query.Limit,
			Completeness:   completeness,
			Source:         source,
		},
	}, nil
}

// leanFilterForSearch applies text, status, type, assignee, label, and priority
// filters in-memory on a pre-fetched summary slice.
func leanFilterForSearch(items []domain.IssueSummary, query domain.SearchIssuesQuery) []domain.IssueSummary {
	out := make([]domain.IssueSummary, 0, len(items))
	for _, item := range items {
		if !leanMatchesSearch(item, query) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func leanMatchesSearch(item domain.IssueSummary, query domain.SearchIssuesQuery) bool {
	if text := strings.TrimSpace(query.Text); text != "" {
		lower := strings.ToLower(text)
		if !strings.Contains(strings.ToLower(item.ID), lower) &&
			!strings.Contains(strings.ToLower(item.Title), lower) {
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

// leanToSearchResults wraps issue summaries in domain.SearchResult containers.
func leanToSearchResults(issues []domain.IssueSummary) []domain.SearchResult {
	out := make([]domain.SearchResult, 0, len(issues))
	for _, s := range issues {
		out = append(out, domain.SearchResult{Issue: s})
	}
	return out
}

// leanSearchMetadataFromBackend builds search metadata for direct-backend results.
func leanSearchMetadataFromBackend(returnedCount, requestedLimit int, source domain.SearchResultSource) domain.SearchResultMetadata {
	completeness := domain.SearchResultCompletenessExact
	if requestedLimit > 0 && returnedCount >= requestedLimit {
		completeness = domain.SearchResultCompletenessPartial
	}
	return domain.SearchResultMetadata{
		ReturnedCount:  returnedCount,
		RequestedLimit: requestedLimit,
		Completeness:   completeness,
		Source:         source,
		Notice:         searchNoticeMaybeMore,
	}
}
