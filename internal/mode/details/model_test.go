package details

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
)

func TestModelViewRendersRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("no selection", func(t *testing.T) {
		t.Parallel()

		m := Model{}
		view := m.View(100, 20, false)
		if !strings.Contains(view, "No selected issue.") {
			t.Fatalf("expected no-selection state, got:\n%s", view)
		}
	})

	t.Run("loading", func(t *testing.T) {
		t.Parallel()

		m := Model{SelectionID: "bw-2", TargetID: "bw-2", Loading: true}
		view := m.View(100, 20, false)
		if !strings.Contains(view, "Loading details for") || !strings.Contains(view, "bw-2") {
			t.Fatalf("expected loading detail state, got:\n%s", view)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		m := Model{SelectionID: "bw-2", Error: "boom"}
		view := m.View(100, 20, false)
		if !strings.Contains(view, "Failed to load details for bw-2") || !strings.Contains(view, "boom") {
			t.Fatalf("expected detail error state, got:\n%s", view)
		}
	})
}

func TestModelViewSelectionChangeRendersSelectedIssueDetail(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:  "bw-2",
		TargetID:     "bw-2",
		Keys:         mustResolveDetailKeys(t, nil),
		BrowserItems: []domain.IssueReference{{ID: "bw-2", Title: "Second issue"}},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-2", Title: "Second issue", Status: "in_progress", Type: "task", Priority: 2},
		},
	}

	view := m.View(100, 20, false)
	if !strings.Contains(view, "Second issue") || !strings.Contains(view, "bw-2") || !strings.Contains(view, "Type    : task") {
		t.Fatalf("expected bw-2 detail rendering, got:\n%s", view)
	}

	// Simulate shell selection change to a different issue and loaded detail update.
	m.SelectionID = "bw-4"
	m.TargetID = "bw-4"
	m.BrowserItems = []domain.IssueReference{{ID: "bw-4", Title: "Fourth issue"}}
	m.Detail = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-4", Title: "Fourth issue", Status: "open", Type: "bug", Priority: 1},
	}

	view = m.View(100, 20, false)
	if !strings.Contains(view, "Fourth issue") || !strings.Contains(view, "bw-4") || !strings.Contains(view, "Type    : bug") {
		t.Fatalf("expected bw-4 detail rendering after selection change, got:\n%s", view)
	}
	if strings.Contains(view, "bw-2\n") {
		t.Fatalf("expected previous detail bw-2 to be replaced, got:\n%s", view)
	}
}

func TestModelDetailUsesConfiguredBindings(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-2",
		TargetID:    "bw-2",
		Keys: mustResolveDetailKeys(t, &config.KeyBindingOverride{
			Detail: map[string][]string{
				config.DetailActionScrollDown: {"n"},
				config.DetailActionScrollUp:   {"p"},
				config.DetailActionPageDown:   {"ctrl+f"},
				config.DetailActionPageUp:     {"ctrl+b"},
				config.DetailActionHome:       {"g"},
				config.DetailActionEnd:        {"G"},
			},
		}),
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-2", Title: "Long issue", Status: "open", Type: "task", Priority: 1},
			Description: strings.Repeat("line\n", 60),
		},
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, 80, 10); !consumed || m.ScrollOffset == 0 {
		t.Fatalf("expected configured scroll-down key to move viewport, offset=%d", m.ScrollOffset)
	}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, 80, 10); !consumed {
		t.Fatal("expected configured scroll-up key to be consumed")
	}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlF}, 80, 10); !consumed {
		t.Fatal("expected configured page-down key to be consumed")
	}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, 80, 10); !consumed {
		t.Fatal("expected configured end key to be consumed")
	}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}, 80, 10); !consumed || m.ScrollOffset != 0 {
		t.Fatalf("expected configured home key to reset offset, got %d", m.ScrollOffset)
	}
}

func mustResolveDetailKeys(t *testing.T, override *config.KeyBindingOverride) config.ResolvedKeyBindings {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), override))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return keys
}

