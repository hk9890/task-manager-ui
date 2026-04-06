package board

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/testing/ui"
)

func assertGoldenNormalized(t *testing.T, output []byte, golden string) {
	t.Helper()

	normalize := func(bts []byte) []byte {
		trimmedNewline := bytes.TrimSuffix(bts, []byte("\n"))
		lines := strings.Split(string(trimmedNewline), "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " ")
		}
		return []byte(strings.Join(lines, "\n"))
	}

	got := normalize(output)
	want := normalize(ui.ReadGolden(t, golden))
	if !bytes.Equal(got, want) {
		t.Fatalf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", golden, string(want), string(got))
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
						Rows: []Row{
							{ID: "beads-workbench-u5s", Title: "Redesign board rows and metadata for compact scanability", Type: "task", Status: "open", Priority: 0, Selected: true},
							{ID: "beads-workbench-1ri", Title: "Dashboard UX runtime harness and width matrix assertions", Type: "feature", Status: "in_progress", Priority: 1},
							{ID: "beads-workbench-y3x", Title: "Runtime bug: board rows hide readiness/type signals", Type: "bug", Status: "blocked", Priority: 0},
						},
					},
					{
						Title: "Blocked",
						Rows: []Row{
							{ID: "beads-workbench-9oo", Title: "Acceptance review board UX regressions and fixes", Type: "task", Status: "blocked", Priority: 0},
						},
					},
					{
						Title: "Recent Work",
						Rows: []Row{
							{ID: "beads-workbench-j34", Title: "Remove large sidebar from board view", Type: "task", Status: "closed", Priority: 1},
						},
					},
					{
						Title: "Assigned",
						Rows: []Row{
							{ID: "beads-workbench-7qz", Title: "Preserve four-column grouping in dashboard renderer", Type: "epic", Status: "in_progress", Priority: 1},
						},
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
			{Title: "Ready", Rows: []Row{{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Type: "task", Status: "open", Priority: 1}}},
			{Title: "In Progress", Rows: []Row{{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Type: "feature", Status: "in_progress", Priority: 1}}},
			{Title: "Blocked", Rows: []Row{{ID: "beads-workbench-yze.4.5", Title: "Add editor and launcher integration tests", Type: "bug", Status: "blocked", Priority: 1, Selected: true}}},
			{Title: "Recent Work", Rows: []Row{{ID: "beads-workbench-yze.4", Title: "Deliver editor and launcher flows", Type: "task", Status: "closed", Priority: 1}}},
			{Title: "Assigned", Rows: []Row{{ID: "beads-workbench-yze.1", Title: "Acceptance Review: Beads Workbench v1 standalone product", Type: "epic", Status: "open", Priority: 1}}},
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
