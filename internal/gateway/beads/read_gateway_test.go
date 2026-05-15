package beads

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestGatewayListIssuesBuildsCommandAndMapsSummaries(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json", "--status", "open,blocked", "--type", "task,bug", "--assignee", "alice", "--label", "ui", "--label", "backend", "--sort", "updated", "--reverse", "--limit", "2"}): {
			result: ExecResult{Stdout: readFixture(t, "list_issues.json")},
		},
	}

	gateway, exec := newTestGateway(routes)

	got, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{
		Statuses:  []string{"open", "blocked"},
		Types:     []string{"task", "bug"},
		Assignee:  "alice",
		Labels:    []string{"ui", "backend"},
		SortBy:    domain.SortFieldUpdatedAt,
		SortOrder: domain.SortDirectionDescending,
		Limit:     1,
		Offset:    1,
	})
	if err != nil {
		t.Fatalf("ListIssues returned error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(got))
	}

	if got[0].ID != "bw-102" || got[0].Type != "bug" || got[0].Assignee != "bob" {
		t.Fatalf("unexpected issue summary: %#v", got[0])
	}

	if len(exec.calls) != 1 {
		t.Fatalf("expected one command invocation, got %d", len(exec.calls))
	}
}

func TestGatewayReadyIssuesPaginatesResults(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--json", "--limit", "2"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":2,"owner":"a","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"bug","priority":1,"owner":"b","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ReadyIssues(context.Background(), domain.ReadyIssuesQuery{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ReadyIssues returned error: %v", err)
	}

	if len(got) != 1 || got[0].ID != "bw-2" || got[0].Assignee != "b" {
		t.Fatalf("unexpected ready issues: %#v", got)
	}
}

func TestGatewayBlockedIssuesMapsBlockedBy(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"blocked", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"blocked","status":"blocked","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","blocked_by":["bw-0"]}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.BlockedIssues(context.Background(), domain.BlockedIssuesQuery{})
	if err != nil {
		t.Fatalf("BlockedIssues returned error: %v", err)
	}

	if len(got) != 1 || len(got[0].BlockedBy) != 1 || got[0].BlockedBy[0].ID != "bw-0" {
		t.Fatalf("unexpected blocked issues: %#v", got)
	}
}

func TestGatewayShowIssueMapsDetail(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-4", "--json"}): {
			result: ExecResult{Stdout: readFixture(t, "show_issue.json")},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-4"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.Summary.ID != "bw-201" || got.Description != "Detailed issue body from fixture" {
		t.Fatalf("unexpected summary/detail: %#v", got)
	}

	if got.Notes != "Follow-up notes from fixture" {
		t.Fatalf("unexpected notes: %q", got.Notes)
	}

	if len(got.BlockedBy) != 1 || got.BlockedBy[0].ID != "bw-150" {
		t.Fatalf("unexpected blocked-by: %#v", got.BlockedBy)
	}

	if got.BlockedBy[0].Title != "Dependency issue" || got.BlockedBy[0].Type != "bug" || got.BlockedBy[0].Priority != 1 || got.BlockedBy[0].Status != "open" {
		t.Fatalf("unexpected blocked-by reference metadata: %#v", got.BlockedBy[0])
	}

	if len(got.Blocks) != 1 || got.Blocks[0].ID != "bw-250" {
		t.Fatalf("unexpected blocks: %#v", got.Blocks)
	}

	if got.Blocks[0].Title != "Dependent issue" || got.Blocks[0].Type != "task" || got.Blocks[0].Priority != 2 || got.Blocks[0].Status != "in_progress" {
		t.Fatalf("unexpected blocks reference metadata: %#v", got.Blocks[0])
	}

	if len(got.Related) != 1 || got.Related[0].ID != "bw-350" {
		t.Fatalf("unexpected related: %#v", got.Related)
	}

	if got.Related[0].Title != "Related issue" || got.Related[0].Type != "spike" || got.Related[0].Priority != 3 || got.Related[0].Status != "blocked" {
		t.Fatalf("unexpected related reference metadata: %#v", got.Related[0])
	}

	if len(got.Comments) != 1 || got.Comments[0].Body != "Looks good" {
		t.Fatalf("unexpected comments: %#v", got.Comments)
	}

	if got.Creator != "carol" {
		t.Fatalf("unexpected creator: %q", got.Creator)
	}

	expectedClosedAt := time.Date(2026, time.April, 8, 16, 0, 0, 0, time.UTC)
	if !got.ClosedAt.Equal(expectedClosedAt) {
		t.Fatalf("unexpected closed_at: got %s want %s", got.ClosedAt.Format(time.RFC3339), expectedClosedAt.Format(time.RFC3339))
	}

	if got.CloseReason != "completed" {
		t.Fatalf("unexpected close reason: %q", got.CloseReason)
	}

	if got.ParentGroupBrowser.Parent.ID != "" || len(got.ParentGroupBrowser.Children) != 0 {
		t.Fatalf("expected empty parent-group browser context when parent is absent, got %#v", got.ParentGroupBrowser)
	}
}

