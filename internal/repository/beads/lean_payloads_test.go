package beads

import (
	"testing"
)

// sp is a convenience helper that returns a pointer to a string literal.
func sp(s string) *string { return &s }

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
