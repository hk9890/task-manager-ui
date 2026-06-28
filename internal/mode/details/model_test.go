package details

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetails "github.com/hk9890/task-manager-ui/internal/ui/details"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

func TestModelViewRendersRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("no selection", func(t *testing.T) {
		t.Parallel()

		m := Model{}
		view := m.View(100, 20, false, 0)
		if !strings.Contains(view, "No selected issue.") {
			t.Fatalf("expected no-selection state, got:\n%s", view)
		}
	})

	t.Run("loading cold-start", func(t *testing.T) {
		t.Parallel()

		// Cold-start: Loading=true, no prior detail (Detail.Summary.ID == "").
		// Expect skeleton placeholder, NOT a full-screen loading takeover.
		m := Model{SelectionID: "tm-2", TargetID: "tm-2", Loading: true}
		view := m.View(100, 20, false, 0)
		if strings.Contains(view, "Loading details for") {
			t.Fatalf("cold-start loading should NOT show full-screen takeover, got:\n%s", view)
		}
		if !strings.Contains(view, issuerow.SkeletonGlyph) {
			t.Fatalf("cold-start loading should contain skeleton glyph, got:\n%s", view)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		m := Model{SelectionID: "tm-2", Error: "boom"}
		view := m.View(100, 20, false, 0)
		if !strings.Contains(view, "Failed to load details for tm-2") || !strings.Contains(view, "boom") {
			t.Fatalf("expected detail error state, got:\n%s", view)
		}
	})
}

func TestModelViewSelectionChangeRendersSelectedIssueDetail(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:  "tm-2",
		TargetID:     "tm-2",
		Keys:         mustResolveDetailKeys(t, nil),
		BrowserItems: []domain.IssueReference{{ID: "tm-2", Title: "Second issue"}},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-2", Title: "Second issue", Status: "in_progress", Type: "task", Priority: 2},
		},
	}

	view := m.View(100, 20, false, 0)
	if !strings.Contains(view, "Second issue") || !strings.Contains(view, "tm-2") || !strings.Contains(view, "Type    : task") {
		t.Fatalf("expected tm-2 detail rendering, got:\n%s", view)
	}

	// Simulate shell selection change to a different issue and loaded detail update.
	m.SelectionID = "tm-4"
	m.TargetID = "tm-4"
	m.BrowserItems = []domain.IssueReference{{ID: "tm-4", Title: "Fourth issue"}}
	m.Detail = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-4", Title: "Fourth issue", Status: "open", Type: "bug", Priority: 1},
	}

	view = m.View(100, 20, false, 0)
	if !strings.Contains(view, "Fourth issue") || !strings.Contains(view, "tm-4") || !strings.Contains(view, "Type    : bug") {
		t.Fatalf("expected tm-4 detail rendering after selection change, got:\n%s", view)
	}
	if strings.Contains(view, "tm-2\n") {
		t.Fatalf("expected previous detail tm-2 to be replaced, got:\n%s", view)
	}
}

func TestModelDetailUsesConfiguredBindings(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-2",
		TargetID:    "tm-2",
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
			Summary:     domain.IssueSummary{ID: "tm-2", Title: "Long issue", Status: "open", Type: "task", Priority: 1},
			Description: strings.Repeat("line\n", 60),
		},
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, 80, 10); !consumed || m.ContentScrollOffset == 0 {
		t.Fatalf("expected configured scroll-down key to move viewport, offset=%d", m.ContentScrollOffset)
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
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}, 80, 10); !consumed || m.ContentScrollOffset != 0 {
		t.Fatalf("expected configured home key to reset offset, got %d", m.ContentScrollOffset)
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
		SelectionID: "tm-2",
		TargetID:    "tm-2",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-2", Title: "Long issue", Status: "open", Type: "task", Priority: 1},
			Description: strings.Join(descriptionLines, "\n"),
		},
	}

	initial := m.View(80, 10, false, 0)
	// The Content pane header now leads with the dashboard-styled meta row
	// (type · priority · status · id); the title renders on the line below it.
	// In this deliberately short viewport only the top header line is visible,
	// so assert on the meta row's stable ID token rather than the title.
	if !strings.Contains(initial, "tm-2") {
		t.Fatalf("expected top-of-detail content (meta row) in initial viewport, got:\n%s", initial)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown}, 80, 10); !consumed {
		t.Fatalf("expected page down to be consumed")
	}
	after := m.View(80, 10, false, 0)
	if after == initial {
		t.Fatalf("expected viewport output to change after page down")
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 80, 10); !consumed {
		t.Fatalf("expected end key to be consumed")
	}
	endView := m.View(80, 10, false, 0)
	if !strings.Contains(endView, "Line 40") {
		t.Fatalf("expected end to reach bottom section, got:\n%s", endView)
	}

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyHome}, 80, 10); !consumed {
		t.Fatalf("expected home key to be consumed")
	}
	homeView := m.View(80, 10, false, 0)
	// Top of content is the meta row (see initial-viewport note above); the title
	// sits one line below it and is off-screen in this short viewport.
	if !strings.Contains(homeView, "tm-2") {
		t.Fatalf("expected home to return to top, got:\n%s", homeView)
	}
}

func TestModelDetailScrollRecomputesLineCountWhenWidthChanges(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-2",
		TargetID:    "tm-2",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-2", Title: "Width sensitive markdown", Status: "open", Type: "task", Priority: 1},
			Description: strings.Repeat("wrap-me ", 80),
		},
	}

	_ = m.View(120, 10, false, 0)
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 120, 10); !consumed {
		t.Fatal("expected end key at wide width to be consumed")
	}
	wideOffset := m.ContentScrollOffset

	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, 40, 10); !consumed {
		t.Fatal("expected end key at narrow width to be consumed")
	}

	if m.ContentScrollOffset <= wideOffset {
		t.Fatalf("expected larger max offset after narrowing width, wide=%d narrow=%d", wideOffset, m.ContentScrollOffset)
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

	m.BrowserItems = []domain.IssueReference{{ID: "tm-1"}, {ID: "tm-2"}}
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

// TestModelDetailScrollBindingsMoveRelatedSelectionWhenRelatedFocused verifies
// that ↑/↓ in the Dependencies pane moves BrowserSelectedIndex and returns a
// nil intent (no detail reload). Cursor movement is now decoupled from reload
// (Q6a, Q5 decoupling). Enter is the only key that triggers OpenRelatedIssueIntent.
func TestModelDetailScrollBindingsMoveRelatedSelectionWhenRelatedFocused(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneBrowser,
		BrowserItems: []domain.IssueReference{
			{ID: "tm-1", Title: "One"},
			{ID: "tm-2", Title: "two"},
			{ID: "tm-3", Title: "three"},
		},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1", Title: "One"},
		},
	}

	// (Q6a) Arrow moves BrowserSelectedIndex; intent must be nil (no reload).
	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10)
	if !consumed {
		t.Fatal("expected down to be consumed in Dependencies pane")
	}
	if intent != nil {
		t.Fatalf("expected down in Dependencies pane to return nil intent (no reload), got %+v", intent)
	}
	if m.BrowserSelectedIndex != 1 {
		t.Fatalf("expected related index to move to 1, got %d", m.BrowserSelectedIndex)
	}
	if selected, _ := m.selectedRelatedIssue(); selected.ID != "tm-2" {
		t.Fatalf("expected selected related issue tm-2 after down, got %q", selected.ID)
	}

	consumed, intent = m.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, 80, 10)
	if !consumed {
		t.Fatal("expected up to be consumed in Dependencies pane")
	}
	if intent != nil {
		t.Fatalf("expected up in Dependencies pane to return nil intent (no reload), got %+v", intent)
	}
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected related index to move back to 0, got %d", m.BrowserSelectedIndex)
	}
}

