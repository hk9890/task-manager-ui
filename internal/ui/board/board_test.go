package board

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

func assertGoldenNormalized(t *testing.T, output []byte, golden string) {
	t.Helper()

	normalize := func(bts []byte) []byte {
		withoutANSI := testui.AnsiEscapePattern.ReplaceAll(bts, nil)
		// Strip \r\n and lone \r to handle Windows CRLF in golden files.
		normalized := bytes.ReplaceAll(withoutANSI, []byte("\r\n"), []byte("\n"))
		normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
		trimmedNewline := bytes.TrimSuffix(normalized, []byte("\n"))
		lines := strings.Split(string(trimmedNewline), "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " ")
		}
		return []byte(strings.Join(lines, "\n"))
	}

	got := normalize(output)
	want := normalize(testui.ReadGolden(t, golden))
	if !bytes.Equal(got, want) {
		t.Fatalf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", golden, string(want), string(got))
	}
}

func TestRenderColumnRowsStylesMetadataAndSelectionIndicator(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	line := renderColumnRows(Column{
		Rows: []domain.IssueSummary{{
			ID:       "beads-workbench-u5s",
			Title:    "Redesign board rows and metadata for compact scanability",
			Type:     "task",
			Status:   "open",
			Priority: 0,
		}},
		SelectedRow: 0,
	}, 72, 0, 0)[0]

	if !strings.Contains(line, "\x1b[") {
		t.Fatalf("expected ANSI styling in rendered row, got: %q", line)
	}
	if !strings.Contains(line, "›") {
		t.Fatalf("expected selected-row indicator, got: %q", line)
	}
	if strings.Contains(line, "\x1b[48;") {
		t.Fatalf("expected no full-row background fill, got: %q", line)
	}

	plain := testui.AnsiEscapePattern.ReplaceAllString(line, "")
	if !strings.Contains(plain, "T P0 OPN u5s") {
		t.Fatalf("expected compact metadata tokens in row, got: %q", plain)
	}
}

func TestRenderColumnRowsUsesSharedIssueRowRenderer(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "beads-workbench-u5s", Title: "Shared renderer", Status: "open", Type: "task", Priority: 1}
	rows := renderColumnRows(Column{Rows: []domain.IssueSummary{issue}, SelectedRow: 0}, 60, 0, 0)
	if len(rows) != 1 {
		t.Fatalf("expected exactly one rendered row, got %d", len(rows))
	}

	want := issuerow.RenderCompact(issuerow.RenderConfig{Issue: issue, Selected: true, Width: 60, Styled: true})
	if rows[0] != want {
		t.Fatalf("expected board row to use shared renderer\nwant: %q\ngot:  %q", want, rows[0])
	}
}

func TestRenderBoardColumnsGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
		golden string
	}{
		{name: "w80", width: 80, height: 24, golden: "board_columns_w80.golden"},
		{name: "w120", width: 120, height: 28, golden: "board_columns_w120.golden"},
		{name: "w180", width: 180, height: 30, golden: "board_columns_w180.golden"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			state := State{
				DashboardTitle: "Default",
				FocusedColumn:  0,
				Width:          tc.width,
				Height:         tc.height,
				Columns: []Column{
					{
						Title: "Ready",
						Rows: []domain.IssueSummary{
							{ID: "beads-workbench-u5s", Title: "Redesign board rows and metadata for compact scanability", Type: "task", Status: "open", Priority: 0},
							{ID: "beads-workbench-1ri", Title: "Dashboard UX runtime harness and width matrix assertions", Type: "feature", Status: "in_progress", Priority: 1},
							{ID: "beads-workbench-y3x", Title: "Runtime bug: board rows hide readiness/type signals", Type: "bug", Status: "blocked", Priority: 0},
						},
						SelectedRow:  0,
						Total:        7,
						TotalIsExact: true,
					},
					{
						Title: "Blocked",
						Rows: []domain.IssueSummary{
							{ID: "beads-workbench-9oo", Title: "Acceptance review board UX regressions and fixes", Type: "task", Status: "blocked", Priority: 0},
						},
						SelectedRow:  -1,
						Total:        3,
						TotalIsExact: true,
					},
					{
						Title: "Recent Work",
						Rows: []domain.IssueSummary{
							{ID: "beads-workbench-j34", Title: "Remove large sidebar from board view", Type: "task", Status: "closed", Priority: 1},
						},
						SelectedRow:  -1,
						Total:        42,
						TotalIsExact: true,
					},
					{
						Title: "Assigned",
						Rows: []domain.IssueSummary{
							{ID: "beads-workbench-7qz", Title: "Preserve four-column grouping in dashboard renderer", Type: "epic", Status: "in_progress", Priority: 1},
						},
						SelectedRow:  -1,
						Total:        5,
						TotalIsExact: true,
					},
				},
			}

			assertEqualColumnHeights(t, Render(state))
			assertGoldenNormalized(t, []byte(Render(state)), tc.golden)
		})
	}
}

