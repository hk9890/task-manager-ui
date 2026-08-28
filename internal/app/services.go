package app

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// execCmdWrapper wraps an *exec.Cmd so it satisfies the tea.ExecCommand interface.
type execCmdWrapper struct{ cmd *exec.Cmd }

func (w *execCmdWrapper) Run() error { return w.cmd.Run() }
func (w *execCmdWrapper) SetStdin(r io.Reader) {
	if w.cmd.Stdin == nil {
		w.cmd.Stdin = r
	}
}
func (w *execCmdWrapper) SetStdout(wr io.Writer) {
	if w.cmd.Stdout == nil {
		w.cmd.Stdout = wr
	}
}
func (w *execCmdWrapper) SetStderr(wr io.Writer) {
	if w.cmd.Stderr == nil {
		w.cmd.Stderr = wr
	}
}

// defaultExecCommandFactory wraps an *exec.Cmd as a tea.ExecCommand using the
// same "set if unset" semantics as Bubble Tea's own wrapExecCommand helper.
func defaultExecCommandFactory(cmd *exec.Cmd) tea.ExecCommand {
	return &execCmdWrapper{cmd: cmd}
}

// Services is the intentionally small root app container.
//
// Allowed dependencies:
//   - Repository (all issue reads/writes)
//   - config model (runtime preferences)
//
// This shell intentionally excludes orchestration/control-plane, SQL,
// caching, pub/sub, and watcher wiring.
// Launcher integration stays shell-owned so browse/detail modes can emit intent
// while launch execution stays centralized and reusable.
type Services struct {
	Repo     repository.Repository
	Launcher launcher.Service
	Editor   launchereditor.Service
	Config   config.Model
	// ExecCommandFactory wraps a *exec.Cmd as a tea.ExecCommand for the editor
	// launch flow. It defaults to a thin wrapper with Bubble Tea's "set if unset"
	// stdin/stdout/stderr semantics. Tests can inject a no-op implementation to
	// avoid launching real editor processes.
	ExecCommandFactory func(*exec.Cmd) tea.ExecCommand
	// Logger is the optional root runtime logger. It must NOT carry a
	// "component" attribute; NewModelWithOptions derives per-mode loggers
	// (component=board, component=search, …) via logging.WithComponent. When
	// nil, each mode falls back to slog.Default().
	Logger *slog.Logger
}

// LaunchableActions are the launcher action names the shell can actually start:
// "editor" through the edit keybinding, and one per launch keybinding. The
// keybinding action set is fixed (see internal/config), so a definition with any
// other action name is configured but unreachable.
var LaunchableActions = []string{"editor", "nvim", "opencode", "shell-command"}

// LauncherDefinitions converts the configured launcher definitions into the
// launcher package's own type. Exported so --check-config validates exactly the
// definition set an interactive start builds.
func LauncherDefinitions(cfg config.Model) []launcher.Definition {
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
	return definitions
}

// ValidateLauncherDefinitions reports whether the configured launcher
// definitions would be accepted by an interactive start.
func ValidateLauncherDefinitions(cfg config.Model) error {
	return launcher.ValidateDefinitions(LauncherDefinitions(cfg))
}

// UnlaunchableActions returns the configured launcher actions no keybinding can
// start, in config order. Startup warns about each one rather than leaving the
// operator with a definition that validates and then never runs.
func UnlaunchableActions(cfg config.Model) []string {
	launchable := make(map[string]struct{}, len(LaunchableActions))
	for _, action := range LaunchableActions {
		launchable[action] = struct{}{}
	}

	var unreachable []string
	for _, definition := range cfg.Launcher.Definitions {
		action := strings.TrimSpace(definition.Action)
		if action == "" {
			continue
		}
		if _, ok := launchable[action]; !ok {
			unreachable = append(unreachable, action)
		}
	}
	return unreachable
}

// NewServices constructs the minimal app services container.
func NewServices(repo repository.Repository, cfg config.Model, projectRoot string) (Services, error) {
	if repo == nil {
		return Services{}, errors.New("repo is required")
	}

	launcherService, err := launcher.NewService(LauncherDefinitions(cfg), projectRoot, launcher.NewExecProcessRunner())
	if err != nil {
		return Services{}, err
	}

	editorService, err := launchereditor.NewIssueEditor(repo, cfg.Editor.Command)
	if err != nil {
		return Services{}, err
	}

	return Services{
		Repo:               repo,
		Launcher:           launcherService,
		Editor:             editorService,
		Config:             cfg,
		ExecCommandFactory: defaultExecCommandFactory,
	}, nil
}

// SweepStaleTempFiles removes taskmgr-ui-issue-*.md files in os.TempDir() older
// than 24 hours — leftover editor documents from sessions killed by SIGKILL or a
// panic, where the normal defer os.Remove never ran.
//
// This is scheduled by Model.Init rather than fired from a constructor: deleting
// files is not something a caller can expect from constructing a value, and
// doing it in one of the two constructors made them non-substitutable.
func (s Services) SweepStaleTempFiles() tea.Cmd {
	return func() tea.Msg {
		cleanStaleTempFilesInDir(s.Logger, os.TempDir())
		return nil
	}
}

// cleanStaleTempFilesInDir is the testable core of cleanStaleTempFiles.
// It scans dir for taskmgr-ui-issue-*.md files older than 24h and removes them.
func cleanStaleTempFilesInDir(logger *slog.Logger, dir string) {
	if logger == nil {
		logger = slog.Default()
	}

	matches, err := filepath.Glob(filepath.Join(dir, "taskmgr-ui-issue-*.md"))
	if err != nil {
		logger.Warn("temp cleanup: glob failed", "dir", dir, "error", err.Error())
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			// File may have been removed concurrently; skip silently.
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil {
			logger.Warn("temp cleanup: remove failed", "path", path, "error", err.Error())
		} else {
			logger.Info("temp cleanup: removed stale temp file", "path", path, "age_hours", time.Since(info.ModTime()).Hours())
		}
	}
}

// NewServicesWithLauncher constructs services with an injected launcher seam.
func NewServicesWithLauncher(repo repository.Repository, cfg config.Model, launcherService launcher.Service) (Services, error) {
	if repo == nil {
		return Services{}, errors.New("repo is required")
	}
	if launcherService == nil {
		return Services{}, errors.New("launcher service is required")
	}

	editorService, err := launchereditor.NewIssueEditor(repo, cfg.Editor.Command)
	if err != nil {
		return Services{}, err
	}

	return Services{
		Repo:               repo,
		Launcher:           launcherService,
		Editor:             editorService,
		Config:             cfg,
		ExecCommandFactory: defaultExecCommandFactory,
	}, nil
}
