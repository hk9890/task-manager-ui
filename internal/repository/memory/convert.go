package memory

import (
	"sort"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// toSummaryLocked projects a storedIssue to domain.IssueSummary.
// Caller must hold at least RLock.
func (r *Repository) toSummaryLocked(si *storedIssue) domain.IssueSummary {
	labels := make([]string, len(si.labels))
	copy(labels, si.labels)
	return domain.IssueSummary{
		ID:        si.id,
		Title:     si.title,
		Status:    si.status,
		Type:      si.issueType,
		Priority:  si.priority,
		Assignee:  si.assignee,
		Labels:    labels,
		CreatedAt: si.created,
		UpdatedAt: si.updated,
	}
}

// toDetailLocked projects a storedIssue to domain.IssueDetail with resolved
// dep references. Caller must hold at least RLock.
func (r *Repository) toDetailLocked(si *storedIssue) domain.IssueDetail {
	sum := r.toSummaryLocked(si)

	// Resolve DependsOn as BlockedBy references from the in-memory map.
	blockedBy := make([]domain.IssueReference, 0, len(si.dependsOn))
	for _, depID := range si.dependsOn {
		dep, ok := r.issues[depID]
		if !ok {
			blockedBy = append(blockedBy, domain.IssueReference{ID: depID})
			continue
		}
		blockedBy = append(blockedBy, domain.IssueReference{
			ID:       dep.id,
			Title:    dep.title,
			Type:     dep.issueType,
			Priority: dep.priority,
			Status:   dep.status,
		})
	}

	// Resolve Blocks: if blocksIDs is explicitly set, use it; otherwise fall
	// back to reverse-lookup (find issues whose dependsOn contains si.id).
	blocks := make([]domain.IssueReference, 0)
	if len(si.blocksIDs) > 0 {
		for _, blockedID := range si.blocksIDs {
			other, ok := r.issues[blockedID]
			if !ok {
				blocks = append(blocks, domain.IssueReference{ID: blockedID})
				continue
			}
			blocks = append(blocks, domain.IssueReference{
				ID:       other.id,
				Title:    other.title,
				Type:     other.issueType,
				Priority: other.priority,
				Status:   other.status,
			})
		}
	} else {
		for _, other := range r.issues {
			if other.id == si.id {
				continue
			}
			for _, depID := range other.dependsOn {
				if depID == si.id {
					blocks = append(blocks, domain.IssueReference{
						ID:       other.id,
						Title:    other.title,
						Type:     other.issueType,
						Priority: other.priority,
						Status:   other.status,
					})
					break
				}
			}
		}
	}

	// Resolve Children: issues for which this issue is the parent (reverse
	// parentID lookup), mirroring how the task-manager backend derives the Children
	// group from parent-child dependents. Without this the in-memory fake would
	// diverge from real taskmgr, which always returns the Children group.
	childrenGroup := make([]domain.IssueReference, 0)
	for _, other := range r.issues {
		if other.id == si.id || other.parentID != si.id {
			continue
		}
		childrenGroup = append(childrenGroup, domain.IssueReference{
			ID:       other.id,
			Title:    other.title,
			Type:     other.issueType,
			Priority: other.priority,
			Status:   other.status,
		})
	}
	sort.Slice(childrenGroup, func(i, j int) bool { return childrenGroup[i].ID < childrenGroup[j].ID })

	// Project comments.
	comments := make([]domain.IssueComment, len(si.comments))
	for i, c := range si.comments {
		comments[i] = domain.IssueComment{
			ID:        c.id,
			Author:    c.author,
			Body:      c.body,
			CreatedAt: c.createdAt,
		}
	}

	// Resolve Related references from the in-memory map.
	related := make([]domain.IssueReference, 0, len(si.related))
	for _, relID := range si.related {
		rel, ok := r.issues[relID]
		if !ok {
			related = append(related, domain.IssueReference{ID: relID})
			continue
		}
		related = append(related, domain.IssueReference{
			ID:       rel.id,
			Title:    rel.title,
			Type:     rel.issueType,
			Priority: rel.priority,
			Status:   rel.status,
		})
	}

	// Resolve ParentGroupBrowserContext from the in-memory map.
	var parentGroupBrowser domain.ParentGroupBrowserContext
	if si.parentID != "" {
		parent, ok := r.issues[si.parentID]
		if ok {
			parentGroupBrowser.Parent = domain.IssueReference{
				ID:       parent.id,
				Title:    parent.title,
				Type:     parent.issueType,
				Priority: parent.priority,
				Status:   parent.status,
			}
		} else {
			parentGroupBrowser.Parent = domain.IssueReference{ID: si.parentID}
		}
	}
	return domain.IssueDetail{
		Summary:            sum,
		Creator:            si.creator,
		Description:        si.description,
		Notes:              si.notes,
		ClosedAt:           si.closed,
		CloseReason:        si.closeReason,
		BlockedBy:          blockedBy,
		Blocks:             blocks,
		Children:           childrenGroup,
		Comments:           comments,
		Related:            related,
		ParentGroupBrowser: parentGroupBrowser,
	}
}

// depStateLocked checks whether all dependency IDs point to closed issues.
// Returns (true, nil) when all are closed; (false, openDeps) listing IDs of
// non-closed or unknown deps. Caller must hold at least RLock.
func (r *Repository) depStateLocked(dependsOn []string) (allClosed bool, openDeps []string) {
	if len(dependsOn) == 0 {
		return true, nil
	}

	for _, depID := range dependsOn {
		dep, ok := r.issues[depID]
		if !ok || dep.status != "closed" {
			openDeps = append(openDeps, depID)
		}
	}

	return len(openDeps) == 0, openDeps
}
