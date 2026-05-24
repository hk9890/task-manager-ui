package memory

import "github.com/hk9890/beads-workbench/internal/domain"

// Forget removes an issue from the in-memory store. Used by callers that
// know an external authority (e.g. bd) is the source of truth and want to
// drop the cached copy. No-op if id is not present.
func (r *Repository) Forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.issues, id)
}

// Reset removes all issues from the in-memory store. Used by external
// invalidation sources (e.g. CachingRepository background refresh) that need
// to flush the entire per-ID cache when the backing store state has changed
// in a way that affects an unknown set of issues.
func (r *Repository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issues = make(map[string]*storedIssue)
}

// SeedDetail inserts a domain.IssueDetail into the store as a first-class
// cached entry. It maps all domain fields back to storedIssue so that
// subsequent Issue(id) calls return an equivalent value.
//
// This is used by CachingRepository to seed backing-store results into the
// local memory cache without a lossy round-trip through memory.Seed +
// memory.Issue. The caller always returns the backing's IssueDetail directly
// to the end caller, so any minor projection differences (e.g. reverse Blocks
// lookup) do not affect the user-visible result.
func (r *Repository) SeedDetail(detail domain.IssueDetail) {
	sum := detail.Summary

	labels := make([]string, len(sum.Labels))
	copy(labels, sum.Labels)

	// Translate BlockedBy references back to raw dep IDs.
	dependsOn := make([]string, 0, len(detail.BlockedBy))
	for _, ref := range detail.BlockedBy {
		dependsOn = append(dependsOn, ref.ID)
	}

	// Translate Blocks references back to explicit blocksIDs.
	blocksIDs := make([]string, 0, len(detail.Blocks))
	for _, ref := range detail.Blocks {
		blocksIDs = append(blocksIDs, ref.ID)
	}

	// Translate Related references back to raw IDs.
	related := make([]string, 0, len(detail.Related))
	for _, ref := range detail.Related {
		related = append(related, ref.ID)
	}

	// Translate Comments.
	comments := make([]storedComment, len(detail.Comments))
	for i, c := range detail.Comments {
		comments[i] = storedComment{
			id:        c.ID,
			author:    c.Author,
			body:      c.Body,
			createdAt: c.CreatedAt,
		}
	}

	// Translate ParentGroupBrowser.
	parentID := detail.ParentGroupBrowser.Parent.ID

	childrenIDs := make([]string, 0, len(detail.ParentGroupBrowser.Children))
	for _, ref := range detail.ParentGroupBrowser.Children {
		childrenIDs = append(childrenIDs, ref.ID)
	}

	si := &storedIssue{
		id:          sum.ID,
		title:       sum.Title,
		status:      sum.Status,
		priority:    sum.Priority,
		issueType:   sum.Type,
		assignee:    sum.Assignee,
		labels:      labels,
		description: detail.Description,
		notes:       detail.Notes,
		dependsOn:   dependsOn,
		blocksIDs:   blocksIDs,
		related:     related,
		parentID:    parentID,
		childrenIDs: childrenIDs,
		comments:    comments,
		created:     sum.CreatedAt,
		updated:     sum.UpdatedAt,
		closed:      detail.ClosedAt,
		closeReason: detail.CloseReason,
	}

	r.mu.Lock()
	r.issues[sum.ID] = si
	r.mu.Unlock()
}
