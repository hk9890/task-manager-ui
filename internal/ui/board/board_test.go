package board

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/beads-workbench/internal/domain"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
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
	}, 72)[0]

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
	rows := renderColumnRows(Column{Rows: []domain.IssueSummary{issue}, SelectedRow: 0}, 60)
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
			{Title: "Assigned", Rows: []domain.IssueSummary{{ID: "beads-workbench-yze.1", Title: "Acceptance Review: Beads Workbench v1 standalone product", Type: "epic", Status: "open", Priority: 1}}, SelectedRow: -1, Total: 3, TotalIsExact: true},
		},
	}

	assertEqualColumnHeights(t, Render(state))
	assertGoldenNormalized(t, []byte(Render(state)), "board_responsive_wide.golden")
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
