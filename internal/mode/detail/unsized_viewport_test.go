package detail

// The unsized-viewport guards on HandleKey and ClampScroll. Every other test
// passes a positive viewport height, so neither guard was executed by anything
// — deleting either left the whole repository green. These are new scenarios
// rather than stronger assertions.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// unsizedModel is a detail model with scrollable content and a non-zero scroll
// offset, so a guard that wrongly ran would visibly move something.
func unsizedModel(t *testing.T) *Model {
	t.Helper()

	m := &Model{
		selectionID: "tm-9",
		targetID:    "tm-9",
		Keys:        mustResolveDetailKeys(t, nil),
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-9", Title: "Unsized", Status: "open", Type: "task", Priority: 1},
			Description: strings.Repeat("line\n", 60),
		},
		ContentScrollOffset: 7,
	}

	return m
}

// TestHandleKeyDeclinesEveryKeyBeforeThePaneIsSized covers the guard that hands
// keys back to the shell until detail mode knows its own height.
//
// Without it, detail mode swallows shell keys during the window between mode
// entry and the first WindowSizeMsg, and can emit a drill-in intent computed
// against a viewport of zero rows.
func TestHandleKeyDeclinesEveryKeyBeforeThePaneIsSized(t *testing.T) {
	t.Parallel()

	keys := []tea.KeyMsg{
		{Type: tea.KeyDown},
		{Type: tea.KeyUp},
		{Type: tea.KeyEnter},
		{Type: tea.KeyLeft},
		{Type: tea.KeyRight},
		{Type: tea.KeyRunes, Runes: []rune("j")},
	}

	for _, height := range []int{0, -1} {
		for _, key := range keys {
			m := unsizedModel(t)
			before := *m

			consumed, intent, cmd := m.HandleKey(key, 80, height)

			if consumed {
				t.Errorf("height %d: key %v was consumed, want it handed back to the shell", height, key)
			}
			if intent != nil {
				t.Errorf("height %d: key %v produced a drill-in intent %+v", height, key, intent)
			}
			if cmd != nil {
				t.Errorf("height %d: key %v produced a command", height, key)
			}
			if m.ContentScrollOffset != before.ContentScrollOffset {
				t.Errorf("height %d: key %v moved the content offset %d -> %d",
					height, key, before.ContentScrollOffset, m.ContentScrollOffset)
			}
			if m.FocusPane != before.FocusPane {
				t.Errorf("height %d: key %v moved focus", height, key)
			}
		}
	}
}

// ClampScroll carries the identical guard and shares its failure mode: with no
// height there is no geometry to clamp against, so clamping would drive every
// offset to zero and lose the operator's scroll position.
func TestClampScrollLeavesOffsetsAloneBeforeThePaneIsSized(t *testing.T) {
	t.Parallel()

	for _, height := range []int{0, -1} {
		m := unsizedModel(t)
		m.DependenciesScrollOffset = 3
		m.MetadataScrollOffset = 2

		m.ClampScroll(80, height)

		if m.ContentScrollOffset != 7 || m.DependenciesScrollOffset != 3 || m.MetadataScrollOffset != 2 {
			t.Errorf("height %d: offsets moved to content=%d deps=%d meta=%d, want 7/3/2",
				height, m.ContentScrollOffset, m.DependenciesScrollOffset, m.MetadataScrollOffset)
		}
	}
}

// The positive-height case proves the guards are a precondition rather than a
// blanket refusal: with a real viewport the same key is consumed.
func TestHandleKeyConsumesTheSameKeyOnceThePaneIsSized(t *testing.T) {
	t.Parallel()

	m := unsizedModel(t)

	consumed, _, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, 80, 10)
	if !consumed {
		t.Fatal("a sized pane declined a scroll key")
	}
}
