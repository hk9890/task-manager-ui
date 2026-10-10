package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
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

// Search holds an auto-refresh back while its query is typed. The change is
// still owed once the typing ends.
func TestStoreChangeWhileTheSearchQueryIsTypedReloadsWhenTheTypingEnds(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})
	// Two steps: a landing search ends the typing, so the first one lands first.
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlAt}})
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}})
	if m.active != mode.Search {
		t.Fatalf("setup: active = %v, want Search", m.active)
	}

	mark := repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	if got := repo.CallCountSince(mark, fakes.MethodSearch); got != 0 {
		t.Fatalf("search reads while the query is typed = %d, want 0", got)
	}

	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlU}})
	if got := repo.CallCountSince(mark, fakes.MethodSearch); got != 1 {
		t.Errorf("search reads once the typing ended = %d, want 1: the change was dropped", got)
	}
}

// The same debt on a store with no watch, where a write of this process leaves
// the tabs dirty: a tab that could not reload is still owed the reload. The
// search the operator submits pays it, and no second one follows.
func TestOwnWriteWhileTheSearchQueryIsTypedReloadsWhenTheTypingEnds(t *testing.T) {
	t.Parallel()

	for name, end := range map[string]tea.KeyMsg{
		"draft cleared":   {Type: tea.KeyCtrlU},
		"draft submitted": {Type: tea.KeyEnter},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, _ := watchedModel(t, repo, RuntimeOptions{DisableAutoRefresh: true})
			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlAt}})
			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}})
			if m.active != mode.Search || m.storeWatched() {
				t.Fatalf("setup: active = %v, watched = %v; want Search on a store with no watch", m.active, m.storeWatched())
			}

			mark := repo.CallCount()
			m = applyMessages(t, m, []tea.Msg{mutationResultMsg{kind: mutationComment, issueID: "tm-1"}})
			if got := repo.CallCountSince(mark, fakes.MethodSearch); got != 0 {
				t.Fatalf("search reads while the query is typed = %d, want 0", got)
			}

			applyMessages(t, m, []tea.Msg{end})
			if got := repo.CallCountSince(mark, fakes.MethodSearch); got != 1 {
				t.Errorf("search reads once the typing ended = %d, want 1", got)
			}
		})
	}
}

// The Search preview draws the selected issue from the detail the shell holds,
// so that detail must follow the results it sits next to.
func TestStoreChangeReloadsTheSearchPreviewWithTheResults(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyCtrlAt}})
	if m.active != mode.Search || firstSelectionID(m, mode.Search) != "tm-1" {
		t.Fatalf("setup: active = %v, selection = %q", m.active, firstSelectionID(m, mode.Search))
	}

	mark := repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	if got := repo.CallCountSince(mark, fakes.MethodSearch); got != 1 {
		t.Errorf("search reads after one store change = %d, want 1", got)
	}
	if got := repo.CallCountSince(mark, fakes.MethodIssue); got != 1 {
		t.Errorf("preview reads after one store change = %d, want 1: the preview is behind its row", got)
	}

	mark = repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})
	if got := storeReads(repo, mark); got != 0 {
		t.Errorf("store reads on a later message = %d, want 0", got)
	}
}

// The watch starts a load at a moment the operator does not choose, so its
// result can land under an overlay opened a moment later.
func TestALoadAStoreChangeStartedLandsUnderAnOverlay(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(m *Model){
		"help modal":   func(m *Model) { m.showHelp = true },
		"action modal": func(m *Model) { m.showActionModal = true },
	} {
		t.Run(name+"/detail", func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, _ := watchedModel(t, repo, RuntimeOptions{})
			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEnter}})
			if m.active != mode.Detail {
				t.Fatalf("setup: active = %v, want Detail", m.active)
			}

			next, reload := m.Update(storeChangedMsg{})
			m = next.(Model)
			open(&m)
			m = applyMessages(t, m, runBatch(reload))

			if m.detail.IsLoading() {
				t.Error("the overlay swallowed the detail result: Detail loads for good, and no later change or reload key reaches it")
			}
		})

		t.Run(name+"/selection", func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, _ := watchedModel(t, repo, RuntimeOptions{})

			next, reload := m.Update(storeChangedMsg{})
			m = next.(Model)
			open(&m)
			seedReady(repo.TrackedRepository, "tm-1", "Retitled by another process", "task", 1)
			m = applyMessages(t, m, runBatch(reload))

			selection := m.selectedByMode[mode.Board]
			if selection == nil || selection.Issue.Title != "Retitled by another process" {
				t.Errorf("the overlay swallowed the selection of the reloaded board: the shell still acts on %+v", selection)
			}
		})
	}
}