func TestRenderBoardResponsiveWideGolden(t *testing.T) {
	t.Parallel()

	state := State{
		DashboardTitle: "Default",
		FocusedColumn:  2,
		Width:          120,
		Height:         24,
		Columns: []Column{
			{Title: "Ready", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Type: "task", Status: "open", Priority: 1}}, SelectedRow: -1, Total: 4, TotalIsExact: true},
			{Title: "In Progress", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Type: "feature", Status: "in_progress", Priority: 1}}, SelectedRow: -1, Total: 1, TotalIsExact: true},
			{Title: "Blocked", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.4.5", Title: "Add editor and launcher integration tests", Type: "bug", Status: "blocked", Priority: 1}}, SelectedRow: 0, Total: 2, TotalIsExact: true},
			{Title: "Recent Work", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.4", Title: "Deliver editor and launcher flows", Type: "task", Status: "closed", Priority: 1}}, SelectedRow: -1, Total: 15, TotalIsExact: true},
			{Title: "Assigned", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.1", Title: "Acceptance Review: Task Manager UI v1 standalone product", Type: "epic", Status: "open", Priority: 1}}, SelectedRow: -1, Total: 3, TotalIsExact: true},
		},
	}

	assertEqualColumnHeights(t, Render(state))
	assertGoldenNormalized(t, []byte(Render(state)), "board_responsive_wide.golden")
}

// TestRefreshBoardCarriesDimPhaseStyle verifies that when a board column is
// in the refresh state (Loading=true, existing rows present), the rendered
// output contains the SkeletonShades[phase] ANSI color sequence, confirming
// that Dim+Phase are threaded to RenderCompact.
func TestRefreshBoardCarriesDimPhaseStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	const phase = 1 // pick a non-zero phase for a distinct shade

	rows := renderColumnRows(Column{
		Loading: true,
		Rows: []domain.IssueSummary{
			{ID: "bw-1", Title: "Stale Board Issue", Status: "open", Type: "task", Priority: 1},
		},
		SelectedRow: -1,
	}, 80, phase, 0)

	if len(rows) == 0 {
		t.Fatal("expected at least one rendered row")
	}

	// The row must be visibly present after ANSI stripping.
	plain := testui.AnsiEscapePattern.ReplaceAllString(rows[0], "")
	if !strings.Contains(plain, "Stale Board Issue") {
		t.Fatalf("stale row title not visible (ANSI-stripped), got: %q", plain)
	}

	// The row must contain the SkeletonShades[phase] dark-theme hex embedded in an ANSI escape.
	// SkeletonShades[1] dark = "#696969" → RGB(105,105,105) → ANSI 38;2;105;105;105
	const wantANSI = "38;2;105;105;105"
	if !strings.Contains(rows[0], wantANSI) {
		t.Fatalf("expected dim ANSI sequence %q in refresh row, got: %q", wantANSI, rows[0])
	}
}

func TestSkeletonRows(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	const width = 60

	// Each column index draws its row count from skeletonRowCounts so adjacent
	// columns differ in length; the count is stable across animation phases.
	for colIndex, want := range skeletonRowCounts {
		for _, phase := range []int{0, 1, 2} {
			rows := skeletonRows(width, phase, colIndex)
			if len(rows) != want {
				t.Errorf("col=%d phase=%d: skeletonRows returned %d rows, want %d", colIndex, phase, len(rows), want)
			}
			for i, row := range rows {
				if row == "" {
					t.Errorf("col=%d phase=%d row[%d]: expected non-empty skeleton string", colIndex, phase, i)
				}
			}
		}
	}

	// colIndex wraps via safe-modulo for boards with more columns than the table.
	if got := len(skeletonRows(width, 0, len(skeletonRowCounts))); got != skeletonRowCounts[0] {
		t.Errorf("colIndex wrap: got %d rows, want %d", got, skeletonRowCounts[0])
	}
}

