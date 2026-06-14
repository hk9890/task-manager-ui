package memory

import "github.com/hk9890/task-manager-ui/internal/domain"

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

// SeedFromSnapshot inserts or replaces an issue in the store from a
// SnapshotIssue value produced by [Repository.Snapshot]. It is used by
// [filestorage.LoadWithManifest] to restore persisted issues.
//
// When any of snap.BlockedByRefs, snap.RelatedRefs, or snap.ParentRef is
// non-nil, SeedFromSnapshot stores them verbatim on the storedIssue so that a
// subsequent Issue call returns the full cross-reference metadata without
// re-resolving against the memory map. This preserves the
// Title/Status/Type/Priority of cross-referenced issues across a Save+Load cycle.
//
// When all three fields are nil (e.g. an old on-disk JSONL written before these
// fields existed), SeedFromSnapshot falls back to the same re-resolution path
// as Seed — a subsequent Issue call re-resolves references from the memory map.
//
// Comments and closed state are also restored from the snapshot.
func (r *Repository) SeedFromSnapshot(snap SnapshotIssue) {
	// Base fields — same as Seed.
	r.Seed(Issue{
		ID:          snap.ID,
		Title:       snap.Title,
		Status:      snap.Status,
		Priority:    snap.Priority,
		Type:        snap.Type,
		Assignee:    snap.Assignee,
		Labels:      snap.Labels,
		Description: snap.Description,
		Notes:       snap.Notes,
		DependsOn:   snap.DependsOn,
		Related:     snap.Related,
		ParentID:    snap.ParentID,
		Created:     snap.Created,
		Updated:     snap.Updated,
	})

	// Restore cross-reference metadata and Creator when present.
	// The nil check is the sentinel for refs: nil → leave storedIssue field nil (re-resolve).
	hasExtras := snap.BlockedByRefs != nil ||
		snap.RelatedRefs != nil ||
		snap.ParentRef != nil ||
		snap.Creator != ""

	if hasExtras {
		r.mu.Lock()
		si := r.issues[snap.ID]

		if snap.Creator != "" {
			si.creator = snap.Creator
		}
		if snap.BlockedByRefs != nil {
			refs := make([]domain.IssueReference, len(snap.BlockedByRefs))
			copy(refs, snap.BlockedByRefs)
			si.blockedByRefs = refs
		}
		if snap.RelatedRefs != nil {
			refs := make([]domain.IssueReference, len(snap.RelatedRefs))
			copy(refs, snap.RelatedRefs)
			si.relatedRefs = refs
		}
		if snap.ParentRef != nil {
			ref := *snap.ParentRef
			si.parentRef = &ref
		}

		r.mu.Unlock()
	}

	// Restore comments.
	if len(snap.Comments) > 0 {
		memComments := make([]Comment, len(snap.Comments))
		for i, c := range snap.Comments {
			memComments[i] = Comment(c)
		}
		r.SeedComments(snap.ID, memComments...)
	}

	// Restore closed state.
	if snap.Status == "closed" && !snap.Closed.IsZero() {
		r.SeedClosed(snap.ID, snap.Closed, snap.CloseReason)
	}
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

	// Translate ParentGroupBrowser: store the raw parent ID (for
	// backward-compatible re-resolution paths) and the full IssueReference
	// metadata (for verbatim projection on cache hit).
	parentID := detail.ParentGroupBrowser.Parent.ID
	var parentRef *domain.IssueReference
	if parentID != "" {
		ref := detail.ParentGroupBrowser.Parent
		parentRef = &ref
	}

	si := &storedIssue{
		id:            sum.ID,
		title:         sum.Title,
		status:        sum.Status,
		priority:      sum.Priority,
		issueType:     sum.Type,
		assignee:      sum.Assignee,
		creator:       detail.Creator,
		labels:        labels,
		description:   detail.Description,
		notes:         detail.Notes,
		dependsOn:     dependsOn,
		blocksIDs:     blocksIDs,
		related:       related,
		parentID:      parentID,
		comments:      comments,
		created:       sum.CreatedAt,
		updated:       sum.UpdatedAt,
		closed:        detail.ClosedAt,
		closeReason:   detail.CloseReason,
		blockedByRefs: blockedByRefs,
		relatedRefs:   relatedRefs,
		parentRef:     parentRef,
	}

	r.mu.Lock()
	r.issues[sum.ID] = si
	r.mu.Unlock()
}
