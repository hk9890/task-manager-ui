package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/config"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

func TestNewServicesWithLauncherRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := NewServicesWithLauncher(nil, config.Default(), &fakes.FakeLauncher{})
	if err == nil {
		t.Fatal("expected error when repository is nil")
	}

	_, err = NewServicesWithLauncher(memoryrepo.New(), config.Default(), nil)
	if err == nil {
		t.Fatal("expected error when launcher service is nil")
	}
}

func TestNewServicesBuildsLauncherFromConfigDefinitions(t *testing.T) {
	t.Parallel()

	repository := memoryrepo.New()
	cfg := config.Default()
	cfg.Launcher.Definitions = []config.LauncherDefinition{
		{
			Action:  "editor",
			Command: "opencode",
			Args:    []string{"run", "--issue", "{{issue.id}}"},
			Env:     []string{"BWB_ISSUE_ID={{issue.id}}", "BWB_PROJECT_ROOT={{project.root}}"},
			WorkDir: "{{project.root}}",
		},
	}

	services, err := NewServices(repository, cfg, "/tmp/beads-workbench")
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	if services.Launcher == nil {
		t.Fatal("expected launcher service to be configured")
	}
}

// TestCleanStaleTempFiles verifies that cleanStaleTempFiles removes files
// older than 24 hours and leaves recent files untouched.
func TestCleanStaleTempFiles(t *testing.T) {
	t.Parallel()

	// Create a temp directory so we can control which files exist.
	dir := t.TempDir()

	createFile := func(name string, mtime time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatalf("create test file %q: %v", path, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("set mtime on %q: %v", path, err)
		}
		return path
	}

	now := time.Now()
	stale := createFile("bwb-issue-abc123-001.md", now.Add(-25*time.Hour))
	recent := createFile("bwb-issue-def456-002.md", now.Add(-1*time.Hour))
	unrelated := createFile("unrelated.md", now.Add(-48*time.Hour))

	// Override os.TempDir by patching: instead, call the internal helper
	// directly but we can't change TempDir. Use a wrapper approach:
	// We call cleanStaleTempFilesInDir (introduced for testability).
	cleanStaleTempFilesInDir(nil, dir)

	// Stale file must be gone.
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected stale file %q to be removed", stale)
	}

	// Recent file must still exist.
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("expected recent file %q to still exist: %v", recent, err)
	}

	// Unrelated file (no bwb-issue prefix) must not be touched; Glob won't match it.
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("expected unrelated file %q to still exist: %v", unrelated, err)
	}
}
