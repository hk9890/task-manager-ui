//go:build integration

package taskmgr

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Tagged integration: the watch is a real filesystem notification, and the SDK
// holds each signal for a quiet period.

func TestWatchChangesSignalsAWriteFromAnotherHandle(t *testing.T) {
	repo, store := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes, err := repo.WatchChanges(ctx)
	if err != nil {
		t.Fatalf("WatchChanges: %v", err)
	}

	other, err := tasks.Open(store.Root())
	if err != nil {
		t.Fatalf("tasks.Open: %v", err)
	}
	if _, err := other.Create(tasks.CreateInput{Title: "written by another handle"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	select {
	case _, open := <-changes:
		if !open {
			t.Fatal("the channel closed instead of signalling the write")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no signal within 5s of a write from another handle")
	}

	data, err := repo.Dashboard(ctx, repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if len(data.ReadyExplain.Ready) != 1 || data.ReadyExplain.Ready[0].Title != "written by another handle" {
		t.Errorf("the read after the signal does not hold the write: %#v", data.ReadyExplain.Ready)
	}

	cancel()
	// A signal that was already pending comes first; the close follows it.
	deadline := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-changes:
		case <-deadline:
			t.Fatal("the channel did not close within 5s of the cancel")
		}
	}
}

func TestWatchChangesOnAStoreThatIsGoneReturnsARepositoryError(t *testing.T) {
	repo, store := newTestRepo(t)
	if err := os.RemoveAll(store.Dir()); err != nil {
		t.Fatalf("remove the store: %v", err)
	}

	_, err := repo.WatchChanges(context.Background())
	if _, ok := err.(domain.RepositoryError); !ok {
		t.Fatalf("WatchChanges on a removed store returned %T (%v), want domain.RepositoryError", err, err)
	}
}