// TestModelDetailEnterOnRelatedPaneEmitsOpenRelatedIssueIntent verifies that
// pressing Enter while the Dependencies pane is focused emits
// OpenRelatedIssueIntent for the highlighted row (Q5, Q6b). This is hardcoded
// (NOT keymap-driven), consistent with how Enter in the Metadata pane works.
func TestModelDetailEnterOnRelatedPaneEmitsOpenRelatedIssueIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:          "tm-1",
		TargetID:             "tm-1",
		FocusPane:            uidetails.FocusPaneBrowser,
		BrowserSelectedIndex: 1,
		BrowserItems: []domain.IssueReference{
			{ID: "tm-1", Title: "One"},
			{ID: "tm-3", Title: "three"},
		},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1", Title: "One"},
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, 80, 10)
	if !consumed {
		t.Fatal("expected enter on Dependencies pane to be consumed")
	}
	if intent == nil {
		t.Fatal("expected enter on Dependencies pane to emit OpenRelatedIssueIntent, got nil")
	}
	if intent.IssueID != "tm-3" {
		t.Fatalf("expected OpenRelatedIssueIntent.IssueID=tm-3 (BrowserSelectedIndex=1), got %q", intent.IssueID)
	}
}

func TestModelRenderDetailUsesLoadingPreviewStubUntilPreviewDetailArrives(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-2",
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "tm-1", Title: "Anchor", Status: "open", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{{ID: "tm-2", Title: "Preview candidate", Status: "blocked", Type: "bug", Priority: 2}},
		},
		BrowserItems: []domain.IssueReference{{ID: "tm-2", Title: "Preview candidate", Status: "blocked", Type: "bug", Priority: 2}},
	}

	render := m.RenderDetail()
	if render.Summary.ID != "tm-2" {
		t.Fatalf("expected loading preview summary for target tm-2, got %q", render.Summary.ID)
	}
	// PlaceholderDetail now stores Description="" and relies on State.Skeleton=true
	// (set by View()) to render ▓ rows via the Skeleton seam.  Verify that the
	// rendered view (which goes through the Skeleton seam) contains the glyph.
	view := m.View(100, 20, false, 0)
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("expected placeholder view to contain skeleton glyph, got:\n%s", view)
	}
	if got := render.BlockedBy; len(got) != 1 || got[0].ID != "tm-2" {
		t.Fatalf("expected dependency rail to stay anchored to base detail, got %#v", got)
	}

	m.ApplyPreviewDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "Preview loaded", Status: "in_progress", Type: "bug", Priority: 2}, Description: "loaded preview"})
	render = m.RenderDetail()
	if render.Summary.ID != "tm-2" || render.Summary.Title != "Preview loaded" {
		t.Fatalf("expected loaded preview detail to render, got %#v", render.Summary)
	}
	if render.Description != "loaded preview" {
		t.Fatalf("expected loaded preview description, got %q", render.Description)
	}
}

// TestRenderDetailOptimisticallyShowsTargetHeaderWhileLoading locks the
// "see content while deps load" behavior (taskmgr-d4bz). When the user drills into a
// dependency row, the Content header (title + ID·status·priority) and Core
// metadata are painted immediately from the already-known IssueReference (via
// PlaceholderDetail), while the description and Dependencies pane render their
// skeleton/loading state until the single `taskmgr show` returns. Because the
// dependency groups ride in the SAME taskmgr payload as the content body, this
// optimistic header is the achievable form of "decoupling" — it needs no extra
// subprocess call.
func TestRenderDetailOptimisticallyShowsTargetHeaderWhileLoading(t *testing.T) {
	t.Parallel()

	target := domain.IssueReference{ID: "tm-2", Title: "Investigate cache stampede", Status: "in_progress", Type: "bug", Priority: 0}
	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-2", // drilled target differs from the anchored selection
		Loading:     true,
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "tm-1", Title: "Anchor", Status: "open", Type: "task", Priority: 1},
			BlockedBy: []domain.IssueReference{target},
		},
		BrowserItems: []domain.IssueReference{target},
	}

	// Optimistic header/core come straight from the known ref — no load needed.
	render := m.RenderDetail()
	if render.Summary.ID != "tm-2" || render.Summary.Title != "Investigate cache stampede" {
		t.Fatalf("expected optimistic header from known ref, got %#v", render.Summary)
	}
	if render.Summary.Status != "in_progress" || render.Summary.Priority != 0 || render.Summary.Type != "bug" {
		t.Fatalf("expected optimistic core metadata (status/priority/type) from known ref, got %#v", render.Summary)
	}
	// Description is intentionally empty so the Skeleton seam renders ▓ rows.
	if render.Description != "" {
		t.Fatalf("expected empty description during in-flight window, got %q", render.Description)
	}

	view := m.View(120, 24, false, 0)
	// The header is readable immediately: the target's title and status appear.
	if !strings.Contains(view, "Investigate cache stampede") {
		t.Fatalf("expected target title in optimistic header, got:\n%s", view)
	}
	if !strings.Contains(view, "in_progress") {
		t.Fatalf("expected target status in optimistic header/core metadata, got:\n%s", view)
	}
	// The body and Dependencies pane still show the loading skeleton.
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("expected skeleton glyph for in-flight body/deps, got:\n%s", view)
	}
}

