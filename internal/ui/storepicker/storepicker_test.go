package storepicker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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
	// At this width a project path is longer than its line and is cut.
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

	// A name longer than its line is truncated, which is a branch no w100
	// golden with short fixture names reaches.
	t.Run("long_names_w60", func(t *testing.T) {
		view := Render(State{
			Rows: []Row{
				{Name: "a-very-long-registry-store-name-indeed-that-outruns-the-whole-row", ProjectPath: "/home/hans/dev/long", Health: "ok", Usable: true},
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

		// Height 10 is seven content lines: three rows and a line to spare.
		// The offset is where the controller leaves it with row 7 selected
		// last in the window.
		view := Render(State{
			Rows:         rows,
			SelectedRow:  7,
			ScrollOffset: 5,
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
// the renderer actually draws puts the selection bar on a row that is not on
// screen. A row is two lines, so the heights alternate between a content
// height the rows fill and one that leaves a line spare.
func TestRowCapacityMatchesRenderedRows(t *testing.T) {
	t.Parallel()

	for _, hasError := range []bool{false, true} {
		for _, height := range []int{6, 7, 10, 11, 16, 17, 24, 25, 40} {
			capacity := RowCapacity(height, hasError)

			rows := make([]Row, 0, capacity+5)
			for i := 0; i < capacity+5; i++ {
				rows = append(rows, Row{Name: "store", ProjectPath: "/home/hans/dev/store", Health: "ok", Usable: true})
			}
			state := State{Rows: rows, SelectedRow: -1, Width: 100, Height: height, Help: "help"}
			if hasError {
				state.Error = "registry unreadable"
			}

			view := testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")
			if got := len(strings.Split(view, "\n")); got != height {
				t.Errorf("height %d, error %v: the frame is %d lines", height, hasError, got)
			}
			names, paths := strings.Count(view, "│  store "), strings.Count(view, "│  /home/hans/dev/store ")
			if names != capacity || paths != capacity {
				t.Errorf("height %d, error %v: rendered %d names and %d paths, RowCapacity says %d rows:\n%s",
					height, hasError, names, paths, capacity, view)
			}
		}
	}
}

// TestRowCapacityCountsRowsNotLines pins the window at even and odd heights:
// a row is two lines, the frame takes three, and the inline error row costs a
// line — which is a row only when no line was spare.
func TestRowCapacityCountsRowsNotLines(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		height           int
		plain, withError int
	}{
		{height: 0, plain: 10, withError: 10}, // the default height, 24
		{height: 4, plain: 1, withError: 1},   // never less than one row
		{height: 5, plain: 1, withError: 1},
		{height: 6, plain: 1, withError: 1},
		{height: 7, plain: 2, withError: 1},
		{height: 16, plain: 6, withError: 6},
		{height: 17, plain: 7, withError: 6},
		{height: 24, plain: 10, withError: 10},
		{height: 25, plain: 11, withError: 10},
	} {
		if got := RowCapacity(tc.height, false); got != tc.plain {
			t.Errorf("RowCapacity(%d, false) = %d, want %d", tc.height, got, tc.plain)
		}
		if got := RowCapacity(tc.height, true); got != tc.withError {
			t.Errorf("RowCapacity(%d, true) = %d, want %d", tc.height, got, tc.withError)
		}
	}
}

func TestRowCapacityLeavesRoomForTheErrorRow(t *testing.T) {
	t.Parallel()

	for _, height := range []int{16, 17} {
		if with, without := contentLines(height, true), contentLines(height, false); with != without-1 {
			t.Errorf("height %d: an inline error row costs one content line: got %d with, %d without", height, with, without)
		}
	}
	// With no line spare, that line is a store row.
	if withError, withoutError := RowCapacity(17, true), RowCapacity(17, false); withError != withoutError-1 {
		t.Errorf("an inline error row costs one store row: got %d with, %d without", withError, withoutError)
	}
}

// TestRowDrawsTheStatusOnTheNameLineAndThePathUnderIt pins the two lines of a
// row: the name with the status token flush right, then the project path.
func TestRowDrawsTheStatusOnTheNameLineAndThePathUnderIt(t *testing.T) {
	t.Parallel()

	const innerWidth = 40
	strip := func(lines []string) []string {
		out := make([]string, len(lines))
		for idx, line := range lines {
			out[idx] = testui.AnsiEscapePattern.ReplaceAllString(line, "")
		}
		return out
	}
	gutter, _ := styles.SelectionPrefix(true, false)

	lines := strip(renderRow(Row{Name: "alpha", ProjectPath: "/home/hans/dev/alpha", Health: "ok", Usable: true, Active: true}, true, innerWidth))
	if len(lines) != rowLines {
		t.Fatalf("a store row is %d lines, want %d", len(lines), rowLines)
	}
	if want := gutter + "alpha" + strings.Repeat(" ", innerWidth-len("  alpha")-len("active")) + "active"; lines[0] != want {
		t.Errorf("first line:\n got %q\nwant %q", lines[0], want)
	}
	if want := gutter + "/home/hans/dev/alpha"; lines[1] != want {
		t.Errorf("second line:\n got %q\nwant %q", lines[1], want)
	}

	// A row with no token gives the name the whole line.
	lines = strip(renderRow(Row{Name: "bravo", ProjectPath: "/home/hans/dev/bravo", Health: "ok", Usable: true}, false, innerWidth))
	if strings.TrimRight(lines[0], " ") != "  bravo" || lines[1] != "  /home/hans/dev/bravo" {
		t.Errorf("an unmarked row: got %q", lines)
	}

	// An action row keeps the height; its second line is the gutter alone.
	lines = strip(renderRow(Row{Action: "Create a local store in /x"}, true, innerWidth))
	if len(lines) != rowLines || lines[0] != gutter+"Create a local store in /x" || lines[1] != gutter {
		t.Errorf("an action row: got %q", lines)
	}
}

// TestEveryStateFillsTheFrame holds each body to exactly the content lines:
// rows, the empty state, a cold listing and a failed read with nothing cached,
// at content heights that leave no line spare and at ones that leave one.
func TestEveryStateFillsTheFrame(t *testing.T) {
	t.Parallel()

	states := map[string]State{
		"rows":           {Rows: sampleRows()},
		"rows and error": {Rows: sampleRows(), Error: "registry unreadable"},
		"empty":          {},
		"cold listing":   {Loading: true},
		"failed read":    {Error: "registry unreadable"},
	}
	for name, state := range states {
		for height := 4; height <= 13; height++ {
			for _, width := range []int{100, 20, 8, 6} {
				state.Width, state.Height, state.Help = width, height, "help"
				view := Render(state)
				lines := strings.Split(view, "\n")
				if len(lines) != height {
					t.Errorf("%s at %dx%d: the frame is %d lines:\n%s", name, width, height, len(lines), view)
				}
				// The top border is styles.FormSection's: it outruns a frame
				// narrower than the title and the count, and the join pads every
				// line to it. So the body is measured to its right border.
				for _, line := range lines[1:] {
					if got := lipgloss.Width(strings.TrimRight(line, " ")); got > width {
						t.Errorf("%s at %dx%d: a line is %d cells wide:\n%s", name, width, height, got, view)
						break
					}
				}
			}
		}
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

// The path has a line of its own, so it starts in one column whatever the
// names above it are: no path slides sideways as a long name scrolls in or out
// of view.
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

// Within one frame every status token lands in the same place, whatever the
// length of the name it follows.
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
