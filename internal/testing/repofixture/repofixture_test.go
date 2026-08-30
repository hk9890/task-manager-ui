package repofixture_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/repofixture"
)

// TestSaveIsReproducible pins the JSONL line order.
//
// The issues live in a map, and Snapshot ranged it, so two saves of the same
// store wrote their lines in different orders: a fixture meant to pin a board
// order pinned nothing, and a diff of two saves was noise rather than a change.
func TestSaveIsReproducible(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	for i := range 25 {
		repo.Seed(memoryrepo.Issue{
			ID:       fmt.Sprintf("tm-%02d", i),
			Title:    fmt.Sprintf("Work %02d", i),
			Status:   "open",
			Type:     "task",
			Priority: 2,
		})
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "first.jsonl")
	second := filepath.Join(dir, "second.jsonl")

	if err := repofixture.Save(repo, first); err != nil {
		t.Fatalf("Save(first): %v", err)
	}
	if err := repofixture.Save(repo, second); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("ReadFile(first): %v", err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second): %v", err)
	}

	if !bytes.Equal(firstBytes, secondBytes) {
		t.Errorf("two saves of the same store differ:\nfirst:\n%s\nsecond:\n%s", firstBytes, secondBytes)
	}
}
