// Command taskmgr-ui is the terminal UI for browsing task-manager issues.
//
// This package owns the pre-TUI surface only: flag parsing, config resolution,
// logging setup, and repository backend selection. Everything after the Bubble
// Tea program starts belongs to internal/app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/task-manager/sdk/tasks"
	"gopkg.in/yaml.v3"

	"github.com/hk9890/task-manager-ui/internal/app"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/logging"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/repository/filestorage"
	repositorytaskmgr "github.com/hk9890/task-manager-ui/internal/repository/taskmgr"
	appversion "github.com/hk9890/task-manager-ui/internal/version"
)

var configLoad = func(opts config.LoadOptions) (config.Result, error) {
	return config.LoadWithOptions(opts)
}

type startupOptions struct {
	projectRoot string
	storeName   string // central store to open by registry name; empty means resolve from projectRoot
	debug       bool
	autoRefresh bool
	logManager  *logging.Manager
	repoFlag    string // "taskmgr" (default) or "memory"
	repoFile    string // resolved path; source of truth for --repo memory, ignored by taskmgr
}

// buildRepository selects and opens the repository backend for startInteractive.
// Neither backend needs shutdown work: the memory backend is a loaded value, and
// the taskmgr backend's store holds no handle to release.
//
// It also returns the project root to interpolate into launchers. For taskmgr
// that is the store's own project path, not opts.projectRoot: a central store is
// resolved from a registry entry whose project path is the registered root, and
// even a local store resolves by walking up, so both differ from the working
// directory when the app is started from a subdirectory. The memory backend has
// no store to ask, so it keeps the working directory.
func buildRepository(opts startupOptions) (repository.Repository, string, error) {
	switch opts.repoFlag {
	case "memory":
		loaded, err := filestorage.Load(opts.repoFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load memory repository from %q: %w", opts.repoFile, err)
		}
		return loaded, opts.projectRoot, nil

	default: // "taskmgr" (default) or unset
		// Resolve, not Open: Open performs local discovery only, so a project
		// whose store was promoted with `taskmgr store move --central` reports
		// ErrNoStore. Resolve walks up for a local .tasks first and falls back to
		// the central registry, matching what the taskmgr CLI does.
		store, info, err := tasks.Resolve(tasks.ResolveOptions{
			WorkDir:   opts.projectRoot,
			StoreName: opts.storeName,
		})
		if err != nil {
			if opts.storeName != "" {
				return nil, "", fmt.Errorf("failed to open central task-manager store %q: %w", opts.storeName, err)
			}
			return nil, "", fmt.Errorf("failed to open task-manager store for %q: %w", opts.projectRoot, err)
		}
		if opts.logManager != nil {
			startup := opts.logManager.Component("startup")
			startup.Info("resolved task-manager store",
				"kind", info.Kind.String(),
				"store_path", info.StorePath,
				"project_path", info.ProjectPath,
			)
			// Resolution checks the store directory, never the project path the
			// registry records for it, so an entry left behind by a project that
			// was moved or deleted still opens: the board reads fine while every
			// launcher without an explicit work_dir execs somewhere that is gone.
			// Warn rather than substitute the working directory — running an
			// editor in a directory the operator did not name is worse than a
			// launch that fails and says why.
			if _, statErr := os.Stat(info.ProjectPath); statErr != nil {
				startup.Warn("resolved store's project path is not accessible; launchers without an explicit work_dir will fail",
					"project_path", info.ProjectPath,
					"error", statErr.Error(),
				)
			}
		}
		return repositorytaskmgr.New(store, repositorytaskmgr.WithAuthor(resolveAuthor())), info.ProjectPath, nil
	}
}

var startInteractive = func(cfg config.Model, opts startupOptions) error {
	// Cancelled when program.Run returns, so repository reads still in flight at
	// shutdown are abandoned rather than waited on.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo, projectRoot, err := buildRepository(opts)
	if err != nil {
		return err
	}

	services, err := app.NewServices(repo, cfg, projectRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}
	if opts.logManager != nil {
		services.Logger = opts.logManager.Logger()
	}

	model, err := app.NewModelWithOptions(services, app.RuntimeOptions{
		DisableAutoRefresh: !opts.autoRefresh,
		Ctx:                ctx,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize app model: %w", err)
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("taskmgr-ui failed: %w", err)
	}

	return nil
}

type cliOptions struct {
	help        bool
	showVersion bool
	configPath  string
	cwdPath     string
	printConfig bool
	checkConfig bool
	debug       bool
	noAuto      bool
	repo        string // "taskmgr" (default) or "memory"
	repoFile    string // path to JSONL file; required for --repo memory, ignored by taskmgr
	storeName   string // central store registry name; taskmgr backend only
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, configLoad, startInteractive))
}

func run(args []string, stdout, stderr io.Writer, load func(config.LoadOptions) (config.Result, error), start func(config.Model, startupOptions) error) int {
	return runWithLogger(args, stdout, stderr, load, start, logging.New)
}

