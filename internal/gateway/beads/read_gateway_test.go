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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json", "--status", "open,blocked", "--type", "task,bug", "--assignee", "alice", "--label", "ui", "--label", "backend", "--sort", "updated", "--limit", "2"}).Return(ExecResult{Stdout: readFixture(t, "list_issues.json")}, nil)

	gateway, rec := newTestGateway(rec)

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

	if rec.CallCount() != 1 {
		t.Fatalf("expected one command invocation, got %d", rec.CallCount())
	}
}

func TestGatewayReadyIssuesPaginatesResults(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--json", "--limit", "2"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":2,"owner":"a","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"bug","priority":1,"owner":"b","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"blocked", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"blocked","status":"blocked","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","blocked_by":["bw-0"]}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-4", "--json"}).Return(ExecResult{Stdout: readFixture(t, "show_issue.json")}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-42", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-42","title":"child issue","description":"detail","status":"open","issue_type":"task","priority":2,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"parent issue","issue_type":"epic","priority":1,"status":"open","dependency_type":"parent-child"},{"id":"bw-50","title":"blocker issue","issue_type":"bug","priority":1,"status":"open","dependency_type":"blocks"},{"id":"bw-90","title":"dependency-related issue","issue_type":"spike","priority":3,"status":"blocked","dependency_type":"related"}],"related":[{"id":"bw-91","title":"top-level related issue","issue_type":"task","priority":2,"status":"open"}]}
			]`)}, nil)
	rec.OnArgs([]string{"show", "bw-1", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"parent issue","description":"detail","status":"open","issue_type":"epic","priority":1,"created_at":"2026-04-04T09:00:00Z","updated_at":"2026-04-04T10:00:00Z","dependents":[{"id":"bw-42","title":"child issue","issue_type":"task","priority":2,"status":"open","dependency_type":"parent-child"},{"id":"bw-43","title":"sibling issue","issue_type":"task","priority":3,"status":"in_progress","dependency_type":"parent-child"},{"id":"bw-99","title":"non-child dependent","issue_type":"task","priority":3,"status":"open","dependency_type":"blocks"}]}
			]`)}, nil)

	gateway, rec := newTestGateway(rec)

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

	if rec.CallCount() != 2 {
		t.Fatalf("expected child and parent show calls, got %d", rec.CallCount())
	}
}

