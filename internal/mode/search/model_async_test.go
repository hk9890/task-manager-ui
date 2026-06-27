package search

// Controller-async contract tests.
//
// These tests exercise the controller against a deliberately-delayed repository
// so that async command overlap is exercised — the gap that hid the Enter-drop bug.
//
// # Why a separate test tier
//
// The existing model_test.go helpers (pressAndResolve → ApplyControllerKeySequence)
// synchronously drain every Cmd before the next key arrives. That means m.loading
// is always false by the time the next key is processed, making the race window
// that made the Enter-drop race window completely invisible.
//
// Here we use a goroutine-based driver: the search Cmd runs in a goroutine
// (blocked inside fakes.DelayingRepository.Search), while the test synchronously
// sends additional key presses to the model. Release() unblocks the goroutine,
// which returns the Msg to the model for processing. This matches real tea.Program
// cadence: user events can arrive before a prior async Cmd returns its Msg.
//
// # Regression pin
//
// Each of these tests passes on current code (post-commit 2d60d94). If commit
// 2d60d94 were reverted (removing pendingDraft from model.go and re-introducing
// --status all in lean_reads.go), the following tests would fail:
//
//   - TypeAndEnterDuringInitialLoad_EventuallySubmitsTypedQuery
//   - TypeAndEnterDuringPriorSearch_EventuallySubmitsLatestDraft
//   - EnterIsNotSilentlyDropped
//   - HasDraftChangesResolves
//   - EmptyAutoInitDoesNotLeakClosedRowsUnderTypedDraft

import (
	"context"
	"errors"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// ---- queryRecordingRepo ----

// queryRecordingRepo wraps a repository.Repository and records all Search
// queries for inspection in assertions. All calls are delegated to the inner
// repository.
type queryRecordingRepo struct {
	repository.Repository
	mu      sync.Mutex
	queries []domain.SearchIssuesQuery
}

func (r *queryRecordingRepo) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	r.mu.Lock()
	r.queries = append(r.queries, query)
	r.mu.Unlock()
	return r.Repository.Search(ctx, query)
}

func (r *queryRecordingRepo) Queries() []domain.SearchIssuesQuery {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SearchIssuesQuery, len(r.queries))
	copy(out, r.queries)
	return out
}

// ---- erroringSearchRepo ----

// erroringSearchRepo wraps a repository.Repository and forces every Search call
// to return err (delegating all other methods). It drives the searchLoadedMsg
// ERROR branch through the real repository seam — combined with
// fakes.DelayingRepository it produces a genuine in-flight search that resolves
// with an error, exactly as a failing backend would.
type erroringSearchRepo struct {
	repository.Repository
	err error
}

func (r *erroringSearchRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{}, r.err
}

// ---- test driver helpers ----

// runCmdAsync executes cmd in a goroutine and returns a channel that receives
// the single resulting tea.Msg. The caller must eventually read from the
// channel (or the goroutine will leak if the test exits early).
func runCmdAsync(cmd tea.Cmd) <-chan tea.Msg {
	ch := make(chan tea.Msg, 1)
	if cmd == nil {
		close(ch)
		return ch
	}
	go func() {
		ch <- cmd()
	}()
	return ch
}

// ---- bdDefaultFilterRepo ----

// bdDefaultFilterRepo mirrors the real taskmgr search backend's default behaviour:
// when SearchIssuesQuery.Statuses is empty, closed issues are excluded from
// results. This lets async contract tests assert on result set content rather
// than relying only on the query-shape proxy.
//
// In the memory repository, an empty Statuses field returns every seeded issue
// (open and closed alike). Stacking bdDefaultFilterRepo between the inner repo
// and the test doubles restores the taskmgr-default contract so tests can call
//
//	if result.Issue.Status == "closed" { t.Errorf(...) }
//
// without their assertions becoming vacuously true on the memory backend.
type bdDefaultFilterRepo struct{ repository.Repository }