// TestDoneColumnHeaderBadge verifies that the Done column renders the correct
// header badge depending on whether the row list is truncated.
//
// After the scroll-window fix (b38b.4), the renderer shows "N of M" whenever
// the scroll window is smaller than the full row list — regardless of
// TotalIsExact. The invariants are:
//
//   - When all rows fit in the window: show exact count, no "+" or "N of M".
//   - When the window clips rows: show "N of M", never "75+".
//
// This covers the fix for bug beads-workbench-2ev4.3: an exact closed-count
// was rendered as "75+" (lower-bound notation) because the row list was capped.
func TestDoneColumnHeaderBadge(t *testing.T) {
	t.Parallel()

	makeState := func(rows int, total int, exact bool, height int) State {
		issues := make([]domain.IssueSummary, rows)
		for i := range issues {
			issues[i] = domain.IssueSummary{
				ID:     fmt.Sprintf("bw-%d", i),
				Title:  "Closed issue",
				Status: "closed",
				Type:   "task",
			}
		}
		return State{
			DashboardTitle: "Test",
			FocusedColumn:  0,
			Width:          100,
			Height:         height,
			Columns: []Column{{
				Title:        "Done",
				Rows:         issues,
				Total:        total,
				TotalIsExact: exact,
			}},
		}
	}

	t.Run("small exact column fits in window — plain number no plus no N of M", func(t *testing.T) {
		// 5 rows, total=5, exact=true, height=24 → window=21 — all 5 rows fit.
		state := makeState(5, 5, true, 24)
		plain := testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")

		if !strings.Contains(plain, "5") {
			t.Fatalf("expected total count 5 in header, got:\n%s", plain)
		}
		if strings.Contains(plain, "5+") {
			t.Fatalf("exact count must not render with lower-bound '+', got:\n%s", plain)
		}
		if strings.Contains(plain, "of") {
			t.Fatalf("exact count must not render 'N of M' when all rows fit, got:\n%s", plain)
		}
	})

	t.Run("window clips rows — shows N of M not N+", func(t *testing.T) {
		// 75 rows, total=75, height=24 → window ~21 → clips → shows "N of 75".
		// The exact N depends on window arithmetic; verify "of 75" and no "75+".
		state := makeState(75, 75, true, 24)
		plain := testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")

		if !strings.Contains(plain, "of 75") {
			t.Fatalf("expected 'N of 75' badge when window clips rows, got:\n%s", plain)
		}
		if strings.Contains(plain, "75+") {
			t.Fatalf("clipped rows must not render lower-bound '75+', got:\n%s", plain)
		}
	})

	t.Run("truncated list shows N of M not N+", func(t *testing.T) {
		// 50 visible rows, 75 total, TotalIsExact=false, tall terminal → all 50 fit.
		// Window = height-3 = 77, so all 50 rows fit.
		// The header should show "50 of 75" because TotalIsExact=false (DB has more).
		state := makeState(50, 75, false, 80)
		plain := testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")

		if !strings.Contains(plain, "50 of 75") {
			t.Fatalf("expected '50 of 75' badge in header, got:\n%s", plain)
		}
		if strings.Contains(plain, "75+") {
			t.Fatalf("truncated list must not render exact count as lower-bound '75+', got:\n%s", plain)
		}
	})
}