func TestGatewayShowIssueMapsParentGroupBrowserContextFromParentChildRelationships(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-42", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-42","title":"child issue","description":"detail","status":"open","issue_type":"task","priority":2,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"parent issue","issue_type":"epic","priority":1,"status":"open","dependency_type":"parent-child"},{"id":"bw-50","title":"blocker issue","issue_type":"bug","priority":1,"status":"open","dependency_type":"blocks"},{"id":"bw-90","title":"dependency-related issue","issue_type":"spike","priority":3,"status":"blocked","dependency_type":"related"}],"related":[{"id":"bw-91","title":"top-level related issue","issue_type":"task","priority":2,"status":"open"}]}
			]`)},
		},
		argsKey([]string{"show", "bw-1", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"parent issue","description":"detail","status":"open","issue_type":"epic","priority":1,"created_at":"2026-04-04T09:00:00Z","updated_at":"2026-04-04T10:00:00Z","dependents":[{"id":"bw-42","title":"child issue","issue_type":"task","priority":2,"status":"open","dependency_type":"parent-child"},{"id":"bw-43","title":"sibling issue","issue_type":"task","priority":3,"status":"in_progress","dependency_type":"parent-child"},{"id":"bw-99","title":"non-child dependent","issue_type":"task","priority":3,"status":"open","dependency_type":"blocks"}]}
			]`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-42"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.ParentGroupBrowser.Parent.ID != "bw-1" {
		t.Fatalf("expected parent-group parent bw-1, got %#v", got.ParentGroupBrowser.Parent)
	}

	if len(got.ParentGroupBrowser.Children) != 2 {
		t.Fatalf("expected two parent-child siblings, got %#v", got.ParentGroupBrowser.Children)
	}

	if got.ParentGroupBrowser.Children[0].ID != "bw-42" || got.ParentGroupBrowser.Children[1].ID != "bw-43" {
		t.Fatalf("unexpected parent-child sibling mapping: %#v", got.ParentGroupBrowser.Children)
	}

	if len(got.BlockedBy) != 1 || got.BlockedBy[0].ID != "bw-50" {
		t.Fatalf("expected only non-parent blockers in blocked-by, got %#v", got.BlockedBy)
	}

	if len(got.Related) != 2 || got.Related[0].ID != "bw-91" || got.Related[1].ID != "bw-90" {
		t.Fatalf("expected generic related refs to remain separate from parent-group, got %#v", got.Related)
	}

	if len(exec.calls) != 2 {
		t.Fatalf("expected child and parent show calls, got %d", len(exec.calls))
	}
}

