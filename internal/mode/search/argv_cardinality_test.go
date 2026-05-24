package search

// argv_cardinality_test.go — ppja.3
//
// These tests wire the search model against a real *beads.Gateway backed by a
// *fakes.RecordingExecutor (no FakeBeadsGateway). They assert that Init() and
// triggerSearch() emit exactly the expected bd argv shapes.
//
// Pattern mirrors TestBoardInitRealGatewaySubprocessArgvCardinality in
// internal/mode/board/model_test.go.

import (
	"reflect"
	"testing"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	repositorybeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// newSearchRecordingModel wires the search model against a real beads.Gateway
// backed by rec so subprocess calls flow through the real argv assembly logic.
func newSearchRecordingModel(rec *fakes.RecordingExecutor) *Model {
	runner := beads.NewCommandRunner(beads.RunnerConfig{
		Command:  "bd",
		Executor: rec,
	})
	gw := beads.NewCLIGateway(runner)
	repo := repositorybeads.New(gw)
	return NewModel(repo, nil)
}

// driveSearchInitCmd executes the tea.Cmd returned by m.Init() to drive the
// underlying subprocess call.
func driveSearchInitCmd(t *testing.T, m *Model) {
	t.Helper()

	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("search.Model.Init() must return a non-nil command")
	}
	_ = cmd() // executes the subprocess call through the real gateway
}

// assertSearchArgvPresent fails the test unless at least one recorded call
// has args that exactly match want.
func assertSearchArgvPresent(t *testing.T, calls []fakes.RecordedCall, want []string) {
	t.Helper()
	for _, c := range calls {
		if reflect.DeepEqual(c.Args, want) {
			return
		}
	}
	argSlices := make([][]string, len(calls))
	for i, c := range calls {
		argSlices[i] = c.Args
	}
	t.Errorf("expected subprocess call with argv %v; got calls: %v", want, argSlices)
}

// TestSearchModeInitArgvShapeEmptyQuery verifies that search.Model.Init() with
// height=0 (default before any WindowSizeMsg) emits exactly:
//
//	bd list --json --all --limit 20
//
// This is the empty-text / no-WorkState path (searchIssuesFromList).
// Height=0 → searchItemCapacity()=20 → Limit=20.
//
// This is ppja.3 backlog item 4 (search mode consumer level).
func TestSearchModeInitArgvShapeEmptyQuery(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"list", "--json", "--all", "--limit", "20"}

	rec := fakes.NewRecordingExecutor()
	// Pre-register expected argv so the decode path succeeds.
	rec.OnArgs(wantArgv).Return(beads.ExecResult{Stdout: []byte(`[]`)}, nil)

	m := newSearchRecordingModel(rec)
	driveSearchInitCmd(t, m)

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 subprocess call on search.Init(), got %d: %v",
			len(calls), func() [][]string {
				out := make([][]string, len(calls))
				for i, c := range calls {
					out[i] = c.Args
				}
				return out
			}())
	}

	assertSearchArgvPresent(t, calls, wantArgv)
}

// TestSearchModeInitArgvShapeAtLimitBoundaries verifies the --limit value in
// the argv for search.Model.Init() at four representative terminal heights.
// Each height maps through searchItemCapacity() to a specific --limit N.
//
// Height=0 (no WindowSizeMsg) → capacity=20  → bd list --json --all --limit 20
// Height=1                    → capacity=1   → bd list --json --all --limit 1
// Height=8  (chrome=7)        → capacity=1   → bd list --json --all --limit 1
// Height=30                   → capacity=23  → bd list --json --all --limit 23
//
// This is ppja.3 backlog item 4, dynamic-flag boundary coverage.
func TestSearchModeInitArgvShapeAtLimitBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		height int
		want   []string
	}{
		{
			name:   "height=0 (default, capacity=20)",
			height: 0,
			want:   []string{"list", "--json", "--all", "--limit", "20"},
		},
		{
			name:   "height=1 (min terminal, capacity=1)",
			height: 1,
			want:   []string{"list", "--json", "--all", "--limit", "1"},
		},
		{
			name:   "height=8 (chrome=7, capacity=1 after clamp)",
			height: 8,
			want:   []string{"list", "--json", "--all", "--limit", "1"},
		},
		{
			name:   "height=30 (capacity=23)",
			height: 30,
			want:   []string{"list", "--json", "--all", "--limit", "23"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := fakes.NewRecordingExecutor()
			rec.OnArgs(tc.want).Return(beads.ExecResult{Stdout: []byte(`[]`)}, nil)

			m := newSearchRecordingModel(rec)
			m.SetSize(80, tc.height)
			driveSearchInitCmd(t, m)

			calls := rec.Calls()
			if len(calls) != 1 {
				t.Fatalf("expected 1 subprocess call, got %d", len(calls))
			}

			assertSearchArgvPresent(t, calls, tc.want)
		})
	}
}
