package storepicker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func sampleRows() []Row {
	return []Row{
		{Name: "task-manager-ui", ProjectPath: "/home/hans/dev/github/task-manager-ui", Health: "ok", Usable: true, Active: true},
		{Name: "task-manager", ProjectPath: "/home/hans/dev/github/task-manager", Health: "ok", Usable: true},
		{Name: "sandbox", ProjectPath: "/home/hans/dev/sandbox", Health: "ok", Usable: true},
	}
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("populated_w100", func(t *testing.T) {
		view := Render(State{
			Rows:        sampleRows(),
			SelectedRow: 1,
			Help:        "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:       100,
			Height:      16,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_populated_w100.golden")
	})

	// An entry whose store directory is gone or unusable stays on the list with
	// its health named. Hiding it would disagree with `taskmgr store list`, and
	// the operator would have no way to see why the store never opens.
	t.Run("unhealthy_entries_w100", func(t *testing.T) {
		view := Render(State{
			Rows: []Row{
				{Name: "task-manager-ui", ProjectPath: "/home/hans/dev/github/task-manager-ui", Health: "ok", Usable: true, Active: true},
				{Name: "moved-away", ProjectPath: "/home/hans/dev/moved-away", Health: "dangling"},
				{Name: "half-made", ProjectPath: "/home/hans/dev/half-made", Health: "broken"},
			},
			Help:   "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:  100,
			Height: 16,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_unhealthy_w100.golden")
	})

	t.Run("empty_registry_w100", func(t *testing.T) {
		view := Render(State{
			Help:   "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:  100,
			Height: 16,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_empty_w100.golden")
	})

	t.Run("listing_failed_w100", func(t *testing.T) {
		view := Render(State{
			Rows:   sampleRows(),
			Error:  "failed to read the central store registry: yaml: line 3: mapping values are not allowed",
			Help:   "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:  100,
			Height: 16,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_error_w100.golden")
	})

	t.Run("cold_listing_w100", func(t *testing.T) {
		view := Render(State{
			Loading: true,
			Help:    "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:   100,
			Height:  16,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_loading_w100.golden")
	})

	// The window is smaller than the list, so the count reads "N of M" rather
	// than a plain total (docs/DESIGN-GUIDE.md, Selection and scrolling).
	// Width selects the layout branch: below the point where a project path
	// fits, the path is dropped and only the name and the status token remain.
	t.Run("narrow_w40", func(t *testing.T) {
		view := Render(State{
			Rows: []Row{
				{Name: "task-manager-ui", ProjectPath: "/home/hans/dev/github/task-manager-ui", Health: "ok", Usable: true, Active: true},
				{Name: "task-manager", ProjectPath: "/home/hans/dev/github/task-manager", Health: "ok", Usable: true},
				{Name: "moved-away", ProjectPath: "/home/hans/dev/moved-away", Health: "dangling"},
			},
			Help:   "Stores: j/k · r · esc · ctrl+q",
			Width:  40,
			Height: 12,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_narrow_w40.golden")
	})

	// A name past nameColumnMax is truncated, which is a branch no w100 golden
	// with short fixture names reaches.
	t.Run("long_names_w60", func(t *testing.T) {
		view := Render(State{
			Rows: []Row{
				{Name: "a-very-long-registry-store-name-indeed", ProjectPath: "/home/hans/dev/long", Health: "ok", Usable: true},
				{Name: "short", ProjectPath: "/home/hans/dev/short", Health: "ok", Usable: true, Active: true},
			},
			Help:   "Stores: j/k · r · esc · ctrl+q",
			Width:  60,
			Height: 12,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_long_names_w60.golden")
	})

	// A start with no store for the working directory offers to create one,
	// above the stores already registered.
	t.Run("create_rows_w100", func(t *testing.T) {
		rows := []Row{
			{Action: "Create a local store in /home/hans/dev/widget"},
			{Action: "Create a central store for /home/hans/dev/widget"},
		}
		rows = append(rows, sampleRows()[1:]...)
		view := Render(State{
			Rows:   rows,
			Help:   "Stores: j/k move · enter open · r reload · esc quit · ctrl+q quit",
			Width:  100,
			Height: 12,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_create_rows_w100.golden")
	})

	t.Run("clipped_window_w100", func(t *testing.T) {
		rows := make([]Row, 0, 12)
		for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett", "kilo", "lima"} {
			rows = append(rows, Row{Name: name, ProjectPath: "/home/hans/dev/" + name, Health: "ok", Usable: true})
		}

		view := Render(State{
			Rows:         rows,
			SelectedRow:  7,
			ScrollOffset: 4,
			Help:         "Stores: j/k move · r reload · esc back · ctrl+q quit",
			Width:        100,
			Height:       10,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_clipped_w100.golden")
	})
}

// The empty state must name what would be listed here. An empty registry and a
// failed read render identically otherwise, and the operator cannot tell which
// of the two they are looking at.
func TestEmptyStateNamesWhatWouldBeListed(t *testing.T) {
	t.Parallel()

	view := testui.AnsiEscapePattern.ReplaceAllString(Render(State{Width: 100, Height: 16}), "")

	for _, want := range []string{"No central task-manager stores", "taskmgr init --central", "local .tasks store"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in the empty state:\n%s", want, view)
		}
	}
}

// RowCapacity is the controller's scroll window. A window wider than the rows
// the renderer actually draws puts the selection chevron on a row that is not
// on screen.
func TestRowCapacityMatchesRenderedRows(t *testing.T) {
	t.Parallel()

	for _, height := range []int{6, 10, 16, 24, 40} {
		capacity := RowCapacity(height, false)

		rows := make([]Row, 0, capacity+5)
		for i := 0; i < capacity+5; i++ {
			rows = append(rows, Row{Name: "store", ProjectPath: "/home/hans/dev/store", Health: "ok", Usable: true})
		}

		view := Render(State{Rows: rows, Width: 100, Height: height, Help: "help"})
		// Two border lines plus the help line frame the rows.
		got := len(strings.Split(view, "\n")) - 3
		if got != capacity {
			t.Errorf("height %d: rendered %d rows, RowCapacity says %d", height, got, capacity)
		}
	}
}

func TestRowCapacityLeavesRoomForTheErrorRow(t *testing.T) {
	t.Parallel()

	withError := RowCapacity(16, true)
	withoutError := RowCapacity(16, false)
	if withError != withoutError-1 {
		t.Errorf("an inline error row costs one store row: got %d with, %d without", withError, withoutError)
	}
}

// A failed read with nothing cached must not also claim the registry is empty:
// that is the one wrong answer, and the two states are exactly what the empty
// state exists to distinguish.
func TestFailedListingWithNoRowsDoesNotClaimAnEmptyRegistry(t *testing.T) {
	t.Parallel()

	view := testui.AnsiEscapePattern.ReplaceAllString(Render(State{
		Error:  "failed to read the central store registry: permission denied",
		Help:   "help",
		Width:  100,
		Height: 16,
	}), "")

	if !strings.Contains(view, "permission denied") {
		t.Errorf("expected the failure to be named:\n%s", view)
	}
	if strings.Contains(view, "No central task-manager stores") {
		t.Errorf("a failed read rendered the empty-registry text:\n%s", view)
	}
}

// A store can be opened and then have its directory removed from under the
// running app. That row must report its health, not a healthy "active".
func TestAnActiveStoreThatWentUnusableReportsItsHealth(t *testing.T) {
	t.Parallel()

	for _, health := range []string{"dangling", "broken"} {
		token, _ := statusToken(Row{Name: "gone", Health: health, Active: true})
		if token != health {
			t.Errorf("an active but %s store reports %q, want %q", health, token, health)
		}
	}

	if token, _ := statusToken(Row{Name: "fine", Health: "ok", Usable: true, Active: true}); token != "active" {
		t.Errorf("a healthy active store reports %q, want \"active\"", token)
	}
}

// The name column is a property of the whole list. Sized to the visible window
// instead, every path and status token slides sideways as a long name scrolls
// in or out of view.
func TestColumnsDoNotShiftAsTheListScrolls(t *testing.T) {
	t.Parallel()

	rows := []Row{
		{Name: "a-very-long-store-name", ProjectPath: "/p/aa", Health: "ok", Usable: true},
		{Name: "bb", ProjectPath: "/p/bb", Health: "ok", Usable: true},
		{Name: "cc", ProjectPath: "/p/cc", Health: "ok", Usable: true},
		{Name: "dd", ProjectPath: "/p/dd", Health: "ok", Usable: true},
		{Name: "ee", ProjectPath: "/p/ee", Health: "ok", Usable: true},
	}

	// Counted in cells, not bytes: the border and the selection chevron are both
	// multi-byte, so a byte index reports a column that is not there.
	pathColumn := func(offset int) int {
		view := testui.AnsiEscapePattern.ReplaceAllString(
			Render(State{Rows: rows, ScrollOffset: offset, Help: "help", Width: 60, Height: 5}), "")
		for _, line := range strings.Split(view, "\n") {
			if idx := cellIndex(line, "/p/"); idx >= 0 {
				return idx
			}
		}
		t.Fatalf("no path rendered at offset %d:\n%s", offset, view)
		return -1
	}

	if first, last := pathColumn(0), pathColumn(3); first != last {
		t.Errorf("the path column moved from %d to %d as the list scrolled", first, last)
	}
}

// Within one frame every status token lands in the same place, whether or not
// that row had room for its project path.
func TestStatusTokensShareOneRightEdge(t *testing.T) {
	t.Parallel()

	view := testui.AnsiEscapePattern.ReplaceAllString(Render(State{
		Rows: []Row{
			{Name: "task-manager-ui", ProjectPath: "/home/hans/dev/github/task-manager-ui", Health: "ok", Usable: true, Active: true},
			{Name: "moved-away", ProjectPath: "/home/hans/dev/moved-away", Health: "dangling"},
		},
		Help:   "help",
		Width:  40,
		Height: 8,
	}), "")

	ends := make([]int, 0, 2)
	for _, line := range strings.Split(view, "\n") {
		for _, token := range []string{"active", "dangling"} {
			if idx := cellIndex(line, token); idx >= 0 {
				ends = append(ends, idx+lipgloss.Width(token))
			}
		}
	}
	if len(ends) != 2 {
		t.Fatalf("expected two status tokens, found %d:\n%s", len(ends), view)
	}
	if ends[0] != ends[1] {
		t.Errorf("status tokens end at columns %d and %d; they must share one right edge:\n%s", ends[0], ends[1], view)
	}
}

// cellIndex returns the rendered column where want starts in line, or -1. The
// frame is full of multi-byte runes — the border, the chevron, the ellipsis —
// so a byte index is not a column.
func cellIndex(line, want string) int {
	idx := strings.Index(line, want)
	if idx < 0 {
		return -1
	}
	return lipgloss.Width(line[:idx])
}

// The header counts stores. A create row is not a store, so two of them above
// two stores still read "2".
func TestTheCountIgnoresCreateRows(t *testing.T) {
	t.Parallel()

	rows := []Row{{Action: "Create a local store in /x"}, {Action: "Create a central store for /x"}}
	rows = append(rows, sampleRows()[:2]...)

	if got := countLabel(State{Rows: rows}, 20); got != "2" {
		t.Errorf("count: got %q, want 2", got)
	}
}