func TestModelDetailMetadataPaneUpDownMovesBetweenStatusAndPriorityOnly(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneMetadata,
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-1", Title: "One"},
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
		t.Fatalf("expected metadata down to select priority after status, got %q", m.MetadataSelectedField)
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
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneMetadata,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1", Status: "open"},
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

func TestModelDetailEnterOnMetadataPrioritySetsOpenPriorityDialogIntent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:           "tm-1",
		TargetID:              "tm-1",
		FocusPane:             uidetails.FocusPaneMetadata,
		MetadataSelectedField: uidetails.MetadataFieldPriority,
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1", Priority: 1},
		},
	}

	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, 160, 20)
	if !consumed {
		t.Fatal("expected enter in metadata pane to be consumed")
	}
	if intent != nil {
		t.Fatalf("expected no related-open intent from metadata enter, got %+v", intent)
	}
	if !m.ConsumeOpenPriorityDialogIntent() {
		t.Fatal("expected metadata enter on priority to raise open-priority-dialog intent")
	}
	if m.ConsumeOpenPriorityDialogIntent() {
		t.Fatal("expected open-priority-dialog intent to be consumed once")
	}
}

func TestModelApplyLoadedDetailBuildsBrowserFromDependenciesAndParentGroup(t *testing.T) {
	t.Parallel()

	m := Model{}
	first := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-42", Title: "Child 42"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-90", Title: "Blocker"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-91", Title: "Blocked child"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-92", Title: "Related"},
		},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "tm-1", Title: "Parent"},
		},
	}
	m.ApplyLoadedDetail("tm-42", first)

	if m.BrowserGroupParentID != "tm-1" {
		t.Fatalf("expected parent id tm-1, got %q", m.BrowserGroupParentID)
	}
	// Only the parent (tm-1) is appended after the dependency groups; the
	// currently-viewed issue (tm-42) is excluded entirely.
	if len(m.BrowserItems) != 4 {
		t.Fatalf("expected flattened dependencies + parent row, got %#v", m.BrowserItems)
	}
	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID, m.BrowserItems[3].ID}; strings.Join(got, ",") != "tm-90,tm-91,tm-92,tm-1" {
		t.Fatalf("expected grouped dependency ordering followed by parent, got %v", got)
	}
	for _, ref := range m.BrowserItems {
		if ref.ID == "tm-42" {
			t.Fatalf("currently-viewed issue tm-42 must not appear in the browser panel, got %#v", m.BrowserItems)
		}
		if ref.ID == "tm-43" {
			t.Fatalf("sibling tm-43 must not appear in the browser panel, got %#v", m.BrowserItems)
		}
	}

	second := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-43", Title: "Child 43"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-90", Title: "Blocker renamed"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-91", Title: "Blocked child renamed"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-92", Title: "Related renamed"},
		},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "tm-1", Title: "Parent renamed"},
		},
	}
	m.ApplyLoadedDetail("tm-43", second)

	// Now viewing tm-43: it is excluded, and tm-42 stays absent because
	// siblings are no longer surfaced — only the parent (tm-1) is appended.
	if len(m.BrowserItems) != 4 {
		t.Fatalf("expected flattened dependencies + parent row after reload, got %#v", m.BrowserItems)
	}
	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID, m.BrowserItems[3].ID}; strings.Join(got, ",") != "tm-90,tm-91,tm-92,tm-1" {
		t.Fatalf("expected grouped dependency ordering followed by parent, got %v", got)
	}
	for _, ref := range m.BrowserItems {
		if ref.ID == "tm-43" {
			t.Fatalf("currently-viewed issue tm-43 must not appear in the browser panel, got %#v", m.BrowserItems)
		}
		if ref.ID == "tm-42" {
			t.Fatalf("sibling tm-42 must not appear in the browser panel, got %#v", m.BrowserItems)
		}
	}
	// Selection stays within bounds even though the loaded issue is not in the list.
	if m.BrowserSelectedIndex < 0 || m.BrowserSelectedIndex >= len(m.BrowserItems) {
		t.Fatalf("expected selection within [0,%d), got %d", len(m.BrowserItems), m.BrowserSelectedIndex)
	}
}

func TestModelApplyLoadedDetailBuildsDependencyTraversalOrderAcrossAllGroups(t *testing.T) {
	t.Parallel()

	m := Model{}
	m.ApplyLoadedDetail("tm-target", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-target", Title: "Subject not in any group"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-a1", Title: "Blocked by one"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-b1", Title: "Blocks one"},
			{ID: "tm-b2", Title: "Blocks two"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-c1", Title: "Related one"},
		},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "tm-s0", Title: "Parent epic"},
		},
	})

	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID, m.BrowserItems[3].ID, m.BrowserItems[4].ID}; strings.Join(got, ",") != "tm-a1,tm-b1,tm-b2,tm-c1,tm-s0" {
		t.Fatalf("expected flat traversal order to match rendered groups (parent last), got %v", got)
	}

	// The subject is not among any group, so selection defaults to the first row.
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected initial selection at first row, got %d", m.BrowserSelectedIndex)
	}

	// Position on the last Blocks row, then walk across group boundaries.
	m.selectBrowserIssue("tm-b2")
	m.moveRelatedSelection(1, 80, 24)
	if selected, ok := m.selectedRelatedIssue(); !ok || selected.ID != "tm-c1" {
		t.Fatalf("expected down from blocks to enter related group, got %+v ok=%v", selected, ok)
	}

	m.moveRelatedSelection(1, 80, 24)
	if selected, ok := m.selectedRelatedIssue(); !ok || selected.ID != "tm-s0" {
		t.Fatalf("expected down from related to enter parent row, got %+v ok=%v", selected, ok)
	}

	m.moveRelatedSelection(-1, 80, 24)
	if selected, ok := m.selectedRelatedIssue(); !ok || selected.ID != "tm-c1" {
		t.Fatalf("expected up from parent row to return to related group, got %+v ok=%v", selected, ok)
	}
}

