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

// TestDashboardClosedOffsetOverFetchReturnsAll asserts the over-fetch behaviour
// when offset > 0: Dashboard must return the FULL over-fetched list (offset+limit
// items) — NOT a [offset:offset+limit] slice. This is the post-manual-fix contract
// (race-safe against concurrent closes; see lean_reads.go queryClosedPage comment).
//
// Composition responsibility moves to dashboard.Compose, which dedups prior+incoming
// by ID — so returning the full over-fetched list is correct even when the bd
// result shifts under concurrent closes (the composer keeps prior items not
// re-seen in incoming, and incoming items not already in prior).
//
// Drives this via RecordingExecutor + a fake JSON response containing 85 issues
// (over-fetch response for offset=35, limit=50). Asserts: len(Closed)==85, first
// item is fake-001, last item is fake-085.
func TestDashboardClosedOffsetOverFetchReturnsAll(t *testing.T) {
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

	// Must return all over-fetched items (offset+limit), not a [offset:] slice.
	// The composer (dashboard.Compose with PriorClosed set) is responsible for
	// dedup; returning the full list lets it correctly merge the page even when
	// concurrent closes have shifted the bd result.
	if len(data.Closed) != totalFetched {
		t.Errorf("over-fetch math: len(Closed)=%d; want %d (full over-fetch, no slice)", len(data.Closed), totalFetched)
	}

	// First item must be fake-001 (over-fetch starts at the newest, not at offset).
	if len(data.Closed) > 0 {
		wantFirstID := "fake-001"
		if data.Closed[0].ID != wantFirstID {
			t.Errorf("over-fetch math: first item ID=%q; want %q (newest, position 0)", data.Closed[0].ID, wantFirstID)
		}
	}

	// Last item must be fake-085 (last of the over-fetched list).
	if len(data.Closed) == totalFetched {
		wantLastID := fmt.Sprintf("fake-%03d", totalFetched)
		if data.Closed[totalFetched-1].ID != wantLastID {
			t.Errorf("over-fetch math: last item ID=%q; want %q", data.Closed[totalFetched-1].ID, wantLastID)
		}
	}
}

// TestIssueParentedDetailIssuesSingleShowCall pins the performance contract for
// the Parent-only change (bd-jzam): loading the detail of an issue that HAS a
// parent must issue EXACTLY ONE `bd show` subprocess call. The parent ref
// arrives in the issue's own `bd show` payload (a dependency with
// dependency_type "parent-child"), so the previous second `bd show <parent>`
// sibling fetch — ~0.4s of Dolt-open overhead per detail load — is gone. This
// test fails if anyone reintroduces a sibling/second-show fetch in Issue().
func TestIssueParentedDetailIssuesSingleShowCall(t *testing.T) {
	t.Parallel()

	rec := fakes.NewRecordingExecutor()
	rec.OnArgs([]string{"show", "child-1", "--json"}).Return(bd.ExecResult{Stdout: []byte(`[{
		"id": "child-1", "title": "Child issue", "status": "open", "issue_type": "task", "priority": 2,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
		"dependencies": [
			{"id": "parent-1", "title": "Parent epic", "issue_type": "epic", "priority": 1, "status": "open", "dependency_type": "parent-child"}
		]
	}]`)}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{Command: "bd", Executor: rec})
	repo := repobeads.New(runner)

	detail, err := repo.Issue(context.Background(), "child-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	// The Parent group surfaces the parent from the issue's OWN payload.
	if detail.ParentGroupBrowser.Parent.ID != "parent-1" {
		t.Fatalf("expected parent-1 in ParentGroupBrowser.Parent, got %#v", detail.ParentGroupBrowser.Parent)
	}

	// Crucially: exactly one `bd show`. A second `show parent-1` would be the
	// regression this test guards against.
	var showCalls int
	for _, c := range rec.Calls() {
		if len(c.Args) >= 1 && c.Args[0] == "show" {
			showCalls++
		}
	}
	if showCalls != 1 {
		t.Fatalf("expected exactly one `bd show` for a parented issue, got %d: %#v", showCalls, rec.Calls())
	}
}
