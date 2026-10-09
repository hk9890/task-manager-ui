package modal

import (
	"fmt"
	"strings"
	"testing"
)

func tallModal(lines, viewportHeight int) Model {
	message := make([]string, lines)
	for idx := range message {
		message[idx] = fmt.Sprintf("line-%02d", idx)
	}
	m := New(Config{Title: "Tall", Message: strings.Join(message, "\n"), HideButtons: true})
	m.SetSize(80, viewportHeight)
	return m
}

// TestScrollMovesAModalTooTallForTheViewport walks a clipped modal to its end
// and back: the window follows, each clipped edge says how much it hides, and
// the offset stops at both ends.
func TestScrollMovesAModalTooTallForTheViewport(t *testing.T) {
	t.Parallel()

	m := tallModal(30, 14)
	top := m.View()
	if !strings.Contains(top, "line-00") || strings.Contains(top, "earlier lines") || !strings.Contains(top, "more lines") {
		t.Fatalf("unscrolled view must open on the first line with only a 'more' indicator:\n%s", top)
	}
	if got := strings.Count(top, "\n") + 1; got != 12 {
		t.Fatalf("clipped modal is %d lines tall, want viewport minus its margin = 12", got)
	}

	m = m.Scroll(5)
	middle := m.View()
	if strings.Contains(middle, "line-00") || !strings.Contains(middle, "earlier lines") || !strings.Contains(middle, "more lines") {
		t.Fatalf("scrolled view must hide the first line and mark both clipped edges:\n%s", middle)
	}
	if got := strings.Count(middle, "\n") + 1; got != 12 {
		t.Fatalf("scrolled modal is %d lines tall, want 12", got)
	}

	m = m.Scroll(1000)
	bottom := m.View()
	if !strings.Contains(bottom, "line-29") || strings.Contains(bottom, "more lines") {
		t.Fatalf("view scrolled to the end must show the last line and no 'more' indicator:\n%s", bottom)
	}
	if again := m.Scroll(3).View(); again != bottom {
		t.Fatal("scrolling past the end moved the view")
	}

	if back := m.Scroll(-1000).View(); back != top {
		t.Fatalf("scrolling back to the start must restore the first view:\n%s", back)
	}
	if reset := m.ScrollToTop().View(); reset != top {
		t.Fatal("ScrollToTop must restore the first view")
	}
}

// TestScrollStartsFromTheDrawnLineAfterAResize grows the viewport under a
// modal scrolled to its end. Fewer lines are hidden now, so the stored offset
// is past the new end, and the first notch back must still move the view.
func TestScrollStartsFromTheDrawnLineAfterAResize(t *testing.T) {
	t.Parallel()

	m := tallModal(30, 14).Scroll(1000)
	m.SetSize(80, 24)
	bottom := m.View()
	if !strings.Contains(bottom, "line-29") || strings.Contains(bottom, "more lines") {
		t.Fatalf("fixture: the resized view must still end on the last line:\n%s", bottom)
	}

	if up := m.Scroll(-1).View(); up == bottom {
		t.Fatal("the first notch up after the resize did not move the view")
	}
}

// TestAPageShowsEveryLineOnce: a page down starts on the line after the last
// one the view before it drew, and ScrollToEnd lands where scrolling stops.
func TestAPageShowsEveryLineOnce(t *testing.T) {
	t.Parallel()

	m := tallModal(30, 14)
	page := m.PageLines()
	first := m.Scroll(page).View()
	second := m.Scroll(2 * page).View()
	// The title, its divider and the padding row stand above line-00.
	lastDrawn := fmt.Sprintf("line-%02d", 2*page-3)
	if !strings.Contains(first, lastDrawn) || strings.Contains(second, lastDrawn) {
		t.Fatalf("%s must close the first page and not open the second:\n%s\n%s", lastDrawn, first, second)
	}
	if next := fmt.Sprintf("line-%02d", 2*page-2); strings.Contains(first, next) || !strings.Contains(second, next) {
		t.Fatalf("%s must open the second page:\n%s\n%s", next, first, second)
	}

	if end := m.ScrollToEnd().View(); end != m.Scroll(1000).View() {
		t.Fatalf("ScrollToEnd must land on the last view:\n%s", end)
	}
	if fits := tallModal(3, 40); fits.ScrollToEnd().View() != fits.View() {
		t.Fatal("ScrollToEnd changed a modal that fits")
	}
}

func TestScrollDoesNothingToAModalThatFits(t *testing.T) {
	t.Parallel()

	m := tallModal(3, 40)
	before := m.View()
	if after := m.Scroll(5).View(); after != before {
		t.Fatalf("scrolling a modal that fits changed it:\n%s", after)
	}
}