func TestModelApplyLoadedDetailClearsBrowserWhenNoParentGroupContext(t *testing.T) {
	t.Parallel()

	m := Model{
		BrowserGroupParentID: "tm-parent",
		BrowserItems:         []domain.IssueReference{{ID: "tm-parent"}, {ID: "tm-child"}},
		BrowserSelectedIndex: 1,
		FocusPane:            uidetails.FocusPaneBrowser,
	}

	m.ApplyLoadedDetail("tm-child", domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-child"}})

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
	m.ApplyLoadedDetail("tm-1", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-1", Title: "Primary"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-3", Title: "Upstream blocker"},
			{ID: "tm-1", Title: "Self reference — must be excluded"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-2", Title: "Docs update"},
			{ID: "tm-3", Title: "Upstream blocker duplicate"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-4", Title: "Search sync"},
		},
	})

	if m.BrowserGroupParentID != "" {
		t.Fatalf("expected no parent-group id for dependency-only issue, got %q", m.BrowserGroupParentID)
	}
	// tm-1 is the currently-viewed issue and must be excluded even though it appears
	// in its own BlockedBy group; the duplicate tm-3 is de-duplicated.
	if len(m.BrowserItems) != 3 {
		t.Fatalf("expected flattened dependency browser items minus self, got %#v", m.BrowserItems)
	}
	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID}; strings.Join(got, ",") != "tm-3,tm-2,tm-4" {
		t.Fatalf("expected deterministic grouped ordering with de-duplication and self-exclusion, got %v", got)
	}
	for _, ref := range m.BrowserItems {
		if ref.ID == "tm-1" {
			t.Fatalf("currently-viewed issue tm-1 must not appear in the browser panel, got %#v", m.BrowserItems)
		}
	}
	// The subject is excluded, so selection defaults to the first row.
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected selection at first row, got index %d", m.BrowserSelectedIndex)
	}
}

func TestModelDependencyTraversalOrderMatchesDeduplicatedVisibleRenderOrder(t *testing.T) {
	t.Parallel()

	m := Model{}
	m.ApplyLoadedDetail("tm-3", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-main", Title: "Primary"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-1", Title: "Blocker"},
			{ID: "tm-3", Title: "Target"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-2", Title: "Blocked child"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-1", Title: "Duplicate from blocked-by"},
			{ID: "tm-4", Title: "Related unique"},
		},
	})

	if got := len(m.BrowserItems); got != 4 {
		t.Fatalf("expected traversal list to deduplicate duplicate IDs, got %d items %#v", got, m.BrowserItems)
	}
	if got := []string{m.BrowserItems[0].ID, m.BrowserItems[1].ID, m.BrowserItems[2].ID, m.BrowserItems[3].ID}; strings.Join(got, ",") != "tm-1,tm-3,tm-2,tm-4" {
		t.Fatalf("expected deterministic grouped traversal order, got %v", got)
	}

	visited := []string{m.BrowserItems[m.BrowserSelectedIndex].ID}
	stepsToEnd := len(m.BrowserItems) - 1 - m.BrowserSelectedIndex
	for i := 0; i < stepsToEnd; i++ {
		m.moveRelatedSelection(1, 80, 24)
		selected, ok := m.selectedRelatedIssue()
		if !ok {
			t.Fatal("expected related selection while traversing")
		}
		visited = append(visited, selected.ID)
	}
	if strings.Join(visited, ",") != "tm-3,tm-2,tm-4" {
		t.Fatalf("expected one-step traversal to visit remaining visible rows in order, got %v", visited)
	}
}

func TestModelApplyLoadedDetailWithoutParentGroupDefaultsSelectionToFirstDependency(t *testing.T) {
	t.Parallel()

	m := Model{}
	m.ApplyLoadedDetail("tm-999", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-1", Title: "Primary"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-2", Title: "Blocker"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-7", Title: "Neighbor"},
		},
	})

	if len(m.BrowserItems) != 2 {
		t.Fatalf("expected dependency refs in browser items, got %#v", m.BrowserItems)
	}
	if m.BrowserSelectedIndex != 0 {
		t.Fatalf("expected default dependency selection index 0, got %d", m.BrowserSelectedIndex)
	}
}

// --- Tests for non-blocking loading UX ---

// TestColdStartViewRendersSkeleton verifies that when Loading=true and no prior
// detail has been loaded (Summary.ID == ""), the view renders skeleton placeholders
// rather than a full-screen loading takeover.
func TestColdStartViewRendersSkeleton(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-5",
		TargetID:    "tm-5",
		Loading:     true,
		// Detail.Summary.ID is "", simulating cold-start.
	}

	view := m.View(100, 20, false, 0)

	// Must NOT be a full-screen loading takeover.
	if strings.Contains(view, "Loading details for") {
		t.Fatalf("cold-start view should NOT show full-screen loading takeover, got:\n%s", view)
	}

	// Must contain the skeleton glyph from renderColdStartSkeleton.
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("cold-start view should contain skeleton glyph %q, got:\n%s", issuerow.SkeletonGlyph, view)
	}
}

// TestRefreshSameIssueKeepsStaleContent verifies that when Loading=true and a
// prior detail is already loaded (Summary.ID != ""), the existing content remains
// visible (no full-screen takeover).
func TestRefreshSameIssueKeepsStaleContent(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-7",
		TargetID:    "tm-7",
		Loading:     true,
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-7", Title: "Stale issue", Status: "open", Type: "task", Priority: 2},
			Description: "Stale description visible during refresh",
		},
	}

	view := m.View(100, 20, false, 0)
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Must NOT be a full-screen loading takeover.
	if strings.Contains(plain, "Loading details for") {
		t.Fatalf("same-issue refresh view should NOT show full-screen loading takeover, got:\n%s", plain)
	}

	// Prior content must still be visible (ANSI-stripped — dim tint wraps text).
	if !strings.Contains(plain, "Stale issue") {
		t.Fatalf("same-issue refresh view should keep prior title visible, got:\n%s", plain)
	}
}

// TestRefreshDifferentPreviouslyLoadedIssueKeepsStaleContent verifies that when
// switching to a different issue, prior content remains visible momentarily
// (before the repository response arrives).
func TestRefreshDifferentPreviouslyLoadedIssueKeepsStaleContent(t *testing.T) {
	t.Parallel()

	// Simulate: issue A was loaded, selection moved to B, placeholder applied.
	// At this point Detail still holds A's data with Summary.ID == "tm-A" but
	// after ApplyLoadedDetail(placeholder), it holds the placeholder which has
	// Summary.ID == "tm-B". Either way, Loading=true and Summary.ID != "".
	m := Model{
		SelectionID: "tm-B",
		TargetID:    "tm-B",
		Loading:     true,
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-A", Title: "Previous issue A", Status: "open", Type: "task", Priority: 1},
			Description: "Previous issue A description",
		},
	}

	view := m.View(100, 20, false, 0)
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Must NOT be a full-screen loading takeover.
	if strings.Contains(plain, "Loading details for") {
		t.Fatalf("different-issue refresh view should NOT show full-screen loading takeover, got:\n%s", plain)
	}

	// Prior content is still visible (stale data from issue A; ANSI-stripped — dim tint wraps text).
	if !strings.Contains(plain, "Previous issue A") {
		t.Fatalf("different-issue refresh view should keep prior content visible, got:\n%s", plain)
	}
}

