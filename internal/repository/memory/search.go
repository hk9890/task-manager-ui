package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// Search implements repository.Repository.
//
// Text is matched case-insensitively across Title, Description, and Notes using
// AND-of-words: every whitespace-separated word must appear (order-independent,
// per-word substring), matching the taskmgr backend and the task-manager CLI.
// Statuses, Types, Labels, and Assignee are intersection filters (AND
// semantics across fields; OR semantics within Labels). PriorityMin/Max bound
// priority (nil = unbounded). WorkState=ready and WorkState=blocked derive
// from dep-closure state (not stored status). Limit and Offset apply after all
// filters.
//
// The returned Metadata.Completeness is always SearchResultCompletenessExact
// because memory always returns the full result set.
func (r *Repository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if err := ctx.Err(); err != nil {
		return domain.SearchResultPage{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []domain.SearchResult

	for _, si := range r.issues {
		if !r.matchesSearchLocked(si, query) {
			continue
		}

		snippet := r.buildSnippet(si, query.Text)
		results = append(results, domain.SearchResult{
			Issue:   r.toSummaryLocked(si),
			Snippet: snippet,
		})
	}

	// Sort consistently: by ID for determinism (mirrors typical list behavior).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Issue.ID < results[j].Issue.ID
	})

	// Apply offset and limit.
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	} else if query.Offset >= len(results) {
		results = nil
	}

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	if results == nil {
		results = []domain.SearchResult{}
	}

	return domain.SearchResultPage{
		Results: results,
		Metadata: domain.SearchResultMetadata{
			ReturnedCount:  len(results),
			RequestedLimit: query.Limit,
			Completeness:   domain.SearchResultCompletenessExact,
			Source:         domain.SearchResultSourceBDSearch,
		},
	}, nil
}

// matchesSearchLocked reports whether si matches the given SearchIssuesQuery.
// Caller must hold at least RLock.
func (r *Repository) matchesSearchLocked(si *storedIssue, q domain.SearchIssuesQuery) bool {
	// Text filter: case-insensitive AND-of-words across Title and Description.
	// Every whitespace-separated word in q.Text must appear as a substring in at
	// least one of those fields; words may match different fields (order-independent).
	// This mirrors the task-manager SDK's TextAllWords semantics so search behaves
	// identically across the memory and taskmgr backends and the CLI. Notes are
	// intentionally excluded: the taskmgr backend has no notes field (the SDK
	// stores a single markdown body), so matching notes here would let the memory
	// fixture certify search behavior the real backend cannot reproduce. A
	// whitespace-only query imposes no constraint (strings.Fields yields no words).
	if q.Text != "" {
		title := strings.ToLower(si.title)
		desc := strings.ToLower(si.description)
		for _, word := range strings.Fields(strings.ToLower(q.Text)) {
			if !strings.Contains(title, word) &&
				!strings.Contains(desc, word) {
				return false
			}
		}
	}

	// Statuses filter: OR semantics within the list.
	if len(q.Statuses) > 0 {
		matched := false
		for _, s := range q.Statuses {
			if si.status == s {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Types filter: OR semantics within the list.
	if len(q.Types) > 0 {
		matched := false
		for _, t := range q.Types {
			if si.issueType == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Labels filter: issue must contain all requested labels.
	if len(q.Labels) > 0 {
		labelSet := make(map[string]struct{}, len(si.labels))
		for _, l := range si.labels {
			labelSet[l] = struct{}{}
		}
		for _, want := range q.Labels {
			if _, ok := labelSet[want]; !ok {
				return false
			}
		}
	}

	// Assignee filter: exact match.
	if q.Assignee != "" && si.assignee != q.Assignee {
		return false
	}

	// Priority bounds.
	if q.PriorityMin != nil && si.priority < *q.PriorityMin {
		return false
	}
	if q.PriorityMax != nil && si.priority > *q.PriorityMax {
		return false
	}

	// WorkState filter mirrors taskmgr ready / taskmgr blocked semantics:
	// - WorkStateReady: stored status == "open" AND all deps closed (matches taskmgr ready).
	// - WorkStateBlocked: has at least one open dep AND status not "closed" (matches
	//   taskmgr blocked, which returns dep-blocked issues regardless of stored status).
	if q.WorkState != domain.WorkStateAny {
		allDepsClosed, _ := r.depStateLocked(si.dependsOn)

		switch q.WorkState {
		case domain.WorkStateReady:
			// Ready requires literal status=open (taskmgr ready excludes blocked, deferred, etc.)
			// AND no open dependency blockers.
			if si.status != "open" || !allDepsClosed {
				return false
			}
		case domain.WorkStateBlocked:
			// Blocked (dep-closure sense): has open deps AND status is not "closed".
			if si.status == "closed" || allDepsClosed {
				return false
			}
		}
	}

	return true
}

// buildSnippet produces a short snippet from the matched field. Returns the
// field where the needle was found; empty string when no match (shouldn't
// happen after matchesSearch returns true, but guards edge cases).
func (r *Repository) buildSnippet(si *storedIssue, text string) string {
	if text == "" {
		return ""
	}
	needle := strings.ToLower(text)
	if strings.Contains(strings.ToLower(si.title), needle) {
		return si.title
	}
	if strings.Contains(strings.ToLower(si.description), needle) {
		return si.description
	}
	if strings.Contains(strings.ToLower(si.notes), needle) {
		return si.notes
	}
	return ""
}