func TestGatewayShowIssueReturnsEmptyParentGroupBrowserContextWhenNoParent(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-77", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-77","title":"no parent issue","description":"detail","status":"open","issue_type":"task","priority":2,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-50","title":"blocker issue","dependency_type":"blocks"}]}
			]`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-77"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.ParentGroupBrowser.Parent.ID != "" || len(got.ParentGroupBrowser.Children) != 0 {
		t.Fatalf("expected empty parent-group browser context, got %#v", got.ParentGroupBrowser)
	}

	if len(exec.calls) != 1 {
		t.Fatalf("expected no parent lookup when issue has no parent-child dependency, got %d calls", len(exec.calls))
	}
}

func TestGatewayShowIssuePrefersAssigneeOverOwner(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-7", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-7","title":"assignee precedence","description":"detail","status":"open","issue_type":"task","priority":1,"assignee":"bob","owner":"hans.kohlreiter@dynatrace.com","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-7"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.Summary.Assignee != "bob" {
		t.Fatalf("expected assignee to prefer assignee field, got %q", got.Summary.Assignee)
	}
}

func TestGatewayShowIssueFallsBackToOwnerWhenAssigneeMissing(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-8", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-8","title":"assignee fallback","description":"detail","status":"open","issue_type":"task","priority":1,"owner":"legacy-owner","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-8"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.Summary.Assignee != "legacy-owner" {
		t.Fatalf("expected assignee fallback to owner when assignee missing, got %q", got.Summary.Assignee)
	}

	if got.Creator != "legacy-owner" {
		t.Fatalf("expected creator to decode owner, got %q", got.Creator)
	}
}

func TestGatewayShowIssueLeavesCreatorAndCloseMetadataEmptyWhenAbsent(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-9", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-9","title":"metadata absent","description":"detail","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-9"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.Creator != "" {
		t.Fatalf("expected empty creator, got %q", got.Creator)
	}

	if !got.ClosedAt.IsZero() {
		t.Fatalf("expected zero closed_at, got %s", got.ClosedAt.Format(time.RFC3339))
	}

	if got.CloseReason != "" {
		t.Fatalf("expected empty close reason, got %q", got.CloseReason)
	}
}

func TestGatewaySearchIssuesBuildsCommandAndReturnsPage(t *testing.T) {
	t.Parallel()

	priorityMin := 1
	priorityMax := 2

	routes := map[string]routeResponse{
		argsKey([]string{"search", "gateway", "--json", "--status", "open", "--type", "task", "--priority-min", "1", "--priority-max", "2", "--assignee", "alice", "--label", "ui", "--limit", "2"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:        "gateway",
		Statuses:    []string{"open"},
		Types:       []string{"task"},
		PriorityMin: &priorityMin,
		PriorityMax: &priorityMax,
		Assignee:    "alice",
		Labels:      []string{"ui"},
		Limit:       1,
		Offset:      1,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 1 || got.Metadata.Completeness != domain.SearchResultCompletenessMaybeMore || got.Metadata.Source != domain.SearchResultSourceBDSearch || got.Metadata.Notice == "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" || got.Results[0].Issue.Assignee != "bob" {
		t.Fatalf("unexpected search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesEmptyTextUsesListCommandFallback(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json", "--all", "--limit", "2"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 1 || got.Metadata.Completeness != domain.SearchResultCompletenessMaybeMore || got.Metadata.Source != domain.SearchResultSourceBDListFallback || got.Metadata.Notice == "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" {
		t.Fatalf("unexpected fallback search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesEmptyTextWithFiltersUsesListCommandFallback(t *testing.T) {
	t.Parallel()

	priorityMin := 1
	priorityMax := 2
	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json", "--status", "open", "--type", "task", "--priority-min", "1", "--priority-max", "2", "--assignee", "alice", "--label", "ui", "--limit", "2"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Statuses:    []string{"open"},
		Types:       []string{"task"},
		PriorityMin: &priorityMin,
		PriorityMax: &priorityMax,
		Assignee:    "alice",
		Labels:      []string{"ui"},
		Limit:       1,
		Offset:      1,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 1 || got.Metadata.Completeness != domain.SearchResultCompletenessMaybeMore || got.Metadata.Source != domain.SearchResultSourceBDListFallback || got.Metadata.Notice == "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" {
		t.Fatalf("unexpected filtered fallback search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesWorkStateReadyUsesReadyAndLocalFilters(t *testing.T) {
	t.Parallel()

	priorityMin := 1
	priorityMax := 1
	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"gateway parser","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"gateway parser docs","status":"open","issue_type":"task","priority":2,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-3","title":"other","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:        "gateway",
		Statuses:    []string{"open"},
		Types:       []string{"task"},
		PriorityMin: &priorityMin,
		PriorityMax: &priorityMax,
		Assignee:    "alice",
		Labels:      []string{"ui"},
		WorkState:   domain.WorkStateReady,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 0 || got.Metadata.Completeness != domain.SearchResultCompletenessExact || got.Metadata.Source != domain.SearchResultSourceReadyFilter || got.Metadata.Notice != "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
		t.Fatalf("unexpected ready-filtered search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesWorkStateBlockedUsesBlockedAndLocalFilters(t *testing.T) {
	t.Parallel()

	priorityMin := 0
	priorityMax := 1
	routes := map[string]routeResponse{
		argsKey([]string{"blocked", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"gateway deadlock","status":"blocked","issue_type":"bug","priority":1,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"gateway deadlock","status":"blocked","issue_type":"bug","priority":2,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-3","title":"gateway deadlock","status":"blocked","issue_type":"task","priority":1,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:        "gateway",
		Statuses:    []string{"blocked"},
		Types:       []string{"bug"},
		PriorityMin: &priorityMin,
		PriorityMax: &priorityMax,
		Assignee:    "alice",
		Labels:      []string{"backend", "ui"},
		WorkState:   domain.WorkStateBlocked,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 0 || got.Metadata.Completeness != domain.SearchResultCompletenessExact || got.Metadata.Source != domain.SearchResultSourceBlockedFilter || got.Metadata.Notice != "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
		t.Fatalf("unexpected blocked-filtered search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesWithoutLimitMarksBackendResultsPartialNotExact(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"search", "gateway", "--json", "--status", "all"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{Text: "gateway"})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 0 || got.Metadata.Completeness != domain.SearchResultCompletenessPartial || got.Metadata.Source != domain.SearchResultSourceBDSearch || got.Metadata.Notice == "" {
		t.Fatalf("unexpected unlimited search metadata: %#v", got.Metadata)
	}
}

func TestGatewaySearchIssuesRejectsInvalidPriorityRange(t *testing.T) {
	t.Parallel()

	priorityMin := 3
	priorityMax := 1
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: &routingExecutor{routes: map[string]routeResponse{}}}))

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{PriorityMin: &priorityMin, PriorityMax: &priorityMax})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayCatalogReads(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"statuses", "--json"}): {
			result: ExecResult{Stdout: readFixture(t, "statuses.json")},
		},
		argsKey([]string{"types", "--json"}): {
			result: ExecResult{Stdout: readFixture(t, "types.json")},
		},
		argsKey([]string{"label", "list-all", "--json"}): {
			result: ExecResult{Stdout: readFixture(t, "labels.json")},
		},
	}

	gateway, _ := newTestGateway(routes)

	statuses, err := gateway.StatusCatalog(context.Background())
	if err != nil {
		t.Fatalf("StatusCatalog returned error: %v", err)
	}

	types, err := gateway.TypeCatalog(context.Background())
	if err != nil {
		t.Fatalf("TypeCatalog returned error: %v", err)
	}

	labels, err := gateway.LabelCatalog(context.Background())
	if err != nil {
		t.Fatalf("LabelCatalog returned error: %v", err)
	}

	if !reflect.DeepEqual(statuses, []domain.StatusOption{{Name: "open", Description: "Open"}, {Name: "closed", Description: "Closed"}, {Name: "qa", Description: "In QA"}}) {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}

	if !reflect.DeepEqual(types, []domain.TypeOption{{Name: "task", Description: "Task"}, {Name: "bug", Description: "Bug"}, {Name: "spike", Description: "Spike"}}) {
		t.Fatalf("unexpected types: %#v", types)
	}

	if !reflect.DeepEqual(labels, []domain.LabelOption{{Name: "gateway"}, {Name: "backend"}, {Name: "docs"}}) {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestGatewayReadMethodsReturnNormalizedFailures(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json"}): {
			result: ExecResult{ExitCode: 2, Stderr: []byte("bad args")},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayReadMethodsSurfaceExecutorStderr(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json"}): {
			result: ExecResult{Stderr: []byte("permission denied")},
			err:    fmt.Errorf("spawn failed"),
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
	assertContains(t, err.Error(), "permission denied")
}

func TestGatewayReadMappingReturnsDecodeError(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayShowIssueRequiresID(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: &routingExecutor{routes: map[string]routeResponse{}}}))

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayShowIssueReturnsNotFoundOnEmptyResponse(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-404", "--json"}): {
			result: ExecResult{Stdout: []byte(`[]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-404"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeNotFound)
}