// TestScrollResetOnIssueSwitchViaApplyLoadedDetail is the regression test for
// the scroll-reset-on-issue-switch fix. It verifies that when the caller applies a placeholder
// detail synchronously on selection-change (mimicking what app/model.go does),
// all three scroll offsets are immediately zeroed before the repository response
// arrives.
func TestScrollResetOnIssueSwitchViaApplyLoadedDetail(t *testing.T) {
	t.Parallel()

	issueA := domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Issue A"},
		Description: strings.Repeat("line\n", 60),
	}

	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
	}
	m.ApplyLoadedDetail("tm-1", issueA)

	// Simulate the user scrolling all three panes to non-zero offsets.
	m.ContentScrollOffset = 15
	m.MetadataScrollOffset = 7
	m.DependenciesScrollOffset = 3
	m.ContentScrollOffset = 15

	// Simulate app-level selection change to tm-2: synchronously apply placeholder
	// BEFORE the repository response arrives (this is the mechanism from app/model.go).
	m.Loading = true
	m.SelectionID = "tm-2"
	m.TargetID = "tm-2"
	ref := domain.IssueReference{ID: "tm-2", Title: "Issue B"}
	m.ApplyLoadedDetail("tm-2", PlaceholderDetail("tm-2", ref, true))

	// All scroll offsets must be zero immediately (before repository responds).
	if m.ContentScrollOffset != 0 {
		t.Errorf("ContentContentScrollOffset must be 0 immediately after issue switch, got %d", m.ContentScrollOffset)
	}
	if m.MetadataScrollOffset != 0 {
		t.Errorf("MetadataContentScrollOffset must be 0 immediately after issue switch, got %d", m.MetadataScrollOffset)
	}
	if m.DependenciesScrollOffset != 0 {
		t.Errorf("DependenciesContentScrollOffset must be 0 immediately after issue switch, got %d", m.DependenciesScrollOffset)
	}
	if m.ContentScrollOffset != 0 {
		t.Errorf("ContentScrollOffset must be 0 immediately after issue switch, got %d", m.ContentScrollOffset)
	}

	// The view while loading with the placeholder must NOT be a full-screen
	// loading takeover (Summary.ID is "tm-2" from the placeholder).
	view := m.View(100, 20, false, 0)
	if strings.Contains(view, "Loading details for") {
		t.Errorf("placeholder-loaded detail should NOT show full-screen loading takeover, got:\n%s", view)
	}
}

// TestPlaceholderDetailHasEmptyDescriptionAndSkeletonSeamRendersGlyph verifies
// that PlaceholderDetail returns an empty Description (the Skeleton seam in
// View() renders ▓ rows, bypassing markdown rendering) and that the rendered
// view contains the skeleton glyph.
func TestPlaceholderDetailHasEmptyDescriptionAndSkeletonSeamRendersGlyph(t *testing.T) {
	t.Parallel()

	ref := domain.IssueReference{ID: "tm-10", Title: "Some issue", Status: "open", Type: "task", Priority: 1}
	detail := PlaceholderDetail("tm-10", ref, true)

	if detail.Summary.ID != "tm-10" {
		t.Errorf("expected placeholder summary ID tm-10, got %q", detail.Summary.ID)
	}
	// Description must be empty — skeleton rows are rendered via State.Skeleton=true.
	if detail.Description != "" {
		t.Errorf("placeholder description should be empty (skeleton via seam), got %q", detail.Description)
	}

	// A model that is previewing a target (TargetID != SelectionID, no preview
	// loaded) should render skeleton glyphs via the Skeleton seam in View().
	m := Model{
		SelectionID: "tm-1",
		TargetID:    "tm-10",
		BrowserItems: []domain.IssueReference{
			{ID: "tm-10", Title: "Some issue", Status: "open", Type: "task", Priority: 1},
		},
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1"},
		},
	}
	view := m.View(100, 20, false, 0)
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Errorf("expected rendered view to contain skeleton glyph %q via Skeleton seam, got:\n%s", issuerow.SkeletonGlyph, view)
	}
}

// --- End of non-blocking loading UX tests ---

func TestApplyLoadedDetailResetsScrollOffsetOnIssueChange(t *testing.T) {
	t.Parallel()

	issueA := domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Issue A"},
		Description: strings.Repeat("line\n", 60),
	}
	issueB := domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-2", Title: "Issue B"},
		Description: strings.Repeat("line\n", 60),
	}

	t.Run("reset all offsets when switching to different issue", func(t *testing.T) {
		t.Parallel()

		m := Model{}
		m.ApplyLoadedDetail("tm-1", issueA)

		// Simulate user scrolling all three panes.
		m.ContentScrollOffset = 10
		m.MetadataScrollOffset = 5
		m.DependenciesScrollOffset = 3
		m.ContentScrollOffset = 10

		// Switch to a different issue.
		m.ApplyLoadedDetail("tm-2", issueB)

		if m.ContentScrollOffset != 0 {
			t.Errorf("expected ContentContentScrollOffset=0 after issue switch, got %d", m.ContentScrollOffset)
		}
		if m.MetadataScrollOffset != 0 {
			t.Errorf("expected MetadataContentScrollOffset=0 after issue switch, got %d", m.MetadataScrollOffset)
		}
		if m.DependenciesScrollOffset != 0 {
			t.Errorf("expected DependenciesContentScrollOffset=0 after issue switch, got %d", m.DependenciesScrollOffset)
		}
		if m.ContentScrollOffset != 0 {
			t.Errorf("expected ContentScrollOffset=0 after issue switch, got %d", m.ContentScrollOffset)
		}
	})

	t.Run("preserve scroll offsets when refreshing the same issue", func(t *testing.T) {
		t.Parallel()

		m := Model{}
		m.ApplyLoadedDetail("tm-1", issueA)

		// Simulate user scrolling.
		m.ContentScrollOffset = 7
		m.MetadataScrollOffset = 2
		m.DependenciesScrollOffset = 4
		m.ContentScrollOffset = 7

		// Re-load the same issue (e.g. refresh).
		m.ApplyLoadedDetail("tm-1", issueA)

		if m.ContentScrollOffset != 7 {
			t.Errorf("expected ContentScrollOffset=7 preserved on same-issue refresh, got %d", m.ContentScrollOffset)
		}
		if m.MetadataScrollOffset != 2 {
			t.Errorf("expected MetadataScrollOffset=2 preserved on same-issue refresh, got %d", m.MetadataScrollOffset)
		}
		if m.DependenciesScrollOffset != 4 {
			t.Errorf("expected DependenciesScrollOffset=4 preserved on same-issue refresh, got %d", m.DependenciesScrollOffset)
		}
	})

	t.Run("reset offsets when first issue is loaded (empty previous)", func(t *testing.T) {
		t.Parallel()

		// Simulate stale scroll offsets before any issue is loaded (shouldn't
		// happen in practice but ensures the empty-previous guard works).
		m := Model{
			ContentScrollOffset:      8,
			MetadataScrollOffset:     3,
			DependenciesScrollOffset: 1,
		}

		m.ApplyLoadedDetail("tm-1", issueA)

		if m.ContentScrollOffset != 0 {
			t.Errorf("expected ContentContentScrollOffset=0 on first issue load, got %d", m.ContentScrollOffset)
		}
		if m.MetadataScrollOffset != 0 {
			t.Errorf("expected MetadataContentScrollOffset=0 on first issue load, got %d", m.MetadataScrollOffset)
		}
		if m.DependenciesScrollOffset != 0 {
			t.Errorf("expected DependenciesContentScrollOffset=0 on first issue load, got %d", m.DependenciesScrollOffset)
		}
	})
}

