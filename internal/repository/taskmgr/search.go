package taskmgr

import (
	"context"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// Search maps the structured query onto a tasks.Criteria and runs FindPage. The
// closed partition is always scanned so search spans active and closed work
// (mirroring the memory backend); status/work predicates in the criteria still
// narrow the result.
//
// SearchResult.Snippet is left empty: the task-manager backend does not produce
// match snippets. Result ordering is work order (priority, then created), which
// is not a documented contract.
func (r *Repository) Search(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if err := ctx.Err(); err != nil {
		return domain.SearchResultPage{}, err
	}
	criteria, opt := buildCriteria(q)

	// If the caller requested a status/type filter but every value was unknown,
	// nothing can match. Return empty rather than letting the now-empty filter
	// widen the result to all statuses/types (which would also trip the
	// validating decorator's filter-respected check, one warn per result).
	if (len(q.Statuses) > 0 && len(criteria.Statuses) == 0) ||
		(len(q.Types) > 0 && len(criteria.Types) == 0) {
		return emptySearchPage(q.Limit), nil
	}

	page, err := r.store.FindPage(criteria, opt)
	if err != nil {
		return domain.SearchResultPage{}, mapReadErr("search", err)
	}

	results := make([]domain.SearchResult, 0, len(page.Issues))
	for _, i := range page.Issues {
		results = append(results, domain.SearchResult{Issue: toSummary(i)})
	}

	// The window is complete unless a positive Limit truncated matches that lie
	// beyond it (Page.Total counts all matches in scope before Offset/Limit).
	completeness := domain.SearchResultCompletenessExact
	if q.Limit > 0 && page.Total > q.Offset+len(results) {
		completeness = domain.SearchResultCompletenessMaybeMore
	}

	return domain.SearchResultPage{
		Results: results,
		Metadata: domain.SearchResultMetadata{
			ReturnedCount:  len(results),
			RequestedLimit: q.Limit,
			Completeness:   completeness,
			Source:         domain.SearchResultSourceTaskmgrFind,
		},
	}, nil
}

// emptySearchPage returns a complete, zero-result page (used when a requested
// filter cannot match anything).
func emptySearchPage(limit int) domain.SearchResultPage {
	return domain.SearchResultPage{
		Results: []domain.SearchResult{},
		Metadata: domain.SearchResultMetadata{
			ReturnedCount:  0,
			RequestedLimit: limit,
			Completeness:   domain.SearchResultCompletenessExact,
			Source:         domain.SearchResultSourceTaskmgrFind,
		},
	}
}

// buildCriteria translates a SearchIssuesQuery into a tasks.Criteria plus the
// presentation FindOptions. Label matching defaults to LabelMatchAll.
//
// Free-text matching uses TextAllWords (AND-of-words): every whitespace-separated
// word in Text must appear (order-independent, per-word substring), matching the
// task-manager CLI `search` command and SDK SearchExpr so the UI and CLI search
// identically. The SDK leaves the zero value at TextPhrase, so this must be set
// explicitly.
//
// Per the Repository.Search contract, unrecognized filter values are forwarded
// without validation rather than surfaced as errors: unknown status/type tokens
// and negative priority bounds (which Criteria.Build would reject) are dropped so
// a search never hard-fails on an odd filter.
func buildCriteria(q domain.SearchIssuesQuery) (tasks.Criteria, tasks.FindOptions) {
	criteria := tasks.Criteria{
		Text:      q.Text,
		TextMatch: tasks.TextAllWords,
		Assignee:  q.Assignee,
	}
	if q.PriorityMin != nil && *q.PriorityMin >= 0 {
		criteria.PriorityMin = q.PriorityMin
	}
	if q.PriorityMax != nil && *q.PriorityMax >= 0 {
		criteria.PriorityMax = q.PriorityMax
	}
	for _, s := range q.Statuses {
		if st := tasks.Status(s); st.Valid() {
			criteria.Statuses = append(criteria.Statuses, st)
		}
	}
	for _, t := range q.Types {
		if tt := tasks.Type(t); tt.Valid() {
			criteria.Types = append(criteria.Types, tt)
		}
	}
	if len(q.Labels) > 0 {
		criteria.Labels = q.Labels
	}
	switch q.WorkState {
	case domain.WorkStateReady:
		criteria.Work = tasks.WorkReady
	case domain.WorkStateBlocked:
		criteria.Work = tasks.WorkBlocked
	}

	return criteria, tasks.FindOptions{
		IncludeClosed: true,
		Sort:          tasks.SortWork,
		Offset:        q.Offset,
		Limit:         q.Limit,
	}
}