func TestModelDetailScrollMovesViewportForLongContent(t *testing.T) {
	t.Parallel()

	descriptionLines := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		descriptionLines = append(descriptionLines, "Line "+strconv.Itoa(i))
	}

	m := Model{
		SelectionID: "bw-2",
		TargetID:    "bw-2",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-2", Title: "Long issue", Status: "open", Type: "task", Priority: 1},
			Description: strings.Join(descriptionLines, "\n"),
		},
	}

	initial := m.View(80, 10, false)
	if !strings.Contains(initial, "Long issue") {
		t.Fatalf("expected top-of-detail content in initial viewport, got:\n%s", initial)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown}, 80, 10); !consumed {
		t.Fatalf("expected page down to be consumed")
	}
	after := m.View(80, 10, false)
	if after == initial {
		t.Fatalf("expected viewport output to change after page down")
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 80, 10); !consumed {
		t.Fatalf("expected end key to be consumed")
	}
	endView := m.View(80, 10, false)
	if !strings.Contains(endView, "Line 40") {
		t.Fatalf("expected end to reach bottom section, got:\n%s", endView)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyHome}, 80, 10); !consumed {
		t.Fatalf("expected home key to be consumed")
	}
	homeView := m.View(80, 10, false)
	if !strings.Contains(homeView, "Long issue") {
		t.Fatalf("expected home to return to top, got:\n%s", homeView)
	}
}

func TestModelDetailScrollRecomputesLineCountWhenWidthChanges(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-2",
		TargetID:    "bw-2",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-2", Title: "Width sensitive markdown", Status: "open", Type: "task", Priority: 1},
			Description: strings.Repeat("wrap-me ", 80),
		},
	}

	_ = m.View(120, 10, false)
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 120, 10); !consumed {
		t.Fatal("expected end key at wide width to be consumed")
	}
	wideOffset := m.ScrollOffset

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 40, 10); !consumed {
		t.Fatal("expected end key at narrow width to be consumed")
	}

	if m.ScrollOffset <= wideOffset {
		t.Fatalf("expected larger max offset after narrowing width, wide=%d narrow=%d", wideOffset, m.ScrollOffset)
	}
}

func TestModelDetailPaneFocusMovesWithArrowKeys(t *testing.T) {
	t.Parallel()

	m := Model{}

	if got := m.focusPane(); got != 0 {
		t.Fatalf("expected default focus pane content, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyLeft}, 80, 10); !consumed {
		t.Fatal("expected left key to be consumed in detail mode")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneDependencies {
		t.Fatalf("expected left from content to focus dependencies pane, got %v", got)
	}

	m.BrowserItems = []domain.IssueReference{{ID: "bw-1"}, {ID: "bw-2"}}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyLeft}, 80, 10); !consumed {
		t.Fatal("expected left key to be consumed")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneBrowser {
		t.Fatalf("expected left from content to focus browser when present, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyLeft}, 80, 10); !consumed {
		t.Fatal("expected left key to be consumed")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneBrowser {
		t.Fatalf("expected left from browser to stay on browser, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneContent {
		t.Fatalf("expected right from browser to focus content, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneMetadata {
		t.Fatalf("expected right from content to focus metadata, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != uidetails.FocusPaneMetadata {
		t.Fatalf("expected right from metadata to stay on metadata, got %v", got)
	}
}

func TestModelDetailScrollBindingsMoveRelatedSelectionWhenRelatedFocused(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-1",
		TargetID:    "bw-1",
		FocusPane:   uidetails.FocusPaneBrowser,
		BrowserItems: []domain.IssueReference{
			{ID: "bw-1", Title: "One"},
			{ID: "bw-2", Title: "two"},
			{ID: "bw-3", Title: "three"},
		},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-1", Title: "One"},
		},
	}

	if consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10); !consumed || intent != nil {
		t.Fatalf("expected down to move related selection without intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.BrowserSelectedIndex != 1 {
		t.Fatalf("expected related index to move to 1, got %d", m.BrowserSelectedIndex)
	}

	if consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, 80, 10); !consumed || intent != nil {
		t.Fatalf("expected up to move related selection without intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected related index to move back to 0, got %d", m.BrowserSelectedIndex)
	}
}

func TestModelDetailEnterOnRelatedPaneEmitsOpenIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:          "bw-1",
		TargetID:             "bw-1",
		FocusPane:            uidetails.FocusPaneBrowser,
		BrowserSelectedIndex: 1,
		BrowserItems: []domain.IssueReference{
			{ID: "bw-1", Title: "One"},
			{ID: "bw-3", Title: "three"},
		},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-1", Title: "One"},
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, 80, 10)
	if !consumed {
		t.Fatal("expected enter on related pane to be consumed")
	}
	if intent == nil {
		t.Fatal("expected enter on related pane to emit open intent")
	}
	if intent.IssueID != "bw-3" {
		t.Fatalf("expected intent to target bw-3, got %q", intent.IssueID)
	}
}

func TestModelDetailMetadataPaneUpDownMovesBetweenStatusAndPriorityOnly(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-1",
		TargetID:    "bw-1",
		FocusPane:   uidetails.FocusPaneMetadata,
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-1", Title: "One"},
			Description: strings.Repeat("line\n", 50),
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10)
	if !consumed {
		t.Fatal("expected metadata pane to consume scroll bindings")
	}
	if intent != nil {
		t.Fatalf("expected no intent in metadata pane, got %+v", intent)
	}
	if m.MetadataSelectedField != uidetails.MetadataFieldPriority {
		t.Fatalf("expected metadata down to select priority, got %q", m.MetadataSelectedField)
	}

	consumed, intent = m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10)
	if !consumed || intent != nil {
		t.Fatalf("expected metadata down to remain consumed with no intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.MetadataSelectedField != uidetails.MetadataFieldPriority {
		t.Fatalf("expected metadata selection clamped to priority, got %q", m.MetadataSelectedField)
	}

	consumed, intent = m.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, 80, 10)
	if !consumed || intent != nil {
		t.Fatalf("expected metadata up to remain consumed with no intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.MetadataSelectedField != uidetails.MetadataFieldStatus {
		t.Fatalf("expected metadata up to select status, got %q", m.MetadataSelectedField)
	}
}

