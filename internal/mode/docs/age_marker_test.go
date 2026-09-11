package docs

import (
	"testing"
	"time"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestDocsModeOrdersByLastChangeAndDrawsAgeMarkers seeds three docs out of
// date order and asserts the column reads newest change first, with the
// dividers the board draws for the same ages.
func TestDocsModeOrdersByLastChangeAndDrawsAgeMarkers(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	const day = 24 * time.Hour

	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Three days old", Type: "doc", Updated: now.Add(-3 * day)})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Fresh", Type: "doc", Updated: now.Add(-time.Hour)})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "A month old", Type: "doc", Updated: now.Add(-30 * day)})

	m := newModel(t, gw)
	m.now = func() time.Time { return now }
	resolve(t, m, m.Init())

	if got := []string{m.issues[0].ID, m.issues[1].ID, m.issues[2].ID}; got[0] != "tm-2" || got[1] != "tm-1" || got[2] != "tm-3" {
		t.Fatalf("expected docs ordered tm-2, tm-1, tm-3 by last change, got %v", got)
	}

	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, "› D P0 OPN tm-2", "older than 1 day", "tm-1", "older than 1 week", "tm-3")
}
