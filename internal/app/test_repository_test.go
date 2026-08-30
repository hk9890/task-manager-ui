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
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// seedSearchResult seeds an issue so it will be found by the memory repo's
// text-matching search. The issue must have the search term in title,
// description, or notes.
func seedSearchResult(g *fakes.TrackedRepository, iss memoryrepo.Issue) {
	g.Memory.Seed(iss)
}

// seedIssueSummary seeds an issue from a domain.IssueSummary into the memory
// repository. Use this to translate old FakeRepo response-field patterns.
func seedIssueSummary(g *fakes.TrackedRepository, s domain.IssueSummary) {
	g.Memory.Seed(memoryrepo.Issue{
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
// IDs, and ParentGroupBrowser → ParentID so memory repo's
// toDetailLocked can project them back correctly.
func seedIssueDetail(g *fakes.TrackedRepository, d domain.IssueDetail) {
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

	g.Memory.Seed(memoryrepo.Issue{
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
	})
}

// seedCatalogs seeds catalog data into the repository.
func seedCatalogs(g *fakes.TrackedRepository, statuses []domain.StatusOption, types []domain.TypeOption, labels []domain.LabelOption) {
	g.Memory.SeedCatalogs(repository.Catalogs{
		Statuses: statuses,
		Types:    types,
		Labels:   labels,
	})
}

// issueState fetches the current state of an issue from the memory repo.
// Returns nil if not found.
func issueState(g *fakes.TrackedRepository, id string) *domain.IssueDetail {
	d, err := g.Memory.Issue(context.Background(), id)
	if err != nil {
		return nil
	}
	return &d
}

// seedDepBlocked seeds id as dep-blocked by a fresh open blocker so that it
// appears in the NotReady (dep-blocked) column of the board. Use this for issues
// that the old FakeRepo placed in ReadyExplainResponse.Blocked.
func seedDepBlocked(g *fakes.TrackedRepository, id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	blockerID := id + "-blocker"
	// Status "deferred": not "closed" so depStateLocked treats it as an open dep,
	// but not "open" so the blocker itself won't appear in the Ready lane.
	g.Memory.Seed(memoryrepo.Issue{ID: blockerID, Title: "blocker for " + id, Status: "deferred"})
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "open", Type: issueType, Priority: priority, DependsOn: []string{blockerID}}
	for _, fn := range extra {
		fn(&iss)
	}
	g.Memory.Seed(iss)
}

// seedReady seeds an open issue with no deps so it appears in the Ready column.
func seedReady(g *fakes.TrackedRepository, id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "open", Type: issueType, Priority: priority}
	for _, fn := range extra {
		fn(&iss)
	}
	g.Memory.Seed(iss)
}

// seedInProgress seeds an in-progress issue (appears in InProgress column).
func seedInProgress(g *fakes.TrackedRepository, id, title string, issueType string, priority int, extra ...func(*memoryrepo.Issue)) {
	iss := memoryrepo.Issue{ID: id, Title: title, Status: "in_progress", Type: issueType, Priority: priority}
	for _, fn := range extra {
		fn(&iss)
	}
	g.Memory.Seed(iss)
}

// mustNewModel wraps NewModel and fails the test if an error is returned.
// It pre-sets sizeKnown=true and installs no-op scheduler functions so that
// tests run without real time-based ticks and without any global shared state.
// Tests that specifically validate the sizeKnown=false/empty-view behaviour
// should call NewModelWithOptions directly and leave sizeKnown at its zero value.
func mustNewModel(t *testing.T, services Services) Model {
	t.Helper()
	m, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel returned unexpected error: %v", err)
	}
	m.sizeKnown = true
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }
	return m
}

// mustNewModelWithOptions wraps NewModelWithOptions and fails the test if an error is returned.
// It pre-sets sizeKnown=true and installs no-op scheduler functions (same as
// mustNewModel). Tests that specifically validate the sizeKnown=false/empty-view
// behaviour should call NewModelWithOptions directly and leave sizeKnown at its
// zero value.
func mustNewModelWithOptions(t *testing.T, services Services, runtime RuntimeOptions) Model {
	t.Helper()
	m, err := NewModelWithOptions(services, runtime)
	if err != nil {
		t.Fatalf("NewModelWithOptions returned unexpected error: %v", err)
	}
	m.sizeKnown = true
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }
	return m
}

// runBatch runs cmd to completion, flattening nested tea.BatchMsg values. It is
// the shared testui.DrainCmd under the name 230 call sites in this package
// already use; the flattening loop lived here in a second copy.
func runBatch(cmd tea.Cmd) []tea.Msg {
	return testui.DrainCmd(cmd)
}

func applyMessages(t *testing.T, model Model, msgs []tea.Msg) Model {
	t.Helper()

	m := model
	queue := append([]tea.Msg(nil), msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		next, cmd := m.Update(msg)
		m = next.(Model)
		queue = append(queue, runBatch(cmd)...)
	}

	return m
}

func firstSelectionID(m Model, modeID mode.ID) string {
	sel := m.selectedByMode[modeID]
	if sel == nil {
		return ""
	}
	return sel.Issue.ID
}

func browserIDs(refs []domain.IssueReference) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func withModelNow(t *testing.T, now time.Time) {
	t.Helper()
	original := modelNow
	modelNow = func() time.Time { return now }
	t.Cleanup(func() {
		modelNow = original
	})
}