func TestGatewayShowIssueDecodeFailureWhenDependencyIsMissingTitle(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-9", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-9","title":"bad dependency","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1"}]}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-9"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayShowIssueReferenceMetadataIsOptional(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-10", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-10","title":"optional refs","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"dep"}],"dependents":[{"id":"bw-2","title":"child"}],"related":[{"id":"bw-3","title":"rel"}]}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-10"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.BlockedBy[0].Type != "" || got.BlockedBy[0].Priority != 0 || got.BlockedBy[0].Status != "" {
		t.Fatalf("expected zero-value optional metadata on dependency reference, got %#v", got.BlockedBy[0])
	}

	if got.Blocks[0].Type != "" || got.Blocks[0].Priority != 0 || got.Blocks[0].Status != "" {
		t.Fatalf("expected zero-value optional metadata on dependent reference, got %#v", got.Blocks[0])
	}

	if got.Related[0].Type != "" || got.Related[0].Priority != 0 || got.Related[0].Status != "" {
		t.Fatalf("expected zero-value optional metadata on related reference, got %#v", got.Related[0])
	}
}

func TestGatewayShowIssueMapsRelatedFromDependenciesWhenDependencyTypeIsRelatedOrRelatesTo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		dependencyType string
	}{
		{name: "related", dependencyType: "related"},
		{name: "relates-to", dependencyType: "relates-to"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			routes := map[string]routeResponse{
				argsKey([]string{"show", "bw-11", "--json"}): {
					result: ExecResult{Stdout: []byte(fmt.Sprintf(`[
						{"id":"bw-11","title":"related in dependencies","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"real blocker","dependency_type":"blocks"},{"id":"bw-3","title":"real related","dependency_type":"%s"}],"dependents":[{"id":"bw-2","title":"child"}]}
					]`, tc.dependencyType))},
				},
			}

			gateway, _ := newTestGateway(routes)

			got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-11"})
			if err != nil {
				t.Fatalf("ShowIssue returned error: %v", err)
			}

			if len(got.BlockedBy) != 1 || got.BlockedBy[0].ID != "bw-1" {
				t.Fatalf("expected only non-related dependency refs in blocked-by, got %#v", got.BlockedBy)
			}

			if len(got.Related) != 1 || got.Related[0].ID != "bw-3" {
				t.Fatalf("expected related refs from dependencies, got %#v", got.Related)
			}
		})
	}
}

