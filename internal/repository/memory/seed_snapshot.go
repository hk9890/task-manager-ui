package memory

import "github.com/hk9890/task-manager-ui/internal/domain"

// SeedFromSnapshot inserts or replaces an issue in the store from a
// SnapshotIssue value produced by [Repository.Snapshot]. It is used by
// filestorage.Load to restore persisted issues.
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
