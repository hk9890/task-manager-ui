package app

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// watchedRepository gives a tracked repository the optional change watch. The
// tests send storeChangedMsg themselves; the channel is only what the shell
// holds to know the store is watched.
type watchedRepository struct {
	*fakes.TrackedRepository
	changes  chan struct{}
	err      error
	watches  int
	watchCtx context.Context
}

func (w *watchedRepository) WatchChanges(ctx context.Context) (<-chan struct{}, error) {
	w.watches++
	w.watchCtx = ctx
	if w.err != nil {
		return nil, w.err
	}
	return w.changes, nil
}

func newWatchedRepository() *watchedRepository {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}})
	return &watchedRepository{TrackedRepository: gw, changes: make(chan struct{}, 1)}
}

// watchedModel returns a started model over repo and a counter of the waits
// armed on the store's change channel.
func watchedModel(t *testing.T, repo *watchedRepository, runtime RuntimeOptions) (Model, *int) {
	t.Helper()

	services, err := NewServices(repo, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModelWithOptions(t, services, runtime)
	waits := new(int)
	m.awaitStoreChange = func(<-chan struct{}) tea.Cmd {
		*waits++
		return nil
	}
	return applyMessages(t, m, runBatch(m.Init())), waits
}

func storeReads(repo *watchedRepository, mark int) int {
	return repo.CallCountSince(mark, fakes.MethodDashboard) +
		repo.CallCountSince(mark, fakes.MethodSearch) +
		repo.CallCountSince(mark, fakes.MethodIssue)
}

func TestStoreWatchStartsWithTheStoreAndArmsOneWait(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, waits := watchedModel(t, repo, RuntimeOptions{})

	if repo.watches != 1 {
		t.Fatalf("WatchChanges calls = %d, want 1", repo.watches)
	}
	if repo.watchCtx != m.ctx {
		t.Error("the watch does not run on the store's context, so a store switch would not end it")
	}
	if *waits != 1 {
		t.Errorf("waits armed by Init = %d, want 1", *waits)
	}
}

func TestStoreWatchIsOffUnderNoAutoRefresh(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	_, waits := watchedModel(t, repo, RuntimeOptions{DisableAutoRefresh: true})

	if repo.watches != 0 || *waits != 0 {
		t.Errorf("WatchChanges calls = %d, waits = %d; --no-auto-refresh must start no watch", repo.watches, *waits)
	}
}

func TestStoreWatchThatCannotStartLeavesTheAppOnTheTick(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	repo.err = errors.New("too many open files")
	m, waits := watchedModel(t, repo, RuntimeOptions{})

	if m.storeChanges != nil || *waits != 0 {
		t.Errorf("a failed watch left a channel or armed a wait (waits = %d)", *waits)
	}
	if m.fatalErrTitle != "" || m.toast.Visible() {
		t.Error("a failed watch is not the operator's problem: no fatal screen and no toast")
	}
}

func TestBackendWithoutChangeWatchArmsNoWait(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)

	if m.storeChanges != nil || m.waitForStoreChangeCmd() != nil {
		t.Error("a backend that is no ChangeWatcher must leave the shell without a watch")
	}
}

func TestStoreChangeReloadsTheBoardAlsoWhenTheTerminalIsNotFocused(t *testing.T) {
	t.Parallel()

	for name, blur := range map[string]bool{"focused": false, "not focused": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, waits := watchedModel(t, repo, RuntimeOptions{})
			if blur {
				m = applyMessages(t, m, []tea.Msg{tea.BlurMsg{}})
			}

			mark, armed := repo.CallCount(), *waits
			m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})

			if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
				t.Errorf("board reads after one store change = %d, want 1", got)
			}
			if *waits != armed+1 {
				t.Errorf("waits armed after one store change = %d, want 1", *waits-armed)
			}

			mark = repo.CallCount()
			m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})
			if got := storeReads(repo, mark); got != 0 {
				t.Errorf("store reads on a later message = %d, want 0: one change is one reload", got)
			}
		})
	}
}