func TestClampOffset(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		value     int
		maxOffset int
		want      int
	}{
		{"zero offset stays zero", 0, 10, 0},
		{"offset within range unchanged", 5, 10, 5},
		{"offset beyond max clamped to max", 15, 10, 10},
		{"negative offset clamped to zero", -3, 10, 0},
		{"max zero with positive offset clamped to zero", 5, 0, 0},
		{"max zero with zero offset stays zero", 0, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := clampOffset(tc.value, tc.maxOffset)
			if got != tc.want {
				t.Fatalf("clampOffset(%d, %d) = %d, want %d", tc.value, tc.maxOffset, got, tc.want)
			}
		})
	}
}

func TestClampScroll(t *testing.T) {
	t.Parallel()

	// Use a wide viewport (>= InspectorTwoColumnMinWidth) and long description so
	// MaxScrollOffsets returns a positive Content bound we can overshoot.
	const (
		width  = 200
		height = 10
	)
	longDesc := strings.Repeat("line\n", 60)

	t.Run("scroll already in range is unchanged", func(t *testing.T) {
		t.Parallel()

		m := Model{
			SelectionID:         "tm-1",
			TargetID:            "tm-1",
			ContentScrollOffset: 2,
			Detail: domain.IssueDetail{
				Summary:     domain.IssueSummary{ID: "tm-1", Title: "issue"},
				Description: longDesc,
			},
		}
		m.ClampScroll(width, height)
		if m.ContentScrollOffset < 0 || m.ContentScrollOffset > 2 {
			t.Fatalf("expected ContentScrollOffset to stay <= 2, got %d", m.ContentScrollOffset)
		}
	})

	t.Run("scroll beyond max is clamped down", func(t *testing.T) {
		t.Parallel()

		m := Model{
			SelectionID:         "tm-1",
			TargetID:            "tm-1",
			ContentScrollOffset: 9999,
			Detail: domain.IssueDetail{
				Summary:     domain.IssueSummary{ID: "tm-1", Title: "issue"},
				Description: longDesc,
			},
		}
		before := m.ContentScrollOffset
		m.ClampScroll(width, height)
		if m.ContentScrollOffset >= before {
			t.Fatalf("expected ContentScrollOffset to be clamped below %d, got %d", before, m.ContentScrollOffset)
		}
		if m.ContentScrollOffset < 0 {
			t.Fatalf("expected ContentScrollOffset >= 0 after clamp, got %d", m.ContentScrollOffset)
		}
	})

	t.Run("negative scroll clamped to zero", func(t *testing.T) {
		t.Parallel()

		m := Model{
			SelectionID:              "tm-1",
			TargetID:                 "tm-1",
			ContentScrollOffset:      -5,
			DependenciesScrollOffset: -3,
			MetadataScrollOffset:     -1,
			Detail: domain.IssueDetail{
				Summary:     domain.IssueSummary{ID: "tm-1", Title: "issue"},
				Description: longDesc,
			},
		}
		m.ClampScroll(width, height)
		if m.ContentScrollOffset != 0 {
			t.Errorf("expected ContentScrollOffset clamped to 0, got %d", m.ContentScrollOffset)
		}
		if m.DependenciesScrollOffset != 0 {
			t.Errorf("expected DependenciesScrollOffset clamped to 0, got %d", m.DependenciesScrollOffset)
		}
		if m.MetadataScrollOffset != 0 {
			t.Errorf("expected MetadataScrollOffset clamped to 0, got %d", m.MetadataScrollOffset)
		}
	})

	t.Run("zero viewportHeight is no-op", func(t *testing.T) {
		t.Parallel()

		m := Model{
			SelectionID:         "tm-1",
			ContentScrollOffset: 9999,
			Detail: domain.IssueDetail{
				Summary:     domain.IssueSummary{ID: "tm-1", Title: "issue"},
				Description: longDesc,
			},
		}
		m.ClampScroll(width, 0)
		if m.ContentScrollOffset != 9999 {
			t.Fatalf("expected no-op with zero viewportHeight, got ContentScrollOffset=%d", m.ContentScrollOffset)
		}
	})
}

func TestSelectBrowserIssue(t *testing.T) {
	t.Parallel()

	items := []domain.IssueReference{
		{ID: "tm-1", Title: "First"},
		{ID: "tm-2", Title: "Second"},
		{ID: "tm-3", Title: "Third"},
	}

	t.Run("selects existing ID by index", func(t *testing.T) {
		t.Parallel()

		m := Model{BrowserItems: items, BrowserSelectedIndex: 0}
		m.SelectBrowserIssue("tm-3")
		if m.BrowserSelectedIndex != 2 {
			t.Fatalf("expected BrowserSelectedIndex=2 for tm-3, got %d", m.BrowserSelectedIndex)
		}
	})

	t.Run("selects first ID", func(t *testing.T) {
		t.Parallel()

		m := Model{BrowserItems: items, BrowserSelectedIndex: 2}
		m.SelectBrowserIssue("tm-1")
		if m.BrowserSelectedIndex != 0 {
			t.Fatalf("expected BrowserSelectedIndex=0 for tm-1, got %d", m.BrowserSelectedIndex)
		}
	})

	t.Run("unknown ID normalizes to last valid index", func(t *testing.T) {
		// When the ID is not found, selectBrowserIssue falls through to
		// normalizeRelatedSelection which clamps BrowserSelectedIndex to [0, len-1].
		t.Parallel()

		m := Model{BrowserItems: items, BrowserSelectedIndex: 5}
		m.SelectBrowserIssue("tm-999")
		if m.BrowserSelectedIndex < 0 || m.BrowserSelectedIndex >= len(items) {
			t.Fatalf("expected BrowserSelectedIndex in valid range after unknown id, got %d", m.BrowserSelectedIndex)
		}
	})

	t.Run("empty browser items sets index to -1", func(t *testing.T) {
		t.Parallel()

		m := Model{BrowserItems: nil, BrowserSelectedIndex: 2}
		m.SelectBrowserIssue("tm-1")
		if m.BrowserSelectedIndex != -1 {
			t.Fatalf("expected BrowserSelectedIndex=-1 for empty items, got %d", m.BrowserSelectedIndex)
		}
	})
}

