//go:build integration

package taskmgr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// A store the catalog opens gives up a write when another writer holds the
// real flock on .tasks/.lock for longer than the lock timeout, and the write
// reaches the app as a timeout. The lock is taken through a second open file,
// as another process takes it.
func TestOpenedStoreWriteTimesOutWhileAnotherWriterHoldsTheLock(t *testing.T) {
	isolateCentralHome(t)
	ctx := context.Background()

	catalog := New("tester")
	if catalog.lockTimeout <= 0 {
		t.Fatalf("New sets lockTimeout %v, want a bound", catalog.lockTimeout)
	}
	catalog.lockTimeout = 100 * time.Millisecond

	dir := t.TempDir()
	opened, err := catalog.CreateLocal(ctx, dir, "lck")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	release := holdWriteLock(t, filepath.Join(dir, tasks.DataDirName, ".lock"))

	_, err = opened.Repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "blocked"})
	var re domain.RepositoryError
	if !errors.As(err, &re) || re.Code != domain.ErrorCodeTimeout {
		t.Fatalf("CreateIssue = %v, want a domain.RepositoryError with code %q", err, domain.ErrorCodeTimeout)
	}

	release()
	if _, err := opened.Repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "free"}); err != nil {
		t.Fatalf("CreateIssue after the release = %v, want success", err)
	}
}

// holdWriteLock takes an exclusive flock on path through its own open file and
// returns the func that releases it. The cleanup releases it if the test did not.
func holdWriteLock(t *testing.T, path string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open the lock file: %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		t.Fatalf("flock: %v", err)
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		if err := f.Close(); err != nil {
			t.Errorf("release the lock: %v", err)
		}
	}
	t.Cleanup(release)
	return release
}