func TestGatewayShowIssueReturnsEmptyParentGroupBrowserContextWhenNoParent(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-77", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-77","title":"no parent issue","description":"detail","status":"open","issue_type":"task","priority":2,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-50","title":"blocker issue","dependency_type":"blocks"}]}
			]`)}, nil)

	gateway, rec := newTestGateway(rec)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-77"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.ParentGroupBrowser.Parent.ID != "" || len(got.ParentGroupBrowser.Children) != 0 {
		t.Fatalf("expected empty parent-group browser context, got %#v", got.ParentGroupBrowser)
	}

	if rec.CallCount() != 1 {
		t.Fatalf("expected no parent lookup when issue has no parent-child dependency, got %d calls", rec.CallCount())
	}
}

func TestGatewayShowIssuePrefersAssigneeOverOwner(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-7", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-7","title":"assignee precedence","description":"detail","status":"open","issue_type":"task","priority":1,"assignee":"bob","owner":"hans.kohlreiter@dynatrace.com","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-8", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-8","title":"assignee fallback","description":"detail","status":"open","issue_type":"task","priority":1,"owner":"legacy-owner","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-9", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-9","title":"metadata absent","description":"detail","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"search", "gateway", "--json", "--status", "open", "--type", "task", "--priority-min", "1", "--priority-max", "2", "--assignee", "alice", "--label", "ui", "--limit", "2"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json", "--all", "--limit", "2"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json", "--status", "open", "--type", "task", "--priority-min", "1", "--priority-max", "2", "--assignee", "alice", "--label", "ui", "--limit", "2"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"two","status":"open","issue_type":"task","priority":2,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"gateway parser","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"gateway parser docs","status":"open","issue_type":"task","priority":2,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-3","title":"other","status":"open","issue_type":"task","priority":1,"owner":"alice","labels":["ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 0 || got.Metadata.Completeness != domain.SearchResultCompletenessPartial || got.Metadata.Source != domain.SearchResultSourceReadyFilter || got.Metadata.Notice != "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
		t.Fatalf("unexpected ready-filtered search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesWorkStateBlockedUsesBlockedAndLocalFilters(t *testing.T) {
	t.Parallel()

	priorityMin := 0
	priorityMax := 1
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"blocked", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"gateway deadlock","status":"blocked","issue_type":"bug","priority":1,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"gateway deadlock","status":"blocked","issue_type":"bug","priority":2,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-3","title":"gateway deadlock","status":"blocked","issue_type":"task","priority":1,"owner":"alice","labels":["backend","ui"],"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	if got.Metadata.ReturnedCount != 1 || got.Metadata.RequestedLimit != 0 || got.Metadata.Completeness != domain.SearchResultCompletenessPartial || got.Metadata.Source != domain.SearchResultSourceBlockedFilter || got.Metadata.Notice != "" || len(got.Results) != 1 || got.Results[0].Issue.ID != "bw-1" {
		t.Fatalf("unexpected blocked-filtered search result page: %#v", got)
	}
}

func TestGatewaySearchIssuesWithoutLimitMarksBackendResultsPartialNotExact(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"search", "gateway", "--json", "--status", "all"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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
	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: newTestRecordingExecutor()}))

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{PriorityMin: &priorityMin, PriorityMax: &priorityMax})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayCatalogReads(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"statuses", "--json"}).Return(ExecResult{Stdout: readFixture(t, "statuses.json")}, nil)
	rec.OnArgs([]string{"types", "--json"}).Return(ExecResult{Stdout: readFixture(t, "types.json")}, nil)
	rec.OnArgs([]string{"label", "list-all", "--json"}).Return(ExecResult{Stdout: readFixture(t, "labels.json")}, nil)

	gateway, _ := newTestGateway(rec)

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

	// custom_types in bd 1.0.4 are bare strings with no description (puy3).
	if !reflect.DeepEqual(types, []domain.TypeOption{{Name: "task", Description: "Task"}, {Name: "bug", Description: "Bug"}, {Name: "spike", Description: ""}}) {
		t.Fatalf("unexpected types: %#v", types)
	}

	if !reflect.DeepEqual(labels, []domain.LabelOption{{Name: "gateway"}, {Name: "backend"}, {Name: "docs"}}) {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestGatewayReadMethodsReturnNormalizedFailures(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json"}).Return(ExecResult{ExitCode: 2, Stderr: []byte("bad args")}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayReadMethodsSurfaceExecutorStderr(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json"}).Return(ExecResult{Stderr: []byte("permission denied")}, fmt.Errorf("spawn failed"))

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
	assertContains(t, err.Error(), "permission denied")
}

func TestGatewayReadMappingReturnsDecodeError(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ListIssues(context.Background(), domain.IssueListQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayShowIssueRequiresID(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: newTestRecordingExecutor()}))

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayShowIssueReturnsNotFoundOnEmptyResponse(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-404", "--json"}).Return(ExecResult{Stdout: []byte(`[]`)}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-404"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeNotFound)
}

func TestGatewayShowIssueDecodeFailureWhenDependencyIsMissingTitle(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-9", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-9","title":"bad dependency","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1"}]}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-9"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayShowIssueReferenceMetadataIsOptional(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-10", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-10","title":"optional refs","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"dep"}],"dependents":[{"id":"bw-2","title":"child"}],"related":[{"id":"bw-3","title":"rel"}]}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

			rec := newTestRecordingExecutor()
			rec.OnArgs([]string{"show", "bw-11", "--json"}).Return(ExecResult{Stdout: []byte(fmt.Sprintf(`[
						{"id":"bw-11","title":"related in dependencies","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"real blocker","dependency_type":"blocks"},{"id":"bw-3","title":"real related","dependency_type":"%s"}],"dependents":[{"id":"bw-2","title":"child"}]}
					]`, tc.dependencyType))}, nil)

			gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-13", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-13","title":"related in dependents","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependents":[{"id":"bw-21","title":"true dependent","dependency_type":"blocks"},{"id":"bw-22","title":"related dependent","dependency_type":"relates-to"}]}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-12", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-12","title":"mixed related","description":"x","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-4","title":"from deps related","dependency_type":"related"},{"id":"bw-5","title":"non related dep"}],"related":[{"id":"bw-4","title":"from top-level related"},{"id":"bw-6","title":"top-level only"}]}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"count", "--by-status", "--json"}).Return(ExecResult{Stdout: []byte(`{"groups":[{"count":5,"group":"open"},{"count":353,"group":"closed"}],"schema_version":1,"total":358}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"count", "--by-status", "--json", "--status", "open"}).Return(ExecResult{Stdout: []byte(`{"groups":[{"count":5,"group":"open"}],"schema_version":1,"total":5}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"count", "--by-status", "--json"}).Return(ExecResult{Stdout: []byte(`{"groups":[],"schema_version":1,"total":0}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"count", "--by-status", "--json", "--status", "closed", "--type", "bug", "--assignee", "alice", "--label", "backend"}).Return(ExecResult{Stdout: []byte(`{"groups":[{"count":10,"group":"closed"}],"schema_version":1,"total":10}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"list", "--json", "--status", "closed", "--sort", "closed", "--limit", "5"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-B","title":"B closed recent","status":"closed","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","closed_at":"2026-04-01T00:00:00Z"},
				{"id":"bw-A","title":"A closed earlier updated later","status":"closed","issue_type":"task","priority":1,"owner":"bob","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-05-01T00:00:00Z","closed_at":"2026-01-01T00:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

// TestGatewaySortDirectionDescendingEmitsNoReverseFlag is a regression test for the
// sort-direction inversion bug (beads-workbench-zhef). bd --sort <field> defaults to
// DESCENDING; emitting --reverse inverts to ASCENDING — the opposite of intent.
//
// Correct behaviour:
//   - SortDirectionDescending: no --reverse flag (bd default = DESC = correct)
//   - SortDirectionAscending: --reverse flag (inverts bd default DESC → ASC)
//
// To verify this test catches the regression: comment out the fix in read_gateway.go
// (change SortDirectionAscending back to SortDirectionDescending), run this test, and
// confirm it fails with "unexpected args" for the argv without --reverse.
func TestGatewaySortDirectionDescendingEmitsNoReverseFlag(t *testing.T) {
	t.Parallel()

	minimalIssueJSON := `[{"id":"bw-1","title":"one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}]`

	// SortDirectionDescending must NOT emit --reverse (bd default is already DESC).
	t.Run("DescendingOmitsReverse", func(t *testing.T) {
		t.Parallel()

		rec := newTestRecordingExecutor()
		rec.OnArgs([]string{"list", "--json", "--sort", "updated"}).Return(ExecResult{Stdout: []byte(minimalIssueJSON)}, nil)
		gw, _ := newTestGateway(rec)
		_, err := gw.ListIssues(context.Background(), domain.IssueListQuery{
			SortBy:    domain.SortFieldUpdatedAt,
			SortOrder: domain.SortDirectionDescending,
		})
		if err != nil {
			t.Fatalf("SortDirectionDescending should not emit --reverse; got error: %v", err)
		}
	})

	// SortDirectionAscending MUST emit --reverse (to invert bd's default DESC → ASC).
	t.Run("AscendingEmitsReverse", func(t *testing.T) {
		t.Parallel()

		rec := newTestRecordingExecutor()
		rec.OnArgs([]string{"list", "--json", "--sort", "updated", "--reverse"}).Return(ExecResult{Stdout: []byte(minimalIssueJSON)}, nil)
		gw, _ := newTestGateway(rec)
		_, err := gw.ListIssues(context.Background(), domain.IssueListQuery{
			SortBy:    domain.SortFieldUpdatedAt,
			SortOrder: domain.SortDirectionAscending,
		})
		if err != nil {
			t.Fatalf("SortDirectionAscending should emit --reverse; got error: %v", err)
		}
	})

	// Same contract for Query (Done column path).
	t.Run("QueryDescendingOmitsReverse", func(t *testing.T) {
		t.Parallel()

		rec := newTestRecordingExecutor()
		rec.OnArgs([]string{"query", "status=closed", "--json", "--sort", "closed"}).Return(ExecResult{Stdout: []byte(minimalIssueJSON)}, nil)
		gw, _ := newTestGateway(rec)
		_, err := gw.Query(context.Background(), "status=closed", domain.QueryOptions{
			SortBy:    domain.SortFieldClosedAt,
			SortOrder: domain.SortDirectionDescending,
		})
		if err != nil {
			t.Fatalf("Query SortDirectionDescending should not emit --reverse; got error: %v", err)
		}
	})

	// Query Ascending must emit --reverse.
	t.Run("QueryAscendingEmitsReverse", func(t *testing.T) {
		t.Parallel()

		rec := newTestRecordingExecutor()
		rec.OnArgs([]string{"query", "status=closed", "--json", "--sort", "closed", "--reverse"}).Return(ExecResult{Stdout: []byte(minimalIssueJSON)}, nil)
		gw, _ := newTestGateway(rec)
		_, err := gw.Query(context.Background(), "status=closed", domain.QueryOptions{
			SortBy:    domain.SortFieldClosedAt,
			SortOrder: domain.SortDirectionAscending,
		})
		if err != nil {
			t.Fatalf("Query SortDirectionAscending should emit --reverse; got error: %v", err)
		}
	})
}

func TestGatewayHealthCheckIssuesPingJSON(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ping", "--json"}).Return(ExecResult{Stdout: []byte(`{"status":"ok","total_ms":42}`)}, nil)

	gateway, rec := newTestGateway(rec)

	err := gateway.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned unexpected error: %v", err)
	}

	if rec.CallCount() != 1 {
		t.Fatalf("expected one command invocation, got %d", rec.CallCount())
	}

	calls := rec.Calls()
	if len(calls[0].Args) != 2 || calls[0].Args[0] != "ping" || calls[0].Args[1] != "--json" {
		t.Fatalf("expected argv [ping --json], got %v", calls[0].Args)
	}
}

func TestGatewayHealthCheckNoDatabaseReturnsNoDatabaseFound(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ping", "--json"}).Return(ExecResult{ExitCode: 1, Stderr: []byte("Error: no beads database found")}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.HealthCheck(context.Background())
	assertGatewayErrorCode(t, err, domain.ErrorCodeNoDatabaseFound)
}

func TestGatewayHealthCheckBdNotFoundReturnsCommandUnavailable(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ping", "--json"}).Return(ExecResult{}, exec.ErrNotFound)

	gateway, _ := newTestGateway(rec)

	err := gateway.HealthCheck(context.Background())
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandUnavailable)
}

func newTestGateway(rec *testRecordingExecutor) (*Gateway, *testRecordingExecutor) {
	runner := NewCommandRunner(RunnerConfig{Executor: rec})
	return NewCLIGatewayRaw(runner), rec
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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "status=in_progress", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"in progress one","status":"in_progress","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"in progress two","status":"in_progress","issue_type":"bug","priority":1,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "2"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-A","title":"closed A","status":"closed","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-02-01T00:00:00Z"},
				{"id":"bw-B","title":"closed B","status":"closed","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-02-01T00:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "status=open", "--json", "--sort", "updated", "--limit", "5"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"open one","status":"open","issue_type":"task","priority":1,"owner":"a","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

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
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "priority=1", "--json"}).Return(ExecResult{Stdout: []byte(`[]`)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.Query(context.Background(), "priority=1", domain.QueryOptions{})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if rec.CallCount() != 1 {
		t.Fatalf("expected one call, got %d", rec.CallCount())
	}

	calls := rec.Calls()
	for _, arg := range calls[0].Args {
		if arg == "--limit" {
			t.Fatalf("unexpected --limit flag in argv: %v", calls[0].Args)
		}
	}
}

func TestGatewayQueryRejectsEmptyExpression(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: newTestRecordingExecutor()}))

	_, err := gateway.Query(context.Background(), "", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayQueryRejectsWhitespaceOnlyExpression(t *testing.T) {
	t.Parallel()

	gateway := NewCLIGateway(NewCommandRunner(RunnerConfig{Executor: newTestRecordingExecutor()}))

	_, err := gateway.Query(context.Background(), "   ", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeValidationFailed)
}

func TestGatewayQueryReturnsCommandFailedOnNonZeroExit(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "status=open", "--json"}).Return(ExecResult{ExitCode: 2, Stderr: []byte("bad expression")}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.Query(context.Background(), "status=open", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayQueryReturnsDecodeErrorOnMissingID(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"query", "status=open", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.Query(context.Background(), "status=open", domain.QueryOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayReadyExplainHappyPathDecodesSummaryAndQueues(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{
				"ready": [
					{"id":"bw-1","title":"ready one","status":"open","issue_type":"task","priority":2,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
					{"id":"bw-2","title":"ready two","status":"open","issue_type":"bug","priority":1,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
				],
				"blocked": [
					{"id":"bw-3","title":"blocked one","status":"blocked","issue_type":"task","priority":2,"owner":"carol","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","blocked_by":[{"id":"bw-10","title":"blocker","priority":1,"status":"open"}]}
				],
				"summary": {"total_ready": 2, "total_blocked": 1, "cycle_count": 0}
			}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json", "--limit", "5"}).Return(ExecResult{Stdout: []byte(`{
				"ready": [],
				"blocked": [],
				"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 0}
			}`)}, nil)

	gateway, rec := newTestGateway(rec)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{Limit: 5})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if len(got.Ready) != 0 || len(got.Blocked) != 0 {
		t.Fatalf("expected empty queues, got ready=%d blocked=%d", len(got.Ready), len(got.Blocked))
	}

	if rec.CallCount() != 1 {
		t.Fatalf("expected one command call, got %d", rec.CallCount())
	}

	calls := rec.Calls()
	found := false
	for i, arg := range calls[0].Args {
		if arg == "--limit" && i+1 < len(calls[0].Args) && calls[0].Args[i+1] == "5" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected --limit 5 in argv, got: %v", calls[0].Args)
	}
}

func TestGatewayReadyExplainNoLimitProducesNoLimitFlag(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{"ready":[],"blocked":[],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}`)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	calls := rec.Calls()
	for _, arg := range calls[0].Args {
		if arg == "--limit" {
			t.Fatalf("unexpected --limit flag in argv when Limit=0: %v", calls[0].Args)
		}
	}
}

func TestGatewayReadyExplainEmptyArraysDecodeToNonNilSlices(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{"ready":[],"blocked":[],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{ExitCode: 1, Stderr: []byte("no database")}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

func TestGatewayReadyExplainReturnsDecodeErrorOnMissingID(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{
				"ready": [{"title":"missing id","status":"open","issue_type":"task","priority":1,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}],
				"blocked": [],
				"summary": {"total_ready":1,"total_blocked":0,"cycle_count":0}
			}`)}, nil)

	gateway, _ := newTestGateway(rec)

	_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
}

func TestGatewayReadyExplainBlockedItemMultipleBlockedByRefs(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{
				"ready": [],
				"blocked": [
					{"id":"bw-5","title":"multi-blocked","status":"blocked","issue_type":"bug","priority":2,"owner":"dave","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z",
					 "blocked_by":[
						{"id":"bw-A","title":"blocker A","priority":0,"status":"open"},
						{"id":"bw-B","title":"blocker B","priority":1,"status":"in_progress"}
					 ]}
				],
				"summary": {"total_ready":0,"total_blocked":1,"cycle_count":0}
			}`)}, nil)

	gateway, _ := newTestGateway(rec)

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

// Null optional field tolerance tests (beads-workbench-db0z.15)

func TestGatewayCatalogAcceptsNullStatusDescription(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"statuses", "--json"}).Return(ExecResult{Stdout: readFixture(t, "statuses_null_description.json")}, nil)

	gateway, _ := newTestGateway(rec)

	statuses, err := gateway.StatusCatalog(context.Background())
	if err != nil {
		t.Fatalf("StatusCatalog returned error on null description: %v", err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d: %#v", len(statuses), statuses)
	}

	// The custom status "qa" has null description — expect it decoded as empty string.
	qa := statuses[2]
	if qa.Name != "qa" {
		t.Fatalf("expected third status to be 'qa', got %q", qa.Name)
	}

	if qa.Description != "" {
		t.Fatalf("expected empty description for null-description status, got %q", qa.Description)
	}
}

func TestGatewayCatalogAcceptsNullTypeDescription(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"types", "--json"}).Return(ExecResult{Stdout: readFixture(t, "types_null_description.json")}, nil)

	gateway, _ := newTestGateway(rec)

	types, err := gateway.TypeCatalog(context.Background())
	if err != nil {
		t.Fatalf("TypeCatalog returned error on null description: %v", err)
	}

	if len(types) != 3 {
		t.Fatalf("expected 3 types, got %d: %#v", len(types), types)
	}

	// Core type "bug" has null description — expect it decoded as empty string.
	bug := types[1]
	if bug.Name != "bug" {
		t.Fatalf("expected second type to be 'bug', got %q", bug.Name)
	}

	if bug.Description != "" {
		t.Fatalf("expected empty description for null-description core type, got %q", bug.Description)
	}

	// Custom type "spike" is a bare string in bd 1.0.4 — description is always empty (puy3).
	spike := types[2]
	if spike.Name != "spike" {
		t.Fatalf("expected third type to be 'spike', got %q", spike.Name)
	}

	if spike.Description != "" {
		t.Fatalf("expected empty description for custom type bare-string, got %q", spike.Description)
	}
}

func TestGatewayCatalogAcceptsNullLabelsList(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"label", "list-all", "--json"}).Return(ExecResult{Stdout: readFixture(t, "labels_null_list.json")}, nil)

	gateway, _ := newTestGateway(rec)

	labels, err := gateway.LabelCatalog(context.Background())
	if err != nil {
		t.Fatalf("LabelCatalog returned error on null list: %v", err)
	}

	if len(labels) != 0 {
		t.Fatalf("expected empty labels for null list, got %d: %#v", len(labels), labels)
	}
}

