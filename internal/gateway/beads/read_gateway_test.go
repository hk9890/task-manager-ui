package beads

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

	if len(got.Blocks) != 1 || got.Blocks[0].ID != "bw-250" {
		t.Fatalf("unexpected blocks: %#v", got.Blocks)
	}

	if len(got.Related) != 1 || got.Related[0].ID != "bw-350" {
		t.Fatalf("unexpected related: %#v", got.Related)
	}

	if len(got.Comments) != 1 || got.Comments[0].Body != "Looks good" {
		t.Fatalf("unexpected comments: %#v", got.Comments)
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
