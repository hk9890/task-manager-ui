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
//
// SeedDetail stores the full IssueReference metadata for BlockedBy, Related,
// and ParentGroupBrowser cross-references on the storedIssue. This ensures
// that a subsequent Issue(id) call (cache hit) returns the full metadata
// verbatim, even when the referenced issues have never been seeded themselves.
func (r *Repository) SeedDetail(detail domain.IssueDetail) {
	sum := detail.Summary

	labels := make([]string, len(sum.Labels))
	copy(labels, sum.Labels)

	// Translate BlockedBy references back to raw dep IDs (for depStateLocked /
	// matchesSearchLocked / Dashboard which only need IDs), and also store the
	// full IssueReference slice verbatim so toDetailLocked can return them
	// without re-resolving.
	dependsOn := make([]string, 0, len(detail.BlockedBy))
	blockedByRefs := make([]domain.IssueReference, len(detail.BlockedBy))
	for i, ref := range detail.BlockedBy {
		dependsOn = append(dependsOn, ref.ID)
		blockedByRefs[i] = ref
	}

	// Translate Blocks references back to explicit blocksIDs.
	// Note: the stored blocksRefs are NOT stored separately; the Blocks bucket
	// in toDetailLocked remains a computed reverse-lookup (or explicit blocksIDs
	// re-resolution). This matches the task scope — only BlockedBy, Related, and
	// ParentGroupBrowser cross-refs are preserved verbatim.
	blocksIDs := make([]string, 0, len(detail.Blocks))
	for _, ref := range detail.Blocks {
		blocksIDs = append(blocksIDs, ref.ID)
	}

	// Translate Related references back to raw IDs and store full refs.
	related := make([]string, 0, len(detail.Related))
	relatedRefs := make([]domain.IssueReference, len(detail.Related))
	for i, ref := range detail.Related {
		related = append(related, ref.ID)
		relatedRefs[i] = ref
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

	// Translate ParentGroupBrowser: store both the raw ID fields (for
	// backward-compatible re-resolution paths) and the full IssueReference
	// metadata (for verbatim projection on cache hit).
	parentID := detail.ParentGroupBrowser.Parent.ID
	var parentRef *domain.IssueReference
	if parentID != "" {
		ref := detail.ParentGroupBrowser.Parent
		parentRef = &ref
	}

	childrenIDs := make([]string, 0, len(detail.ParentGroupBrowser.Children))
	childrenRefs := make([]domain.IssueReference, len(detail.ParentGroupBrowser.Children))
	for i, ref := range detail.ParentGroupBrowser.Children {
		childrenIDs = append(childrenIDs, ref.ID)
		childrenRefs[i] = ref
	}

	si := &storedIssue{
		id:            sum.ID,
		title:         sum.Title,
		status:        sum.Status,
		priority:      sum.Priority,
		issueType:     sum.Type,
		assignee:      sum.Assignee,
		labels:        labels,
		description:   detail.Description,
		notes:         detail.Notes,
		dependsOn:     dependsOn,
		blocksIDs:     blocksIDs,
		related:       related,
		parentID:      parentID,
		childrenIDs:   childrenIDs,
		comments:      comments,
		created:       sum.CreatedAt,
		updated:       sum.UpdatedAt,
		closed:        detail.ClosedAt,
		closeReason:   detail.CloseReason,
		blockedByRefs: blockedByRefs,
		relatedRefs:   relatedRefs,
		parentRef:     parentRef,
		childrenRefs:  childrenRefs,
	}

	r.mu.Lock()
	r.issues[sum.ID] = si
	r.mu.Unlock()
}