func runWithLogger(args []string, stdout, stderr io.Writer, load func(config.LoadOptions) (config.Result, error), start func(config.Model, startupOptions) error, newLogger func(logging.Options) *logging.Manager) int {
	opts, code, ok := parseCLI(args, stderr)
	if !ok {
		return code
	}

	if opts.help {
		printUsage(stdout)
		return 0
	}

	if opts.showVersion {
		_, _ = fmt.Fprintf(stdout, "taskmgr-ui %s (commit %s, built %s)\n", appversion.Version, appversion.Commit, appversion.Date)
		return 0
	}

	startCWD, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to resolve process start cwd: %v\n", err)
		return 1
	}

	resolvedCWD, err := resolveAndValidateCWD(startCWD, opts.cwdPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to resolve --cwd: %v\n", err)
		return 1
	}

	var logManager *logging.Manager
	if newLogger != nil {
		logManager = newLogger(logging.Options{
			Debug:        opts.debug,
			Stderr:       stderr,
			ProjectRoot:  resolvedCWD,
			BuildVersion: appversion.Version,
		})
		if logManager != nil {
			defer func() {
				_ = logManager.Close()
			}()
		}
	}

	resolvedConfigPath := resolveAgainstStartCWD(startCWD, opts.configPath)
	var startupLogger *slog.Logger
	if logManager != nil {
		startupLogger = logManager.Component("startup")
	}

	loadOpts := config.LoadOptions{Path: resolvedConfigPath, RequireExplicit: opts.configPath != ""}
	configResult, err := load(loadOpts)
	if err != nil {
		if startupLogger != nil {
			startupLogger.Error("failed to load config", "error", err.Error(), "path", resolvedConfigPath)
		} else {
			_, _ = fmt.Fprintf(stderr, "failed to load config: %v\n", err)
		}
		return 1
	}

	for _, warning := range configResult.Warnings {
		if startupLogger != nil {
			startupLogger.Warn("config warning", "warning", warning)
		} else {
			_, _ = fmt.Fprintf(stderr, "taskmgr-ui config warning: %s\n", warning)
		}
	}

	autoRefresh := !opts.noAuto

	// Resolve --repo-file: only --repo=memory consumes it (the JSONL source of
	// truth); taskmgr ignores it. Relative paths resolve against the start cwd.
	resolvedRepoFile := opts.repoFile
	if opts.repo == "memory" {
		resolvedRepoFile = resolveAgainstStartCWD(startCWD, resolvedRepoFile)
	}

	if startupLogger != nil {
		startupLogger.Info("resolved config path", "path", configResult.Path)
		startupLogger.Info("resolved cwd", "cwd", resolvedCWD)
		startupLogger.Info("auto-refresh", "enabled", autoRefresh)
		startupLogger.Info("repo backend", "repo", opts.repo, "repo_file", resolvedRepoFile, "store_name", opts.storeName)
	}

	if opts.printConfig {
		encoded, err := yaml.Marshal(configResult.Config)
		if err != nil {
			if startupLogger != nil {
				startupLogger.Error("failed to encode resolved config", "error", err.Error())
			} else {
				_, _ = fmt.Fprintf(stderr, "failed to encode resolved config: %v\n", err)
			}
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "# source: %s\n", configResult.Path)
		_, _ = stdout.Write(encoded)
		return 0
	}

	if opts.checkConfig {
		_, _ = fmt.Fprintln(stdout, "config OK")
		return 0
	}

	// Suppress stderr writes for the duration of the interactive session.
	// tea.NewProgram (called inside start) owns the alt-screen TTY; any slog
	// write to os.Stderr during this window corrupts the rendered frame.
	// All log records still reach the persistent JSON file.
	// Suppression is lifted after start() returns so post-exit error messages
	// reach the terminal normally.
	// Note: --debug does NOT re-enable stderr during interactive mode; debug
	// output belongs in the file only. Users can tail -f the persistent log.
	if logManager != nil {
		logManager.SetStderrSuppressed(true)
	}
	startErr := start(configResult.Config, startupOptions{
		projectRoot: resolvedCWD,
		storeName:   opts.storeName,
		debug:       opts.debug,
		autoRefresh: autoRefresh,
		logManager:  logManager,
		repoFlag:    opts.repo,
		repoFile:    resolvedRepoFile,
	})
	if logManager != nil {
		logManager.SetStderrSuppressed(false)
	}
	if startErr != nil {
		if startupLogger != nil {
			startupLogger.Error("interactive startup failed", "error", startErr.Error())
		} else {
			_, _ = fmt.Fprintln(stderr, startErr.Error())
		}
		return 1
	}

	return 0
}

// resolveAuthor determines the identity recorded as the creator of new issues
// and the author of comments for the task-manager backend. It prefers $USER
// ($USERNAME on Windows) and falls back to a stable default.
func resolveAuthor() string {
	for _, env := range []string{"USER", "USERNAME"} {
		if u := strings.TrimSpace(os.Getenv(env)); u != "" {
			return u
		}
	}
	return "taskmgr-ui"
}