func TestGatewayShowIssueAcceptsNullCommentText(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-301", "--json"}).Return(ExecResult{Stdout: readFixture(t, "show_issue_null_comment_text.json")}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-301"})
	if err != nil {
		t.Fatalf("ShowIssue returned error on null comment text: %v", err)
	}

	if len(got.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(got.Comments))
	}

	// Null comment text must decode as empty string, not cause an error.
	if got.Comments[0].Body != "" {
		t.Fatalf("expected empty body for null comment text, got %q", got.Comments[0].Body)
	}
}

func TestGatewayShowIssueAcceptsNullLabelsAndUnknownFields(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-401", "--json"}).Return(ExecResult{Stdout: readFixture(t, "show_issue_production_noise.json")}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-401"})
	if err != nil {
		t.Fatalf("ShowIssue returned error on production-noise fixture: %v", err)
	}

	if got.Summary.ID != "bw-401" {
		t.Fatalf("unexpected issue ID: %q", got.Summary.ID)
	}

	// Null labels list must decode to nil/empty slice — not cause a decode error.
	if len(got.Summary.Labels) != 0 {
		t.Fatalf("expected empty labels for null labels list, got %#v", got.Summary.Labels)
	}

	// Multi-paragraph description must decode intact.
	if !strings.HasPrefix(got.Description, "First paragraph") {
		t.Fatalf("expected multi-paragraph description to start with 'First paragraph', got %q", got.Description)
	}

	if !strings.Contains(got.Description, "Second paragraph") {
		t.Fatalf("expected multi-paragraph description to contain 'Second paragraph', got %q", got.Description)
	}
}

