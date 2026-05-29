package beads

import (
	"testing"
)

// sp is a convenience helper that returns a pointer to a string literal.
func sp(s string) *string { return &s }

// TestLeanDepsFromPayload_NoDepsBlockedByNotNil verifies that leanDepsFromPayload
// returns a non-nil (empty) BlockedBy slice when the issue has no depends-on
// dependencies. A nil slice would violate the validating decorator's
// BlockedByNotNil contract rule.
func TestLeanDepsFromPayload_NoDepsBlockedByNotNil(t *testing.T) {
	blockedBy, related, _, hasParent, err := leanDepsFromPayload(nil, "test-op")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasParent {
		t.Errorf("hasParent: want false for empty input, got true")
	}
	if blockedBy == nil {
		t.Error("BlockedBy must not be nil for an issue with no depends-on deps (BlockedByNotNil contract)")
	}
	if len(blockedBy) != 0 {
		t.Errorf("BlockedBy: want 0 entries, got %d", len(blockedBy))
	}
	// related is not contract-checked but we initialize it for symmetry.
	if related == nil {
		t.Error("related must not be nil (symmetry with blockedBy)")
	}
}

// TestLeanDependentsFromPayload_ParentChildNotInBlocks verifies that entries
// with dependency_type="parent-child" are routed to the Children bucket and
// never appear in Blocks.
func TestLeanDependentsFromPayload_ParentChildNotInBlocks(t *testing.T) {
	records := []leanIssueRefPayload{
		{ID: sp("b1"), Title: sp("Blocks entry"), DependencyType: sp("blocks")},
		{ID: sp("c1"), Title: sp("Child entry"), DependencyType: sp("parent-child")},
		{ID: sp("r1"), Title: sp("Related entry"), DependencyType: sp("related")},
	}

	blocks, related, children, err := leanDependentsFromPayload(records, "test-op")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Blocks must contain exactly b1.
	if len(blocks) != 1 {
		t.Errorf("Blocks: want 1 entry, got %d", len(blocks))
	} else if blocks[0].ID != "b1" {
		t.Errorf("Blocks[0].ID: want %q, got %q", "b1", blocks[0].ID)
	}

	// c1 must NOT appear in Blocks.
	for _, ref := range blocks {
		if ref.ID == "c1" {
			t.Errorf("Blocks must not contain parent-child entry c1")
		}
	}

	// Related must contain exactly r1.
	if len(related) != 1 {
		t.Errorf("Related: want 1 entry, got %d", len(related))
	} else if related[0].ID != "r1" {
		t.Errorf("Related[0].ID: want %q, got %q", "r1", related[0].ID)
	}

	// Children must contain exactly c1.
	if len(children) != 1 {
		t.Errorf("Children: want 1 entry, got %d", len(children))
	} else if children[0].ID != "c1" {
		t.Errorf("Children[0].ID: want %q, got %q", "c1", children[0].ID)
	}
}