// TestRenderLargeColumnScrollWindowGolden verifies the board renderer with a
// large column + non-zero ScrollOffset. The header must show "N of M" and the
// visible rows must match the scroll window (offset=10, window ~8 rows).
func TestRenderLargeColumnScrollWindowGolden(t *testing.T) {
	t.Parallel()

	// 50 rows, ScrollOffset=20, height=12 → innerHeight=10 → window shows rows 20..29.
	const rowCount = 50
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("bw-%d", i),
			Title: fmt.Sprintf("Issue number %d", i),
			Type:  "task",
		}
	}

	state := State{
		DashboardTitle: "Test",
		FocusedColumn:  0,
		Width:          80,
		Height:         12,
		Columns: []Column{{
			Title:        "Ready",
			Rows:         issues,
			SelectedRow:  20,
			ScrollOffset: 20,
			Total:        rowCount,
			TotalIsExact: true,
		}},
	}

	rendered := Render(state)
	plain := testui.AnsiEscapePattern.ReplaceAllString(rendered, "")

	// Header must show "N of 50".
	if !strings.Contains(plain, "of 50") {
		t.Errorf("expected 'of 50' in header, got:\n%s", plain)
	}

	// First row in window must be row 20.
	if !strings.Contains(plain, "number 20") {
		t.Errorf("expected row 20 to be first visible, got:\n%s", plain)
	}

	// Row 0 must NOT appear (before the window).
	if strings.Contains(plain, "number 0") {
		t.Errorf("expected row 0 to be hidden by scroll, got:\n%s", plain)
	}

	assertGoldenNormalized(t, []byte(rendered), "large_column_window.golden")
}

// TestRenderDoneLoadingMore verifies the "load more in flight" affordance: when
// col.Loading=true, ScrollOffset>0, and rows are present (60 of 736 loaded),
// the renderer shows real rows plus a skeleton row at the bottom of the visible
// window and the header reads "60 of 736".
func TestRenderDoneLoadingMore(t *testing.T) {
	t.Parallel()

	const rowCount = 60
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:     fmt.Sprintf("bw-done-%d", i),
			Title:  fmt.Sprintf("Closed issue %d", i),
			Type:   "task",
			Status: "closed",
		}
	}

	state := State{
		DashboardTitle: "Default",
		FocusedColumn:  0,
		Width:          80,
		Height:         24,
		Columns: []Column{{
			Title:        "Done",
			Rows:         issues,
			SelectedRow:  58,
			ScrollOffset: 42,
			Total:        736,
			TotalIsExact: false,
			Loading:      true,
		}},
	}

	rendered := Render(state)
	plain := testui.AnsiEscapePattern.ReplaceAllString(rendered, "")

	// Header must show loaded count of total.
	if !strings.Contains(plain, "60 of 736") {
		t.Errorf("expected '60 of 736' header badge, got:\n%s", plain)
	}

	// Selected row 58 must be visible with the cursor indicator.
	if !strings.Contains(plain, "›") {
		t.Errorf("expected cursor indicator '›' in visible window, got:\n%s", plain)
	}

	// Skeleton row affordance must appear at the bottom of the visible window.
	if !strings.Contains(plain, issuerow.SkeletonMetaGlyph) {
		t.Errorf("expected skeleton row affordance in load-more state, got:\n%s", plain)
	}

	// Rows before the scroll window must NOT be visible.
	if strings.Contains(plain, "Closed issue 0") {
		t.Errorf("expected row 0 to be hidden by scroll window, got:\n%s", plain)
	}

	assertGoldenNormalized(t, []byte(rendered), "done_loading_more.golden")
}

// TestRenderDoneDeepNavigation verifies the last-page loaded state: Done column
// with 60 of 60 issues loaded (TotalIsExact=true), Selected at the very end
// (row 58). The cursor must be visible and the header must read "60" — no "of M"
// suffix because TotalIsExact=true and all rows fit in the terminal.
func TestRenderDoneDeepNavigation(t *testing.T) {
	t.Parallel()

	const rowCount = 60
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:     fmt.Sprintf("bw-done-%d", i),
			Title:  fmt.Sprintf("Closed issue %d", i),
			Type:   "task",
			Status: "closed",
		}
	}

	// Use a tall terminal (height=66 → innerHeight=63) so all 60 rows fit in
	// the window without clipping; this lets the header show the exact count.
	state := State{
		DashboardTitle: "Default",
		FocusedColumn:  0,
		Width:          80,
		Height:         66,
		Columns: []Column{{
			Title:        "Done",
			Rows:         issues,
			SelectedRow:  58,
			ScrollOffset: 0,
			Total:        60,
			TotalIsExact: true,
			Loading:      false,
		}},
	}

	rendered := Render(state)
	plain := testui.AnsiEscapePattern.ReplaceAllString(rendered, "")

	// Header must show exact count only — no "of M" suffix.
	if !strings.Contains(plain, "60") {
		t.Errorf("expected exact count '60' in header, got:\n%s", plain)
	}
	if strings.Contains(plain, "of 60") {
		t.Errorf("expected no 'of M' suffix when TotalIsExact and all rows visible, got:\n%s", plain)
	}

	// Cursor must be visible at the selected row.
	if !strings.Contains(plain, "›") {
		t.Errorf("expected cursor indicator '›' visible, got:\n%s", plain)
	}

	// Row 58 (second from last) must be visible.
	if !strings.Contains(plain, "bw-done-58") {
		t.Errorf("expected issue bw-done-58 visible at selected row, got:\n%s", plain)
	}

	assertGoldenNormalized(t, []byte(rendered), "done_deep_navigation.golden")
}