// TestGatewayShowIssueHandlesMissingDescription verifies that ShowIssue succeeds
// and returns an empty Description when bd show omits the "description" key entirely
// (the common case for issues created without --description).
func TestGatewayShowIssueHandlesMissingDescription(t *testing.T) {
	t.Parallel()

	// JSON deliberately omits the "description" key — exactly what bd show emits
	// for an issue that was created without a description.
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-500", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-500","title":"no desc test","status":"open","issue_type":"task","priority":2,"created_at":"2026-05-16T10:00:00Z","updated_at":"2026-05-16T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-500"})
	if err != nil {
		t.Fatalf("ShowIssue returned error for issue with no description: %v", err)
	}

	if got.Description != "" {
		t.Fatalf("expected empty Description when key absent, got %q", got.Description)
	}

	if got.Summary.ID != "bw-500" {
		t.Fatalf("unexpected summary ID: %q", got.Summary.ID)
	}
}

// TestGatewayShowIssueChildIssueIssuesAtMostOneBdShowSynchronouslyOnCachedParent
// asserts that repeated detail loads for a child issue with the same parent do
// not re-issue a bd show for the parent after the first fetch — the parent
// sibling lookup is served from cache on the second call.
func TestGatewayShowIssueChildIssueIssuesAtMostOneBdShowSynchronouslyOnCachedParent(t *testing.T) {
	t.Parallel()

	childJSON := []byte(`[
		{"id":"bw-42","title":"child issue","description":"detail","status":"open","issue_type":"task","priority":2,"created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z","dependencies":[{"id":"bw-1","title":"parent issue","issue_type":"epic","priority":1,"status":"open","dependency_type":"parent-child"}]}
	]`)
	parentJSON := []byte(`[
		{"id":"bw-1","title":"parent issue","description":"detail","status":"open","issue_type":"epic","priority":1,"created_at":"2026-04-04T09:00:00Z","updated_at":"2026-04-04T10:00:00Z","dependents":[{"id":"bw-42","title":"child issue","issue_type":"task","priority":2,"status":"open","dependency_type":"parent-child"},{"id":"bw-43","title":"sibling issue","issue_type":"task","priority":3,"status":"in_progress","dependency_type":"parent-child"}]}
	]`)

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-42", "--json"}).Return(ExecResult{Stdout: childJSON}, nil)
	rec.OnArgs([]string{"show", "bw-1", "--json"}).Return(ExecResult{Stdout: parentJSON}, nil)

	gateway, rec := newTestGateway(rec)

	// First detail load: fetches child and parent (2 bd show calls).
	if _, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-42"}); err != nil {
		t.Fatalf("first ShowIssue returned error: %v", err)
	}
	if rec.CallCount() != 2 {
		t.Fatalf("expected 2 bd show calls on first load (child + parent), got %d", rec.CallCount())
	}

	callsAfterFirst := rec.CallCount()

	// Second detail load for the same child: parent siblings must come from
	// cache — at most one bd show (for the child itself) is issued synchronously.
	if _, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-42"}); err != nil {
		t.Fatalf("second ShowIssue returned error: %v", err)
	}

	newCalls := rec.CallCount() - callsAfterFirst
	if newCalls > 1 {
		t.Fatalf("expected at most 1 bd show on second detail load for child issue (parent cached), got %d new calls", newCalls)
	}
}

