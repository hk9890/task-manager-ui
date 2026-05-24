package beads

import (
	"fmt"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// TestLeanSearchMetadataFromBackend_CompletenessRules verifies that
// leanSearchMetadataFromBackend returns Exact when no limit was requested and
// Partial only when the result count hit the cap.
func TestLeanSearchMetadataFromBackend_CompletenessRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		requestedLimit int
		returnedCount  int
		want           domain.SearchResultCompleteness
	}{
		{
			name:           "no limit, some results",
			requestedLimit: 0,
			returnedCount:  5,
			want:           domain.SearchResultCompletenessExact,
		},
		{
			name:           "no limit, empty result",
			requestedLimit: 0,
			returnedCount:  0,
			want:           domain.SearchResultCompletenessExact,
		},
		{
			name:           "limit not reached",
			requestedLimit: 10,
			returnedCount:  5,
			want:           domain.SearchResultCompletenessExact,
		},
		{
			name:           "limit exactly reached",
			requestedLimit: 10,
			returnedCount:  10,
			want:           domain.SearchResultCompletenessPartial,
		},
		{
			name:           "limit exceeded (defensive)",
			requestedLimit: 10,
			returnedCount:  15,
			want:           domain.SearchResultCompletenessPartial,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := leanSearchMetadataFromBackend(tc.returnedCount, tc.requestedLimit, domain.SearchResultSourceBDSearch)
			if got.Completeness != tc.want {
				t.Errorf("requestedLimit=%d returnedCount=%d: Completeness = %q, want %q",
					tc.requestedLimit, tc.returnedCount, got.Completeness, tc.want)
			}
		})
	}
}

// minimalIssuePayload returns a leanIssuePayload with all required fields
// populated so it survives leanMapIssueSummaries without error.
func minimalIssuePayload(id string) leanIssuePayload {
	title := "Test issue " + id
	status := "open"
	issueType := "task"
	priority := 3
	ts := "2024-01-01T00:00:00Z"
	return leanIssuePayload{
		ID:        &id,
		Title:     &title,
		Status:    &status,
		IssueType: &issueType,
		Priority:  &priority,
		CreatedAt: &ts,
		UpdatedAt: &ts,
	}
}

// TestSearchPageFromRecords_CompletenessRules verifies that searchPageFromRecords
// applies the same Exact/Partial logic as leanSearchMetadataFromBackend.
func TestSearchPageFromRecords_CompletenessRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		limit       int
		resultCount int
		want        domain.SearchResultCompleteness
	}{
		{
			name:        "no limit, some results",
			limit:       0,
			resultCount: 5,
			want:        domain.SearchResultCompletenessExact,
		},
		{
			name:        "no limit, empty result",
			limit:       0,
			resultCount: 0,
			want:        domain.SearchResultCompletenessExact,
		},
		{
			name:        "limit not reached",
			limit:       10,
			resultCount: 5,
			want:        domain.SearchResultCompletenessExact,
		},
		{
			name:        "limit exactly reached",
			limit:       10,
			resultCount: 10,
			want:        domain.SearchResultCompletenessPartial,
		},
		{
			name:        "limit exceeded (defensive)",
			limit:       10,
			resultCount: 15,
			want:        domain.SearchResultCompletenessPartial,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build minimal valid payloads. The in-memory filter applies no
			// text/status filter when the query fields are zero-valued, so all
			// items pass through and the count equals tc.resultCount.
			items := make([]leanIssuePayload, tc.resultCount)
			for i := range items {
				items[i] = minimalIssuePayload(fmt.Sprintf("issue-%d", i))
			}

			repo := &Repository{}
			query := domain.SearchIssuesQuery{Limit: tc.limit}
			page, err := repo.searchPageFromRecords(items, query, domain.SearchResultSourceBDSearch)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if page.Metadata.Completeness != tc.want {
				t.Errorf("limit=%d resultCount=%d: Completeness = %q, want %q",
					tc.limit, tc.resultCount, page.Metadata.Completeness, tc.want)
			}
		})
	}
}
