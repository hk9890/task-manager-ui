package filestorage_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	// Build a seeded memory repo with a representative set of issues.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	staticClock := func() time.Time { return t2 }
	r := memory.New(memory.WithClock(staticClock))

	r.Seed(memory.Issue{
		ID:          "rt-1",
		Title:       "Open issue",
		Status:      "open",
		Priority:    2,
		Type:        "task",
		Assignee:    "alice",
		Labels:      []string{"fix", "ui"},
		Description: "A description with unicode: こんにちは",
		Notes:       "Some notes",
		DependsOn:   []string{"rt-3"},
		Created:     t0,
		Updated:     t1,
	})

	r.Seed(memory.Issue{
		ID:      "rt-2",
		Title:   "In-progress issue",
		Status:  "in_progress",
		Type:    "bug",
		Created: t1,
		Updated: t1,
	})

	r.Seed(memory.Issue{
		ID:      "rt-3",
		Title:   "Closed blocker",
		Status:  "closed",
		Type:    "chore",
		Created: t0,
		Updated: t2,
	})
	r.SeedClosed("rt-3", t2, "done")

	r.SeedComments("rt-1",
		memory.Comment{
			ID:        "c-1",
			Author:    "bob",
			Body:      "First comment",
			CreatedAt: t1,
		},
		memory.Comment{
			ID:        "c-2",
			Author:    "carol",
			Body:      "Second comment",
			CreatedAt: t2,
		},
	)

	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Save.
	if err := filestorage.Save(r, path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	// Verify files exist.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Save: jsonl file not created: %v", err)
	}
	manifestPath := path + ".manifest.json"
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("Save: manifest file not created: %v", err)
	}

	// Load.
	loaded, err := filestorage.Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	// Verify round-trip via Snapshot.
	snap := loaded.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Load: expected 3 issues, got %d", len(snap))
	}

	byID := make(map[string]memory.SnapshotIssue, len(snap))
	for _, s := range snap {
		byID[s.ID] = s
	}

	// Verify rt-1 in full detail.
	rt1, ok := byID["rt-1"]
	if !ok {
		t.Fatal("Load: rt-1 missing from snapshot")
	}
	if rt1.Title != "Open issue" {
		t.Errorf("rt-1 Title: got %q, want %q", rt1.Title, "Open issue")
	}
	if rt1.Status != "open" {
		t.Errorf("rt-1 Status: got %q, want %q", rt1.Status, "open")
	}
	if rt1.Priority != 2 {
		t.Errorf("rt-1 Priority: got %d, want %d", rt1.Priority, 2)
	}
	if rt1.Assignee != "alice" {
		t.Errorf("rt-1 Assignee: got %q, want %q", rt1.Assignee, "alice")
	}
	if rt1.Description != "A description with unicode: こんにちは" {
		t.Errorf("rt-1 Description: got %q", rt1.Description)
	}
	if rt1.Notes != "Some notes" {
		t.Errorf("rt-1 Notes: got %q", rt1.Notes)
	}
	if len(rt1.Labels) != 2 {
		t.Errorf("rt-1 Labels: got %v, want [fix ui]", rt1.Labels)
	}
	if len(rt1.DependsOn) != 1 || rt1.DependsOn[0] != "rt-3" {
		t.Errorf("rt-1 DependsOn: got %v, want [rt-3]", rt1.DependsOn)
	}
	if !rt1.Created.Equal(t0) {
		t.Errorf("rt-1 Created: got %v, want %v", rt1.Created, t0)
	}
	if !rt1.Updated.Equal(t1) {
		t.Errorf("rt-1 Updated: got %v, want %v", rt1.Updated, t1)
	}
	if len(rt1.Comments) != 2 {
		t.Fatalf("rt-1 Comments: got %d, want 2", len(rt1.Comments))
	}
	if rt1.Comments[0].ID != "c-1" || rt1.Comments[0].Author != "bob" || rt1.Comments[0].Body != "First comment" {
		t.Errorf("rt-1 Comments[0]: got %+v", rt1.Comments[0])
	}
	if rt1.Comments[1].ID != "c-2" || rt1.Comments[1].Author != "carol" || rt1.Comments[1].Body != "Second comment" {
		t.Errorf("rt-1 Comments[1]: got %+v", rt1.Comments[1])
	}
	if !rt1.Comments[0].CreatedAt.Equal(t1) {
		t.Errorf("rt-1 Comments[0] CreatedAt: got %v, want %v", rt1.Comments[0].CreatedAt, t1)
	}

	// Verify rt-3 closed state.
	rt3, ok := byID["rt-3"]
	if !ok {
		t.Fatal("Load: rt-3 missing from snapshot")
	}
	if rt3.Status != "closed" {
		t.Errorf("rt-3 Status: got %q, want %q", rt3.Status, "closed")
	}
	if !rt3.Closed.Equal(t2) {
		t.Errorf("rt-3 Closed: got %v, want %v", rt3.Closed, t2)
	}
	if rt3.CloseReason != "done" {
		t.Errorf("rt-3 CloseReason: got %q, want %q", rt3.CloseReason, "done")
	}

	// Verify rt-2 (in_progress, no comments).
	rt2, ok := byID["rt-2"]
	if !ok {
		t.Fatal("Load: rt-2 missing from snapshot")
	}
	if rt2.Status != "in_progress" {
		t.Errorf("rt-2 Status: got %q", rt2.Status)
	}
	if len(rt2.Comments) != 0 {
		t.Errorf("rt-2 Comments: got %d, want 0", len(rt2.Comments))
	}
}

func TestLoadSchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Write JSONL file (even empty is fine).
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	// Write manifest with wrong schema version.
	manifestPath := path + ".manifest.json"
	badManifest := `{"schema_version": 999, "synced_at": "2026-01-01T00:00:00Z", "bd_commit_hash": ""}`
	if err := os.WriteFile(manifestPath, []byte(badManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := filestorage.Load(path)
	if err == nil {
		t.Fatal("Load with wrong schema version: expected error, got nil")
	}
	if !errors.Is(err, repository.ErrSchemaMismatch) {
		t.Errorf("Load with wrong schema version: expected ErrSchemaMismatch, got %v", err)
	}
}

func TestLoadMissingManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// JSONL exists but manifest does not.
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	_, err := filestorage.Load(path)
	if err == nil {
		t.Fatal("Load with missing manifest: expected error, got nil")
	}
}

func TestSaveEmptyRepository(t *testing.T) {
	r := memory.New()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	if err := filestorage.Save(r, path); err != nil {
		t.Fatalf("Save empty repo: unexpected error: %v", err)
	}

	loaded, err := filestorage.Load(path)
	if err != nil {
		t.Fatalf("Load empty repo: unexpected error: %v", err)
	}
	snap := loaded.Snapshot()
	if len(snap) != 0 {
		t.Errorf("Load empty repo: expected 0 issues, got %d", len(snap))
	}
}
