package repository_test

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/repository"
)

// TestDashboardOptionsClosedOffsetField verifies that DashboardOptions has a
// ClosedOffset int field and that its zero value is 0 (first page, existing
// behavior). Both backends honor the field.
func TestDashboardOptionsClosedOffsetField(t *testing.T) {
	// Zero value must be 0.
	var opts repository.DashboardOptions
	if opts.ClosedOffset != 0 {
		t.Fatalf("zero-value DashboardOptions.ClosedOffset = %d, want 0", opts.ClosedOffset)
	}

	// Explicit non-zero assignment round-trips.
	opts.ClosedOffset = 10
	if opts.ClosedOffset != 10 {
		t.Fatalf("DashboardOptions.ClosedOffset = %d after assignment, want 10", opts.ClosedOffset)
	}

	// Named-field construction used by callers is non-breaking: existing
	// literal forms compile and still default ClosedOffset to 0.
	byName := repository.DashboardOptions{ClosedLimit: 5}
	if byName.ClosedOffset != 0 {
		t.Fatalf("named-field literal DashboardOptions.ClosedOffset = %d, want 0", byName.ClosedOffset)
	}
}
