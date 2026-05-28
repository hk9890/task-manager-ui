package beads_test

// lean_reads_argv_test.go — beads-workbench-vtvb.2, updated vtvb.13
//
// Argv-pinning tests for Dashboard's closed-page fetch path. Asserts the exact
// bd argv slice for ClosedOffset=0 (no --offset flag) and ClosedOffset=35
// (over-fetch --limit 85, no --offset, bd 1.0.4 workaround). Uses
// fakes.RecordingExecutor — no real subprocess.
//
// # bd 1.0.4 workaround (vtvb.13)
//
// bd 1.0.4 does not support --offset. When offset>0, queryClosedPage emits
// --limit (offset+limit) instead and slices [offset:offset+limit] in Go.
// See TODO(bd-upstream) in lean_reads.go for the revert path when bd ships
// --offset support.
//
// # Why package beads_test (external test package)
//
// Same import-cycle reason as lean_writes_test.go: fakes imports internal/bd,
// so the test must be in the external package to avoid a cycle.

import (
	"context"
	"encoding/json"
	"fmt"
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
	// bd 1.0.4 workaround (vtvb.13): over-fetch --limit (35+50=85); no --offset.
	// TODO(bd-upstream): when bd ships --offset, this becomes
	//   ["query","status=closed","--json","-a","--sort","closed","--limit","50","--offset","35"]
	offsetArgv := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "85"}

	tests := []struct {
		name      string
		opts      repository.DashboardOptions
		wantArgv  []string
		wantNoArg string // flag that must NOT appear
	}{
		{
			name:      "offset_zero_no_offset_flag",
			opts:      repository.DashboardOptions{ClosedLimit: 50, ClosedOffset: 0},
			wantArgv:  baseArgv,
			wantNoArg: "--offset",
		},
		{
			name:      "offset_35_overfetch_no_offset_flag",
			opts:      repository.DashboardOptions{ClosedLimit: 50, ClosedOffset: 35},
			wantArgv:  offsetArgv,
			wantNoArg: "--offset",
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

			// Assert --offset does not appear anywhere in argv.
			if tc.wantNoArg != "" {
				for _, arg := range closedCall.Args {
					if arg == tc.wantNoArg {
						t.Errorf("argv must not contain %q, but got: %v", tc.wantNoArg, closedCall.Args)
						break
					}
				}
			}
		})
	}
}

// TestDashboardClosedOffsetSliceMath asserts the in-memory slice behaviour when
// offset > 0: Dashboard must return exactly `limit` items starting at position
// `offset` in the over-fetched list.
//
// Drives this via RecordingExecutor + a fake JSON response containing 85 issues
// (simulating the over-fetch response for offset=35, limit=50). The returned
// slice must have len==50 and the first item must be the 36th issue in the fake
// list (fake-036), the last must be fake-085.
func TestDashboardClosedOffsetSliceMath(t *testing.T) {
	t.Parallel()

	const offset = 35
	const limit = 50
	const totalFetched = offset + limit // 85

	// Build a fake JSON array of 85 issues: fake-001 … fake-085.
	// All required fields must be non-nil for leanToIssueSummary to decode
	// without error: id, title, status, issue_type, priority, created_at, updated_at.
	type issueStub struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		IssueType string `json:"issue_type"`
		Priority  int    `json:"priority"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	fakeItems := make([]issueStub, totalFetched)
	for i := range fakeItems {
		fakeItems[i] = issueStub{
			ID:        fmt.Sprintf("fake-%03d", i+1),
			Title:     fmt.Sprintf("Issue %03d", i+1),
			Status:    "closed",
			IssueType: "task",
			Priority:  1,
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		}
	}
	fakeJSON, err := json.Marshal(fakeItems)
	if err != nil {
		t.Fatalf("marshal fake JSON: %v", err)
	}

	overfetchArgv := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "85"}

	rec := fakes.NewRecordingExecutor()
	cannedDashboardOtherArgvs(rec)
	rec.OnArgs(overfetchArgv).Return(bd.ExecResult{Stdout: fakeJSON}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	data, err := repo.Dashboard(context.Background(), repository.DashboardOptions{
		ClosedLimit:  limit,
		ClosedOffset: offset,
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}

	// Must return exactly `limit` items.
	if len(data.Closed) != limit {
		t.Errorf("slice math: len(Closed)=%d; want %d", len(data.Closed), limit)
	}

	// First item must be the 36th issue in the fake list (fake-036).
	if len(data.Closed) > 0 {
		wantFirstID := fmt.Sprintf("fake-%03d", offset+1) // "fake-036"
		if data.Closed[0].ID != wantFirstID {
			t.Errorf("slice math: first item ID=%q; want %q (36th-newest)", data.Closed[0].ID, wantFirstID)
		}
	}

	// Last item must be fake-085.
	if len(data.Closed) == limit {
		wantLastID := fmt.Sprintf("fake-%03d", totalFetched) // "fake-085"
		if data.Closed[limit-1].ID != wantLastID {
			t.Errorf("slice math: last item ID=%q; want %q", data.Closed[limit-1].ID, wantLastID)
		}
	}
}
