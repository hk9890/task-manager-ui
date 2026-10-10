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
