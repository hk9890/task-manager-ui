package beads

import (
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// TestBeadsRepository_Invalidate_ClearsParentSiblingCache verifies that
// Invalidate empties parentSiblingCache under its own lock, so the next
// parentChildSiblings call will re-run the bd subprocess rather than serving
// stale data.
//
// White-box test: the test lives in package beads so it can access the
// unexported parentSiblingCache and parentSiblingCacheMu fields directly.
func TestBeadsRepository_Invalidate_ClearsParentSiblingCache(t *testing.T) {
	// New(nil) is safe here because Invalidate only touches parentSiblingCache /
	// parentSiblingCacheMu and never calls runner.
	r := New(nil)

	// Seed the cache with a value as parentChildSiblings would.
	r.parentSiblingCacheMu.Lock()
	r.parentSiblingCache["parent-1"] = []domain.IssueReference{
		{ID: "child-1"},
		{ID: "child-2"},
	}
	r.parentSiblingCacheMu.Unlock()

	// Pre-condition: cache is non-empty.
	r.parentSiblingCacheMu.RLock()
	before := len(r.parentSiblingCache)
	r.parentSiblingCacheMu.RUnlock()
	if before != 1 {
		t.Fatalf("pre-condition: expected 1 cache entry, got %d", before)
	}

	// Call Invalidate.
	r.Invalidate()

	// Post-condition: cache is empty.
	r.parentSiblingCacheMu.RLock()
	after := len(r.parentSiblingCache)
	r.parentSiblingCacheMu.RUnlock()
	if after != 0 {
		t.Fatalf("after Invalidate: expected 0 cache entries, got %d", after)
	}
}