// TestRenderErrorRowPinnedAboveScrolledWindow locks in FIX #6: when a column
// has BOTH an inline error row AND a scrolled issue window (ScrollOffset > 0),
// the error row must stay pinned at the top and the scroll window must apply to
// the issue rows only (the error row counts against innerHeight).
//
// The pre-fix renderer treated ScrollOffset (an issue index) as a raw offset
// into the rendered-rows slice — which already includes the prepended error row
// — so the window shifted up by one: the error row was sliced off entirely and
// the issue immediately ABOVE the intended window top leaked in.
//
// Layout: Height=10 -> columnHeight=9 -> innerHeight=7. With the error row
// pinned, six issue rows fit. ScrollOffset=10 puts issues 10..15 in the window;
// SelectedRow=15 is the last visible issue.
//
//   - Buggy:  rows[10:17] -> issues 09..15, error row dropped.
//   - Fixed:  [⚠ error] + issues 10..15.
//
// Discriminating assertions (fail if the fix is reverted): the pinned error row
// is present (buggy slices it off) and off-window issue 09 is absent (buggy
// leaks it via the one-row shift). The selected issue 15 is asserted present to
// pin the intended bottom edge.
func TestRenderErrorRowPinnedAboveScrolledWindow(t *testing.T) {
	t.Parallel()

	const rowCount = 20
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("bw-%02d", i),
			Title: fmt.Sprintf("Issue marker %02d", i),
			Type:  "task",
		}
	}

	state := State{
		DashboardTitle: "Test",
		FocusedColumn:  0,
		Width:          80,
		Height:         10,
		Columns: []Column{{
			Title:        "Done",
			Rows:         issues,
			SelectedRow:  15,
			ScrollOffset: 10,
			Error:        "bd query timed out",
			Total:        rowCount,
			TotalIsExact: true,
		}},
	}

	plain := testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")

	// (a) The pinned error row must survive the scroll window. The pre-fix
	// renderer sliced it off the top whenever ScrollOffset > 0.
	if !strings.Contains(plain, "⚠ load failed") {
		t.Errorf("expected pinned error row '⚠ load failed' alongside scrolled issues, got:\n%s", plain)
	}

	// (b) The selected issue (the last visible issue) must be present.
	if !strings.Contains(plain, "marker 15") {
		t.Errorf("expected selected issue 15 visible at the bottom edge, got:\n%s", plain)
	}

	// (c) Off-by-one leak: issue 09 sits one row above the intended window top.
	// The fix keeps it out of view; the buggy raw-offset slice pulled it in.
	if strings.Contains(plain, "marker 09") {
		t.Errorf("issue 09 is above the scroll window and must not appear (one-row shift), got:\n%s", plain)
	}

	// Issue 16 is below the window and must never appear (bottom-edge sanity).
	if strings.Contains(plain, "marker 16") {
		t.Errorf("issue 16 is below the scroll window and must not appear, got:\n%s", plain)
	}
}

func assertEqualColumnHeights(t *testing.T, view string) {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multi-line board view, got:\n%s", view)
	}

	columnLines := lines[1:]
	maxPipes := 0
	for _, line := range columnLines {
		count := strings.Count(line, "│")
		if count > maxPipes {
			maxPipes = count
		}
	}
	if maxPipes < 2 {
		t.Fatalf("expected bordered columns in board view, got:\n%s", view)
	}

	for _, line := range columnLines {
		count := strings.Count(line, "│")
		if count != 0 && count != maxPipes {
			t.Fatalf("expected equal-height visible columns, inconsistent bordered row %q in view:\n%s", line, view)
		}
	}
}
