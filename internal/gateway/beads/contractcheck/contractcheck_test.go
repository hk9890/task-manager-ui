package contractcheck

import (
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestViolation_String(t *testing.T) {
	v := Violation{Method: "ListIssues", Rule: "NonEmptyID", Sample: "items[0]: ID is empty"}
	got := v.String()
	want := "[ListIssues] NonEmptyID: items[0]: ID is empty"
	if got != want {
		t.Errorf("Violation.String() = %q, want %q", got, want)
	}
}

func TestValidateIssueSummaries(t *testing.T) {
	t.Parallel()

	validItem := domain.IssueSummary{
		ID:     "x-1",
		Title:  "My issue",
		Status: "open",
		Type:   "task",
	}

	tests := []struct {
		name      string
		items     []domain.IssueSummary
		wantRules []string // expected Rule values; nil means no violations
	}{
		{
			name:      "valid single item",
			items:     []domain.IssueSummary{validItem},
			wantRules: nil,
		},
		{
			name:      "empty slice",
			items:     []domain.IssueSummary{},
			wantRules: nil,
		},
		{
			name: "empty ID",
			items: []domain.IssueSummary{
				{ID: "", Title: "Title", Status: "open", Type: "task"},
			},
			wantRules: []string{"NonEmptyID"},
		},
		{
			name: "empty Title",
			items: []domain.IssueSummary{
				{ID: "x-1", Title: "", Status: "open", Type: "task"},
			},
			wantRules: []string{"NonEmptyTitle"},
		},
		{
			name: "empty Status",
			items: []domain.IssueSummary{
				{ID: "x-1", Title: "Title", Status: "", Type: "task"},
			},
			wantRules: []string{"NonEmptyStatus"},
		},
		{
			name: "empty Type",
			items: []domain.IssueSummary{
				{ID: "x-1", Title: "Title", Status: "open", Type: ""},
			},
			wantRules: []string{"NonEmptyType"},
		},
		{
			name: "multiple rules on same item",
			items: []domain.IssueSummary{
				{ID: "", Title: "", Status: "", Type: ""},
			},
			wantRules: []string{"NonEmptyID", "NonEmptyTitle", "NonEmptyStatus", "NonEmptyType"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateIssueSummaries("ListIssues", tc.items)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateListIssuesStatusFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		items             []domain.IssueSummary
		requestedStatuses []string
		wantRules         []string
	}{
		{
			name:              "no filter: always passes",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "closed", Type: "task"}},
			requestedStatuses: nil,
			wantRules:         nil,
		},
		{
			name:              "filter respected",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "open", Type: "task"}},
			requestedStatuses: []string{"open"},
			wantRules:         nil,
		},
		{
			name:              "filter violated",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "closed", Type: "task"}},
			requestedStatuses: []string{"open"},
			wantRules:         []string{"StatusFilterRespected"},
		},
		{
			name: "multiple statuses, all match",
			items: []domain.IssueSummary{
				{ID: "x-1", Title: "T", Status: "open", Type: "task"},
				{ID: "x-2", Title: "T", Status: "in_progress", Type: "task"},
			},
			requestedStatuses: []string{"open", "in_progress"},
			wantRules:         nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateListIssuesStatusFilter("ListIssues", tc.items, tc.requestedStatuses)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateListIssuesClosedExcluded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		items             []domain.IssueSummary
		requestedStatuses []string
		wantRules         []string
	}{
		{
			name:              "default query, no closed issues",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "open", Type: "task"}},
			requestedStatuses: nil,
			wantRules:         nil,
		},
		{
			name:              "default query, closed issue returned — violation",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "closed", Type: "task"}},
			requestedStatuses: nil,
			wantRules:         []string{"ClosedExcludedByDefault"},
		},
		{
			name:              "status filter applied: closed is ok",
			items:             []domain.IssueSummary{{ID: "x-1", Title: "T", Status: "closed", Type: "task"}},
			requestedStatuses: []string{"closed"},
			wantRules:         nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateListIssuesClosedExcluded("ListIssues", tc.items, tc.requestedStatuses)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateBlockedViews(t *testing.T) {
	t.Parallel()

	validView := domain.BlockedIssueView{
		Issue:     domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"},
		BlockedBy: []domain.IssueReference{{ID: "x-0"}},
	}

	tests := []struct {
		name      string
		views     []domain.BlockedIssueView
		wantRules []string
	}{
		{
			name:      "valid",
			views:     []domain.BlockedIssueView{validView},
			wantRules: nil,
		},
		{
			name:      "empty slice",
			views:     []domain.BlockedIssueView{},
			wantRules: nil,
		},
		{
			name: "empty Issue.ID",
			views: []domain.BlockedIssueView{
				{
					Issue:     domain.IssueSummary{ID: "", Title: "T", Status: "open", Type: "task"},
					BlockedBy: []domain.IssueReference{{ID: "x-0"}},
				},
			},
			wantRules: []string{"NonEmptyID"},
		},
		{
			name: "empty BlockedBy slice",
			views: []domain.BlockedIssueView{
				{
					Issue:     domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"},
					BlockedBy: []domain.IssueReference{},
				},
			},
			wantRules: []string{"NonEmptyBlockedBySlice"},
		},
		{
			name: "nil BlockedBy slice",
			views: []domain.BlockedIssueView{
				{
					Issue:     domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"},
					BlockedBy: nil,
				},
			},
			wantRules: []string{"NonEmptyBlockedBySlice"},
		},
		{
			name: "blocker with empty ID",
			views: []domain.BlockedIssueView{
				{
					Issue:     domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"},
					BlockedBy: []domain.IssueReference{{ID: ""}},
				},
			},
			wantRules: []string{"BlockerIDsNonEmpty"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateBlockedViews("BlockedIssues", tc.views)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateReadyExplain(t *testing.T) {
	t.Parallel()

	validResult := domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "x-1", Title: "Ready", Status: "open", Type: "task"},
		},
		Blocked: []domain.BlockedIssueView{
			{
				Issue:     domain.IssueSummary{ID: "x-2", Title: "Blocked", Status: "blocked", Type: "task"},
				BlockedBy: []domain.IssueReference{{ID: "x-0", Title: "Blocker", Status: "open"}},
			},
		},
		TotalReady:   1,
		TotalBlocked: 1,
	}

	tests := []struct {
		name         string
		result       domain.ReadyExplainResult
		limitApplied bool
		wantRules    []string
	}{
		{
			name:         "valid no-limit result",
			result:       validResult,
			limitApplied: false,
			wantRules:    nil,
		},
		{
			name: "empty ready ID",
			result: domain.ReadyExplainResult{
				Ready:        []domain.IssueSummary{{ID: "", Title: "X"}},
				TotalReady:   1,
				TotalBlocked: 0,
			},
			limitApplied: false,
			wantRules:    []string{"NonEmptyReadyIDs"},
		},
		{
			name: "empty blocked Issue.ID",
			result: domain.ReadyExplainResult{
				Blocked: []domain.BlockedIssueView{
					{
						Issue:     domain.IssueSummary{ID: "", Title: "T"},
						BlockedBy: []domain.IssueReference{{ID: "x-0", Title: "B", Status: "open"}},
					},
				},
				TotalBlocked: 1,
			},
			limitApplied: false,
			wantRules:    []string{"NonEmptyBlockedIDs"},
		},
		{
			name: "ready and blocked disjoint violation",
			result: domain.ReadyExplainResult{
				Ready: []domain.IssueSummary{{ID: "x-1", Title: "X", Status: "open", Type: "task"}},
				Blocked: []domain.BlockedIssueView{
					{
						Issue:     domain.IssueSummary{ID: "x-1", Title: "X", Status: "open", Type: "task"},
						BlockedBy: []domain.IssueReference{{ID: "x-0", Title: "B", Status: "open"}},
					},
				},
				TotalReady:   1,
				TotalBlocked: 1,
			},
			limitApplied: false,
			wantRules:    []string{"ReadyAndBlockedDisjoint"},
		},
		{
			name: "TotalReady mismatch when no limit",
			result: domain.ReadyExplainResult{
				Ready:        []domain.IssueSummary{{ID: "x-1", Title: "X", Status: "open", Type: "task"}},
				TotalReady:   5, // mismatch: len(Ready)=1
				TotalBlocked: 0,
			},
			limitApplied: false,
			wantRules:    []string{"TotalReadyMatchesLenReady"},
		},
		{
			name: "TotalReady mismatch but limit applied — no violation",
			result: domain.ReadyExplainResult{
				Ready:        []domain.IssueSummary{{ID: "x-1", Title: "X", Status: "open", Type: "task"}},
				TotalReady:   5,
				TotalBlocked: 0,
			},
			limitApplied: true,
			wantRules:    nil,
		},
		{
			name: "TotalBlocked mismatch when no limit",
			result: domain.ReadyExplainResult{
				Blocked: []domain.BlockedIssueView{
					{
						Issue:     domain.IssueSummary{ID: "x-2", Title: "T", Status: "open", Type: "task"},
						BlockedBy: []domain.IssueReference{{ID: "x-0", Title: "B", Status: "open"}},
					},
				},
				TotalReady:   0,
				TotalBlocked: 9, // mismatch: len(Blocked)=1
			},
			limitApplied: false,
			wantRules:    []string{"TotalBlockedMatchesLenBlocked"},
		},
		{
			name: "BlockedBy enrichment: missing Title",
			result: domain.ReadyExplainResult{
				Blocked: []domain.BlockedIssueView{
					{
						Issue: domain.IssueSummary{ID: "x-2", Title: "T", Status: "open", Type: "task"},
						BlockedBy: []domain.IssueReference{
							{ID: "x-0", Title: "", Status: "open"}, // Title empty
						},
					},
				},
				TotalBlocked: 1,
			},
			limitApplied: false,
			wantRules:    []string{"BlockedByEnriched"},
		},
		{
			name: "BlockedBy enrichment: missing Status",
			result: domain.ReadyExplainResult{
				Blocked: []domain.BlockedIssueView{
					{
						Issue: domain.IssueSummary{ID: "x-2", Title: "T", Status: "open", Type: "task"},
						BlockedBy: []domain.IssueReference{
							{ID: "x-0", Title: "Blocker", Status: ""}, // Status empty
						},
					},
				},
				TotalBlocked: 1,
			},
			limitApplied: false,
			wantRules:    []string{"BlockedByEnriched"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateReadyExplain("ReadyExplain", tc.result, tc.limitApplied)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateShowIssue(t *testing.T) {
	t.Parallel()

	validDetail := domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}

	tests := []struct {
		name        string
		detail      domain.IssueDetail
		requestedID string
		wantRules   []string
	}{
		{
			name:        "valid",
			detail:      validDetail,
			requestedID: "x-1",
			wantRules:   nil,
		},
		{
			name: "ID mismatch",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "wrong-id"},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			requestedID: "x-1",
			wantRules:   []string{"ReturnedIDMatchesInput"},
		},
		{
			name: "empty requestedID: no ID mismatch violation",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "anything"},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			requestedID: "",
			wantRules:   nil,
		},
		{
			name: "nil Comments",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1"},
				Comments:  nil,
				BlockedBy: []domain.IssueReference{},
			},
			requestedID: "x-1",
			wantRules:   []string{"CommentsNotNil"},
		},
		{
			name: "nil BlockedBy",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1"},
				Comments:  []domain.IssueComment{},
				BlockedBy: nil,
			},
			requestedID: "x-1",
			wantRules:   []string{"BlockedByNotNil"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateShowIssue("ShowIssue", tc.detail, tc.requestedID)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateSearchPage(t *testing.T) {
	t.Parallel()

	validPage := domain.SearchResultPage{
		Results: []domain.SearchResult{
			{Issue: domain.IssueSummary{ID: "x-1", Title: "T", Status: "open", Type: "task"}},
		},
		Metadata: domain.SearchResultMetadata{ReturnedCount: 1},
	}

	tests := []struct {
		name      string
		page      domain.SearchResultPage
		wantRules []string
	}{
		{
			name:      "valid",
			page:      validPage,
			wantRules: nil,
		},
		{
			name: "nil Results",
			page: domain.SearchResultPage{
				Results: nil,
			},
			wantRules: []string{"ResultsNotNil"},
		},
		{
			name: "empty Result Issue.ID",
			page: domain.SearchResultPage{
				Results: []domain.SearchResult{
					{Issue: domain.IssueSummary{ID: "", Title: "T"}},
				},
				Metadata: domain.SearchResultMetadata{ReturnedCount: 1},
			},
			wantRules: []string{"NonEmptyIDs"},
		},
		{
			name: "ReturnedCount mismatch",
			page: domain.SearchResultPage{
				Results: []domain.SearchResult{
					{Issue: domain.IssueSummary{ID: "x-1", Title: "T"}},
				},
				Metadata: domain.SearchResultMetadata{ReturnedCount: 5},
			},
			wantRules: []string{"ReturnedCountMatchesLen"},
		},
		{
			name: "empty results with count 0",
			page: domain.SearchResultPage{
				Results:  []domain.SearchResult{},
				Metadata: domain.SearchResultMetadata{ReturnedCount: 0},
			},
			wantRules: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateSearchPage("SearchIssues", tc.page)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateCountIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    domain.IssueCountResult
		wantRules []string
	}{
		{
			name: "valid",
			result: domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{{Status: "open", Count: 3}},
				Total:  3,
			},
			wantRules: nil,
		},
		{
			name: "empty groups with zero total",
			result: domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{},
				Total:  0,
			},
			wantRules: nil,
		},
		{
			name: "Total does not equal sum of Groups",
			result: domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{{Status: "open", Count: 3}},
				Total:  7,
			},
			wantRules: []string{"TotalEqualsSumOfGroups"},
		},
		{
			name: "group with empty Status",
			result: domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{{Status: "", Count: 2}},
				Total:  2,
			},
			wantRules: []string{"GroupStatusNonEmpty"},
		},
		{
			name: "group with zero Count",
			result: domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{{Status: "open", Count: 0}},
				Total:  0,
			},
			wantRules: []string{"NoZeroCountGroups"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateCountIssues("CountIssues", tc.result)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateStatusCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []domain.StatusOption
		wantRules []string
	}{
		{
			name:      "valid",
			opts:      []domain.StatusOption{{Name: "open"}, {Name: "closed"}},
			wantRules: nil,
		},
		{
			name:      "empty slice",
			opts:      []domain.StatusOption{},
			wantRules: []string{"NonEmpty"},
		},
		{
			name:      "nil slice",
			opts:      nil,
			wantRules: []string{"NonEmpty"},
		},
		{
			name:      "option with empty Name",
			opts:      []domain.StatusOption{{Name: ""}},
			wantRules: []string{"AllNamesNonEmpty"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateStatusCatalog("StatusCatalog", tc.opts)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateTypeCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []domain.TypeOption
		wantRules []string
	}{
		{
			name:      "valid",
			opts:      []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			wantRules: nil,
		},
		{
			name:      "empty slice",
			opts:      []domain.TypeOption{},
			wantRules: []string{"NonEmpty"},
		},
		{
			name:      "nil slice",
			opts:      nil,
			wantRules: []string{"NonEmpty"},
		},
		{
			name:      "option with empty Name",
			opts:      []domain.TypeOption{{Name: ""}},
			wantRules: []string{"AllNamesNonEmpty"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateTypeCatalog("TypeCatalog", tc.opts)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateLabelCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []domain.LabelOption
		wantRules []string
	}{
		{
			name:      "valid non-empty",
			opts:      []domain.LabelOption{{Name: "area:ui"}},
			wantRules: nil,
		},
		{
			name:      "empty slice — no violation (labels are optional)",
			opts:      []domain.LabelOption{},
			wantRules: nil,
		},
		{
			name:      "nil slice — no violation",
			opts:      nil,
			wantRules: nil,
		},
		{
			name:      "label with empty Name",
			opts:      []domain.LabelOption{{Name: ""}},
			wantRules: []string{"AllNamesNonEmpty"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateLabelCatalog("LabelCatalog", tc.opts)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateSsomInvariant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statuses   []string
		countTotal int
		listLen    int
		wantRules  []string
	}{
		{
			name:       "no status filter: invariant skipped",
			statuses:   nil,
			countTotal: 0,
			listLen:    5,
			wantRules:  nil,
		},
		{
			name:       "count >= listLen: valid",
			statuses:   []string{"open"},
			countTotal: 5,
			listLen:    5,
			wantRules:  nil,
		},
		{
			name:       "count > listLen: valid",
			statuses:   []string{"open"},
			countTotal: 10,
			listLen:    5,
			wantRules:  nil,
		},
		{
			name:       "count < listLen: violation",
			statuses:   []string{"closed"},
			countTotal: 0,
			listLen:    1,
			wantRules:  []string{"SsomCountGreaterThanOrEqualToListSize"},
		},
		{
			name:       "empty statuses: invariant skipped",
			statuses:   []string{},
			countTotal: 0,
			listLen:    1,
			wantRules:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateSsomInvariant("ListIssues", tc.statuses, tc.countTotal, tc.listLen)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestSpotCheckIndices(t *testing.T) {
	t.Parallel()

	t.Run("n=0: returns nil", func(t *testing.T) {
		t.Parallel()
		if got := SpotCheckIndices(0); got != nil {
			t.Errorf("SpotCheckIndices(0) = %v, want nil", got)
		}
	})

	t.Run("n=1: returns nil (within threshold)", func(t *testing.T) {
		t.Parallel()
		if got := SpotCheckIndices(1); got != nil {
			t.Errorf("SpotCheckIndices(1) = %v, want nil", got)
		}
	})

	t.Run("n=highCardinalityThreshold: returns nil", func(t *testing.T) {
		t.Parallel()
		if got := SpotCheckIndices(highCardinalityThreshold); got != nil {
			t.Errorf("SpotCheckIndices(%d) = %v, want nil", highCardinalityThreshold, got)
		}
	})

	t.Run("n=highCardinalityThreshold+1: returns non-nil indices", func(t *testing.T) {
		t.Parallel()
		n := highCardinalityThreshold + 1
		got := SpotCheckIndices(n)
		if got == nil {
			t.Fatalf("SpotCheckIndices(%d) = nil, want non-nil", n)
		}
		// All indices must be within [0, n).
		for _, idx := range got {
			if idx < 0 || idx >= n {
				t.Errorf("index %d is out of [0, %d)", idx, n)
			}
		}
		// Must have at least some head indices and at least some tail indices.
		hasHead := false
		hasTail := false
		for _, idx := range got {
			if idx < spotCheckCount {
				hasHead = true
			}
			if idx >= n-spotCheckCount {
				hasTail = true
			}
		}
		if !hasHead {
			t.Error("expected head indices (< spotCheckCount) but found none")
		}
		if !hasTail {
			t.Error("expected tail indices (>= n-spotCheckCount) but found none")
		}
	})

	t.Run("n=1000: returns nil (below threshold)", func(t *testing.T) {
		t.Parallel()
		if got := SpotCheckIndices(1000); got != nil {
			t.Errorf("SpotCheckIndices(1000) = %v, want nil", got)
		}
	})

	t.Run("n=-1: returns nil (below threshold)", func(t *testing.T) {
		t.Parallel()
		// Negative n is below threshold; function should return nil without panic.
		if got := SpotCheckIndices(-1); got != nil {
			t.Errorf("SpotCheckIndices(-1) = %v, want nil", got)
		}
	})

	t.Run("no duplicate indices", func(t *testing.T) {
		t.Parallel()
		n := highCardinalityThreshold + 100
		got := SpotCheckIndices(n)
		seen := make(map[int]bool)
		for _, idx := range got {
			if seen[idx] {
				t.Errorf("duplicate index %d in SpotCheckIndices(%d)", idx, n)
			}
			seen[idx] = true
		}
	})
}

func TestSelectIssueSummaries(t *testing.T) {
	t.Parallel()

	makeItems := func(n int) []domain.IssueSummary {
		items := make([]domain.IssueSummary, n)
		for i := range items {
			items[i] = domain.IssueSummary{ID: "x", Title: "T", Status: "open", Type: "task"}
		}
		return items
	}

	t.Run("nil indices: returns all items unchanged", func(t *testing.T) {
		t.Parallel()
		items := makeItems(5)
		got := SelectIssueSummaries(items, nil)
		if len(got) != len(items) {
			t.Errorf("len(got)=%d, want %d", len(got), len(items))
		}
	})

	t.Run("empty items with nil indices: returns empty", func(t *testing.T) {
		t.Parallel()
		got := SelectIssueSummaries([]domain.IssueSummary{}, nil)
		if len(got) != 0 {
			t.Errorf("len(got)=%d, want 0", len(got))
		}
	})

	t.Run("specific indices: returns only those items", func(t *testing.T) {
		t.Parallel()
		items := makeItems(10)
		got := SelectIssueSummaries(items, []int{0, 2, 9})
		if len(got) != 3 {
			t.Errorf("len(got)=%d, want 3", len(got))
		}
	})

	t.Run("indices smaller than list length", func(t *testing.T) {
		t.Parallel()
		items := makeItems(3)
		got := SelectIssueSummaries(items, []int{0, 1})
		if len(got) != 2 {
			t.Errorf("len(got)=%d, want 2", len(got))
		}
	})

	t.Run("out-of-range index is skipped", func(t *testing.T) {
		t.Parallel()
		items := makeItems(3)
		got := SelectIssueSummaries(items, []int{0, 100})
		if len(got) != 1 {
			t.Errorf("len(got)=%d, want 1 (index 100 should be skipped)", len(got))
		}
	})

	t.Run("negative index is skipped", func(t *testing.T) {
		t.Parallel()
		items := makeItems(3)
		got := SelectIssueSummaries(items, []int{-1, 0})
		if len(got) != 1 {
			t.Errorf("len(got)=%d, want 1 (index -1 should be skipped)", len(got))
		}
	})

	t.Run("empty index slice: returns empty (not nil)", func(t *testing.T) {
		t.Parallel()
		items := makeItems(5)
		got := SelectIssueSummaries(items, []int{})
		if got == nil {
			t.Error("expected non-nil slice for empty index set, got nil")
		}
		if len(got) != 0 {
			t.Errorf("len(got)=%d, want 0", len(got))
		}
	})
}

func TestValidateCreateIssueResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    domain.CreateIssueResult
		wantRules []string
	}{
		{
			name:      "valid",
			result:    domain.CreateIssueResult{IssueID: "x-42"},
			wantRules: nil,
		},
		{
			name:      "empty IssueID",
			result:    domain.CreateIssueResult{IssueID: ""},
			wantRules: []string{"NonEmptyID"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateCreateIssueResult("CreateIssue", tc.result)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateWriteVisibility(t *testing.T) {
	t.Parallel()

	detailWithTitle := func(title string) domain.IssueDetail {
		return domain.IssueDetail{
			Summary:   domain.IssueSummary{ID: "x-1", Title: title, Status: "open", Type: "task"},
			Comments:  []domain.IssueComment{},
			BlockedBy: []domain.IssueReference{},
		}
	}

	tests := []struct {
		name      string
		rule      string
		detail    domain.IssueDetail
		want      string
		wantRules []string
	}{
		// TitleRoundTrip
		{
			name:      "TitleRoundTrip: match",
			rule:      "TitleRoundTrip",
			detail:    detailWithTitle("My title"),
			want:      "My title",
			wantRules: nil,
		},
		{
			name:      "TitleRoundTrip: mismatch",
			rule:      "TitleRoundTrip",
			detail:    detailWithTitle("Wrong title"),
			want:      "My title",
			wantRules: []string{"TitleRoundTrip"},
		},
		// StatusAfterClose
		{
			name: "StatusAfterClose: closed",
			rule: "StatusAfterClose",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1", Status: "closed"},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "",
			wantRules: nil,
		},
		{
			name: "StatusAfterClose: still open — violation",
			rule: "StatusAfterClose",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1", Status: "open"},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "",
			wantRules: []string{"StatusAfterClose"},
		},
		// CommentVisible
		{
			name: "CommentVisible: comment present",
			rule: "CommentVisible",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1"},
				Comments:  []domain.IssueComment{{Body: "hello"}},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "hello",
			wantRules: nil,
		},
		{
			name: "CommentVisible: comment missing — violation",
			rule: "CommentVisible",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1"},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "hello",
			wantRules: []string{"CommentVisible"},
		},
		// LabelsAfterClear
		{
			name: "LabelsAfterClear: labels empty",
			rule: "LabelsAfterClear",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1", Labels: nil},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "",
			wantRules: nil,
		},
		{
			name: "LabelsAfterClear: labels still present — violation",
			rule: "LabelsAfterClear",
			detail: domain.IssueDetail{
				Summary:   domain.IssueSummary{ID: "x-1", Labels: []string{"area:ui"}},
				Comments:  []domain.IssueComment{},
				BlockedBy: []domain.IssueReference{},
			},
			want:      "",
			wantRules: []string{"LabelsAfterClear"},
		},
		// Unknown rule
		{
			name:      "unknown rule: emits violation with rule name",
			rule:      "SomeUnknownRule",
			detail:    detailWithTitle("X"),
			want:      "",
			wantRules: []string{"SomeUnknownRule"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateWriteVisibility("UpdateIssue", tc.rule, tc.detail, tc.want)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

func TestValidateCountIncrement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		countBefore int
		countAfter  int
		wantRules   []string
	}{
		{
			name:        "incremented by 1",
			countBefore: 3,
			countAfter:  4,
			wantRules:   nil,
		},
		{
			name:        "no change — violation",
			countBefore: 3,
			countAfter:  3,
			wantRules:   []string{"CountIncreasedByOne"},
		},
		{
			name:        "decreased — violation",
			countBefore: 3,
			countAfter:  2,
			wantRules:   []string{"CountIncreasedByOne"},
		},
		{
			name:        "increased by 2 — violation",
			countBefore: 3,
			countAfter:  5,
			wantRules:   []string{"CountIncreasedByOne"},
		},
		{
			name:        "from 0 to 1",
			countBefore: 0,
			countAfter:  1,
			wantRules:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := ValidateCountIncrement("CreateIssue", "open", tc.countBefore, tc.countAfter)
			assertViolationRules(t, vs, tc.wantRules)
		})
	}
}

// assertViolationRules checks that vs contains exactly the rules in wantRules
// (order-insensitive). If wantRules is nil, asserts len(vs)==0.
func assertViolationRules(t *testing.T, vs []Violation, wantRules []string) {
	t.Helper()
	if len(wantRules) == 0 {
		if len(vs) != 0 {
			t.Errorf("expected no violations, got %d: %v", len(vs), violationSummary(vs))
		}
		return
	}
	if len(vs) != len(wantRules) {
		t.Errorf("expected %d violation(s) %v, got %d: %v",
			len(wantRules), wantRules, len(vs), violationSummary(vs))
		return
	}
	wantSet := make(map[string]int, len(wantRules))
	for _, r := range wantRules {
		wantSet[r]++
	}
	for _, v := range vs {
		if wantSet[v.Rule] <= 0 {
			t.Errorf("unexpected violation rule %q (sample=%q)", v.Rule, v.Sample)
		} else {
			wantSet[v.Rule]--
		}
	}
	for rule, remaining := range wantSet {
		if remaining > 0 {
			t.Errorf("expected violation with rule %q but it was not emitted", rule)
		}
	}
}

func violationSummary(vs []Violation) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Rule
	}
	return strings.Join(parts, ", ")
}
