package search

// Controller-async contract tests.
//
// The helpers of model_test.go run every Cmd to its message before the next
// key arrives, so a search is never in flight when a key is processed. Here
// each Cmd runs in a goroutine, blocked inside fakes.DelayingRepository, while
// the test sends further keys — the cadence of a real tea.Program.

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// startAsync runs cmd in a goroutine and returns the channel its message
// arrives on.
func startAsync(cmd tea.Cmd) <-chan tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	return ch
}

func waitForInFlight(t *testing.T, delayed *fakes.DelayingRepository, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if delayed.InFlight() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%d searches in flight, want %d", delayed.InFlight(), want)
}

func receive(t *testing.T, ch <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("the search did not return")
		return nil
	}
}

// TestSearchControllerAsyncContracts pins what the controller does with a key
// that arrives while a search is still in flight.
func TestSearchControllerAsyncContracts(t *testing.T) {
	t.Run("a result of an older generation is dropped", func(t *testing.T) {
		gw := fakes.NewTracked()
		seedStore(gw)
		delayed := fakes.NewDelayingSearchRepository(gw)
		t.Cleanup(delayed.ReleaseAll)

		m := newModel(t, delayed)
		opening := startAsync(m.Init())
		waitForInFlight(t, delayed, 1)

		// Two keys arrive while the opening search is still out.
		first := startAsync(typeText(m, "tri"))
		waitForInFlight(t, delayed, 2)
		second := startAsync(typeText(m, "age"))
		waitForInFlight(t, delayed, 3)

		// The searches return in the order they were started. Each call of the
		// gate lets one through, and which one is not ours to choose, so all
		// three are released and their messages applied oldest first.
		delayed.ReleaseAll()
		stale := []tea.Msg{receive(t, opening), receive(t, first)}
		latest := receive(t, second)

		for _, msg := range stale {
			if cmd := m.Update(msg); cmd != nil {
				t.Fatalf("a superseded result announced a selection: %#v", msg)
			}
			if len(m.issues) != 0 || !m.IsLoading() {
				t.Fatalf("a superseded result was applied: results %q, loading %v", resultIDs(m), m.IsLoading())
			}
		}

		if got := selection(t, m.Update(latest)); got == nil || got.Issue.ID != "tm-2" {
			t.Fatalf("the latest result did not announce its selection: %#v", got)
		}
		if got := resultIDs(m); got != "tm-2" || m.IsLoading() {
			t.Fatalf("after the latest result: results %q, loading %v; want tm-2 and settled", got, m.IsLoading())
		}
	})

	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// openedSearch is a search with its opening list on screen, led by tm-1,
	// over a store that holds every later search until it is released.
	openedSearch := func(t *testing.T) (*Model, *fakes.TrackedRepository, *fakes.DelayingRepository) {
		t.Helper()
		gw := fakes.NewTracked()
		seedStore(gw)
		delayed := fakes.NewDelayingSearchRepository(gw)
		t.Cleanup(delayed.ReleaseAll)

		m := newModel(t, delayed)
		opening := startAsync(m.Init())
		waitForInFlight(t, delayed, 1)
		delayed.Release()
		m.Update(receive(t, opening))
		if got := m.selectedIssueID(); got != "tm-1" {
			t.Fatalf("setup: %q selected, want tm-1", got)
		}
		return m, gw, delayed
	}

	// inFlightSearch is an opened search with text typed and that search still
	// out: the rows on screen are the opening list. The result of "tri" holds
	// tm-2 alone.
	inFlightSearch := func(t *testing.T, text string) (*Model, *fakes.DelayingRepository, <-chan tea.Msg) {
		t.Helper()
		m, _, delayed := openedSearch(t)
		typed := startAsync(typeText(m, text))
		waitForInFlight(t, delayed, 1)
		return m, delayed, typed
	}

	// settle applies the result of the search in flight and returns the issue
	// the model asks the shell to open the detail of, "" for none. The result
	// announces its selection either way.
	settle := func(t *testing.T, m *Model, delayed *fakes.DelayingRepository, result <-chan tea.Msg) string {
		t.Helper()
		delayed.Release()
		announced, opened := false, ""
		for _, msg := range testui.DrainCmd(m.Update(receive(t, result))) {
			switch msg := msg.(type) {
			case mode.SelectionChangedMsg:
				announced = msg.Mode == mode.Search
			case mode.ActionRequestMsg:
				testui.AssertActionRequest(t, msg, mode.Search, mode.ActionOpenDetail)
				if msg.Selection == nil {
					t.Fatal("the open request carries no selection")
				}
				opened = msg.Selection.Issue.ID
			}
		}
		if !announced {
			t.Fatal("the result announced no selection")
		}
		return opened
	}

	t.Run("enter during a search opens the selected row of its result", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "tri")

		if cmd := m.Update(enter); cmd != nil {
			t.Fatalf("enter opened the detail on the rows of the older query: %#v", cmd())
		}

		if got := settle(t, m, delayed, typed); got != "tm-2" || m.selectedIssueID() != "tm-2" {
			t.Fatalf("the held enter opened %q with %q selected, want tm-2, the row of the result", got, m.selectedIssueID())
		}

		// One enter opens one detail: the next result opens nothing.
		refresh := startAsync(m.AutoRefresh())
		waitForInFlight(t, delayed, 1)
		if got := settle(t, m, delayed, refresh); got != "" {
			t.Fatalf("the held enter fired a second time, on %q", got)
		}
	})

	t.Run("a key after the held enter drops it", func(t *testing.T) {
		m, delayed, _ := inFlightSearch(t, "tri")

		m.Update(enter)
		retyped := startAsync(typeText(m, "a"))
		waitForInFlight(t, delayed, 2)
		// Two searches are out, and settle releases the other one.
		delayed.Release()

		if got := settle(t, m, delayed, retyped); got != "" {
			t.Fatalf("a dropped enter still opened the detail of %q", got)
		}
	})

	t.Run("a click or a wheel notch after the held enter drops it", func(t *testing.T) {
		for name, kind := range map[string]mode.MouseKind{"click": mode.MouseClick, "wheel": mode.MouseWheelDown} {
			m, delayed, typed := inFlightSearch(t, "tri")

			m.Update(enter)
			m.Update(mouseAt(t, m, kind, "Session notes", 0))
			if got := settle(t, m, delayed, typed); got != "" {
				t.Fatalf("%s: a dropped enter still opened the detail of %q", name, got)
			}
		}
	})

	t.Run("a held enter opens nothing on an empty result", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "zzzz")

		m.Update(enter)
		if got := settle(t, m, delayed, typed); got != "" {
			t.Fatalf("an empty result opened the detail of %q", got)
		}
		if got := len(m.issues); got != 0 {
			t.Fatalf("setup: %d results, want none", got)
		}
	})

	t.Run("a second click during a search is held as enter is", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "tri")

		m.Update(mouseAt(t, m, mode.MouseClick, "Fix login prompt", 0))
		if cmd := m.Update(mouseAt(t, m, mode.MouseClick, "Fix login prompt", 200)); cmd != nil {
			t.Fatalf("the second click opened the detail on the rows of the older query: %#v", cmd())
		}

		if got := settle(t, m, delayed, typed); got != "tm-2" || m.selectedIssueID() != "tm-2" {
			t.Fatalf("the held click opened %q with %q selected, want tm-2, the row of the result", got, m.selectedIssueID())
		}
	})

	t.Run("a held enter opens nothing on a failed result", func(t *testing.T) {
		m, gw, delayed := openedSearch(t)

		gw.SetError(fakes.MethodSearch, errors.New("store unreadable"))
		typed := startAsync(typeText(m, "tri"))
		waitForInFlight(t, delayed, 1)
		m.Update(enter)

		// The stale rows stay on screen under the error row: opening one of
		// them is the fault the hold exists to prevent.
		if got := settle(t, m, delayed, typed); got != "" {
			t.Fatalf("a failed result opened the detail of %q", got)
		}
		if m.err == nil || len(m.issues) == 0 {
			t.Fatalf("setup: error %v with %d stale rows, want an error over the stale rows", m.err, len(m.issues))
		}
	})

	t.Run("enter during an auto refresh opens the row the refresh selects", func(t *testing.T) {
		m, gw, delayed := openedSearch(t)

		// The selected issue leaves the open scope while the refresh is out.
		refresh := startAsync(m.AutoRefresh())
		waitForInFlight(t, delayed, 1)
		if err := gw.CloseIssue(context.Background(), "tm-1", domain.CloseIssueInput{}); err != nil {
			t.Fatalf("CloseIssue returned error: %v", err)
		}

		if cmd := m.Update(enter); cmd != nil {
			t.Fatalf("enter opened the detail of an issue the refresh is about to drop: %#v", cmd())
		}
		if got := settle(t, m, delayed, refresh); got != "tm-2" || m.selectedIssueID() != "tm-2" {
			t.Fatalf("the held enter opened %q with %q selected, want tm-2, the row in the place of the closed issue", got, m.selectedIssueID())
		}
	})

	t.Run("enter before the opening search returns opens nothing", func(t *testing.T) {
		gw := fakes.NewTracked()
		seedStore(gw)
		delayed := fakes.NewDelayingSearchRepository(gw)
		t.Cleanup(delayed.ReleaseAll)

		m := newModel(t, delayed)
		opening := startAsync(m.Init())
		waitForInFlight(t, delayed, 1)

		// No rows are on screen yet, so there is nothing the operator chose.
		if cmd := m.Update(enter); cmd != nil {
			t.Fatalf("enter with no rows on screen asked for %#v", cmd())
		}
		if got := settle(t, m, delayed, opening); got != "" {
			t.Fatalf("the opening result opened the detail of %q, which nobody chose", got)
		}
	})

	t.Run("the pointer leaving drops the held enter", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "tri")

		m.Update(enter)
		m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
		if got := settle(t, m, delayed, typed); got != "" {
			t.Fatalf("a held enter outlived the surface leaving the screen and opened %q", got)
		}
	})

	// searchCalls is the Search calls the store received.
	searchCalls := func(gw *fakes.TrackedRepository) int {
		return len(gw.CallsFor(fakes.MethodSearch))
	}

	t.Run("keys typed inside the pause run one search for the whole text", func(t *testing.T) {
		gw := fakes.NewTracked()
		seedStore(gw)
		m := openedModel(t, gw)
		before := searchCalls(gw)

		// Each pause is over, and the model has not been told so yet.
		var pauses []tea.Msg
		for _, key := range []string{"t", "r", "i"} {
			pauses = append(pauses, m.Update(runes(key))())
		}
		if !m.IsLoading() || m.Reload() != nil || m.AutoRefresh() != nil {
			t.Fatalf("the pause does not count as a search in flight: loading %v", m.IsLoading())
		}

		for _, superseded := range pauses[:2] {
			if cmd := m.Update(superseded); cmd != nil {
				t.Fatalf("the pause of an earlier key ran a search: %#v", superseded)
			}
		}
		if got := searchCalls(gw) - before; got != 0 {
			t.Fatalf("%d searches ran before the last pause ended", got)
		}

		if got := selection(t, resolve(t, m, m.Update(pauses[2]))); got == nil || got.Issue.ID != "tm-2" {
			t.Fatalf("the search of the last key did not announce its selection: %#v", got)
		}
		if got := searchCalls(gw) - before; got != 1 {
			t.Fatalf("three keys ran %d searches, want 1", got)
		}
		if got := lastSearch(t, gw); got.Text != "tri" || got.IncludeClosed {
			t.Fatalf("the search asked for %#v, want the whole text tri in open issues", got)
		}
		if got := resultIDs(m); got != "tm-2" || m.IsLoading() {
			t.Fatalf("after the search: results %q, loading %v; want tm-2 and settled", got, m.IsLoading())
		}
	})

	t.Run("the scope key or escape during the pause searches at once, and the pause runs nothing", func(t *testing.T) {
		supersede := map[string]func(*Model) tea.Cmd{
			"scope key": func(m *Model) tea.Cmd { return m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}) },
			"escape": func(m *Model) tea.Cmd {
				_, cmd := m.ClearQuery()
				return cmd
			},
		}
		want := map[string]domain.SearchIssuesQuery{
			"scope key": {Text: "tri", IncludeClosed: true, Limit: resultLimit},
			"escape":    {Limit: resultLimit},
		}
		for name, act := range supersede {
			gw := fakes.NewTracked()
			seedStore(gw)
			m := openedModel(t, gw)
			before := searchCalls(gw)

			pause := m.Update(runes("tri"))()
			immediate := act(m)
			if cmd := m.Update(pause); cmd != nil {
				t.Fatalf("%s: the superseded pause ran a search of its own", name)
			}
			resolve(t, m, immediate)

			if got := searchCalls(gw) - before; got != 1 {
				t.Fatalf("%s: %d searches ran, want 1", name, got)
			}
			got := lastSearch(t, gw)
			if got.Text != want[name].Text || got.IncludeClosed != want[name].IncludeClosed || got.Limit != want[name].Limit {
				t.Fatalf("%s: the search asked for %#v, want %#v", name, got, want[name])
			}
			if m.IsLoading() {
				t.Fatalf("%s: the surface is still loading after its search returned", name)
			}
		}
	})

	t.Run("enter during the pause opens the selected row of the result", func(t *testing.T) {
		m, _, delayed := openedSearch(t)

		pause := m.Update(runes("tri"))()
		if cmd := m.Update(enter); cmd != nil {
			t.Fatalf("enter opened the detail on the rows of the older query: %#v", cmd())
		}
		typed := startAsync(m.Update(pause))
		waitForInFlight(t, delayed, 1)

		if got := settle(t, m, delayed, typed); got != "tm-2" || m.selectedIssueID() != "tm-2" {
			t.Fatalf("the held enter opened %q with %q selected, want tm-2, the row of the result", got, m.selectedIssueID())
		}
	})

	t.Run("a key after an enter during the pause drops it", func(t *testing.T) {
		m, _, delayed := openedSearch(t)

		first := m.Update(runes("tr"))()
		m.Update(enter)
		second := m.Update(runes("i"))()
		if cmd := m.Update(first); cmd != nil {
			t.Fatal("the pause of the earlier key ran a search")
		}
		typed := startAsync(m.Update(second))
		waitForInFlight(t, delayed, 1)

		if got := settle(t, m, delayed, typed); got != "" {
			t.Fatalf("a dropped enter still opened the detail of %q", got)
		}
		if got := resultIDs(m); got != "tm-2" {
			t.Fatalf("the results are %q, want tm-2, the answer to the whole text", got)
		}
	})

	t.Run("a late result of an older generation does not replace the latest", func(t *testing.T) {
		gw := fakes.NewTracked()
		seedStore(gw)
		m := openedModel(t, gw)

		older := typeText(m, "login")()
		resolve(t, m, typeText(m, " prompt"))
		if got := resultIDs(m); got != "tm-1" {
			t.Fatalf("setup: results %q, want tm-1", got)
		}

		if cmd := m.Update(older); cmd != nil {
			t.Fatal("a result that arrived after its successor announced a selection")
		}
		if got := resultIDs(m); got != "tm-1" {
			t.Fatalf("a late result replaced the latest: results %q", got)
		}
	})
}
