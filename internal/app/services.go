package app

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/launcher"
	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
	"github.com/hk9890/beads-workbench/internal/repository"
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
// This shell intentionally excludes BQL, orchestration/control-plane, SQL,
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
	// (component=board, component=search, …) via modeLogger. When nil, each
	// mode falls back to slog.Default().
	Logger *slog.Logger
	// OnEditIssueResult is a test-only hook called after editIssueResultMsg is
	// fully processed and the toast has been set. It is nil in production and
	// must not be set in non-test code. Tests can use it to replace the
	// time.Sleep settle budget with a precise synchronisation point.
	OnEditIssueResult func()
}

// NewServices constructs the minimal app services container.
func NewServices(repo repository.Repository, cfg config.Model, projectRoot string) (Services, error) {
	if repo == nil {
		return Services{}, errors.New("repo is required")
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

	editorService, err := launchereditor.NewIssueEditor(repo, cfg.Editor.Command)
	if err != nil {
		return Services{}, err
	}

	go cleanStaleTempFiles(slog.Default())

	return Services{
		Repo:               repo,
		Launcher:           launcherService,
		Editor:             editorService,
		Config:             cfg,
		ExecCommandFactory: defaultExecCommandFactory,
	}, nil
}

// cleanStaleTempFiles removes bwb-issue-*.md files in os.TempDir() that are
// older than 24 hours. These are leftover temp documents from editor sessions
// that were interrupted by SIGKILL or a panic (the normal defer os.Remove path
// only runs on clean exit).
func cleanStaleTempFiles(logger *slog.Logger) {
	cleanStaleTempFilesInDir(logger, os.TempDir())
}

// cleanStaleTempFilesInDir is the testable core of cleanStaleTempFiles.
// It scans dir for bwb-issue-*.md files older than 24h and removes them.
func cleanStaleTempFilesInDir(logger *slog.Logger, dir string) {
	if logger == nil {
		logger = slog.Default()
	}

	matches, err := filepath.Glob(filepath.Join(dir, "bwb-issue-*.md"))
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