func TestGatewayShowIssueMapsRelatedFromDependentsWhenDependencyTypeIsRelatesTo(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-13", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-13","title":"related in dependents","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependents":[{"id":"bw-21","title":"true dependent","dependency_type":"blocks"},{"id":"bw-22","title":"related dependent","dependency_type":"relates-to"}]}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-13"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if len(got.Blocks) != 1 || got.Blocks[0].ID != "bw-21" {
		t.Fatalf("expected only non-related dependents in blocks, got %#v", got.Blocks)
	}

	if len(got.Related) != 1 || got.Related[0].ID != "bw-22" {
		t.Fatalf("expected relates-to dependent in related, got %#v", got.Related)
	}
}

func TestGatewayShowIssueMergesTopLevelAndDependencyRelatedWithoutDuplicates(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-12", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-12","title":"mixed related","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-4","title":"from deps related","dependency_type":"related"},{"id":"bw-5","title":"non related dep"}],"related":[{"id":"bw-4","title":"from top-level related"},{"id":"bw-6","title":"top-level only"}]}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-12"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if len(got.BlockedBy) != 1 || got.BlockedBy[0].ID != "bw-5" {
		t.Fatalf("expected non-related dependency to remain blocked-by, got %#v", got.BlockedBy)
	}

	if len(got.Related) != 2 {
		t.Fatalf("expected merged related refs with de-duplication, got %#v", got.Related)
	}

	if got.Related[0].ID != "bw-4" || got.Related[1].ID != "bw-6" {
		t.Fatalf("unexpected related merge order/content: %#v", got.Related)
	}
}

