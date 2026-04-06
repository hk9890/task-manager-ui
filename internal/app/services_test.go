package app

import (
	"testing"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

func TestNewServicesWithLauncherRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := NewServicesWithLauncher(nil, config.Default(), &fakes.FakeLauncher{})
	if err == nil {
		t.Fatal("expected error when gateway is nil")
	}

	_, err = NewServicesWithLauncher(fakes.NewFakeBeadsGateway(), config.Default(), nil)
	if err == nil {
		t.Fatal("expected error when launcher service is nil")
	}
}

func TestNewServicesBuildsLauncherFromConfigDefinitions(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
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

	services, err := NewServices(gateway, cfg, "/tmp/beads-workbench")
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	if services.Launcher == nil {
		t.Fatal("expected launcher service to be configured")
	}
}