func TestModelDetailEnterOnMetadataStatusSetsOpenStatusDialogIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-1",
		TargetID:    "bw-1",
		FocusPane:   uidetails.FocusPaneMetadata,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-1", Status: "open"},
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, 160, 20)
	if !consumed {
		t.Fatal("expected enter in metadata pane to be consumed")
	}
	if intent != nil {
		t.Fatalf("expected no related-open intent from metadata enter, got %+v", intent)
	}
	if !m.ConsumeOpenStatusDialogIntent() {
		t.Fatal("expected metadata enter to raise open-status-dialog intent")
	}
	if m.ConsumeOpenStatusDialogIntent() {
		t.Fatal("expected status-dialog intent to be consumed once")
	}
}

func TestModelDetailEnterOnMetadataPrioritySetsCyclePriorityIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:           "bw-1",
		TargetID:              "bw-1",
		FocusPane:             uidetails.FocusPaneMetadata,
		MetadataSelectedField: uidetails.MetadataFieldPriority,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "bw-1", Priority: 1},
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, 160, 20)
	if !consumed {
		t.Fatal("expected enter in metadata pane to be consumed")
	}
	if intent != nil {
		t.Fatalf("expected no related-open intent from metadata enter, got %+v", intent)
	}
	if !m.ConsumeCyclePriorityIntent() {
		t.Fatal("expected metadata enter on priority to raise cycle-priority intent")
	}
	if m.ConsumeCyclePriorityIntent() {
		t.Fatal("expected cycle-priority intent to be consumed once")
	}
}

