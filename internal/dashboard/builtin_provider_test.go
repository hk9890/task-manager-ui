package dashboard

import (
	"context"
	"reflect"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestBuiltInProviderDashboardsWithoutActor(t *testing.T) {
	t.Parallel()

	provider := NewBuiltInProvider()

	dashboards, err := provider.Dashboards(context.Background())
	if err != nil {
		t.Fatalf("Dashboards returned error: %v", err)
	}

	if len(dashboards) != 1 {
		t.Fatalf("expected 1 dashboard, got %d", len(dashboards))
	}

	dashboard := dashboards[0]
	if dashboard.ID != builtInDashboardIDDefault || dashboard.Title != builtInDashboardTitleDefault {
		t.Fatalf("unexpected dashboard metadata: %#v", dashboard)
	}

	if len(dashboard.Sections) != 4 {
		t.Fatalf("expected 4 sections when actor unavailable, got %d", len(dashboard.Sections))
	}

	assertSectionIDs(t, dashboard.Sections, []string{
		builtInSectionIDNotReady,
		builtInSectionIDReady,
		builtInSectionIDInProgress,
		builtInSectionIDDone,
	})
}

func TestBuiltInProviderSectionQueryMapping(t *testing.T) {
	t.Parallel()

	provider := NewBuiltInProvider()

	dashboards, err := provider.Dashboards(context.Background())
	if err != nil {
		t.Fatalf("Dashboards returned error: %v", err)
	}

	sections := dashboards[0].Sections

	notReady := findSectionByID(t, sections, builtInSectionIDNotReady)
	if notReady.Query.Type != QueryTypeBlockedIssues {
		t.Fatalf("expected not-ready section query type %q, got %q", QueryTypeBlockedIssues, notReady.Query.Type)
	}
	if notReady.Query.BlockedIssues.Limit != defaultSectionLimit {
		t.Fatalf("expected not-ready limit %d, got %d", defaultSectionLimit, notReady.Query.BlockedIssues.Limit)
	}

	ready := findSectionByID(t, sections, builtInSectionIDReady)
	if ready.Query.Type != QueryTypeReadyIssues {
		t.Fatalf("expected ready section query type %q, got %q", QueryTypeReadyIssues, ready.Query.Type)
	}
	if ready.Query.ReadyIssues.Limit != defaultSectionLimit {
		t.Fatalf("expected ready limit %d, got %d", defaultSectionLimit, ready.Query.ReadyIssues.Limit)
	}

	inProgress := findSectionByID(t, sections, builtInSectionIDInProgress)
	if inProgress.Query.Type != QueryTypeListIssues {
		t.Fatalf("expected in-progress section query type %q, got %q", QueryTypeListIssues, inProgress.Query.Type)
	}
	if !reflect.DeepEqual(inProgress.Query.ListIssues.Statuses, []string{inProgressStatus}) {
		t.Fatalf("unexpected in-progress statuses: %#v", inProgress.Query.ListIssues.Statuses)
	}
	if inProgress.Query.ListIssues.Limit != defaultSectionLimit {
		t.Fatalf("expected in-progress limit %d, got %d", defaultSectionLimit, inProgress.Query.ListIssues.Limit)
	}

	done := findSectionByID(t, sections, builtInSectionIDDone)
	if done.Query.Type != QueryTypeListIssues {
		t.Fatalf("expected done section query type %q, got %q", QueryTypeListIssues, done.Query.Type)
	}
	if !reflect.DeepEqual(done.Query.ListIssues.Statuses, []string{doneStatus}) {
		t.Fatalf("unexpected done statuses: %#v", done.Query.ListIssues.Statuses)
	}
	if done.Query.ListIssues.SortBy != domain.SortFieldUpdatedAt {
		t.Fatalf("expected done section sort by updated_at, got %q", done.Query.ListIssues.SortBy)
	}
	if done.Query.ListIssues.SortOrder != domain.SortDirectionDescending {
		t.Fatalf("expected done section descending sort order, got %q", done.Query.ListIssues.SortOrder)
	}
	if done.Query.ListIssues.Limit != defaultSectionLimit {
		t.Fatalf("expected done section limit %d, got %d", defaultSectionLimit, done.Query.ListIssues.Limit)
	}
}

func findSectionByID(t *testing.T, sections []Section, id string) Section {
	t.Helper()

	for _, section := range sections {
		if section.ID == id {
			return section
		}
	}

	t.Fatalf("section not found: %s", id)
	return Section{}
}

func assertSectionIDs(t *testing.T, sections []Section, want []string) {
	t.Helper()

	if len(sections) != len(want) {
		t.Fatalf("section count mismatch: got %d want %d", len(sections), len(want))
	}

	for i, expectedID := range want {
		if sections[i].ID != expectedID {
			t.Fatalf("section[%d] id mismatch: got %q want %q", i, sections[i].ID, expectedID)
		}
	}
}
