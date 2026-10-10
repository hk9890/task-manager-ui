package search

// Controller-async contract tests.
//
// The helpers of model_test.go run every Cmd to its message before the next
// key arrives, so a search is never in flight when a key is processed. Here
// each Cmd runs in a goroutine, blocked inside fakes.DelayingRepository, while
// the test sends further keys — the cadence of a real tea.Program.

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
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

	// inFlightSearch is an opened search with "tri" typed and that search still
	// out: the rows on screen are the opening list, led by tm-1, and the result
	// on its way holds tm-2 alone.
	inFlightSearch := func(t *testing.T, text string) (*Model, *fakes.DelayingRepository, <-chan tea.Msg) {
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

		typed := startAsync(typeText(m, text))
		waitForInFlight(t, delayed, 1)
		return m, delayed, typed
	}

	// settle applies the result of the search in flight and hands the model the
	// selection change it announced, as the shell does. It returns what the
	// model asked for next.
	settle := func(t *testing.T, m *Model, delayed *fakes.DelayingRepository, result <-chan tea.Msg) tea.Cmd {
		t.Helper()
		delayed.Release()
		announced := m.Update(receive(t, result))
		if announced == nil {
			t.Fatal("the result announced no selection")
		}
		return m.Update(announced())
	}

	t.Run("enter during a search opens the first row of its result", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "tri")

		if cmd := m.Update(enter); cmd != nil {
			t.Fatalf("enter opened the detail on the rows of the older query: %#v", cmd())
		}

		next := settle(t, m, delayed, typed)
		if next == nil {
			t.Fatal("the held enter opened nothing when the result arrived")
		}
		request, ok := next().(mode.ActionRequestMsg)
		if !ok || request.Mode != mode.Search || request.Action != mode.ActionOpenDetail {
			t.Fatalf("the held enter asked for %#v, want the search to open its detail", next())
		}
		if got := m.selectedIssueID(); got != "tm-2" {
			t.Fatalf("the detail opens on %q, want tm-2", got)
		}

		// One enter opens one detail: a later selection change asks for nothing.
		if cmd := m.Update(mode.SelectionChangedMsg{Mode: mode.Search}); cmd != nil {
			t.Fatalf("the held enter fired a second time: %#v", cmd())
		}
	})

	t.Run("a key after the held enter drops it", func(t *testing.T) {
		m, delayed, _ := inFlightSearch(t, "tri")

		m.Update(enter)
		retyped := startAsync(typeText(m, "a"))
		waitForInFlight(t, delayed, 2)
		delayed.Release()
		delayed.Release()

		if next := settle(t, m, delayed, retyped); next != nil {
			t.Fatalf("a dropped enter still opened the detail: %#v", next())
		}
	})

	t.Run("a held enter opens nothing on an empty result", func(t *testing.T) {
		m, delayed, typed := inFlightSearch(t, "zzzz")

		m.Update(enter)
		if next := settle(t, m, delayed, typed); next != nil {
			t.Fatalf("an empty result opened a detail: %#v", next())
		}
		if got := len(m.issues); got != 0 {
			t.Fatalf("setup: %d results, want none", got)
		}
	})

	t.Run("enter during an auto refresh opens the selected row at once", func(t *testing.T) {
		gw := fakes.NewTracked()
		seedStore(gw)
		delayed := fakes.NewDelayingSearchRepository(gw)
		t.Cleanup(delayed.ReleaseAll)

		m := newModel(t, delayed)
		opening := startAsync(m.Init())
		waitForInFlight(t, delayed, 1)
		delayed.Release()
		m.Update(receive(t, opening))

		refresh := startAsync(m.AutoRefresh())
		waitForInFlight(t, delayed, 1)

		cmd := m.Update(enter)
		if cmd == nil {
			t.Fatal("enter waited for an auto refresh, which keeps the selection where it is")
		}
		if request, ok := cmd().(mode.ActionRequestMsg); !ok || request.Action != mode.ActionOpenDetail {
			t.Fatalf("enter asked for %#v, want the open-detail request", cmd())
		}
		delayed.Release()
		receive(t, refresh)
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
