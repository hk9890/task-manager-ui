package app

// test_repository_test.go provides a thin test helper for the app package's test
// suite. It bundles a memory repository (for seeding) with an error-injecting
// wrapper (for call tracking and error injection), making it a drop-in
// replacement for the old fakes.FakeRepo pattern.
//
// It is intentionally minimal — only the helpers needed by this package are
// included here. No new fakes or shims are introduced.

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
)

// appTestRepository bundles a memory repository (for seeding) and an
// ErrorInjectingRepository (for call tracking). It satisfies
// repository.Repository via the embedded *ErrorInjectingRepository.
type appTestRepository struct {
	repo *memoryrepo.Repository
	*repository.ErrorInjectingRepository
}

// newTestRepository creates an appTestRepository with an empty memory repository.
func newTestRepository() *appTestRepository {
	repo := memoryrepo.New()
	return &appTestRepository{
		repo:                     repo,
		ErrorInjectingRepository: repository.NewErrorInjecting(repo),
	}
}

// hasCall reports whether the given method appears in the recorded calls.
func (g *appTestRepository) hasCall(m repository.Method) bool {
	for _, c := range g.ErrorInjectingRepository.Calls() {
		if c.Method == m {
			return true
		}
	}
	return false
}

// callCountSince returns the number of calls to method m recorded after
// index start (inclusive). Use len(g.Calls()) as the start marker before an
// action to measure only the calls produced by that action.
func (g *appTestRepository) callCountSince(start int, m repository.Method) int {
	all := g.ErrorInjectingRepository.Calls()
	n := 0
	for i := start; i < len(all); i++ {
		if all[i].Method == m {
			n++
		}
	}
	return n
}

// hasDashboardCall reports whether a Dashboard call appears in the recorded calls.
func (g *appTestRepository) hasDashboardCall() bool { return g.hasCall(repository.MethodDashboard) }

// hasIssueCall reports whether an Issue (detail fetch) call appears.
func (g *appTestRepository) hasIssueCall() bool { return g.hasCall(repository.MethodIssue) }

// hasSearchCall reports whether a Search call appears.
func (g *appTestRepository) hasSearchCall() bool { return g.hasCall(repository.MethodSearch) }

// hasUpdateIssueCall reports whether an UpdateIssue call appears.
func (g *appTestRepository) hasUpdateIssueCall() bool { return g.hasCall(repository.MethodUpdateIssue) }

// hasCatalogsCall reports whether a Catalogs call appears.
func (g *appTestRepository) hasCatalogsCall() bool { return g.hasCall(repository.MethodCatalogs) }

// hasCreateIssueCall reports whether a CreateIssue call appears.
func (g *appTestRepository) hasCreateIssueCall() bool { return g.hasCall(repository.MethodCreateIssue) }

// hasCloseIssueCall reports whether a CloseIssue call appears.
func (g *appTestRepository) hasCloseIssueCall() bool { return g.hasCall(repository.MethodCloseIssue) }

// hasAddCommentCall reports whether an AddComment call appears.
func (g *appTestRepository) hasAddCommentCall() bool { return g.hasCall(repository.MethodAddComment) }

// hasHealthCheckCall reports whether a HealthCheck call appears.
func (g *appTestRepository) hasHealthCheckCall() bool { return g.hasCall(repository.MethodHealthCheck) }

// seedSearchResult seeds an issue so it will be found by the memory repo's
// text-matching search. The issue must have the search term in title,
// description, or notes.
func (g *appTestRepository) seedSearchResult(iss memoryrepo.Issue) {
	g.repo.Seed(iss)
}

// callCount returns the total number of recorded calls.
func (g *appTestRepository) callCount() int { return len(g.ErrorInjectingRepository.Calls()) }

// resetMark returns the current call count as a "reset mark" for measuring
// subsequent calls via callCountSince/hasCallSince.
func (g *appTestRepository) resetMark() int { return len(g.ErrorInjectingRepository.Calls()) }

// hasCallSince reports whether method m was called after the given mark.
func (g *appTestRepository) hasCallSince(mark int, m repository.Method) bool {
	return g.callCountSince(mark, m) > 0
}

