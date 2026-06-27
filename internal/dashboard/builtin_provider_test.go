package dashboard

import (
	"context"
	"testing"
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
		t.Fatalf("expected 4 sections, got %d", len(dashboard.Sections))
	}

	assertSectionIDs(t, dashboard.Sections, []string{
		builtInSectionIDNotReady,
		builtInSectionIDReady,
		builtInSectionIDInProgress,
		builtInSectionIDDone,
	})
}

func TestBuiltInProviderSectionTitles(t *testing.T) {
	t.Parallel()

	provider := NewBuiltInProvider()

	dashboards, err := provider.Dashboards(context.Background())
	if err != nil {
		t.Fatalf("Dashboards returned error: %v", err)
	}

	sections := dashboards[0].Sections

	wantTitles := map[string]string{
		builtInSectionIDNotReady:   builtInSectionTitleNotReady,
		builtInSectionIDReady:      builtInSectionTitleReady,
		builtInSectionIDInProgress: builtInSectionTitleInProgress,
		builtInSectionIDDone:       builtInSectionTitleDone,
	}

	for _, section := range sections {
		want, ok := wantTitles[section.ID]
		if !ok {
			t.Fatalf("unexpected section id: %q", section.ID)
		}
		if section.Title != want {
			t.Fatalf("section %q: expected title %q, got %q", section.ID, want, section.Title)
		}
	}

	if err := ValidateDefinitions(dashboards); err != nil {
		t.Fatalf("expected built-in definitions to satisfy dashboard contract, got error: %v", err)
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