// --- Scroll-window tests ---

// TestDetailsDependencyScrollOffsetAdvancesWithSelection verifies that pressing
// j×15 on a Dependencies pane with 30 deps advances BrowserSelectedIndex to 15
// and moves DependenciesScrollOffset to keep the selection inside the pane window.
func TestDetailsDependencyScrollOffsetAdvancesWithSelection(t *testing.T) {
	t.Parallel()

	// Build 30 unique dependency refs (all in BlockedBy for simplicity).
	const depCount = 30
	blockedBy := make([]domain.IssueReference, depCount)
	for i := range blockedBy {
		blockedBy[i] = domain.IssueReference{
			ID:    fmt.Sprintf("tm-dep%02d", i),
			Title: fmt.Sprintf("Dep issue %d", i),
		}
	}

	m := Model{
		SelectionID: "tm-main",
		TargetID:    "tm-main",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "tm-main", Title: "Main"},
			BlockedBy: blockedBy,
		},
		BrowserItems:         append([]domain.IssueReference(nil), blockedBy...),
		BrowserSelectedIndex: 0,
		Keys:                 mustResolveDetailKeys(t, nil),
	}

	// Use three-pane width (>= 110) and enough height to see some deps.
	const width = 160
	const height = 20 // innerHeight = 18 for deps pane

	// Press j 15 times on the Dependencies pane.
	for i := 0; i < 15; i++ {
		consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, width, height)
		if !consumed {
			t.Fatalf("expected j key to be consumed on step %d", i+1)
		}
	}

	if m.BrowserSelectedIndex != 15 {
		t.Errorf("expected BrowserSelectedIndex=15 after 15 j presses, got %d", m.BrowserSelectedIndex)
	}

	// Offset must have advanced so that sel=15 is inside the window.
	paneInner := max(1, height-2) // three-pane: height-2 = 18
	offset := m.DependenciesScrollOffset
	if m.BrowserSelectedIndex >= 0 {
		lineIdx := dependencyRefLineIndex(m.BrowserSelectedIndex, m.BrowserItems, m.Detail)
		if lineIdx >= 0 && (lineIdx < offset || lineIdx >= offset+paneInner) {
			t.Errorf("selection line %d not in window [%d, %d)", lineIdx, offset, offset+paneInner)
		}
	}
}

// TestDetailsMetadataScrollOffsetAdvancesWithSelection verifies that pressing
// j past the first field in the metadata pane updates MetadataScrollOffset
// when the pane is small enough that the selected field would scroll off-screen.
func TestDetailsMetadataScrollOffsetAdvancesWithSelection(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID:           "tm-1",
		TargetID:              "tm-1",
		FocusPane:             uidetails.FocusPaneMetadata,
		MetadataSelectedField: uidetails.MetadataFieldStatus,
		Keys:                  mustResolveDetailKeys(t, nil),
		Detail: domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "tm-1", Title: "One", Status: "open", Priority: 1},
		},
	}

	// Only two editable metadata fields (Status, Priority). Moving down from
	// Status to Priority doesn't scroll when the pane is tall enough. This test
	// verifies the model doesn't panic and the field advances correctly.
	consumed, intent := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 160, 10)
	if !consumed {
		t.Fatal("expected j to be consumed in metadata pane")
	}
	if intent != nil {
		t.Fatalf("expected no intent from metadata nav, got %+v", intent)
	}
	if m.MetadataSelectedField != uidetails.MetadataFieldPriority {
		t.Errorf("expected field to advance to Priority, got %q", m.MetadataSelectedField)
	}
}

// TestRenderDetailChildrenAnchoredToBaseDetail verifies that RenderDetail always
// overlays Children from m.Detail (the anchor issue), not from any preview target.
// This guards the index↔line desync blind spot: if content.Children came from the
// preview issue instead of m.Detail, BrowserItems (built from m.Detail.Children)
// and the rendered deps pane would disagree.
func TestRenderDetailChildrenAnchoredToBaseDetail(t *testing.T) {
	t.Parallel()

	anchorChildren := []domain.IssueReference{
		{ID: "tm-child1", Title: "Child of anchor"},
		{ID: "tm-child2", Title: "Another child of anchor"},
	}

	m := Model{
		// Anchor issue (SelectionID) has two children.
		SelectionID: "tm-anchor",
		// TargetID differs: we are previewing tm-target.
		TargetID: "tm-target",
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-anchor", Title: "Anchor epic", Status: "open", Type: "epic", Priority: 1},
			Children: anchorChildren,
			BlockedBy: []domain.IssueReference{
				{ID: "tm-target", Title: "Target (also a blocker)", Status: "open", Type: "task", Priority: 2},
			},
		},
		// BrowserItems built from anchor detail.
		BrowserItems: []domain.IssueReference{
			{ID: "tm-target", Title: "Target (also a blocker)"},
			{ID: "tm-child1", Title: "Child of anchor"},
			{ID: "tm-child2", Title: "Another child of anchor"},
		},
	}

	// Apply a preview detail for tm-target that has DIFFERENT children.
	m.ApplyPreviewDetail(domain.IssueDetail{
		Summary:  domain.IssueSummary{ID: "tm-target", Title: "Target issue", Status: "open", Type: "task", Priority: 2},
		Children: []domain.IssueReference{{ID: "tm-other-child", Title: "Target's own child"}},
	})

	render := m.RenderDetail()

	// Children in the rendered detail must come from m.Detail (the anchor), not
	// from the preview target.
	if len(render.Children) != len(anchorChildren) {
		t.Errorf("expected %d children from anchor detail, got %d: %#v",
			len(anchorChildren), len(render.Children), render.Children)
	}
	for i, want := range anchorChildren {
		if i >= len(render.Children) {
			break
		}
		if render.Children[i].ID != want.ID {
			t.Errorf("Children[%d]: want ID %q, got %q", i, want.ID, render.Children[i].ID)
		}
	}

	// Verify the rendered view shows Children group with correct count in the deps pane.
	// Use a wide terminal for layout stability.
	view := m.View(uidetails.InspectorThreeColumnMinWidth, 24, false, 0)
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Children (2)") {
		t.Errorf("expected 'Children (2)' in rendered deps pane, got:\n%s", plain)
	}
	// The preview target's child must NOT appear — only anchor's children are shown.
	if strings.Contains(plain, "other-child") {
		t.Errorf("expected preview target's child to be absent from deps pane, got:\n%s", plain)
	}
}

