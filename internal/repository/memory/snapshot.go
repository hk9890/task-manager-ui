package memory

import "time"

// SnapshotIssue is a read-only exported view of a storedIssue for use by
// file persistence (Save). It preserves all fields, including timestamps and
// comments, so a round-trip through Save/Load is lossless.
type SnapshotIssue struct {
	ID          string
	Title       string
	Status      string
	Priority    int
	Type        string
	Assignee    string
	Creator     string
	Labels      []string
	Description string
	Notes       string
	DependsOn   []string
	Related     []string
	ParentID    string
	Comments    []SnapshotComment
	Created     time.Time
	Updated     time.Time
	Closed      time.Time
	CloseReason string
}

// SnapshotComment is the exported view of a storedComment used by Snapshot.
type SnapshotComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
}

// Snapshot returns a read-only copy of all issues in the store. The slice is
// safe to use after the call without holding any lock. It is used by
// [repository.Save] to serialize the in-memory store; callers outside the
// persistence layer should prefer the normal Repository interface.
func (r *Repository) Snapshot() []SnapshotIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]SnapshotIssue, 0, len(r.issues))
	for _, si := range r.issues {
		labels := make([]string, len(si.labels))
		copy(labels, si.labels)

		deps := make([]string, len(si.dependsOn))
		copy(deps, si.dependsOn)

		related := make([]string, len(si.related))
		copy(related, si.related)

		comments := make([]SnapshotComment, len(si.comments))
		for i, c := range si.comments {
			comments[i] = SnapshotComment{
				ID:        c.id,
				Author:    c.author,
				Body:      c.body,
				CreatedAt: c.createdAt,
			}
		}

		out = append(out, SnapshotIssue{
			ID:          si.id,
			Title:       si.title,
			Status:      si.status,
			Priority:    si.priority,
			Type:        si.issueType,
			Assignee:    si.assignee,
			Creator:     si.creator,
			Labels:      labels,
			Description: si.description,
			Notes:       si.notes,
			DependsOn:   deps,
			Related:     related,
			ParentID:    si.parentID,
			Comments:    comments,
			Created:     si.created,
			Updated:     si.updated,
			Closed:      si.closed,
			CloseReason: si.closeReason,
		})
	}
	return out
}

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