func TestGatewayCountIssuesDecodesMultiGroup(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"count", "--by-status", "--json"}): {
			result: ExecResult{Stdout: []byte(`{"groups":[{"count":5,"group":"open"},{"count":353,"group":"closed"}],"schema_version":1,"total":358}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	if got.Total != 358 {
		t.Fatalf("expected Total=358, got %d", got.Total)
	}

	if len(got.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %#v", len(got.Groups), got.Groups)
	}

	openGroup := got.Groups[0]
	if openGroup.Status != "open" || openGroup.Count != 5 {
		t.Fatalf("expected open group {open 5}, got %#v", openGroup)
	}

	closedGroup := got.Groups[1]
	if closedGroup.Status != "closed" || closedGroup.Count != 353 {
		t.Fatalf("expected closed group {closed 353}, got %#v", closedGroup)
	}
}

func TestGatewayCountIssuesDecodesSingleGroup(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"count", "--by-status", "--json", "--status", "open"}): {
			result: ExecResult{Stdout: []byte(`{"groups":[{"count":5,"group":"open"}],"schema_version":1,"total":5}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{Statuses: []string{"open"}})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	if got.Total != 5 {
		t.Fatalf("expected Total=5, got %d", got.Total)
	}

	if len(got.Groups) != 1 {
		t.Fatalf("expected 1 group (closed omitted because zero), got %d: %#v", len(got.Groups), got.Groups)
	}

	if got.Groups[0].Status != "open" || got.Groups[0].Count != 5 {
		t.Fatalf("expected open group {open 5}, got %#v", got.Groups[0])
	}
}

func TestGatewayCountIssuesDecodesEmptyGroups(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"count", "--by-status", "--json"}): {
			result: ExecResult{Stdout: []byte(`{"groups":[],"schema_version":1,"total":0}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	if got.Total != 0 {
		t.Fatalf("expected Total=0, got %d", got.Total)
	}

	if len(got.Groups) != 0 {
		t.Fatalf("expected empty Groups, got %#v", got.Groups)
	}
}

func TestGatewayCountIssuesPassesFilters(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"count", "--by-status", "--json", "--status", "closed", "--type", "bug", "--assignee", "alice", "--label", "backend"}): {
			result: ExecResult{Stdout: []byte(`{"groups":[{"count":10,"group":"closed"}],"schema_version":1,"total":10}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{
		Statuses: []string{"closed"},
		Types:    []string{"bug"},
		Assignee: "alice",
		Labels:   []string{"backend"},
	})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	if got.Total != 10 {
		t.Fatalf("expected Total=10, got %d", got.Total)
	}
}

func TestMapListSortFieldClosedAtMapsToClosedFlag(t *testing.T) {
	t.Parallel()

	got := mapListSortField(domain.SortFieldClosedAt)
	if got != "closed" {
		t.Fatalf("expected SortFieldClosedAt to map to %q, got %q", "closed", got)
	}
}

func TestGatewayListIssuesUsesClosedSortFlagForClosedAtField(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"list", "--json", "--status", "closed", "--sort", "closed", "--reverse", "--limit", "5"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-B","title":"B closed recent","status":"closed","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","closed_at":"2026-04-01T00:00:00Z"},
				{"id":"bw-A","title":"A closed earlier updated later","status":"closed","issue_type":"task","priority":1,"owner":"bob","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-05-01T00:00:00Z","closed_at":"2026-01-01T00:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{
		Statuses:  []string{"closed"},
		SortBy:    domain.SortFieldClosedAt,
		SortOrder: domain.SortDirectionDescending,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("ListIssues returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(got))
	}

	// B (closed 2026-04-01) should appear before A (closed 2026-01-01, updated 2026-05-01)
	// because the backend sorts by closed_at, not updated_at.
	if got[0].ID != "bw-B" || got[1].ID != "bw-A" {
		t.Fatalf("expected B before A when sorted by closed_at desc, got %s, %s", got[0].ID, got[1].ID)
	}
}

func TestGatewayHealthCheckIssuesPingJSON(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ping", "--json"}): {
			result: ExecResult{Stdout: []byte(`{"status":"ok","total_ms":42}`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	err := gateway.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned unexpected error: %v", err)
	}

	if len(exec.calls) != 1 {
		t.Fatalf("expected one command invocation, got %d", len(exec.calls))
	}

	if len(exec.calls[0]) != 2 || exec.calls[0][0] != "ping" || exec.calls[0][1] != "--json" {
		t.Fatalf("expected argv [ping --json], got %v", exec.calls[0])
	}
}

func TestGatewayHealthCheckNoDatabaseReturnsNoDatabaseFound(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ping", "--json"}): {
			result: ExecResult{ExitCode: 1, Stderr: []byte("Error: no beads database found")},
		},
	}

	gateway, _ := newTestGateway(routes)

	err := gateway.HealthCheck(context.Background())
	assertGatewayErrorCode(t, err, domain.ErrorCodeNoDatabaseFound)
}

func TestGatewayHealthCheckBdNotFoundReturnsCommandUnavailable(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ping", "--json"}): {
			err: exec.ErrNotFound,
		},
	}

	gateway, _ := newTestGateway(routes)

	err := gateway.HealthCheck(context.Background())
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandUnavailable)
}

type routeResponse struct {
	result ExecResult
	err    error
}

type routingExecutor struct {
	routes map[string]routeResponse
	calls  [][]string
}

func (e *routingExecutor) Run(_ context.Context, _ string, args []string, _ string, _ []string) (ExecResult, error) {
	e.calls = append(e.calls, append([]string(nil), args...))

	resp, ok := e.routes[argsKey(args)]
	if !ok {
		return ExecResult{}, fmt.Errorf("unexpected args: %s", strings.Join(args, " "))
	}

	if resp.err != nil {
		return resp.result, resp.err
	}

	return resp.result, nil
}

func newTestGateway(routes map[string]routeResponse) (*Gateway, *routingExecutor) {
	exec := &routingExecutor{routes: routes}
	runner := NewCommandRunner(RunnerConfig{Executor: exec})
	return NewCLIGateway(runner), exec
}

