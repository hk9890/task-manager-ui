package taskmgr

import (
	"context"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// TestIssueParentRefPopulated exercises the toDetail ParentRef branch in
// convert.go against the real store: a child issue created with a parent must
// surface that parent in ParentGroupBrowser.Parent with full metadata. The
// parent/child edge is created via the SDK because the Repository create surface
// does not carry a parent field.
func TestIssueParentRefPopulated(t *testing.T) {
	r, store := newTestRepo(t)
	ctx := context.Background()

	parentRes, err := store.Create(tasks.CreateInput{Title: "Parent epic", Type: "task"})
	if err != nil {
		t.Fatalf("store.Create parent: %v", err)
	}
	parentID := parentRes.Issue.ID

	childRes, err := store.Create(tasks.CreateInput{Title: "Child task", Parent: parentID})
	if err != nil {
		t.Fatalf("store.Create child: %v", err)
	}

	d, err := r.Issue(ctx, childRes.Issue.ID)
	if err != nil {
		t.Fatalf("Issue(child): %v", err)
	}

	got := d.ParentGroupBrowser.Parent
	if got.ID != parentID {
		t.Errorf("ParentGroupBrowser.Parent.ID = %q, want %q", got.ID, parentID)
	}
	if got.Title != "Parent epic" {
		t.Errorf("ParentGroupBrowser.Parent.Title = %q, want %q", got.Title, "Parent epic")
	}
	if got.Type != "task" {
		t.Errorf("ParentGroupBrowser.Parent.Type = %q, want task", got.Type)
	}
}

// TestIssueNoParentRefEmpty confirms an issue without a parent yields an empty
// ParentGroupBrowser.Parent (the d.ParentRef == nil path in toDetail).
func TestIssueNoParentRefEmpty(t *testing.T) {
	r, _ := newTestRepo(t)
	id := mustCreate(t, r, domain.CreateIssueInput{Title: "Orphan"})

	d, err := r.Issue(context.Background(), id)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if d.ParentGroupBrowser.Parent.ID != "" {
		t.Errorf("expected empty ParentGroupBrowser.Parent, got %+v", d.ParentGroupBrowser.Parent)
	}
}
