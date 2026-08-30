package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
)

// detailModelOnIssue returns a model in Detail with tm-1 loaded and settled.
func detailModelOnIssue(t *testing.T, gw *fakes.TrackedRepository) Model {
	t.Helper()

	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail body",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("setup: expected Detail active, got %s", m.active)
	}
	return m
}

// TestPostMutationDetailReloadReportsItsLoad pins the pairing the load protocol
// requires: "BeginLoad must be what precedes every loadDetailCmd"
// (internal/mode/detail/model.go).
//
// handleMutationResult was the one site that issued a bare loadDetailCmd, so
// after update / close / comment / status / priority the header showed neither
// the spinner nor "Loading: detail" while the read ran, and a browse selection
// that moved in that window made the response fail the target-id guard and be
// thrown away.
func TestPostMutationDetailReloadReportsItsLoad(t *testing.T) {
	t.Parallel()

	for _, kind := range []mutationKind{mutationUpdate, mutationClose, mutationComment, mutationStatus, mutationPriority} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			gw := fakes.NewTracked()
			m := detailModelOnIssue(t, gw)

			next, _ := m.Update(mutationResultMsg{kind: kind, issueID: "tm-1"})
			m = next.(Model)

			if !m.detail.IsLoading() {
				t.Error("the post-mutation reload runs with detail.loading false: no spinner and no status line while it waits")
			}
			if m.detail.TargetID() != "tm-1" {
				t.Errorf("detail target after the mutation = %q, want tm-1; the response fails the target guard", m.detail.TargetID())
			}

			states := m.loadingStates()
			found := false
			for _, state := range states {
				if state.Scope == loading.ScopeDetail {
					found = true
				}
			}
			if !found {
				t.Errorf("loadingStates() reports %v, with no detail scope for the reload in flight", states)
			}
		})
	}
}

// TestRepeatedDetailRefreshIsSuppressedWhileOneIsInFlight pins the in-flight
// guard every browse tab has and Detail did not. Each repeat press issued
// another read against the store, and each extra ApplyLoadedDetail decremented
// the live drill-focus counter.
func TestRepeatedDetailRefreshIsSuppressedWhileOneIsInFlight(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := detailModelOnIssue(t, gw)

	reload := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}

	mark := gw.CallCount()
	next, first := m.Update(reload)
	m = next.(Model)
	if first == nil {
		t.Fatal("the first reload issued no command")
	}

	// Press r again without letting the first response land.
	next, second := m.Update(reload)
	m = next.(Model)
	if second != nil {
		if msgs := runBatch(second); len(msgs) > 0 {
			t.Errorf("a second reload dispatched work while the first was in flight: %d messages", len(msgs))
		}
	}

	runBatch(first)
	if got := gw.CallCountSince(mark, fakes.MethodIssue); got != 1 {
		t.Errorf("holding r issued %d reads of the same issue, want 1", got)
	}
}

// TestGuardedSelectionPathStillAnchorsTheDetail pins the writes that moved into
// BeginLoad. ensureDetailForCurrentSelectionCmd skips BeginLoad on two paths —
// the target is already loading, or already loaded and not due for a refresh —
// and the selection id and the rail anchor were written nowhere else, so a
// re-fired selection for the row already on screen left the rail highlighting
// whichever dependency row the cursor was last moved to.
func TestGuardedSelectionPathStillAnchorsTheDetail(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := detailModelOnIssue(t, gw)

	if got := m.detail.SelectionID(); got != "tm-1" {
		t.Fatalf("setup: selection id = %q, want tm-1", got)
	}

	// Something clears the anchor while the same issue stays on screen — the
	// rail cursor moving off the issue is what does it in the app.
	m.detail.SelectBrowserIssue("")
	m.detail.AnchorSelection("tm-other")

	// The selection is re-reported for the row already loaded: both guards hit.
	m = applyMessages(t, m, []tea.Msg{mode.SelectionChangedMsg{
		Mode:      mode.Board,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-1", Title: "Ready first"}},
	}})

	if got := m.detail.SelectionID(); got != "tm-1" {
		t.Errorf("selection id after a guarded re-selection = %q, want tm-1; RenderDetail decides preview-vs-full from this value", got)
	}
}