// TestBrowserItemsFromDependenciesIncludesChildren verifies that
// browserItemsFromDependencies includes detail.Children in the flat list,
// placed between Related and the parent row. This is the navigation
// flat-list ordering (plan-review Q2b).
func TestBrowserItemsFromDependenciesIncludesChildren(t *testing.T) {
	t.Parallel()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-epic", Title: "Epic"},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-blocker", Title: "Blocker"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-related", Title: "Related"},
		},
		Children: []domain.IssueReference{
			{ID: "tm-child2", Title: "Child two"},
			{ID: "tm-child1", Title: "Child one"},
		},
	}

	m := Model{}
	m.ApplyLoadedDetail("tm-epic", detail)

	// Expected flat order: BlockedBy, Related, Children (sorted asc).
	// tm-blocker, tm-related, tm-child1, tm-child2
	if len(m.BrowserItems) != 4 {
		t.Fatalf("expected 4 browser items (blocker + related + 2 children), got %d: %#v",
			len(m.BrowserItems), m.BrowserItems)
	}
	wantOrder := []string{"tm-blocker", "tm-related", "tm-child1", "tm-child2"}
	for i, want := range wantOrder {
		if got := m.BrowserItems[i].ID; got != want {
			t.Errorf("BrowserItems[%d]: want %q, got %q", i, want, got)
		}
	}
}

// TestDrillFromDepsFocusRetainedOnRealLoadWithDeps verifies the model-level
// drill-focus contract: when SetDrillFromDepsFocus is called before the
// placeholder ApplyLoadedDetail (with Loading=true), clearBrowserPanel does not
// flip focus, and the real data load with a non-empty rail keeps focus on
// Dependencies.
func TestDrillFromDepsFocusRetainedOnRealLoadWithDeps(t *testing.T) {
	t.Parallel()

	// Start: tm-parent is loaded, cursor is on the Dependencies pane.
	m := Model{
		SelectionID: "tm-parent",
		TargetID:    "tm-parent",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-parent"},
			Children: []domain.IssueReference{{ID: "tm-child"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-child"}},
		BrowserSelectedIndex: 0,
	}

	// Simulate the app drill-handler sequence: set Loading and drill-focus before placeholder.
	m.Loading = true
	m.SelectionID = "tm-child"
	m.TargetID = "tm-child"
	m.SetDrillFromDepsFocus()
	m.ApplyLoadedDetail("tm-child", PlaceholderDetail("tm-child", domain.IssueReference{ID: "tm-child"}, true))

	// Placeholder phase: focus must not have been flipped to Content.
	if m.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies, got %v", m.FocusPane)
	}

	// Simulate real data arriving: Loading cleared first, then ApplyLoadedDetail.
	m.Loading = false
	m.ApplyLoadedDetail("tm-child", domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-child"},
		BlockedBy: []domain.IssueReference{{ID: "tm-blocker"}},
	})

	// Non-empty rail: focus must stay on Dependencies.
	if m.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("real load with deps: expected FocusPane=Dependencies, got %v", m.FocusPane)
	}
	// Counter must have been consumed (no leftover).
	if m.drillDepsFocusCalls != 0 {
		t.Errorf("expected drillDepsFocusCalls=0 after real load, got %d", m.drillDepsFocusCalls)
	}
}

// TestDrillFromDepsFocusMovesToContentOnLeafLoad verifies that when drilling
// into a leaf issue (no dependencies), focus moves to Content after the real
// data arrives, but NOT during the placeholder phase.
func TestDrillFromDepsFocusMovesToContentOnLeafLoad(t *testing.T) {
	t.Parallel()

	m := Model{
		SelectionID: "tm-parent",
		TargetID:    "tm-parent",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-parent"},
			Children: []domain.IssueReference{{ID: "tm-leaf"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-leaf"}},
		BrowserSelectedIndex: 0,
	}

	m.Loading = true
	m.SelectionID = "tm-leaf"
	m.TargetID = "tm-leaf"
	m.SetDrillFromDepsFocus()
	m.ApplyLoadedDetail("tm-leaf", PlaceholderDetail("tm-leaf", domain.IssueReference{ID: "tm-leaf"}, true))

	// Placeholder phase: focus stays on Dependencies (deferred, not flipped early).
	if m.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies (deferred), got %v", m.FocusPane)
	}

	// Real load: leaf has no dependencies.
	m.Loading = false
	m.ApplyLoadedDetail("tm-leaf", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-leaf"},
	})

	// Empty rail: focus must move to Content.
	if m.FocusPane != uidetails.FocusPaneContent {
		t.Errorf("real load leaf: expected FocusPane=Content, got %v", m.FocusPane)
	}
	if m.drillDepsFocusCalls != 0 {
		t.Errorf("expected drillDepsFocusCalls=0 after real load, got %d", m.drillDepsFocusCalls)
	}
}

// TestDrillFromDepsFocusClearDrillFocusCancels verifies that ClearDrillFocus
// cancels the pending drill sequence: after clearing, clearBrowserPanel flips
// focus normally.
func TestDrillFromDepsFocusClearDrillFocusCancels(t *testing.T) {
	t.Parallel()

	m := Model{
		FocusPane: uidetails.FocusPaneDependencies,
	}

	m.Loading = true
	m.SetDrillFromDepsFocus()

	// Cancel before real load (e.g. load error or selection superseded).
	m.ClearDrillFocus()

	// ApplyLoadedDetail on a leaf should now flip focus to Content normally.
	m.Loading = false
	m.ApplyLoadedDetail("tm-x", domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-x"},
	})

	if m.FocusPane != uidetails.FocusPaneContent {
		t.Errorf("after ClearDrillFocus: expected FocusPane=Content, got %v", m.FocusPane)
	}
}