// TestGatewaySearchIssuePageFromRecordsCappedReadyBackendIsNotExact verifies G5:
// searchIssuePageFromRecords no longer falsely claims Exact completeness.
// When bd ready returns results at the limit boundary, completeness must be MaybeMore,
// not Exact, because the backend may have capped additional matches.
func TestGatewaySearchIssuePageFromRecordsCappedReadyBackendIsNotExact(t *testing.T) {
	t.Parallel()

	// Simulate a capped backend: bd ready returns exactly 2 items and limit=2
	// so completeness should be MaybeMore (can't know if backend had more).
	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"cap test one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"},
				{"id":"bw-2","title":"cap test two","status":"open","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T11:00:00Z","updated_at":"2026-04-05T12:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:      "cap test",
		WorkState: domain.WorkStateReady,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got.Results))
	}

	if got.Metadata.Completeness == domain.SearchResultCompletenessExact {
		t.Fatalf("completeness must not be Exact when backend results may be capped; got %q", got.Metadata.Completeness)
	}

	if got.Metadata.Completeness != domain.SearchResultCompletenessMaybeMore {
		t.Fatalf("expected MaybeMore completeness for capped backend result at limit boundary, got %q", got.Metadata.Completeness)
	}

	if got.Metadata.Source != domain.SearchResultSourceReadyFilter {
		t.Fatalf("expected source ReadyFilter, got %q", got.Metadata.Source)
	}

	// Text IS set — no-text-filter notice must not be present.
	if got.Metadata.Notice != "" {
		t.Fatalf("expected no notice when text filter is applied, got %q", got.Metadata.Notice)
	}
}

// TestGatewaySearchIssuesEmptyTextWithWorkStateReadyAttachesNotice verifies G7:
// when text is empty and WorkState is Ready, the result includes a typed notice
// indicating no text filter was applied.
func TestGatewaySearchIssuesEmptyTextWithWorkStateReadyAttachesNotice(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"some ready issue","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateReady,
		// Text intentionally empty
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.Notice == "" {
		t.Fatal("expected a non-empty notice for empty-text + WorkStateReady path")
	}

	if got.Metadata.Notice != searchNoticeNoTextFilter {
		t.Fatalf("expected searchNoticeNoTextFilter notice, got %q", got.Metadata.Notice)
	}

	if got.Metadata.Source != domain.SearchResultSourceReadyFilter {
		t.Fatalf("expected source ReadyFilter, got %q", got.Metadata.Source)
	}
}

// TestGatewaySearchIssuesEmptyTextWithWorkStateBlockedAttachesNotice verifies G7
// for the blocked queue path.
func TestGatewaySearchIssuesEmptyTextWithWorkStateBlockedAttachesNotice(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"blocked", "--json"}).Return(ExecResult{Stdout: []byte(`[
				{"id":"bw-1","title":"some blocked issue","status":"blocked","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
			]`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateBlocked,
		// Text intentionally empty
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	if got.Metadata.Notice == "" {
		t.Fatal("expected a non-empty notice for empty-text + WorkStateBlocked path")
	}

	if got.Metadata.Notice != searchNoticeNoTextFilter {
		t.Fatalf("expected searchNoticeNoTextFilter notice, got %q", got.Metadata.Notice)
	}

	if got.Metadata.Source != domain.SearchResultSourceBlockedFilter {
		t.Fatalf("expected source BlockedFilter, got %q", got.Metadata.Source)
	}
}

// =============================================================================
// ppja.3 — argv cardinality tests
//
// Each test below pins EXACTLY the argv shape emitted for a given gateway call
// path. Failing assertions cite the exact shape that was recorded vs. what was
// expected so accidental argv mutations trip immediately.
// =============================================================================

// assertExactArgv fails the test unless exactly one call was recorded and its
// argv exactly matches want.
func assertExactArgv(t *testing.T, rec *testRecordingExecutor, want []string) {
	t.Helper()

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 subprocess call, got %d: %v",
			len(calls), allArgvSlices(calls))
	}

	if !reflect.DeepEqual(calls[0].Args, want) {
		t.Fatalf("argv mismatch:\n  got:  %v\n  want: %v", calls[0].Args, want)
	}
}

// allArgvSlices formats recorded calls for failure messages.
func allArgvSlices(calls []testRecordedCall) [][]string {
	out := make([][]string, len(calls))
	for i, c := range calls {
		out[i] = c.Args
	}
	return out
}

// minimalIssueJSONArray is the smallest valid bd issue JSON array: a single
// issue with all required fields populated.
const minimalIssueJSONArray = `[{"id":"bw-1","title":"t","status":"open","issue_type":"task","priority":1,"owner":"a","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`

// minimalReadyExplainJSON is the smallest valid bd ready --explain payload.
const minimalReadyExplainJSON = `{"ready":[],"blocked":[],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}`

// --- 1. ShowIssue parent-sibling fetch path ---

// TestShowIssueParentSiblingArgvShape pins the second bd show call emitted by
// ShowIssue when the queried issue has a parent-child dependency. The parent
// sibling lookup issues `bd show <parentID> --json` as a second call.
//
// This is ppja.3 backlog item 1.
func TestShowIssueParentSiblingArgvShape(t *testing.T) {
	t.Parallel()

	childJSON := `[{"id":"bw-child","title":"child","status":"open","issue_type":"task","priority":1,"owner":"a","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"id":"bw-parent","title":"parent","issue_type":"epic","priority":1,"status":"open","dependency_type":"parent-child"}]}]`
	parentJSON := `[{"id":"bw-parent","title":"parent","status":"open","issue_type":"epic","priority":1,"owner":"a","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependents":[{"id":"bw-child","title":"child","issue_type":"task","priority":1,"status":"open","dependency_type":"parent-child"}]}]`

	wantChildArgv := []string{"show", "bw-child", "--json"}
	wantParentArgv := []string{"show", "bw-parent", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantChildArgv).Return(ExecResult{Stdout: []byte(childJSON)}, nil)
	rec.OnArgs(wantParentArgv).Return(ExecResult{Stdout: []byte(parentJSON)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-child"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 subprocess calls (child + parent sibling lookup), got %d: %v",
			len(calls), allArgvSlices(calls))
	}

	// First call: the primary issue lookup.
	if !reflect.DeepEqual(calls[0].Args, wantChildArgv) {
		t.Errorf("first call argv mismatch:\n  got:  %v\n  want: %v", calls[0].Args, wantChildArgv)
	}

	// Second call: the parent sibling lookup — this is the contract we are pinning.
	if !reflect.DeepEqual(calls[1].Args, wantParentArgv) {
		t.Errorf("parent-sibling fetch argv mismatch:\n  got:  %v\n  want: %v", calls[1].Args, wantParentArgv)
	}
}

