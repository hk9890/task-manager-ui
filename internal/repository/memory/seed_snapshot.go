package memory

// SeedFromSnapshot inserts or replaces an issue in the store from a
// SnapshotIssue value produced by [Repository.Snapshot]. It is used by
// filestorage.Load to restore persisted issues.
//
// Comments, Creator, and closed state are restored from the snapshot.
// Cross-reference metadata (BlockedBy, Related, Parent) is re-resolved from the
// in-memory map on demand, the same as the Seed path.
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

	// Restore Creator when present.
	if snap.Creator != "" {
		r.mu.Lock()
		si := r.issues[snap.ID]
		si.creator = snap.Creator
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
