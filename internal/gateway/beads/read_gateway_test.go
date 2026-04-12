package beads

import (
	"context"
	"fmt"
	"os"
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

	if got.Total != 2 || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" || got.Results[0].Issue.Assignee != "bob" {
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

	if got.Total != 2 || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" {
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

	if got.Total != 2 || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-2" {
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

	if got.Total != 1 || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
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

	if got.Total != 1 || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
		t.Fatalf("unexpected blocked-filtered search result page: %#v", got)
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

func TestGatewayShowIssueMapsRelatedFromDependenciesWhenDependencyTypeIsRelated(t *testing.T) {
	t.Parallel()

	routes := map[string]routeResponse{
		argsKey([]string{"show", "bw-11", "--json"}): {
			result: ExecResult{Stdout: []byte(`[
				{"id":"bw-11","title":"related in dependencies","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"real blocker","dependency_type":"blocks"},{"id":"bw-3","title":"real related","dependency_type":"related"}],"dependents":[{"id":"bw-2","title":"child"}]}
			]`)},
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