// --- 2. ReadyExplain with non-zero limit — boundary parametrization ---

// TestReadyExplainArgvBoundaryLimits pins the exact argv for ReadyExplain at
// limit=1 (minimum), limit=20 (default before first WindowSizeMsg), and
// limit=21 (one past the cap boundary).
//
// This is ppja.3 backlog item 2.
func TestReadyExplainArgvBoundaryLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		limit int
		want  []string
	}{
		{"limit=1 (min boundary)", 1, []string{"ready", "--explain", "--json", "--limit", "1"}},
		{"limit=20 (default cap)", 20, []string{"ready", "--explain", "--json", "--limit", "20"}},
		{"limit=21 (one past default)", 21, []string{"ready", "--explain", "--json", "--limit", "21"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := newTestRecordingExecutor()
			rec.OnArgs(tc.want).Return(ExecResult{Stdout: []byte(minimalReadyExplainJSON)}, nil)

			gateway, rec := newTestGateway(rec)

			_, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{Limit: tc.limit})
			if err != nil {
				t.Fatalf("ReadyExplain returned error: %v", err)
			}

			assertExactArgv(t, rec, tc.want)
		})
	}
}

// --- 3. Query general form — dynamic limit boundary variants ---

// TestQueryArgvBoundaryLimits pins the exact argv for Query at representative
// limit values beyond the board's two pinned cases (status=in_progress without
// limit, status=closed with limit=50). This covers limit=1, limit=50 (the
// closedLimit floor), and limit=51 (one past the floor).
//
// This is ppja.3 backlog item 3.
func TestQueryArgvBoundaryLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		expr string
		opts domain.QueryOptions
		want []string
	}{
		{
			name: "status=in_progress limit=1 (min boundary)",
			expr: "status=in_progress",
			opts: domain.QueryOptions{Limit: 1},
			want: []string{"query", "status=in_progress", "--json", "--limit", "1"},
		},
		{
			name: "status=closed limit=50 (closedLimit floor)",
			expr: "status=closed",
			opts: domain.QueryOptions{IncludeClosed: true, SortBy: domain.SortFieldClosedAt, SortOrder: domain.SortDirectionDescending, Limit: 50},
			want: []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"},
		},
		{
			name: "status=closed limit=51 (one past floor)",
			expr: "status=closed",
			opts: domain.QueryOptions{IncludeClosed: true, SortBy: domain.SortFieldClosedAt, SortOrder: domain.SortDirectionDescending, Limit: 51},
			want: []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "51"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := newTestRecordingExecutor()
			rec.OnArgs(tc.want).Return(ExecResult{Stdout: []byte(`[]`)}, nil)

			gateway, rec := newTestGateway(rec)

			_, err := gateway.Query(context.Background(), tc.expr, tc.opts)
			if err != nil {
				t.Fatalf("Query returned error: %v", err)
			}

			assertExactArgv(t, rec, tc.want)
		})
	}
}

// --- 4. SearchIssues → empty text / no WorkState (bd list --json --all) ---

// TestSearchIssuesEmptyTextNoWorkStateArgvShape pins the exact argv for the
// SearchIssues path that routes to searchIssuesFromList with empty text and no
// status filter: `bd list --json --all --limit N`.
//
// This is ppja.3 backlog item 4.
func TestSearchIssuesEmptyTextNoWorkStateArgvShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		limit int
		want  []string
	}{
		{
			name:  "limit=0 (no limit flag)",
			limit: 0,
			want:  []string{"list", "--json", "--all"},
		},
		{
			name:  "limit=20 (default capacity)",
			limit: 20,
			want:  []string{"list", "--json", "--all", "--limit", "20"},
		},
		{
			name:  "limit=1 (min boundary)",
			limit: 1,
			want:  []string{"list", "--json", "--all", "--limit", "1"},
		},
		{
			name:  "limit=21 (one past default)",
			limit: 21,
			want:  []string{"list", "--json", "--all", "--limit", "21"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := newTestRecordingExecutor()
			rec.OnArgs(tc.want).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

			gateway, rec := newTestGateway(rec)

			_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
				Text:      "", // empty text → searchIssuesFromList
				WorkState: domain.WorkStateAny,
				Limit:     tc.limit,
			})
			if err != nil {
				t.Fatalf("SearchIssues returned error: %v", err)
			}

			assertExactArgv(t, rec, tc.want)
		})
	}
}

// --- 5. SearchIssues → status-filtered list path ---

// TestSearchIssuesStatusFilteredListArgvShape pins the argv for the
// searchIssuesFromList path when a status filter is applied:
// `bd list --json --status <csv> --limit N`.
//
// This is ppja.3 backlog item 5.
func TestSearchIssuesStatusFilteredListArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"list", "--json", "--status", "open,in_progress", "--limit", "20"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:      "",
		WorkState: domain.WorkStateAny,
		Statuses:  []string{"open", "in_progress"},
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 6. SearchIssues → text search path ---

// TestSearchIssuesTextSearchArgvShape pins the argv for the non-empty text
// search path: `bd search <text> --json --status all --limit N`.
//
// This is ppja.3 backlog item 6.
func TestSearchIssuesTextSearchArgvShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{
			name:  "text+limit=20 (default)",
			text:  "gateway",
			limit: 20,
			want:  []string{"search", "gateway", "--json", "--status", "all", "--limit", "20"},
		},
		{
			name:  "text+limit=1 (min boundary)",
			text:  "gateway",
			limit: 1,
			want:  []string{"search", "gateway", "--json", "--status", "all", "--limit", "1"},
		},
		{
			name:  "text+limit=21 (one past default)",
			text:  "gateway",
			limit: 21,
			want:  []string{"search", "gateway", "--json", "--status", "all", "--limit", "21"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := newTestRecordingExecutor()
			rec.OnArgs(tc.want).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

			gateway, rec := newTestGateway(rec)

			_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
				Text:      tc.text,
				WorkState: domain.WorkStateAny,
				Limit:     tc.limit,
			})
			if err != nil {
				t.Fatalf("SearchIssues returned error: %v", err)
			}

			assertExactArgv(t, rec, tc.want)
		})
	}
}

// --- 7. SearchIssues → WorkState=Ready ---

// TestSearchIssuesWorkStateReadyArgvShape pins the argv for the WorkState=Ready
// path which routes to bd ready --json.
//
// This is ppja.3 backlog item 7.
func TestSearchIssuesWorkStateReadyArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"ready", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateReady,
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 8. SearchIssues → WorkState=Blocked ---