func argsKey(args []string) string {
	return strings.Join(args, "\x00")
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %q: %v", name, err)
	}

	return data
}

func TestGatewayQueryHappyPathReturnsIssueSummaries(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"query", "status=in_progress", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"in progress one","status":"in_progress","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"in progress two","status":"in_progress","issue_type":"bug","priority":1,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.Query(context.Background(), "status=in_progress", domain.QueryOptions{})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(got))
	}

	if got[0].ID != "bw-1" || got[0].Status != "in_progress" || got[0].Assignee != "alice" {
		t.Fatalf("unexpected first issue: %#v", got[0])
	}
}

func TestGatewayQueryArgvAssemblyWithAllOptions(t *testing.T) {
	t.Parallel()

	// Limit=1, Offset=1 → withOffsetWindow(1,1)=2 → --limit 2; caller gets page [1:2]
	routes := map[string]routeResponse{
		argsKey([]string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--reverse", "--limit", "2"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-A","title":"closed A","status":"closed","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-02-01T00:00:00Z"},
				{"id":"bw-B","title":"closed B","status":"closed","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-02-01T00:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.Query(context.Background(), "status=closed", domain.QueryOptions{
		Limit:         1,
		Offset:        1,
		IncludeClosed: true,
		SortBy:        domain.SortFieldClosedAt,
		SortOrder:     domain.SortDirectionDescending,
	})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	// Offset=1 so we get the second item only
	if len(got) != 1 || got[0].ID != "bw-B" {
		t.Fatalf("expected one issue (bw-B after offset), got %#v", got)
	}
}

