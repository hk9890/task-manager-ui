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

// A store opened through resolve gives up a write when another process holds
// the real flock on .tasks/.lock for longer than the lock timeout, and the
// write reaches the app as a timeout. The lock is taken through a second open
// file, as another process takes it.
func TestResolvedStoreWriteTimesOutWhileAnotherWriterHoldsTheLock(t *testing.T) {
	t.Setenv("TASKMGR_HOME", t.TempDir())
	ctx := context.Background()

	dir := t.TempDir()
	if _, err := tasks.Init(dir, "lck"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	store, _, err := resolve(tasks.ResolveOptions{WorkDir: dir}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	repo := New(store)

	release := holdWriteLock(t, filepath.Join(dir, tasks.DataDirName, ".lock"))

	_, err = repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "blocked"})
	var re domain.RepositoryError
	if !errors.As(err, &re) || re.Code != domain.ErrorCodeTimeout {
		t.Fatalf("CreateIssue = %v, want a domain.RepositoryError with code %q", err, domain.ErrorCodeTimeout)
	}

	release()
	if _, err := repo.CreateIssue(ctx, domain.CreateIssueInput{Title: "free"}); err != nil {
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