// An overlay consumes keys and the messages of its own modal. A mutation result
// is neither, and its handler is the only thing that reports the write.
func TestMutationResultUnderTheHelpOverlayShowsItsToastAndReloadsWhenTheOverlayCloses(t *testing.T) {
	t.Parallel()

	for name, runtime := range map[string]RuntimeOptions{
		"watched store":   {},
		"unwatched store": {DisableAutoRefresh: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, _ := watchedModel(t, repo, runtime)

			m.showHelp = true
			mark := repo.CallCount()
			m = applyMessages(t, m, []tea.Msg{mutationResultMsg{kind: mutationComment, issueID: "tm-1"}})
			if m.storeWatched() {
				m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
			}

			if !m.toast.Visible() || !strings.Contains(m.toast.View(), "Added comment to tm-1") {
				t.Errorf("the overlay swallowed the mutation result: toast = %q", m.toast.View())
			}
			if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 0 {
				t.Errorf("board reads under the overlay = %d, want 0", got)
			}

			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}})
			if m.showHelp {
				t.Fatal("setup: the help key did not close the overlay")
			}
			if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
				t.Errorf("board reads once the overlay closed = %d, want 1", got)
			}
		})
	}
}

// A dialog that opened over an overlay would replace the modal on screen.
func TestCatalogResultUnderAnOverlayOpensNoDialog(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})

	m.pendingDialog = pendingDialogGuard{active: true, kind: mutationStatus}
	m.showHelp = true
	m = applyMessages(t, m, []tea.Msg{mutationCatalogsLoadedMsg{kind: mutationStatus}})

	if m.showActionModal || m.pendingDialog.active {
		t.Errorf("a status dialog opened or stayed pending under the help overlay (open = %v, pending = %v)",
			m.showActionModal, m.pendingDialog.active)
	}
}

// The watch signals the writes of this process too. The handler of the write
// and the signal must not both reload.
func TestOneCommentSubmitIsOneBoardRead(t *testing.T) {
	t.Parallel()

	for name, runtime := range map[string]RuntimeOptions{
		"watched store":   {},
		"unwatched store": {DisableAutoRefresh: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newWatchedRepository()
			m, _ := watchedModel(t, repo, runtime)
			// The modal is opened without its Init: a drain would follow the
			// cursor blink of the input for good.
			m.actionState = buildMutationDialog(mutationComment, domain.IssueSummary{ID: "tm-1"}, nil, nil, nil)
			m.showActionModal = true

			mark := repo.CallCount()
			m = applyMessages(t, m, []tea.Msg{modal.SubmitMsg{Values: map[string]string{"body": "seen"}}})
			if m.storeWatched() {
				m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
			}

			if got := repo.CallCountSince(mark, fakes.MethodAddComment); got != 1 {
				t.Fatalf("setup: comment writes = %d, want 1", got)
			}
			if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
				t.Errorf("board reads for one comment submit = %d, want 1", got)
			}
		})
	}
}

func TestFocusRegainOnAWatchedStoreReadsNothing(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})

	mark := repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{tea.BlurMsg{}, tea.FocusMsg{}})
	if got := storeReads(repo, mark); got != 0 {
		t.Errorf("store reads on focus regain = %d, want 0: the watch kept the board current", got)
	}
	if !m.terminalFocused {
		t.Error("the focus regain was not recorded")
	}
}

// The watch reloads a Detail whose load failed on every store change. The
// operator is told one time that the issue is gone.
func TestDetailLoadFailureToastShowsOnceForOneIssue(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEnter}})
	if m.active != mode.Detail {
		t.Fatalf("setup: active = %v, want Detail", m.active)
	}

	// Another process deleted the issue.
	repo.SetError(fakes.MethodIssue, errors.New("issue not found"))
	mark, shown := repo.CallCount(), m.toast.Seq()
	// One at a time: changes that arrive during one load are one reload.
	for range 3 {
		m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	}

	if got := repo.CallCountSince(mark, fakes.MethodIssue); got != 3 {
		t.Fatalf("setup: detail reads after three store changes = %d, want 3", got)
	}
	if got := m.toast.Seq() - shown; got != 1 {
		t.Errorf("failure toasts after three failed reloads = %d, want 1", got)
	}
	if !strings.Contains(m.toast.View(), "Failed to load selected issue details") {
		t.Errorf("toast = %q, want the detail load failure", m.toast.View())
	}

	// A load that succeeds ends the failure, so the next one is news again.
	repo.SetError(fakes.MethodIssue, nil)
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	repo.SetError(fakes.MethodIssue, errors.New("issue not found"))
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})
	if got := m.toast.Seq() - shown; got != 2 {
		t.Errorf("failure toasts after a recovery and a new failure = %d, want 2", got)
	}

	// The reload key is a question, and the answer is not held back.
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}})
	if got := m.toast.Seq() - shown; got != 3 {
		t.Errorf("failure toasts after the reload key = %d, want 3", got)
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

// The watch can end in the burst of a write and send no signal for it: the
// operating system refused the watch on the directory the write created. No
// handler reloads for a write on a watched store, so the end of the watch must.
func TestAWriteWhoseSignalTheEndingWatchDroppedIsRead(t *testing.T) {
	t.Parallel()

	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})

	mark := repo.CallCount()
	m = applyMessages(t, m, []tea.Msg{
		mutationResultMsg{kind: mutationComment, issueID: "tm-1"},
		storeWatchEndedMsg{},
	})

	if m.storeWatched() {
		t.Fatal("setup: the store is still watched")
	}
	if got := repo.CallCountSince(mark, fakes.MethodDashboard); got != 1 {
		t.Errorf("board reads after a write and the end of the watch = %d, want 1", got)
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