func TestGatewayQueryArgvAssemblyLimitWithoutOffset(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"query", "status=open", "--json", "--sort", "updated", "--limit", "5"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"open one","status":"open","issue_type":"task","priority":1,"owner":"a","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.Query(context.Background(), "status=open", domain.QueryOptions{
		Limit:  5,
		SortBy: domain.SortFieldUpdatedAt,
	})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if len(got) != 1 || got[0].ID != "bw-1" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestGatewayQueryArgvNoLimitProducesNoLimitFlag(t *testing.T) {
	t.Parallel()

	// When Limit=0 and Offset=0, no --limit flag should be appended.
	routes := map[string]routeResponse{
		argsKey([]string{"query", "priority=1", "--json"}): {
			result: ExecResult{Stdout: []byte(`[]`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	_, err := gateway.Query(context.Background(), "priority=1", domain.QueryOptions{})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if len(exec.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(exec.calls))
	}

	for _, arg := range exec.calls[0] {
		if arg == "--limit" {
			t.Fatalf("unexpected --limit flag in argv: %v", exec.calls[0])
		}
	}
}

func TestGatewayQueryRejectsEmptyExpression(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: &routingExecutor{routes: map[string]routeResponse{}}}))

	_, err := gateway.Query(context.Background(), "", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayQueryRejectsWhitespaceOnlyExpression(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: &routingExecutor{routes: map[string]routeResponse{}}}))

	_, err := gateway.Query(context.Background(), "   ", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayQueryReturnsCommandFailedOnNonZeroExit(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"query", "status=open", "--json"}): {
			result: ExecResult{ExitCode: 2, Stderr: []byte("bad expression")},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.Query(context.Background(), "status=open", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayQueryReturnsDecodeErrorOnMissingID(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"query", "status=open", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.Query(context.Background(), "status=open", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayReadyExplainHappyPathDecodesSummaryAndQueues(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{Stdout: []byte(`{
				"ready": [
					{"id":"bw-1","title":"ready one","status":"open","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
					{"id":"bw-2","title":"ready two","status":"open","issue_type":"bug","priority":1,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
				],
				"blocked": [
					{"id":"bw-3","title":"blocked one","status":"blocked","issue_type":"task","priority":2,"owner":"carol","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","blocked_by":[{"id":"bw-10","title":"blocker","priority":1,"status":"open"}]}
				],
				"summary": {"total_ready": 2, "total_blocked": 1, "cycle_count": 0}
			}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if len(got.Ready) != 2 {
		t.Fatalf("expected 2 ready issues, got %d", len(got.Ready))
	}

	if got.Ready[0].ID != "bw-1" || got.Ready[0].Assignee != "alice" {
		t.Fatalf("unexpected first ready issue: %#v", got.Ready[0])
	}

	if got.Ready[1].ID != "bw-2" || got.Ready[1].Assignee != "bob" {
		t.Fatalf("unexpected second ready issue: %#v", got.Ready[1])
	}

	if len(got.Blocked) != 1 {
		t.Fatalf("expected 1 blocked issue, got %d", len(got.Blocked))
	}

	if got.Blocked[0].Issue.ID != "bw-3" || got.Blocked[0].Issue.Assignee != "carol" {
		t.Fatalf("unexpected blocked issue: %#v", got.Blocked[0].Issue)
	}

	if len(got.Blocked[0].BlockedBy) != 1 {
		t.Fatalf("expected 1 blocked_by ref, got %d", len(got.Blocked[0].BlockedBy))
	}

	ref := got.Blocked[0].BlockedBy[0]
	if ref.ID != "bw-10" || ref.Title != "blocker" || ref.Priority != 1 || ref.Status != "open" {
		t.Fatalf("unexpected blocked_by reference: %#v", ref)
	}

	if got.TotalReady != 2 || got.TotalBlocked != 1 || got.CycleCount != 0 {
		t.Fatalf("unexpected summary: TotalReady=%d TotalBlocked=%d CycleCount=%d", got.TotalReady, got.TotalBlocked, got.CycleCount)
	}
}

func TestGatewayReadyExplainWithLimitAppendsLimitFlag(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json", "--limit", "5"}): {
			result: ExecResult{Stdout: []byte(`{
				"ready": [],
				"blocked": [],
				"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 0}
			}`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{Limit: 5})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if len(got.Ready) != 0 || len(got.Blocked) != 0 {
		t.Fatalf("expected empty queues, got ready=%d blocked=%d", len(got.Ready), len(got.Blocked))
	}

	if len(exec.calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(exec.calls))
	}

	found := false
	for i, arg := range exec.calls[0] {
		if arg == "--limit" && i+1 < len(exec.calls[0]) && exec.calls[0][i+1] == "5" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected --limit 5 in argv, got: %v", exec.calls[0])
	}
}

func TestGatewayReadyExplainNoLimitProducesNoLimitFlag(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{Stdout: []byte(`{"ready":[],"blocked":[],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}`)},
		},
	}

	gateway, exec := newTestGateway(routes)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	for _, arg := range exec.calls[0] {
		if arg == "--limit" {
			t.Fatalf("unexpected --limit flag in argv when Limit=0: %v", exec.calls[0])
		}
	}
}

func TestGatewayReadyExplainEmptyArraysDecodeToNonNilSlices(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{Stdout: []byte(`{"ready":[],"blocked":[],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if got.Ready == nil {
		t.Fatal("Ready slice must be non-nil for empty array")
	}

	if got.Blocked == nil {
		t.Fatal("Blocked slice must be non-nil for empty array")
	}
}

func TestGatewayReadyExplainReturnsCommandFailedOnNonZeroExit(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{ExitCode: 1, Stderr: []byte("no database")},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayReadyExplainReturnsDecodeErrorOnMissingID(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{Stdout: []byte(`{
				"ready": [{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}],
				"blocked": [],
				"summary": {"total_ready":1,"total_blocked":0,"cycle_count":0}
			}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayReadyExplainBlockedItemMultipleBlockedByRefs(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"ready", "--explain", "--json"}): {
			result: ExecResult{Stdout: []byte(`{
				"ready": [],
				"blocked": [
					{"id":"bw-5","title":"multi-blocked","status":"blocked","issue_type":"bug","priority":2,"owner":"dave","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z",
					 "blocked_by":[
						{"id":"bw-A","title":"blocker A","priority":0,"status":"open"},
						{"id":"bw-B","title":"blocker B","priority":1,"status":"in_progress"}
					 ]}
				],
				"summary": {"total_ready":0,"total_blocked":1,"cycle_count":0}
			}`)},
		},
	}

	gateway, _ := newTestGateway(routes)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if len(got.Blocked) != 1 {
		t.Fatalf("expected 1 blocked issue, got %d", len(got.Blocked))
	}

	if len(got.Blocked[0].BlockedBy) != 2 {
		t.Fatalf("expected 2 blocked_by refs, got %d", len(got.Blocked[0].BlockedBy))
	}

	if got.Blocked[0].BlockedBy[0].ID != "bw-A" || got.Blocked[0].BlockedBy[1].ID != "bw-B" {
		t.Fatalf("unexpected blocked_by ref IDs: %#v", got.Blocked[0].BlockedBy)
	}
}
