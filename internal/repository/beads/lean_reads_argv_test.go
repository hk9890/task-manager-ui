package beads_test

// lean_reads_argv_test.go — beads-workbench-vtvb.2
//
// Argv-pinning tests for Dashboard's closed-page fetch path. Asserts the exact
// bd argv slice for ClosedOffset=0 (no --offset flag) and ClosedOffset=35
// (--offset 35 appended). Uses fakes.RecordingExecutor — no real subprocess.
//
// # Why package beads_test (external test package)
//
// Same import-cycle reason as lean_writes_test.go: fakes imports internal/bd,
// so the test must be in the external package to avoid a cycle.

import (
	"context"
	"reflect"
	"testing"

	bd "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/repository"
	repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// cannedDashboardOtherArgvs registers canned responses for the four non-closed
// Dashboard fan-out calls so the test's RecordingExecutor does not return empty
// errors for unexpected argv shapes.
func cannedDashboardOtherArgvs(rec *fakes.RecordingExecutor) {
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(bd.ExecResult{Stdout: []byte(`{
		"ready": [], "blocked": [],
		"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 0}
	}`)}, nil)
	rec.OnArgs([]string{"query", "status=in_progress", "--json"}).Return(bd.ExecResult{Stdout: []byte(`[]`)}, nil)
	rec.OnArgs([]string{"count", "--by-status", "--json", "--status", "closed"}).Return(bd.ExecResult{Stdout: []byte(`{
		"groups": [{"group": "closed", "count": 0}], "total": 0, "schema_version": 1
	}`)}, nil)
	rec.OnArgs([]string{"query", "status=blocked", "--json"}).Return(bd.ExecResult{Stdout: []byte(`[]`)}, nil)
}

// TestDashboardClosedOffsetArgv pins the exact bd argv for the two ClosedOffset
// variants. These are the canonical wire shapes for the Done-column closed fetch.
func TestDashboardClosedOffsetArgv(t *testing.T) {
	t.Parallel()

	// baseArgv is the closed-query argv when ClosedOffset == 0.
	// This must remain stable — no --offset flag must ever be added for offset=0.
	baseArgv := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"}

	// offsetArgv is the argv when ClosedOffset == 35.
	offsetArgv := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50", "--offset", "35"}

	tests := []struct {
		name      string
		opts      repository.DashboardOptions
		wantArgv  []string
		wantNoArg string // flag that must NOT appear for offset=0 case
	}{
		{
			name:      "offset_zero_no_offset_flag",
			opts:      repository.DashboardOptions{ClosedLimit: 50, ClosedOffset: 0},
			wantArgv:  baseArgv,
			wantNoArg: "--offset",
		},
		{
			name:     "offset_35_emits_offset_flag",
			opts:     repository.DashboardOptions{ClosedLimit: 50, ClosedOffset: 35},
			wantArgv: offsetArgv,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := fakes.NewRecordingExecutor()
			cannedDashboardOtherArgvs(rec)
			rec.OnArgs(tc.wantArgv).Return(bd.ExecResult{Stdout: []byte(`[]`)}, nil)

			runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
			repo := repobeads.New(runner)

			_, err := repo.Dashboard(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("Dashboard returned error: %v", err)
			}

			calls := rec.Calls()

			// Find the closed-query call among all recorded calls.
			var closedCall *fakes.RecordedCall
			for i := range calls {
				if len(calls[i].Args) >= 2 && calls[i].Args[0] == "query" && calls[i].Args[1] == "status=closed" {
					closedCall = &calls[i]
					break
				}
			}
			if closedCall == nil {
				t.Fatalf("no 'query status=closed' call found among %d recorded calls", len(calls))
			}

			if !reflect.DeepEqual(closedCall.Args, tc.wantArgv) {
				t.Errorf("closed query argv mismatch:\n  got:  %v\n  want: %v", closedCall.Args, tc.wantArgv)
			}

			// For offset=0, additionally assert --offset does not appear anywhere in argv.
			if tc.wantNoArg != "" {
				for _, arg := range closedCall.Args {
					if arg == tc.wantNoArg {
						t.Errorf("argv must not contain %q for ClosedOffset=0, but got: %v", tc.wantNoArg, closedCall.Args)
						break
					}
				}
			}
		})
	}
}