// TestSearchIssuesWorkStateBlockedArgvShape pins the argv for the
// WorkState=Blocked path which routes to bd blocked --json.
//
// This is ppja.3 backlog item 8.
func TestSearchIssuesWorkStateBlockedArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"blocked", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateBlocked,
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 9. StatusCatalog argv cardinality ---

// TestStatusCatalogArgvShape pins the exact argv: `bd statuses --json`.
//
// This is ppja.3 backlog item 9.
func TestStatusCatalogArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"statuses", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: readFixture(t, "statuses.json")}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.StatusCatalog(context.Background())
	if err != nil {
		t.Fatalf("StatusCatalog returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 10. TypeCatalog argv cardinality ---

// TestTypeCatalogArgvShape pins the exact argv: `bd types --json`.
//
// This is ppja.3 backlog item 10.
func TestTypeCatalogArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"types", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: readFixture(t, "types.json")}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.TypeCatalog(context.Background())
	if err != nil {
		t.Fatalf("TypeCatalog returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 11. LabelCatalog argv cardinality ---

// TestLabelCatalogArgvShape pins the exact argv: `bd label list-all --json`.
//
// This is ppja.3 backlog item 11.
func TestLabelCatalogArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"label", "list-all", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: readFixture(t, "labels.json")}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.LabelCatalog(context.Background())
	if err != nil {
		t.Fatalf("LabelCatalog returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// --- 12. CountIssues without status filter ---

// TestCountIssuesNoStatusFilterArgvShape pins the exact argv for CountIssues
// with an empty query: `bd count --by-status --json` (no --status flag).
//
// This is ppja.3 backlog item 12.
func TestCountIssuesNoStatusFilterArgvShape(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"count", "--by-status", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(`{"groups":[],"total":0,"schema_version":1}`)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// =============================================================================
// v2uv — ReadyExplain: CycleCount + schema_version tolerance unit tests
// =============================================================================

// TestGatewayReadyExplainDecodesNonZeroCycleCount seeds a payload with
// cycle_count: 3 and asserts the decoded CycleCount field equals 3.
// This exercises the pass-through field that had no prior test coverage.
func TestGatewayReadyExplainDecodesNonZeroCycleCount(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{
		"ready": [],
		"blocked": [],
		"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 3}
	}`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error: %v", err)
	}

	if got.CycleCount != 3 {
		t.Fatalf("expected CycleCount=3 from payload, got %d", got.CycleCount)
	}
}

// TestGatewayReadyExplainToleratesSchemaVersionField seeds a payload with
// schema_version: 1 and asserts no decode error occurs and the result is valid.
// This exercises the decoder's tolerance for unknown/ignored fields.
func TestGatewayReadyExplainToleratesSchemaVersionField(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(ExecResult{Stdout: []byte(`{
		"ready": [],
		"blocked": [],
		"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 0},
		"schema_version": 1
	}`)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{})
	if err != nil {
		t.Fatalf("ReadyExplain returned error on payload with schema_version field: %v", err)
	}

	// Decoder must ignore schema_version — result should still be valid.
	if got.Ready == nil {
		t.Fatal("expected non-nil Ready slice after tolerating schema_version field")
	}
}

// =============================================================================
// cnam — SearchIssues: INERT multi-status + text argv shape
// =============================================================================

// TestSearchIssuesInertMultiStatusTextArgvShape pins the INERT quirk documented
// in interface.go: when SearchIssues is called with non-empty Text and multiple
// Statuses, the gateway comma-joins the statuses into --status open,closed.
// This is INERT because bd search --status treats comma-joined values as a
// literal status name (not a union), silently returning empty results.
// No UI path currently exercises this path; the test guards against unintended
// regression — e.g., someone "fixing" the comma-join to pass --status all,
// which would change the behavior in a way the interface contract doesn't promise.
func TestSearchIssuesInertMultiStatusTextArgvShape(t *testing.T) {
	t.Parallel()

	// The gateway must emit --status open,closed (comma-joined statuses as-is).
	// Do NOT change this to --status all without updating interface.go INERT note.
	wantArgv := []string{"search", "foo", "--json", "--status", "open,closed"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(minimalIssueJSONArray)}, nil)

	gateway, rec := newTestGateway(rec)

	_, err := gateway.SearchIssues(context.Background(), domain.SearchIssuesQuery{
		Text:     "foo",
		Statuses: []string{"open", "closed"},
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}

	assertExactArgv(t, rec, wantArgv)
}

// =============================================================================
// wz3u — HealthCheck: CommandFailed on non-zero exit with generic stderr
// =============================================================================

// TestGatewayHealthCheckCommandFailed verifies that HealthCheck returns
// ErrorCodeCommandFailed when bd ping exits non-zero with stderr that does NOT
// match the "no beads database found" sentinel. This covers the third error-code
// path that was previously untested at the HealthCheck level.
func TestGatewayHealthCheckCommandFailed(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	// Exit code 1 with generic error message — NOT the NoDatabaseFound sentinel.
	rec.OnArgs([]string{"ping", "--json"}).Return(ExecResult{ExitCode: 1, Stderr: []byte("internal error: unexpected state")}, nil)

	gateway, _ := newTestGateway(rec)

	err := gateway.HealthCheck(context.Background())
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
}

// =============================================================================
// 82lm — ShowIssue: close_reason="Closed" literal default
// =============================================================================

// TestGatewayShowIssueDecodesDefaultCloseReason asserts that the gateway
// correctly decodes and passes through the close_reason="Closed" literal that
// bd stores when an issue is closed without an explicit --reason flag.
// This is bd's default — distinct from both explicit reasons ("completed") and
// absent reason ("").
func TestGatewayShowIssueDecodesDefaultCloseReason(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"show", "bw-800", "--json"}).Return(ExecResult{Stdout: readFixture(t, "show_closed_default_reason.json")}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: "bw-800"})
	if err != nil {
		t.Fatalf("ShowIssue returned error: %v", err)
	}

	if got.CloseReason != "Closed" {
		t.Fatalf("expected CloseReason=%q (bd literal default), got %q", "Closed", got.CloseReason)
	}
}

// =============================================================================
// kmfn — LabelCatalog: whitespace-strip + blank-skip
// =============================================================================

// TestGatewayCatalogStripsWhitespaceLabels verifies that:
//   - An all-space label ("   ") is excluded from the output (toLabelOption maps
//     it to Name="" after TrimSpace, and the gateway skips blank names).
//   - A whitespace-padded label ("  whitespace-label  ") is trimmed to
//     "whitespace-label" in the output.
func TestGatewayCatalogStripsWhitespaceLabels(t *testing.T) {
	t.Parallel()

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"label", "list-all", "--json"}).Return(ExecResult{Stdout: readFixture(t, "labels_whitespace.json")}, nil)

	gateway, _ := newTestGateway(rec)

	labels, err := gateway.LabelCatalog(context.Background())
	if err != nil {
		t.Fatalf("LabelCatalog returned error: %v", err)
	}

	// All-space entry must be excluded.
	for _, l := range labels {
		if l.Name == "" || l.Name == "   " {
			t.Errorf("LabelCatalog: all-space entry should be excluded, found %q in output", l.Name)
		}
	}

	// Padded entry must be trimmed.
	foundTrimmed := false
	for _, l := range labels {
		if l.Name == "whitespace-label" {
			foundTrimmed = true
		}
	}
	if !foundTrimmed {
		t.Errorf("LabelCatalog: whitespace-padded entry should appear trimmed as %q, got %v", "whitespace-label", labels)
	}

	// Normal label must be present unchanged.
	foundNormal := false
	for _, l := range labels {
		if l.Name == "normal-label" {
			foundNormal = true
		}
	}
	if !foundNormal {
		t.Errorf("LabelCatalog: normal label %q should be present, got %v", "normal-label", labels)
	}

	// Total: 2 entries (all-space excluded, padded trimmed, normal kept).
	if len(labels) != 2 {
		t.Errorf("LabelCatalog: expected 2 labels (all-space excluded), got %d: %v", len(labels), labels)
	}
}

// ============================================================
// puy3: TypeCatalog custom_types []string decode fix
// ============================================================

// TestGatewayTypeCatalogDecodesCustomTypesAsStrings verifies that TypeCatalog
// correctly handles the bd 1.0.4 shape where custom_types is a JSON array of
// bare strings, not objects.  Prior to puy3 the gateway would return
// ErrorCodeDecodeFailed for any workspace with bd config set types.custom.
func TestGatewayTypeCatalogDecodesCustomTypesAsStrings(t *testing.T) {
	t.Parallel()

	payload := `{
		"core_types": [
			{"name": "task", "description": "General work item"},
			{"name": "bug", "description": "Bug report"}
		],
		"custom_types": ["widget", "gadget"],
		"schema_version": 1
	}`

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"types", "--json"}).Return(ExecResult{Stdout: []byte(payload)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.TypeCatalog(context.Background())
	if err != nil {
		t.Fatalf("TypeCatalog returned error for custom_types []string payload: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 type options (2 core + 2 custom), got %d: %#v", len(got), got)
	}

	// Core types first, with descriptions.
	if got[0].Name != "task" || got[0].Description != "General work item" {
		t.Errorf("unexpected core type[0]: %#v", got[0])
	}

	if got[1].Name != "bug" || got[1].Description != "Bug report" {
		t.Errorf("unexpected core type[1]: %#v", got[1])
	}

	// Custom types appended as bare names with empty description.
	if got[2].Name != "widget" || got[2].Description != "" {
		t.Errorf("unexpected custom type[0]: %#v", got[2])
	}

	if got[3].Name != "gadget" || got[3].Description != "" {
		t.Errorf("unexpected custom type[1]: %#v", got[3])
	}
}

// TestGatewayTypeCatalogHandlesAbsentCustomTypes verifies that TypeCatalog
// succeeds when custom_types is absent (default workspace, no custom types
// configured) — the pre-existing nil-slice default must still work.
func TestGatewayTypeCatalogHandlesAbsentCustomTypes(t *testing.T) {
	t.Parallel()

	payload := `{
		"core_types": [
			{"name": "task", "description": "General work item"}
		],
		"schema_version": 1
	}`

	rec := newTestRecordingExecutor()
	rec.OnArgs([]string{"types", "--json"}).Return(ExecResult{Stdout: []byte(payload)}, nil)

	gateway, _ := newTestGateway(rec)

	got, err := gateway.TypeCatalog(context.Background())
	if err != nil {
		t.Fatalf("TypeCatalog returned error when custom_types absent: %v", err)
	}

	if len(got) != 1 || got[0].Name != "task" {
		t.Fatalf("unexpected types when custom_types absent: %#v", got)
	}
}

// ============================================================
// g2h5: CountIssues multi-status filter in-memory fallback
// ============================================================

// TestCountIssuesMultiStatusArgvOmitsStatusFlag verifies that when multiple
// statuses are requested, CountIssues does NOT pass --status to bd count
// (which would return empty due to bd 1.0.4 literal-match semantics).
// Instead, the call is issued without --status and filtering happens in-memory.
func TestCountIssuesMultiStatusArgvOmitsStatusFlag(t *testing.T) {
	t.Parallel()

	// The argv must NOT include --status when multiple statuses are given.
	wantArgv := []string{"count", "--by-status", "--json"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(`{
		"groups": [
			{"group": "open", "count": 5},
			{"group": "in_progress", "count": 3},
			{"group": "closed", "count": 10}
		],
		"total": 18,
		"schema_version": 1
	}`)}, nil)

	gateway, rec := newTestGateway(rec)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{
		Statuses: []string{"open", "in_progress"},
	})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	// Only open and in_progress groups should be returned, not closed.
	if len(got.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %#v", len(got.Groups), got.Groups)
	}

	totalInGroups := 0
	for _, g := range got.Groups {
		if g.Status != "open" && g.Status != "in_progress" {
			t.Errorf("unexpected group status %q in result", g.Status)
		}

		totalInGroups += g.Count
	}

	// Total must be the sum of the matched groups only (5+3=8), not the
	// bd-returned total of 18 which includes the unfiltered "closed" group.
	if got.Total != 8 {
		t.Errorf("expected total=8 (sum of matched groups), got %d", got.Total)
	}

	if totalInGroups != got.Total {
		t.Errorf("group counts %d don't sum to result total %d", totalInGroups, got.Total)
	}

	// Verify argv: no --status flag present.
	assertExactArgv(t, rec, wantArgv)
}

// TestCountIssuesSingleStatusArgvIncludesStatusFlag verifies that single-status
// queries still pass --status to bd count (no regression from g2h5 fix).
func TestCountIssuesSingleStatusArgvIncludesStatusFlag(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"count", "--by-status", "--json", "--status", "closed"}

	rec := newTestRecordingExecutor()
	rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(`{
		"groups": [{"group": "closed", "count": 7}],
		"total": 7,
		"schema_version": 1
	}`)}, nil)

	gateway, rec := newTestGateway(rec)

	got, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{
		Statuses: []string{"closed"},
	})
	if err != nil {
		t.Fatalf("CountIssues returned error: %v", err)
	}

	if got.Total != 7 || len(got.Groups) != 1 || got.Groups[0].Status != "closed" {
		t.Fatalf("unexpected result: %#v", got)
	}

	assertExactArgv(t, rec, wantArgv)
}
