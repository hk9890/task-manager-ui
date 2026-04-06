package app

import (
	"errors"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/launcher"
	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
)

// SharedUIState contains shell-wide UI state shared across modes.
type SharedUIState struct {
	ToastMessage string
}

// Services is the intentionally small root app container.
//
// Allowed dependencies:
//   - Beads gateway (all issue reads/writes)
//   - config model (runtime preferences)
//   - shared UI state
//
// This shell intentionally excludes BQL, orchestration/control-plane, SQL,
// caching, pub/sub, and watcher wiring.
// Launcher integration stays shell-owned so browse/detail modes can emit intent
// while launch execution stays centralized and reusable.
type Services struct {
	Gateway  beads.BeadsGateway
	Launcher launcher.Service
	Editor   launchereditor.Service
	Config   config.Model
	UI       SharedUIState
}

// NewServices constructs the minimal app services container.
func NewServices(gateway beads.BeadsGateway, cfg config.Model, projectRoot string) (Services, error) {
	if gateway == nil {
		return Services{}, errors.New("gateway is required")
	}

	definitions := make([]launcher.Definition, 0, len(cfg.Launcher.Definitions))
	for _, definition := range cfg.Launcher.Definitions {
		definitions = append(definitions, launcher.Definition{
			Action:  definition.Action,
			Command: definition.Command,
			Args:    append([]string(nil), definition.Args...),
			Env:     append([]string(nil), definition.Env...),
			WorkDir: definition.WorkDir,
		})
	}

	launcherService, err := launcher.NewService(definitions, projectRoot, launcher.NewExecProcessRunner())
	if err != nil {
		return Services{}, err
	}

	editorService, err := launchereditor.NewIssueEditor(gateway, launchereditor.ProcessOpener{EditorCommand: cfg.Editor.Command})
	if err != nil {
		return Services{}, err
	}

	return Services{
		Gateway:  gateway,
		Launcher: launcherService,
		Editor:   editorService,
		Config:   cfg,
		UI:       SharedUIState{},
	}, nil
}

// NewServicesWithLauncher constructs services with an injected launcher seam.
func NewServicesWithLauncher(gateway beads.BeadsGateway, cfg config.Model, launcherService launcher.Service) (Services, error) {
	if gateway == nil {
		return Services{}, errors.New("gateway is required")
	}
	if launcherService == nil {
		return Services{}, errors.New("launcher service is required")
	}

	editorService, err := launchereditor.NewIssueEditor(gateway, launchereditor.ProcessOpener{EditorCommand: cfg.Editor.Command})
	if err != nil {
		return Services{}, err
	}

	return Services{
		Gateway:  gateway,
		Launcher: launcherService,
		Editor:   editorService,
		Config:   cfg,
		UI:       SharedUIState{},
	}, nil
}