func TestModelApplyLoadedDetailBuildsBrowserFromParentGroupAndKeepsStableAcrossSiblings(t *testing.T) {
	t.Parallel()

	m := Model{}
	first := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-42", Title: "Child 42"},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "bw-1", Title: "Parent"},
			Children: []domain.IssueReference{
				{ID: "bw-42", Title: "Child 42"},
				{ID: "bw-43", Title: "Child 43"},
			},
		},
	}
	m.ApplyLoadedDetail("bw-42", first)

	if m.BrowserGroupParentID != "bw-1" {
		t.Fatalf("expected parent id bw-1, got %q", m.BrowserGroupParentID)
	}
	if len(m.BrowserItems) != 3 {
		t.Fatalf("expected browser parent+siblings, got %d", len(m.BrowserItems))
	}
	if m.BrowserItems[0].ID != "bw-1" {
		t.Fatalf("expected parent row first, got %#v", m.BrowserItems)
	}

	originalFirstRow := &m.BrowserItems[0]
	second := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-43", Title: "Child 43"},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "bw-1", Title: "Parent renamed"},
			Children: []domain.IssueReference{
				{ID: "bw-42", Title: "Child 42 renamed"},
				{ID: "bw-43", Title: "Child 43 renamed"},
			},
		},
	}
	m.ApplyLoadedDetail("bw-43", second)

	if &m.BrowserItems[0] != originalFirstRow {
		t.Fatalf("expected browser items slice to stay stable within same parent-group")
	}
	if m.BrowserSelectedIndex != 2 {
		t.Fatalf("expected selection to move to bw-43 index, got %d", m.BrowserSelectedIndex)
	}
}

func TestModelApplyLoadedDetailClearsBrowserWhenNoParentGroupContext(t *testing.T) {
	t.Parallel()

	m := Model{
		BrowserGroupParentID: "bw-parent",
		BrowserItems:         []domain.IssueReference{{ID: "bw-parent"}, {ID: "bw-child"}},
		BrowserSelectedIndex: 1,
		FocusPane:            uidetails.FocusPaneBrowser,
	}

	m.ApplyLoadedDetail("bw-child", domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-child"}})

	if m.BrowserGroupParentID != "" {
		t.Fatalf("expected browser parent id to clear, got %q", m.BrowserGroupParentID)
	}
	if len(m.BrowserItems) != 0 {
		t.Fatalf("expected browser items to clear, got %#v", m.BrowserItems)
	}
	if m.BrowserSelectedIndex != -1 {
		t.Fatalf("expected browser selection reset to -1, got %d", m.BrowserSelectedIndex)
	}
	if m.FocusPane != uidetails.FocusPaneContent {
		t.Fatalf("expected focus to move back to content when browser absent, got %v", m.FocusPane)
	}
}

func TestModelApplyLoadedDetailWithoutParentGroupBuildsBrowserFromDependencies(t *testing.T) {
	t.Parallel()

	m := Model{}
	m.ApplyLoadedDetail("bw-3", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Primary"},
		BlockedBy: []domain.IssueReference{
			{ID: "bw-3", Title: "Upstream blocker"},
			{ID: "bw-1", Title: "Auth migration"},
		},
		Blocks: []domain.IssueReference{
			{ID: "bw-2", Title: "Docs update"},
			{ID: "bw-3", Title: "Upstream blocker duplicate"},
		},
		Related: []domain.IssueReference{
			{ID: "bw-4", Title: "Search sync"},
		},
	})

	if m.BrowserGroupParentID != "" {
		t.Fatalf("expected no parent-group id for dependency-only issue, got %q", m.BrowserGroupParentID)
	}
	if len(m.BrowserItems) != 4 {
		t.Fatalf("expected flattened dependency browser items, got %#v", m.BrowserItems)
	}
	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID, m.BrowserItems[3].ID}; strings.Join(got, ",") != "bw-1,bw-3,bw-2,bw-4" {
		t.Fatalf("expected deterministic grouped ordering with de-duplication, got %v", got)
	}
	if m.BrowserSelectedIndex != 1 {
		t.Fatalf("expected selection to target loaded issue bw-3, got index %d", m.BrowserSelectedIndex)
	}
}

func TestModelApplyLoadedDetailWithoutParentGroupDefaultsSelectionToFirstDependency(t *testing.T) {
	t.Parallel()

	m := Model{}
	m.ApplyLoadedDetail("bw-999", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Primary"},
		BlockedBy: []domain.IssueReference{
			{ID: "bw-2", Title: "Blocker"},
		},
		Related: []domain.IssueReference{
			{ID: "bw-7", Title: "Neighbor"},
		},
	})

	if len(m.BrowserItems) != 2 {
		t.Fatalf("expected dependency refs in browser items, got %#v", m.BrowserItems)
	}
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected default dependency selection index 0, got %d", m.BrowserSelectedIndex)
	}
}