// seedIssueSummary seeds an issue from a domain.IssueSummary into the memory
// repository. Use this to translate old FakeRepo response-field patterns.
func (g *appTestRepository) seedIssueSummary(s domain.IssueSummary) {
	g.repo.Seed(memoryrepo.Issue{
		ID:       s.ID,
		Title:    s.Title,
		Status:   s.Status,
		Priority: s.Priority,
		Type:     s.Type,
		Assignee: s.Assignee,
		Labels:   s.Labels,
	})
}

// seedIssueDetail seeds a full issue detail into the memory repository.
// It propagates BlockedBy → DependsOn, Blocks → BlocksIDs, Related → Related
// IDs, and ParentGroupBrowser → ParentID/ChildrenIDs so memory repo's
// toDetailLocked can project them back correctly.
func (g *appTestRepository) seedIssueDetail(d domain.IssueDetail) {
	dependsOn := make([]string, 0, len(d.BlockedBy))
	for _, ref := range d.BlockedBy {
		if ref.ID != "" {
			dependsOn = append(dependsOn, ref.ID)
		}
	}

	blocksIDs := make([]string, 0, len(d.Blocks))
	for _, ref := range d.Blocks {
		if ref.ID != "" {
			blocksIDs = append(blocksIDs, ref.ID)
		}
	}

	related := make([]string, 0, len(d.Related))
	for _, ref := range d.Related {
		if ref.ID != "" {
			related = append(related, ref.ID)
		}
	}

	childrenIDs := make([]string, 0, len(d.ParentGroupBrowser.Children))
	for _, ref := range d.ParentGroupBrowser.Children {
		if ref.ID != "" {
			childrenIDs = append(childrenIDs, ref.ID)
		}
	}

	g.repo.Seed(memoryrepo.Issue{
		ID:          d.Summary.ID,
		Title:       d.Summary.Title,
		Status:      d.Summary.Status,
		Priority:    d.Summary.Priority,
		Type:        d.Summary.Type,
		Assignee:    d.Summary.Assignee,
		Labels:      d.Summary.Labels,
		Description: d.Description,
		Notes:       d.Notes,
		DependsOn:   dependsOn,
		BlocksIDs:   blocksIDs,
		Related:     related,
		ParentID:    d.ParentGroupBrowser.Parent.ID,
		ChildrenIDs: childrenIDs,
	})
}

// seedCatalogs seeds catalog data into the repository.
func (g *appTestRepository) seedCatalogs(statuses []domain.StatusOption, types []domain.TypeOption, labels []domain.LabelOption) {
	g.repo.SeedCatalogs(repository.Catalogs{
		Statuses: statuses,
		Types:    types,
		Labels:   labels,
	})
}

// issueState fetches the current state of an issue from the memory repo.
// Returns nil if not found.
func (g *appTestRepository) issueState(id string) *domain.IssueDetail {
	d, err := g.repo.Issue(context.Background(), id)
	if err != nil {
		return nil
	}
	return &d
}

// seedDepBlocked seeds id as dep-blocked by a fresh open blocker so that it
// appears in the NotReady (dep-blocked) column of the board. Use this for issues
// that the old FakeRepo placed in ReadyExplainResponse.Blocked.
func (g *appTestRepository) seedDepBlocked(id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	blockerID := id + "-blocker"
	// Status "deferred": not "closed" so depStateLocked treats it as an open dep,
	// but not "open" so the blocker itself won't appear in the Ready lane.
	g.repo.Seed(memoryrepo.Issue{ID: blockerID, Title: "blocker for " + id, Status: "deferred"})
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "open", Type: issueType, Priority: priority, DependsOn: []string{blockerID}}
	for _, fn := range extra {
		fn(&iss)
	}
	g.repo.Seed(iss)
}

// seedReady seeds an open issue with no deps so it appears in the Ready column.
func (g *appTestRepository) seedReady(id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "open", Type: issueType, Priority: priority}
	for _, fn := range extra {
		fn(&iss)
	}
	g.repo.Seed(iss)
}

// seedInProgress seeds an in-progress issue (appears in InProgress column).
func (g *appTestRepository) seedInProgress(id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "in_progress", Type: issueType, Priority: priority}
	for _, fn := range extra {
		fn(&iss)
	}
	g.repo.Seed(iss)
}