func parseCLI(args []string, stderr io.Writer) (cliOptions, int, bool) {
	var opts cliOptions

	fs := flag.NewFlagSet("taskmgr-ui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printUsage(stderr)
	}

	fs.BoolVar(&opts.help, "h", false, "show help")
	fs.BoolVar(&opts.help, "help", false, "show help")
	fs.BoolVar(&opts.showVersion, "v", false, "show version")
	fs.BoolVar(&opts.showVersion, "version", false, "show version")
	fs.StringVar(&opts.configPath, "c", "", "path to config file")
	fs.StringVar(&opts.configPath, "config", "", "path to config file")
	fs.StringVar(&opts.cwdPath, "cwd", "", "target project directory")
	fs.BoolVar(&opts.printConfig, "print-config", false, "print resolved config")
	fs.BoolVar(&opts.checkConfig, "check-config", false, "validate resolved config")
	fs.BoolVar(&opts.debug, "d", false, "enable debug diagnostics")
	fs.BoolVar(&opts.debug, "debug", false, "enable debug diagnostics")
	fs.BoolVar(&opts.noAuto, "no-auto-refresh", false, "disable periodic auto-refresh")
	fs.StringVar(&opts.repo, "repo", "taskmgr", "repository backend: taskmgr (default) or memory")
	fs.StringVar(&opts.repoFile, "repo-file", "", "path to JSONL repository file (required for --repo=memory; ignored by --repo=taskmgr)")
	fs.StringVar(&opts.storeName, "store-name", "", "open the central task-manager store registered under this name")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliOptions{help: true}, 0, true
		}
		return cliOptions{}, 2, false
	}

	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		fs.Usage()
		return cliOptions{}, 2, false
	}

	// Validate --repo value.
	switch opts.repo {
	case "taskmgr", "memory":
		// valid
	default:
		_, _ = fmt.Fprintf(stderr, "--repo must be taskmgr or memory, got %q\n", opts.repo)
		fs.Usage()
		return cliOptions{}, 2, false
	}

	// --repo=memory requires --repo-file.
	if opts.repo == "memory" && strings.TrimSpace(opts.repoFile) == "" {
		_, _ = fmt.Fprintln(stderr, "--repo=memory requires --repo-file <path>")
		fs.Usage()
		return cliOptions{}, 2, false
	}

	// --store-name names a task-manager store, so it is meaningless for the
	// memory backend. Rejecting it beats silently ignoring it: the flag is how
	// the caller says which issues they expect to see.
	opts.storeName = strings.TrimSpace(opts.storeName)
	if opts.repo == "memory" && opts.storeName != "" {
		_, _ = fmt.Fprintln(stderr, "--store-name is not valid with --repo=memory")
		fs.Usage()
		return cliOptions{}, 2, false
	}

	return opts, 0, true
}

func resolveAgainstStartCWD(startCWD, path string) string {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(startCWD, path)
}

func resolveAndValidateCWD(startCWD, cwdOverride string) (string, error) {
	resolved := startCWD
	if strings.TrimSpace(cwdOverride) != "" {
		resolved = resolveAgainstStartCWD(startCWD, cwdOverride)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path %q does not exist", resolved)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", resolved)
	}

	// Probe for read access. os.Open on a directory succeeds only when the
	// caller has at minimum read+execute permission, catching EACCES before the
	// repository encounters it with a confusing error.
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("path %q is not accessible: %w", resolved, err)
	}
	_ = f.Close()

	return resolved, nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: taskmgr-ui [options]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  -h, --help                 Show help")
	_, _ = fmt.Fprintln(w, "  -v, --version              Show version")
	_, _ = fmt.Fprintln(w, "  -c, --config <path>        Use explicit config file")
	_, _ = fmt.Fprintln(w, "      --cwd <path>           Target project directory")
	_, _ = fmt.Fprintln(w, "  -d, --debug                Enable debug diagnostics")
	_, _ = fmt.Fprintln(w, "      --no-auto-refresh      Disable automatic refresh triggers")
	_, _ = fmt.Fprintln(w, "      --print-config         Print resolved config YAML")
	_, _ = fmt.Fprintln(w, "      --check-config         Validate config and exit")
	_, _ = fmt.Fprintln(w, "      --repo <backend>       Repository backend: taskmgr|memory (default: taskmgr)")
	_, _ = fmt.Fprintln(w, "      --repo-file <path>     JSONL repository file; required when --repo=memory")
	_, _ = fmt.Fprintln(w, "                             (the file is the source of truth). Ignored by taskmgr.")
	_, _ = fmt.Fprintln(w, "      --store-name <name>    Open the central task-manager store registered under")
	_, _ = fmt.Fprintln(w, "                             this name, ignoring --cwd. Not valid with --repo=memory.")
}