func (r *bdDefaultFilterRepo) Search(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	page, err := r.Repository.Search(ctx, q)
	if err != nil || len(q.Statuses) > 0 {
		return page, err
	}
	// No explicit Statuses filter → apply taskmgr default: exclude closed.
	filtered := page.Results[:0]
	for _, res := range page.Results {
		if res.Issue.Status != "closed" {
			filtered = append(filtered, res)
		}
	}
	page.Results = filtered
	return page, nil
}

// ---- controller-async contract tests ----

// TestSearchControllerAsyncContracts is the parent test for the five
// controller-async contract subtests. Each subtest exercises the
// search controller against a fakes.DelayingRepository to simulate real
// tea.Program cadence: user events may arrive before a prior async Cmd
// returns its Msg.
func TestSearchControllerAsyncContracts(t *testing.T) {
	t.Parallel()

	// TypeAndEnterDuringInitialLoad_EventuallySubmitsTypedQuery verifies that
	// when the user types "task" and presses Enter while Init's empty-query
	// search is still in flight, the result set eventually reflects the typed
	// query (not the Init empty-query results).
	//
	// Regression pin: on pre-2d60d94 code (no pendingDraft), Enter while
	// loading was silently dropped and appliedQuery remained "" after Init
	// resolved.
	t.Run("TypeAndEnterDuringInitialLoad_EventuallySubmitsTypedQuery", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task alpha", Status: "open", Type: "task", Priority: 1})
		inner.Seed(memoryrepo.Issue{ID: "bwf-2", Title: "bug beta", Status: "open", Type: "bug", Priority: 2})
		inner.Seed(memoryrepo.Issue{ID: "bwf-3", Title: "closed task", Status: "closed", Type: "task", Priority: 3})

		delayed := fakes.NewDelayedSearchRepository(inner)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		// Start Init — the Cmd will block in delayed.Search.
		initCmd := m.Init()
		if initCmd == nil {
			t.Fatal("expected non-nil Cmd from Init()")
		}
		initMsgCh := runCmdAsync(initCmd)

		// Verify loading before the search returns.
		if !m.loading {
			t.Fatal("expected loading=true before Init resolves")
		}

		// Type "task" synchronously — model updates state only, no Cmds.
		for _, r := range []rune("task") {
			cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			if cmd != nil {
				t.Fatalf("unexpected non-nil Cmd from rune input: %v", cmd)
			}
		}
		if m.draftQuery != "task" {
			t.Fatalf("draftQuery: got %q, want %q", m.draftQuery, "task")
		}

		// Press Enter while Init is still in flight.
		enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if enterCmd != nil {
			t.Fatal("expected nil Cmd from Enter while loading (should queue, not fire)")
		}
		if m.pendingDraft == nil || *m.pendingDraft != "task" {
			t.Fatalf("expected pendingDraft=%q, got %v", "task", m.pendingDraft)
		}

		// Unblock Init search.
		delayed.Release()
		initMsg := <-initMsgCh

		// Deliver Init's result — should consume pendingDraft and fire "task" search.
		pendingCmd := m.Update(initMsg)
		if pendingCmd == nil {
			t.Fatal("expected non-nil Cmd after Init resolves with queued pendingDraft")
		}
		if m.pendingDraft != nil {
			t.Fatalf("expected pendingDraft cleared, got %v", *m.pendingDraft)
		}

		// Unblock and drain the "task" search.
		delayed.Release()
		taskMsgCh := runCmdAsync(pendingCmd)
		taskMsg := <-taskMsgCh

		_ = m.Update(taskMsg)

		// Assert the typed query was applied.
		if m.appliedQuery != "task" {
			t.Fatalf("appliedQuery: got %q, want %q", m.appliedQuery, "task")
		}
		if m.loading {
			t.Fatal("expected loading=false after task search resolves")
		}
	})

	// TypeAndEnterDuringPriorSearch_EventuallySubmitsLatestDraft verifies that
	// when the user types "bar" + Enter while a previous "foo" search is still
	// in flight, the final visible page reflects "bar" (the latest draft wins;
	// the "foo" result is discarded).
	//
	// Regression pin: on pre-2d60d94 code, Enter during a loading state was
	// silently dropped so "bar" would never be applied.
	t.Run("TypeAndEnterDuringPriorSearch_EventuallySubmitsLatestDraft", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "foo issue", Status: "open", Type: "task", Priority: 1})
		inner.Seed(memoryrepo.Issue{ID: "bwf-2", Title: "bar issue", Status: "open", Type: "task", Priority: 2})
		inner.Seed(memoryrepo.Issue{ID: "bwf-3", Title: "unrelated", Status: "open", Type: "task", Priority: 3})

		delayed := fakes.NewDelayedSearchRepository(inner)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		// Step 1: Init fires — let it complete immediately.
		initCmd := m.Init()
		delayed.Release() // unblock Init search
		initMsgCh := runCmdAsync(initCmd)
		initMsg := <-initMsgCh
		cmd := m.Update(initMsg)
		// drain selectionChangedCmd
		for _, msg := range drainCmd(cmd) {
			_ = m.Update(msg)
		}

		if m.loading {
			t.Fatal("setup: expected loading=false after Init resolves")
		}

		// Step 2: Type "foo" + Enter to start a "foo" search (stays in flight).
		for _, r := range []rune("foo") {
			_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		fooCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if fooCmd == nil {
			t.Fatal("expected non-nil Cmd from Enter for 'foo' search")
		}
		if !m.loading {
			t.Fatal("expected loading=true after 'foo' search starts")
		}
		fooMsgCh := runCmdAsync(fooCmd)

		// Step 3: While "foo" is in flight, type "bar" + Enter.
		// Simulate the user clearing the query and typing the next search.
		m.draftQuery = ""
		for _, r := range []rune("bar") {
			_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		barEnterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if barEnterCmd != nil {
			t.Fatal("expected nil Cmd from Enter while 'foo' is loading (should queue)")
		}
		if m.pendingDraft == nil || *m.pendingDraft != "bar" {
			t.Fatalf("expected pendingDraft=%q, got %v", "bar", m.pendingDraft)
		}

		// Step 4: Unblock "foo" search.
		delayed.Release()
		fooMsg := <-fooMsgCh

		// Deliver "foo" result — should consume pendingDraft and fire "bar" search.
		barCmd := m.Update(fooMsg)
		if barCmd == nil {
			t.Fatal("expected non-nil Cmd after 'foo' resolves with queued 'bar' pendingDraft")
		}
		if m.pendingDraft != nil {
			t.Fatalf("expected pendingDraft cleared, got %v", *m.pendingDraft)
		}

		// Step 5: Unblock and drain "bar" search.
		delayed.Release()
		barMsgCh := runCmdAsync(barCmd)
		barMsg := <-barMsgCh
		_ = m.Update(barMsg)

		// Assert: the latest draft wins.
		if m.appliedQuery != "bar" {
			t.Fatalf("appliedQuery: got %q, want %q (latest draft must win)", m.appliedQuery, "bar")
		}
		if m.loading {
			t.Fatal("expected loading=false after 'bar' search resolves")
		}
	})

	// EnterIsNotSilentlyDropped verifies that pressing Enter while Init is in
	// flight is not silently discarded: after all async operations resolve, the
	// applied query must equal the typed draft.
	//
	// Regression pin: on pre-2d60d94 code, m.loading==true caused Enter to
	// return nil without setting pendingDraft, so the query was never applied.
	t.Run("EnterIsNotSilentlyDropped", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task thing", Status: "open", Type: "task", Priority: 1})

		delayed := fakes.NewDelayedSearchRepository(inner)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		initCmd := m.Init()
		initMsgCh := runCmdAsync(initCmd)

		// Type "task" and press Enter while Init is in flight.
		for _, r := range []rune("task") {
			_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Unblock Init.
		delayed.Release()
		initMsg := <-initMsgCh
		pendingCmd := m.Update(initMsg)

		// pendingCmd must be non-nil: Enter was not silently dropped.
		if pendingCmd == nil {
			t.Fatal("Enter was silently dropped: no Cmd emitted after Init resolved with queued pendingDraft")
		}

		// Unblock the "task" search.
		delayed.Release()
		taskMsgCh := runCmdAsync(pendingCmd)
		taskMsg := <-taskMsgCh
		_ = m.Update(taskMsg)

		if m.appliedQuery != "task" {
			t.Fatalf("Enter was silently dropped: appliedQuery=%q, want %q", m.appliedQuery, "task")
		}
	})

	// HasDraftChangesResolves verifies that after the sequence "open → type
	// 'task' → Enter → drain all", the state satisfies draftQuery ==
	// appliedQuery so hasDraftChanges is false and the "stale results" banner
	// is absent.
	//
	// Regression pin: on pre-2d60d94 code, Enter during loading was dropped so
	// appliedQuery never matched draftQuery and the stale banner stayed visible.
	t.Run("HasDraftChangesResolves", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task item", Status: "open", Type: "task", Priority: 1})

		delayed := fakes.NewDelayedSearchRepository(inner)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		initCmd := m.Init()
		initMsgCh := runCmdAsync(initCmd)

		// Type + Enter while Init is in flight.
		for _, r := range []rune("task") {
			_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Unblock Init.
		delayed.Release()
		initMsg := <-initMsgCh
		taskCmd := m.Update(initMsg)

		// Unblock and drain the "task" search.
		delayed.Release()
		taskMsgCh := runCmdAsync(taskCmd)
		taskMsg := <-taskMsgCh
		// Deliver the result; drain any follow-up Cmds (e.g. selectionChangedCmd).
		followCmd := m.Update(taskMsg)
		for _, msg := range drainCmd(followCmd) {
			_ = m.Update(msg)
		}

		// hasDraftChanges is defined as draftQuery != appliedQuery in the view layer.
		if m.draftQuery != m.appliedQuery {
			t.Fatalf("hasDraftChanges should be false: draftQuery=%q, appliedQuery=%q", m.draftQuery, m.appliedQuery)
		}
		if m.loading {
			t.Fatal("expected loading=false after all searches resolve")
		}
	})

	// EmptyAutoInitDoesNotLeakClosedRowsUnderTypedDraft verifies that when the
	// user types a query + Enter after Init, the model does NOT inject a forced
	// Statuses filter into the SearchIssuesQuery it passes to the repository.
	// The absence of Statuses: []string{"all"} is the controller-level contract
	// that corresponds to the lean_reads.go change in commit 2d60d94.
	//
	// Before 2d60d94: lean_reads forced filterStatuses = []string{"all"} when
	// query.Statuses was empty, overriding taskmgr search's own default (which
	// excludes closed). This caused the Init result page — containing closed
	// issues — to remain visible when Enter was silently dropped.
	//
	// Regression pin: if the model were to set Statuses: []string{"all"} in
	// the SearchIssuesQuery it emits, this test would fail. Combined with the
	// pendingDraft fix, this is the complete regression guard.
	t.Run("EmptyAutoInitDoesNotLeakClosedRowsUnderTypedDraft", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task open", Status: "open", Type: "task", Priority: 1})
		inner.Seed(memoryrepo.Issue{ID: "bwf-2", Title: "task closed", Status: "closed", Type: "task", Priority: 2})

		// Stack: inner → bdDefaultFilterRepo (taskmgr-default closed exclusion) → queryRecordingRepo → delayed.
		// This lets assertions check both query-shape (no forced Statuses) and
		// result-set content (no closed issues in the final page).
		filtered := &bdDefaultFilterRepo{Repository: inner}
		recording := &queryRecordingRepo{Repository: filtered}
		delayed := fakes.NewDelayedSearchRepository(recording)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		initCmd := m.Init()
		initMsgCh := runCmdAsync(initCmd)

		// Type "task" + Enter while Init is in flight.
		for _, r := range []rune("task") {
			_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Unblock Init.
		delayed.Release()
		initMsg := <-initMsgCh
		taskCmd := m.Update(initMsg)

		// Unblock and drain the "task" search.
		delayed.Release()
		taskMsgCh := runCmdAsync(taskCmd)
		taskMsg := <-taskMsgCh
		_ = m.Update(taskMsg)

		// Applied query must be the typed query, not the Init empty string.
		if m.appliedQuery != "task" {
			t.Fatalf("appliedQuery: got %q, want %q", m.appliedQuery, "task")
		}

		// Assert: neither the Init query nor the typed "task" query passed any
		// non-empty Statuses. The model must not inject a forced status filter
		// — that responsibility belongs to the repository layer (taskmgr's own
		// default excludes closed issues on real repos).
		queries := recording.Queries()
		if len(queries) == 0 {
			t.Fatal("expected at least one recorded Search query")
		}
		for _, q := range queries {
			if len(q.Statuses) > 0 {
				t.Errorf("controller injected Statuses=%v into Search query (text=%q); model must not force --status all",
					q.Statuses, q.Text)
			}
		}

		// Verify that a typed-query search was actually executed (not just Init).
		foundTypedQuery := false
		for _, q := range queries {
			if q.Text == "task" {
				foundTypedQuery = true
			}
		}
		if !foundTypedQuery {
			t.Errorf("no Search call with Text=%q found; queries seen: %v", "task", queries)
		}

		// Assert result-set content: the final page must not contain any closed
		// issues. This is the direct symptom: when Enter was dropped
		// and lean_reads forced --status all, the Init result (which included the
		// closed issue) remained visible.
		//
		// With bdDefaultFilterRepo in the stack, the memory repo behaves like the
		// real taskmgr backend: closed issues are excluded when Statuses is empty. If
		// the model were to inject Statuses:[]string{"all"} (reverting lean_reads),
		// bdDefaultFilterRepo passes through all issues and this assertion fails.
		if len(m.page.Results) == 0 {
			t.Error("expected non-empty Results after 'task' search resolves")
		}
		for _, result := range m.page.Results {
			if result.Issue.Status == "closed" {
				t.Errorf("closed issue %q leaked into result set; model or repo injected --status all", result.Issue.ID)
			}
		}
	})

	// PendingDraftFiresWhenInFlightSearchErrors verifies that a queued Enter-submit
	// survives an in-flight search that resolves with an ERROR: the user types a
	// query + Enter while a search is in flight (queuing pendingDraft), the
	// in-flight search then fails (searchLoadedMsg{err: ...}), and the model must
	// re-fire a search for the queued text rather than silently dropping it.
	//
	// Regression pin (FIX #4): the searchLoadedMsg ERROR branch used to return
	// without consuming pendingDraft, so a queued submit was lost whenever the
	// in-flight search errored. The fix calls consumePendingDraft(appliedQuery,
	// forceRefire=true) on the error path. If that call were removed, pendingDraft
	// would stay set, no second Search would fire, and m.IsLoading() would stay
	// false — each of the assertions below would fail.
	t.Run("PendingDraftFiresWhenInFlightSearchErrors", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task alpha", Status: "open", Type: "task", Priority: 1})

		// Stack: inner → erroring (every Search fails) → recording (observe queries) → delayed (gate).
		erroring := &erroringSearchRepo{Repository: inner, err: errors.New("backend unavailable")}
		recording := &queryRecordingRepo{Repository: erroring}
		delayed := fakes.NewDelayedSearchRepository(recording)

		m := NewModel(context.Background(), delayed, nil)
		m.SetSize(120, 30)

		// Init fires and blocks inside delayed.Search.
		initCmd := m.Init()
		if initCmd == nil {
			t.Fatal("expected non-nil Cmd from Init()")
		}
		initMsgCh := runCmdAsync(initCmd)
		if !m.loading {
			t.Fatal("expected loading=true before Init resolves")
		}

		// Type "task" + Enter while the search is in flight — Enter must queue,
		// not fire.
		for _, r := range []rune("task") {
			cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			if cmd != nil {
				t.Fatalf("unexpected non-nil Cmd from rune input: %v", cmd)
			}
		}
		enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if enterCmd != nil {
			t.Fatal("expected nil Cmd from Enter while loading (should queue, not fire)")
		}
		if m.pendingDraft == nil || *m.pendingDraft != "task" {
			t.Fatalf("expected pendingDraft=%q, got %v", "task", m.pendingDraft)
		}

		// Unblock the in-flight search — it resolves with an ERROR.
		delayed.Release()
		initMsg := <-initMsgCh
		errMsg, ok := initMsg.(searchLoadedMsg)
		if !ok || errMsg.err == nil {
			t.Fatalf("expected in-flight search to resolve with an error searchLoadedMsg, got %#v", initMsg)
		}

		// Deliver the error result. The ERROR branch must consume the queued draft
		// and re-fire a search for "task" — the queued submit is NOT dropped.
		pendingCmd := m.Update(errMsg)
		if pendingCmd == nil {
			t.Fatal("queued submit dropped: error branch returned no Cmd despite pendingDraft set")
		}
		if m.pendingDraft != nil {
			t.Fatalf("expected pendingDraft cleared after error resolution, got %q", *m.pendingDraft)
		}
		if !m.IsLoading() {
			t.Fatal("expected loading=true: a new search for the queued draft must be in flight")
		}

		// Drain the re-fired search (also errors) so the goroutine does not leak.
		delayed.Release()
		pendingMsg := <-runCmdAsync(pendingCmd)
		_ = m.Update(pendingMsg)

		// A second Search call for the queued text must have been issued.
		queries := recording.Queries()
		foundTask := false
		for _, q := range queries {
			if q.Text == "task" {
				if len(q.Statuses) > 0 {
					t.Errorf("re-fired search injected Statuses=%v; model must not force a status filter", q.Statuses)
				}
				foundTask = true
			}
		}
		if !foundTask {
			t.Errorf("no Search call for queued text %q after in-flight error; queries seen: %v", "task", queries)
		}
	})

	// SameQueryRetryAfterErrorStillRefires pins the forceRefire=true choice on the
	// error path. When the queued draft equals the last applied query, a
	// forceRefire=false consume would NOT re-fire (pending == appliedQuery), so a
	// retry-after-error of the same text would be lost. With forceRefire=true the
	// error branch re-fires regardless. The other sub-test cannot catch this
	// because the failed Init search leaves appliedQuery=="" (≠ the queued text),
	// so it re-fires even with forceRefire=false.
	t.Run("SameQueryRetryAfterErrorStillRefires", func(t *testing.T) {
		t.Parallel()

		inner := memoryrepo.New()
		inner.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task alpha", Status: "open", Type: "task", Priority: 1})
		erroring := &erroringSearchRepo{Repository: inner, err: errors.New("backend unavailable")}
		recording := &queryRecordingRepo{Repository: erroring}

		m := NewModel(context.Background(), recording, nil)
		m.SetSize(120, 30)

		// Simulate: a prior search for "task" already applied, a fresh search for
		// the SAME text now in flight, and the user re-queued "task" via Enter.
		m.appliedQuery = "task"
		m.hasLoadedPage = true
		m.loading = true
		draft := "task"
		m.pendingDraft = &draft

		// The in-flight search resolves with an error and appliedQuery still "task".
		cmd := m.Update(searchLoadedMsg{appliedQuery: "task", err: errors.New("backend unavailable")})
		if cmd == nil {
			t.Fatal("same-query queued submit dropped after error: expected a re-fire (forceRefire must be true)")
		}
		if m.pendingDraft != nil {
			t.Fatalf("expected pendingDraft cleared, got %q", *m.pendingDraft)
		}
		if !m.IsLoading() {
			t.Fatal("expected loading=true: the same-query retry must be in flight (forceRefire=false would NOT re-fire here)")
		}

		// Drain the re-fired search so the recording repo observes the retry.
		_ = m.Update(cmd())
		foundTask := false
		for _, q := range recording.Queries() {
			if q.Text == "task" {
				foundTask = true
			}
		}
		if !foundTask {
			t.Errorf("no re-fired Search for the same queued text %q after error", "task")
		}
	})
}
