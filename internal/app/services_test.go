package app

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
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
			Action:  "opencode",
			Command: "opencode",
			Args:    []string{"run", "--issue", "{{issue.id}}"},
			Env:     []string{"TASKMGR_UI_ISSUE_ID={{issue.id}}", "TASKMGR_UI_PROJECT_ROOT={{project.root}}"},
			WorkDir: "{{project.root}}",
		},
	}

	services, err := NewServices(repository, cfg, "/tmp/task-manager-ui")
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	if services.Launcher == nil {
		t.Fatal("expected launcher service to be configured")
	}
}
