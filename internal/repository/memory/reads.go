package memory

import (
	"context"
	"sort"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Dashboard implements repository.Repository.
//
// ReadyExplain.Ready: issues where status is not "closed" and all DependsOn
// IDs point to closed issues (or DependsOn is empty).
// ReadyExplain.Blocked: issues where status is not "closed" and at least one
// DependsOn ID points to a non-closed issue.
// DashboardData.Blocked: the Not Ready feed — issues where stored status ==
// "blocked" OR "deferred" (matching the taskmgr backend's Not-Ready query).
// DashboardData.InProgress: issues where status == "in_progress".
// DashboardData.Closed: closed issues sorted by ClosedAt DESC, windowed by
// opts.ClosedOffset (skip) then opts.ClosedLimit (cap). When opts.ClosedOffset
// >= total closed count, an empty slice is returned (no error). When
// opts.ClosedLimit <= 0, all remaining issues after the offset are returned.
// DashboardData.ClosedTotal: always the full count of closed issues, independent
// of opts.ClosedOffset and opts.ClosedLimit.
func (r *Repository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	if err := ctx.Err(); err != nil {
		return repository.DashboardData{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var ready []domain.IssueSummary
	var blockedExplain []domain.BlockedIssueView
	var inProgress []domain.IssueSummary
	var closed []domain.IssueSummary
	var blocked []domain.IssueSummary

	for _, si := range r.issues {
		sum := r.toSummaryLocked(si)

		switch si.status {
		case "closed":
			closed = append(closed, sum)
		case "in_progress":
			inProgress = append(inProgress, sum)
		case "blocked", "deferred":
			// The Blocked slot feeds the board's Not Ready column. "deferred" is an
			// active, non-closed status (consciously postponed) that is neither
			// ready nor in-progress, so — matching the taskmgr backend's
			// `status == "blocked" || status == "deferred"` Not-Ready query — it
			// joins blocked-status issues here. Without this a deferred issue with
			// no open dependency blocker matches no column and vanishes from the board.
			blocked = append(blocked, sum)
		}

		// ReadyExplain logic mirrors taskmgr ready --explain semantics:
		// - Ready: stored status == "open" AND all dep-IDs are closed (or no deps).
		// - Blocked: has at least one open dep, AND status is not "closed" (any
		//   non-closed stored status can be dep-blocked, matching taskmgr's behaviour
		//   with bwf-2 status=blocked/has-dep → appears in Blocked).
		if si.status == "closed" {
			continue
		}

		allDepsClosed, openDeps := r.depStateLocked(si.dependsOn)

		if allDepsClosed && si.status == "open" {
			ready = append(ready, sum)
		} else if !allDepsClosed {
			// Build blocker references from open deps.
			blockerRefs := make([]domain.IssueReference, 0, len(openDeps))
			for _, depID := range openDeps {
				dep, ok := r.issues[depID]
				if !ok {
					// Dep not in store — treat as a forward reference, include by ID.
					blockerRefs = append(blockerRefs, domain.IssueReference{ID: depID})
					continue
				}
				blockerRefs = append(blockerRefs, domain.IssueReference{
					ID:       dep.id,
					Title:    dep.title,
					Type:     dep.issueType,
					Priority: dep.priority,
					Status:   dep.status,
				})
			}
			blockedExplain = append(blockedExplain, domain.BlockedIssueView{
				Issue:     sum,
				BlockedBy: blockerRefs,
			})
		}
	}

	// Sort closed DESC by ClosedAt with a deterministic ID tiebreak. The map
	// iteration above is randomized, so a comparator that only orders by ClosedAt
	// leaves equal-timestamp issues in an unspecified relative order that differs
	// between calls; the first-page and load-more (ClosedOffset) windows would then
	// come from two independent orderings and a boundary issue could be skipped or
	// duplicated. The ID tiebreak makes the order a total, stable function of the
	// data so paging is consistent across calls. Zero-closed-time issues sort last.
	sort.Slice(closed, func(i, j int) bool {
		ti := r.issues[closed[i].ID].closed
		tj := r.issues[closed[j].ID].closed
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return closed[i].ID < closed[j].ID
	})

	re := domain.ReadyExplainResult{
		Ready:        ready,
		Blocked:      blockedExplain,
		TotalReady:   len(ready),
		TotalBlocked: len(blockedExplain),
	}

	if re.Ready == nil {
		re.Ready = []domain.IssueSummary{}
	}
	if re.Blocked == nil {
		re.Blocked = []domain.BlockedIssueView{}
	}

	if inProgress == nil {
		inProgress = []domain.IssueSummary{}
	}
	if closed == nil {
		closed = []domain.IssueSummary{}
	}
	if blocked == nil {
		blocked = []domain.IssueSummary{}
	}

	// ClosedTotal must be computed from the full slice BEFORE any windowing is applied.
	closedTotal := len(closed)

	// Apply opts.ClosedOffset: slice from offset before applying limit.
	if opts.ClosedOffset >= len(closed) {
		closed = closed[:0] // beyond end → empty page, no error
	} else {
		closed = closed[opts.ClosedOffset:]
	}

	// Apply opts.ClosedLimit when positive. When <= 0, all closed issues are returned.
	if opts.ClosedLimit > 0 && opts.ClosedLimit < len(closed) {
		closed = closed[:opts.ClosedLimit]
	}

	return repository.DashboardData{
		ReadyExplain: re,
		InProgress:   inProgress,
		Closed:       closed,
		ClosedTotal:  closedTotal,
		Blocked:      blocked,
	}, nil
}

// Issue implements repository.Repository.
//
// Returns repository.ErrIssueNotFound for unknown IDs (local-state carve-out
// as documented in the Repository interface). The domain.RepositoryError path is
// reserved for taskmgr-backed implementations.
func (r *Repository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	if err := ctx.Err(); err != nil {
		return domain.IssueDetail{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	si, ok := r.issues[id]
	if !ok {
		return domain.IssueDetail{}, repository.ErrIssueNotFound
	}

	return r.toDetailLocked(si), nil
}

// HealthCheck implements repository.Repository.
//
// Always returns nil — in-memory stores are always healthy.
func (r *Repository) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Catalogs implements repository.Repository.
func (r *Repository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	if err := ctx.Err(); err != nil {
		return repository.Catalogs{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.catalogs, nil
}
