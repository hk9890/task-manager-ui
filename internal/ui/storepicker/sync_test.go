package storepicker

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

var syncNow = time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)

func syncState(sync Sync) State {
	return State{
		Rows:   sampleRows(),
		Help:   "Stores: j/k move · r reload · esc back · ctrl+q quit",
		Width:  100,
		Height: 16,
		Sync:   &sync,
		Now:    syncNow,
	}
}

func TestRenderSyncGoldens(t *testing.T) {
	t.Parallel()

	t.Run("synced_w100", func(t *testing.T) {
		view := Render(syncState(Sync{Branch: "machine/laptop", LastSync: syncNow.Add(-3*time.Hour - 20*time.Minute)}))
		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_sync_synced_w100.golden")
	})

	t.Run("running_w100", func(t *testing.T) {
		view := Render(syncState(Sync{Branch: "machine/laptop", LastSync: syncNow.Add(-time.Hour), Running: true}))
		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_sync_running_w100.golden")
	})

	t.Run("not_synced_w100", func(t *testing.T) {
		view := Render(syncState(Sync{Branch: "machine/laptop"}))
		testui.AssertMatchesGoldenNormalized(t, []byte(view), "store_picker_sync_not_synced_w100.golden")
	})
}

func TestSyncLineNamesTheStateAndTheBranch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		sync Sync
		want string
	}{
		{Sync{Branch: "machine/a", Running: true, LastSync: syncNow.Add(-time.Hour)}, "Sync running · machine/a"},
		{Sync{Branch: "machine/a"}, "Not synced yet · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-59 * time.Second)}, "Last sync just now · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(time.Minute)}, "Last sync just now · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-time.Minute)}, "Last sync 1 min ago · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-59 * time.Minute)}, "Last sync 59 min ago · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-time.Hour)}, "Last sync 1 h ago · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-23*time.Hour - 59*time.Minute)}, "Last sync 23 h ago · machine/a"},
		{Sync{Branch: "machine/a", LastSync: syncNow.Add(-50 * time.Hour)}, "Last sync 2 d ago · machine/a"},
		{Sync{Branch: "machine/a\nb"}, "Not synced yet · machine/a b"},
	} {
		got := testui.AnsiEscapePattern.ReplaceAllString(renderSync(tc.sync, syncNow, 60), "")
		if want := "  " + tc.want; got != want {
			t.Errorf("renderSync(%+v):\n got %q\nwant %q", tc.sync, got, want)
		}
	}
}

// A narrow box cuts the front of the branch: the time and the end of the
// machine ID stay, which tell the syncs and the machines apart.
func TestSyncLineCutsTheFrontOfTheBranchAndKeepsTheTime(t *testing.T) {
	t.Parallel()

	sync := Sync{Branch: "machine/a-rather-long-machine-id", LastSync: syncNow.Add(-2 * time.Hour)}
	got := testui.AnsiEscapePattern.ReplaceAllString(renderSync(sync, syncNow, 30), "")
	if want := "  Last sync 2 h ago · …hine-id"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The sync line is a detail, not a row: it draws in TextMutedColor.
func TestSyncLineIsMuted(t *testing.T) {
	t.Parallel()

	got := renderSync(Sync{Branch: "machine/a"}, syncNow, 60)
	want := "  " + lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Not synced yet · machine/a")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.Contains(got, "38;2;") {
		t.Errorf("the sync line carries no foreground colour: %q", got)
	}
}

// The sync line is the last line inside the box. It costs the rows that line
// only, and HitTest and RowCapacity still find each row where Render drew it.
func TestSyncLineSitsBelowTheRowsAndHitTestAgrees(t *testing.T) {
	t.Parallel()

	for _, hasError := range []bool{false, true} {
		for _, height := range []int{6, 7, 10, 11, 16, 17, 24, 25} {
			state := hitTestState(height)
			state.Help = "help"
			state.Sync = &Sync{Branch: "machine/box"}
			if hasError {
				state.Error = "registry unreadable"
			}
			capacity := RowCapacity(height, hasError, true)
			if want := contentLines(height, hasError, false) - 1; capacity != max(want/rowLines, 1) {
				t.Errorf("height %d, error %v: RowCapacity %d, want %d", height, hasError, capacity, max(want/rowLines, 1))
			}

			view := Render(state)
			lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")
			if len(lines) != height {
				t.Fatalf("height %d, error %v: the frame is %d lines", height, hasError, len(lines))
			}
			// The bottom border, then the help line.
			if syncLine := lines[height-3]; !strings.Contains(syncLine, "Not synced yet · machine/box") {
				t.Errorf("height %d, error %v: the line above the border is %q, want the sync line:\n%s",
					height, hasError, syncLine, strings.Join(lines, "\n"))
			}

			x, y := testui.FindCell(t, view, "Not synced yet")
			if row, ok := HitTest(state, x, y); ok {
				t.Errorf("height %d, error %v: HitTest on the sync line = row %d, want no hit", height, hasError, row)
			}
			for row := 0; row < capacity; row++ {
				text := state.Rows[row].Action
				if text == "" {
					text = state.Rows[row].Name
				}
				x, y := testui.FindCell(t, view, text)
				// A box too short for a whole row draws its first line only.
				for line := 0; line < rowLines && row*rowLines+line < contentLines(height, hasError, true); line++ {
					if got, ok := HitTest(state, x, y+line); !ok || got != row {
						t.Errorf("height %d, error %v: HitTest on line %d of row %d = %d, %v", height, hasError, line, row, got, ok)
					}
				}
			}
			if next := state.Rows[capacity]; strings.Contains(view, next.Action+next.Name) {
				t.Errorf("height %d, error %v: row %d is drawn past the window", height, hasError, capacity)
			}
		}
	}
}
