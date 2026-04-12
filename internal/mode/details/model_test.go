package details

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
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
		SelectionID: "bw-2",
		TargetID:    "bw-2",
		Keys:        mustResolveDetailKeys(t, nil),
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
	if !strings.Contains(endView, "Related: (none)") {
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
	if got := m.focusPane(); got != 1 {
		t.Fatalf("expected left from content to focus related, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyLeft}, 80, 10); !consumed {
		t.Fatal("expected left key to be consumed")
	}
	if got := m.focusPane(); got != 1 {
		t.Fatalf("expected left from related to stay on related, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != 0 {
		t.Fatalf("expected right from related to focus content, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != 2 {
		t.Fatalf("expected right from content to focus metadata, got %v", got)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, 80, 10); !consumed {
		t.Fatal("expected right key to be consumed")
	}
	if got := m.focusPane(); got != 2 {
		t.Fatalf("expected right from metadata to stay on metadata, got %v", got)
	}
}

func TestModelDetailScrollBindingsMoveRelatedSelectionWhenRelatedFocused(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-1",
		TargetID:    "bw-1",
		FocusPane:   1,
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "bw-1", Title: "One"},
			BlockedBy: []domain.IssueReference{{ID: "bw-3", Title: "three"}},
			Blocks:    []domain.IssueReference{{ID: "bw-2", Title: "two"}},
			Related:   []domain.IssueReference{{ID: "bw-4", Title: "four"}},
		},
	}

	if consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10); !consumed || intent != nil {
		t.Fatalf("expected down to move related selection without intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.SelectedRelatedIndex != 1 {
		t.Fatalf("expected related index to move to 1, got %d", m.SelectedRelatedIndex)
	}

	if consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, 80, 10); !consumed || intent != nil {
		t.Fatalf("expected up to move related selection without intent, consumed=%v intent=%v", consumed, intent)
	}
	if m.SelectedRelatedIndex != 0 {
		t.Fatalf("expected related index to move back to 0, got %d", m.SelectedRelatedIndex)
	}
}

func TestModelDetailEnterOnRelatedPaneEmitsOpenIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:          "bw-1",
		TargetID:             "bw-1",
		FocusPane:            1,
		SelectedRelatedIndex: 1,
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "bw-1", Title: "One"},
			BlockedBy: []domain.IssueReference{{ID: "bw-2", Title: "two"}},
			Related:   []domain.IssueReference{{ID: "bw-3", Title: "three"}},
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

func TestModelDetailMetadataPaneIsNonInteractiveForScrollBindings(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "bw-1",
		TargetID:    "bw-1",
		FocusPane:   2,
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
	if m.ScrollOffset != 0 {
		t.Fatalf("expected metadata pane to keep scroll offset unchanged, got %d", m.ScrollOffset)
	}
}
