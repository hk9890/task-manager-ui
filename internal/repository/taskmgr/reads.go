package taskmgr

import (
	"context"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Dashboard composes the board snapshot from two store reads: the graph of the
// hot set, which gives the ready, blocked, in-progress and not-ready sections
// one consistent view, and the closed page. It is fail-fast: the first error
// aborts the whole call. A close or reopen between the two reads can show an
// issue in both its section and Done, or in neither, until the next refresh.
func (r *Repository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	if err := ctx.Err(); err != nil {
		return repository.DashboardData{}, err
	}

	nodes, err := r.store.Graph()
	if err != nil {
		return repository.DashboardData{}, mapReadErr("dashboard", err)
	}
	var (
		ready, inProgress, notReady []*tasks.Issue
		blocked                     []tasks.BlockedIssue
	)
	for _, n := range nodes {
		if n.Ready {
			ready = append(ready, n.Issue)
		}
		if n.Blocked {
			blocked = append(blocked, tasks.BlockedIssue{Issue: n.Issue, BlockedBy: n.BlockedBy})
		}
		switch n.Issue.Status {
		case tasks.StatusInProgress:
			inProgress = append(inProgress, n.Issue)
		// Feeds the board's Not Ready column (the dashboard composer merges this
		// with the dep-blocked set, deduped by ID). "deferred" is an active,
		// non-closed status (work consciously postponed) that is neither ready
		// nor in-progress, so it joins blocked-status issues here to stay
		// visible on the board.
		case tasks.StatusBlocked, tasks.StatusDeferred:
			notReady = append(notReady, n.Issue)
		}
	}
	// ClosedLimit <= 0 means "all remaining" (Filter.Limit 0 = no limit), matching
	// the memory backend; ClosedOffset pages the closed window. Total is the full
	// closed count regardless of the window. SortClosed is already ClosedAt DESC
	// (newest first), so no Reverse is applied — the window is "most recently
	// closed first", matching the documented contract.
	closedPage, err := r.store.ListPage(tasks.Filter{
		Expr:          `status == "closed"`,
		IncludeClosed: true,
		Sort:          tasks.SortClosed,
		Offset:        opts.ClosedOffset,
		Limit:         opts.ClosedLimit,
	})
	if err != nil {
		return repository.DashboardData{}, mapReadErr("dashboard", err)
	}

	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready:        toSummaries(ready),
			Blocked:      toBlockedViews(blocked),
			TotalReady:   len(ready),
			TotalBlocked: len(blocked),
		},
		InProgress:  toSummaries(inProgress),
		Closed:      toSummaries(closedPage.Issues),
		ClosedTotal: closedPage.Total,
		Blocked:     toSummaries(notReady),
	}, nil
}

// Issue returns the full detail model. An unknown ID yields
// repository.ErrIssueNotFound (local-state carve-out).
func (r *Repository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	if err := ctx.Err(); err != nil {
		return domain.IssueDetail{}, err
	}
	d, err := r.store.Detail(id)
	if err != nil {
		return domain.IssueDetail{}, mapIssueErr("issue", err)
	}
	return toDetail(d), nil
}

// Catalogs returns the fixed status/type enums plus the labels in use.
func (r *Repository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	if err := ctx.Err(); err != nil {
		return repository.Catalogs{}, err
	}
	labels, err := r.store.Labels()
	if err != nil {
		return repository.Catalogs{}, mapReadErr("catalogs", err)
	}
	return staticCatalogs(labels), nil
}

// HealthCheck confirms the store is reachable.
func (r *Repository) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.store.Labels(); err != nil {
		return mapReadErr("health check", err)
	}
	return nil
}
