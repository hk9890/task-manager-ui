package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contract"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// TestFakeGatewayReadContract wires RunReadContract against the fake gateway.
// The fake is pre-seeded from seed.json so it "contains" the same data the real
// bd fixture would return — keeping the two in sync via a single source of truth.
func TestFakeGatewayReadContract(t *testing.T) {
	contract.RunReadContract(t, func(t *testing.T) beads.BeadsGateway {
		t.Helper()

		fake := fakes.NewFakeBeadsGateway()
		primeFakeFromFixtureSpec(t, fake)
		return fake
	})
}

// primeFakeFromFixtureSpec reads seed.json and populates the fake's response
// fields so the fake contains the same data as the real embedded fixture.
func primeFakeFromFixtureSpec(t *testing.T, fake *fakes.FakeBeadsGateway) {
	t.Helper()

	spec := embeddedfixture.ReadSeedSpec(t)

	// Build IssueSummary slice from the fixture spec.
	// Labels is set to nil (not an empty slice) when the issue has no labels,
	// mirroring the real bd behaviour and the postcondition in interface.go.
	summaries := make([]domain.IssueSummary, 0, len(spec.Issues))
	for _, issue := range spec.Issues {
		var labels []string
		if len(issue.Labels) > 0 {
			labels = issue.Labels
		}
		summaries = append(summaries, domain.IssueSummary{
			ID:       issue.ID,
			Title:    issue.Title,
			Type:     issue.Type,
			Priority: issue.Priority,
			Status:   issue.Status,
			Assignee: issue.Assignee,
			Labels:   labels,
		})
	}

	// ListIssues (no filter): bd list --json excludes closed issues by default.
	// Mirror that by seeding only non-closed summaries, sorted by priority
	// descending to satisfy the SortApplied invariant (the fake does not sort
	// dynamically, so we pre-sort the response to match what bd returns).
	for _, s := range summaries {
		if s.Status != "closed" {
			fake.ListIssuesResponse = append(fake.ListIssuesResponse, s)
		}
	}
	slices.SortStableFunc(fake.ListIssuesResponse, func(a, b domain.IssueSummary) int {
		// Higher priority first (descending).
		return b.Priority - a.Priority
	})

	// Query: use QueryResponsesByExpr so each expression returns a correctly
	// filtered slice. This matches real bd's expression-filter behaviour and
	// allows Invariants/Query/StatusFilterRespected to verify that Query
	// results actually have the status the expression selected.
	//
	// Note: the contract tests also call Query(ctx, "status = open", ...) and
	// Query(ctx, "status=open", ...) — both key forms are seeded to be safe.
	var openSummaries, closedSummaries []domain.IssueSummary
	for _, s := range summaries {
		switch s.Status {
		case "open":
			openSummaries = append(openSummaries, s)
		case "closed":
			closedSummaries = append(closedSummaries, s)
		}
	}
	// openSummaries includes bwf-1 and bwf-4 (both status=open).
	fake.QueryResponsesByExpr = map[string][]domain.IssueSummary{
		"status = open":   openSummaries,
		"status=open":     openSummaries,
		"status = closed": closedSummaries,
		"status=closed":   closedSummaries,
	}
	// QueryResponse kept as a verbatim fallback for UI tests that set it directly
	// and don't use QueryResponsesByExpr.
	for _, s := range openSummaries {
		fake.QueryResponse = append(fake.QueryResponse, s)
	}

	// Ready issues: issues with no open blockers (open and not blocked).
	for _, s := range summaries {
		if s.Status == "open" {
			fake.ReadyIssuesResponse = append(fake.ReadyIssuesResponse, s)
		}
	}

	// Blocked issues: blocked-status issues with blocker IDs populated.
	// Build a map from blocked_id → []blocker_ids so we can attach them.
	blockers := make(map[string][]domain.IssueReference)
	for _, dep := range spec.Deps {
		blockers[dep.BlockedID] = append(blockers[dep.BlockedID], domain.IssueReference{ID: dep.BlockerID})
	}
	for _, s := range summaries {
		if s.Status == "blocked" {
			// Only include issues that have actual dependency blockers.
			// bd blocked excludes issues whose status is merely stored as "blocked"
			// with no unresolved dep chain (interface.go postcondition).
			if refs, ok := blockers[s.ID]; ok {
				fake.BlockedIssuesResponse = append(fake.BlockedIssuesResponse, domain.BlockedIssueView{
					Issue:     s,
					BlockedBy: refs,
				})
			}
		}
	}

	// ReadyExplain combines ready + blocked with the same blocker references.
	// The --explain form returns rich blocker objects; mirror that here.
	explainBlocked := make([]domain.BlockedIssueView, 0, len(fake.BlockedIssuesResponse))
	for _, bv := range fake.BlockedIssuesResponse {
		// For each blocker ID, resolve it into a fuller IssueReference from summaries.
		richBlockers := make([]domain.IssueReference, 0, len(bv.BlockedBy))
		for _, ref := range bv.BlockedBy {
			for _, s := range summaries {
				if s.ID == ref.ID {
					richBlockers = append(richBlockers, domain.IssueReference{
						ID:       s.ID,
						Title:    s.Title,
						Type:     s.Type,
						Priority: s.Priority,
						Status:   s.Status,
					})
					break
				}
			}
		}
		explainBlocked = append(explainBlocked, domain.BlockedIssueView{
			Issue:     bv.Issue,
			BlockedBy: richBlockers,
		})
	}
	fake.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:        fake.ReadyIssuesResponse,
		Blocked:      explainBlocked,
		TotalReady:   len(fake.ReadyIssuesResponse),
		TotalBlocked: len(explainBlocked),
		CycleCount:   0,
	}

	// ShowIssue: populate an ID-keyed map so lookup by ID is precise and
	// unknown IDs return ErrorCodeNotFound (matching real bd behaviour).
	fake.ShowIssuesByID = make(map[string]domain.IssueDetail)
	for _, issue := range spec.Issues {
		var showLabels []string
		if len(issue.Labels) > 0 {
			showLabels = issue.Labels
		}
		summary := domain.IssueSummary{
			ID:       issue.ID,
			Title:    issue.Title,
			Type:     issue.Type,
			Priority: issue.Priority,
			Status:   issue.Status,
			Assignee: issue.Assignee,
			Labels:   showLabels,
		}
		detail := domain.IssueDetail{
			Summary:     summary,
			Description: issue.Description,
			// Real bd emits empty arrays (not null) for Comments and BlockedBy
			// when there are none. Initialize to empty slices to match that
			// invariant. Without this, Invariants/ShowIssue/CommentsNotNil and
			// Invariants/ShowIssue/BlockedByNotNil fail.
			Comments:  []domain.IssueComment{},
			BlockedBy: []domain.IssueReference{},
		}
		// Attach BlockedBy references for blocked issues (overrides empty default).
		if refs, ok := blockers[issue.ID]; ok {
			detail.BlockedBy = refs
		}
		fake.ShowIssuesByID[issue.ID] = detail
	}

	// SearchIssues: use SearchResultsByText opt-in map so text filtering is precise
	// without affecting UI tests that use SearchIssuesResponse verbatim.
	//
	// "" (empty query) → bd list --all → all 5 fixture issues.
	// "root"           → bd search root → only bwf-1 (title contains "root").
	allSearchResults := make([]domain.SearchResult, 0, len(summaries))
	for _, s := range summaries {
		allSearchResults = append(allSearchResults, domain.SearchResult{Issue: s})
	}

	rootResults := make([]domain.SearchResult, 0)
	for _, s := range summaries {
		if strings.Contains(strings.ToLower(s.Title), "root") {
			rootResults = append(rootResults, domain.SearchResult{Issue: s})
		}
	}

	fake.SearchResultsByText = map[string]domain.SearchResultPage{
		"": {
			Results:  allSearchResults,
			Metadata: domain.SearchResultMetadata{ReturnedCount: len(allSearchResults)},
		},
		"root": {
			Results:  rootResults,
			Metadata: domain.SearchResultMetadata{ReturnedCount: len(rootResults)},
		},
	}

	// CountIssues: derive group counts from spec.
	statusCounts := make(map[string]int)
	for _, issue := range spec.Issues {
		statusCounts[issue.Status]++
	}
	groups := make([]domain.IssueStatusCount, 0, len(statusCounts))
	for status, count := range statusCounts {
		groups = append(groups, domain.IssueStatusCount{Status: status, Count: count})
	}
	fake.CountIssuesResponse = domain.IssueCountResult{
		Groups: groups,
		Total:  len(spec.Issues),
	}

	// StatusCatalog: seed all 7 bd 1.0.4 built-in statuses.
	fake.StatusCatalogResponse = []domain.StatusOption{
		{Name: "open", Description: "Available to work (default)"},
		{Name: "in_progress", Description: "Actively being worked on"},
		{Name: "blocked", Description: "Blocked by a dependency"},
		{Name: "deferred", Description: "Deliberately put on ice for later"},
		{Name: "closed", Description: "Completed"},
		{Name: "pinned", Description: "Pinned for visibility"},
		{Name: "hooked", Description: "Hooked — waiting on an external trigger"},
	}

	// TypeCatalog: seed all 9 bd core types.
	fake.TypeCatalogResponse = []domain.TypeOption{
		{Name: "task", Description: "General work item (default)"},
		{Name: "bug", Description: "Bug report or defect"},
		{Name: "feature", Description: "New feature or enhancement"},
		{Name: "chore", Description: "Maintenance or housekeeping"},
		{Name: "epic", Description: "Large body of work spanning multiple issues"},
		{Name: "decision", Description: "Architectural or design decision"},
		{Name: "spike", Description: "Time-boxed research or investigation"},
		{Name: "story", Description: "User story"},
		{Name: "milestone", Description: "Project milestone"},
	}

	// LabelCatalog: seed the labels present in the fixture issues.
	labelSet := make(map[string]struct{})
	for _, issue := range spec.Issues {
		for _, label := range issue.Labels {
			labelSet[label] = struct{}{}
		}
	}
	labelOpts := make([]domain.LabelOption, 0, len(labelSet))
	for label := range labelSet {
		labelOpts = append(labelOpts, domain.LabelOption{Name: label})
	}
	fake.LabelCatalogResponse = labelOpts
}
