package storepicker

import (
	"strings"
	"testing"

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