// A message an overlay consumes never reaches its handler, and the wait
// re-arms only from there.
func TestStoreChangeUnderAnOverlayKeepsTheWatchAndReloadsWhenTheOverlayCloses(t *testing.T) {
	t.Parallel()

	for name, overlay := range map[string]struct{ open, shut func(m *Model) }{
		"help modal":   {func(m *Model) { m.showHelp = true }, func(m *Model) { m.showHelp = false }},
		"action modal": {func(m *Model) { m.showActionModal = true }, func(m *Model) { m.showActionModal = false }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, waits := watchedModel(t, repo, RuntimeOptions{})

			overlay.open(&m)
			mark, armed := repo.CallCount(), *waits
			m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})

			if *waits != armed+1 {
				t.Fatal("the overlay swallowed the store change: nothing re-armed the wait, so the watch is dead for the session")
			}
			if got := storeReads(repo, mark); got != 0 {
				t.Errorf("store reads under the overlay = %d, want 0", got)
			}

			overlay.shut(&m)
			m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})
			if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
				t.Errorf("board reads once the overlay closed = %d, want 1", got)
			}
		})
	}
}

func TestStoreChangeDuringALoadReloadsWhenThatLoadLands(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})

	mark := repo.CallCount()
	next, load := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	if !m.board.IsLoading() {
		t.Fatal("setup: the board reload key started no load")
	}

	next, cmd := m.Update(storeChangedMsg{})
	m = next.(Model)
	if msgs := runBatch(cmd); len(msgs) != 0 {
		t.Fatalf("a store change during a load issued work at once: %#v", msgs)
	}

	m = applyMessages(t, m, runBatch(load))
	if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 2 {
		t.Errorf("board reads = %d, want 2: the load in flight and one more for the change it may have missed", got)
	}
	if m.board.IsLoading() {
		t.Error("the board is still loading after every command ran")
	}
}

func TestStoreChangeReloadsTheOpenDetail(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEnter}})
	if m.active != mode.Detail {
		t.Fatalf("setup: active = %v, want Detail", m.active)
	}

	mark := repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	if got := repo.CallCountSince(mark, fakes.MethodIssue); got != 1 {
		t.Errorf("detail reads after one store change = %d, want 1", got)
	}

	// The board below was not reloaded, so it is still behind the store.
	mark = repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEsc}})
	if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
		t.Errorf("board reads on return from Detail = %d, want 1", got)
	}
}

func TestStoreWatchEndStopsTheWaitAndAStaleStoreCannotEndIt(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, waits := watchedModel(t, repo, RuntimeOptions{})

	armed := *waits
	m = applyMessages(t, m, []tea.Msg{scopedMsg{epoch: m.storeEpoch - 1, msg: storeWatchEndedMsg{}}})
	if m.storeChanges == nil {
		t.Fatal("the end of a previous store's watch ended the active store's")
	}

	m = applyMessages(t, m, []tea.Msg{scopedMsg{epoch: m.storeEpoch, msg: storeWatchEndedMsg{}}})
	if m.storeChanges != nil || *waits != armed {
		t.Errorf("an ended watch left a channel or re-armed a wait (waits = %d)", *waits-armed)
	}
}

func TestDefaultAwaitStoreChangeReportsAChangeAndTheEnd(t *testing.T) {
	t.Parallel()

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	if msg, ok := defaultAwaitStoreChange(changes)().(storeChangedMsg); !ok {
		t.Errorf("a value on the channel gave %T, want storeChangedMsg", msg)
	}

	close(changes)
	if msg, ok := defaultAwaitStoreChange(changes)().(storeWatchEndedMsg); !ok {
		t.Errorf("a closed channel gave %T, want storeWatchEndedMsg", msg)
	}
}
