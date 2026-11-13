package docs

import (
	"testing"
	"time"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestDocsModeOrdersByLastChangeWithoutAgeDividers seeds three docs out of
// date order and asserts the column reads newest change first, with none of
// the dividers the board draws between ages.
func TestDocsModeOrdersByLastChangeWithoutAgeDividers(t *testing.T) {
	now := time.Now()
	const day = 24 * time.Hour

	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Three days old", Type: "doc", Updated: now.Add(-3 * day)})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Fresh", Type: "doc", Updated: now.Add(-time.Hour)})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "A month old", Type: "doc", Updated: now.Add(-30 * day)})

	m := newModel(t, gw)
	resolve(t, m, m.Init())

	if got := []string{m.issues[0].ID, m.issues[1].ID, m.issues[2].ID}; got[0] != "tm-2" || got[1] != "tm-1" || got[2] != "tm-3" {
		t.Fatalf("expected docs ordered tm-2, tm-1, tm-3 by last change, got %v", got)
	}

	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, selectedGutter+"D Fresh", "tm-1", "tm-3")
	testui.AssertNotContainsAny(t, plain, "older than")
	assertSelectionDrawn(t, m)
}
