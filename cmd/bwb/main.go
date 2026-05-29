package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/hk9890/beads-workbench/internal/app"
	"github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/logging"
	"github.com/hk9890/beads-workbench/internal/repository"
	repositorybeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	repositorycaching "github.com/hk9890/beads-workbench/internal/repository/caching"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	bwbversion "github.com/hk9890/beads-workbench/internal/version"
)

var configLoad = func(opts config.LoadOptions) (config.Result, error) {
	return config.LoadWithOptions(opts)
}

type startupOptions struct {
	projectRoot string
	debug       bool
	autoRefresh bool
	logManager  *logging.Manager
	repoFlag    string // "beads" (default), "memory", or "caching"
	repoFile    string // resolved path; cache file for caching mode, source of truth for memory, informational for beads
}

// constructRepository builds and wires the repository for startInteractive.
// It returns the repository, a cleanup function to call on exit, and any
// construction error. The cleanup function is always safe to call even when
// an error is returned (it is a no-op in that case).
func constructRepository(ctx context.Context, opts startupOptions) (repository.Repository, func(), error) {
	noop := func() {}

	switch opts.repoFlag {
	case "memory":
		loaded, err := filestorage.Load(opts.repoFile)
		if err != nil {
			return nil, noop, fmt.Errorf("failed to load memory repository from %q: %w", opts.repoFile, err)
		}
		return loaded, noop, nil

	case "caching":
		// Construct backing beads repository.
		runnerCfg := bd.RunnerConfig{WorkDir: opts.projectRoot}
		if opts.logManager != nil {
			runnerCfg.Logger = opts.logManager.Component("repository")
		}
		runner := bd.NewCommandRunner(runnerCfg)
		backing := repository.NewValidating(repositorybeads.New(runner), logManagerComponent(opts.logManager, "validating"))

		// Bind vcStatusFunc.
		vcStatusFunc := func(c context.Context) (string, error) {
			return bd.VCStatusHash(c, runner)
		}

		// Construct CachingRepository.
		cacheLogger := logManagerComponent(opts.logManager, "caching")
		cache := repositorycaching.New(backing, repositorycaching.WithVCStatusFunc(vcStatusFunc))

		// Ensure the cache file's parent directory exists so SaveNow can rename
		// into it. A missing directory is logged as a warning; the session
		// continues without persistence.
		if opts.repoFile != "" {
			if err := os.MkdirAll(filepath.Dir(opts.repoFile), 0o755); err != nil {
				if cacheLogger != nil {
					cacheLogger.Warn("cache dir creation failed; session will run without persistence", "err", err, "path", opts.repoFile)
				}
			}
		}

		// Determine the load path: scan sibling dirs for the most-recent prior
		// session's cache file (same project hash, different session ID).
		// writePath is always the own-session file.
		writePath := opts.repoFile
		var loadPath string
		if opts.repoFile != "" {
			cacheBaseDir := filepath.Dir(filepath.Dir(opts.repoFile))
			// projectHash is the part of the directory name before the "-<sessionID>" suffix.
			ownDir := filepath.Base(filepath.Dir(opts.repoFile))
			projectHash := ownDir
			if idx := strings.LastIndexByte(ownDir, '-'); idx >= 0 {
				projectHash = ownDir[:idx]
			}
			loadPath = findLatestProjectCacheFile(cacheBaseDir, projectHash, cacheLogger)
		}

		// Hydrate from prior session file (loadPath) and write to own session
		// file (writePath). Warn on error, continue cold.
		if err := cache.Hydrate(ctx, loadPath, writePath); err != nil {
			if cacheLogger != nil {
				cacheLogger.Warn("cache hydrate failed; starting cold", "err", err, "load_path", loadPath, "write_path", writePath)
			}
		}

		// Start background refresh + save tick loop.
		cache.Start(ctx)

		// Log startup mode hint.
		if cacheLogger != nil {
			cacheLogger.Info("Using caching repository backend; --repo beads disables", "cache_file", opts.repoFile)
		}

		cleanup := func() {
			if err := cache.SaveNow(); err != nil {
				if cacheLogger != nil {
					cacheLogger.Warn("cache save on shutdown failed", "err", err)
				}
			}
			cache.Stop()
		}
		return cache, cleanup, nil

	default: // "beads" or unset (legacy path)
		runnerCfg := bd.RunnerConfig{WorkDir: opts.projectRoot}
		if opts.logManager != nil {
			runnerCfg.Logger = opts.logManager.Component("repository")
		}
		runner := bd.NewCommandRunner(runnerCfg)
		return repository.NewValidating(repositorybeads.New(runner), logManagerComponent(opts.logManager, "validating")), noop, nil
	}
}

var startInteractive = func(cfg config.Model, opts startupOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo, cleanup, err := constructRepository(ctx, opts)
	if err != nil {
		return err
	}
	defer cleanup()

	services, err := app.NewServices(repo, cfg, opts.projectRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}
	if opts.logManager != nil {
		services.Logger = opts.logManager.Logger()
	}

	model, err := app.NewModelWithOptions(services, app.RuntimeOptions{DisableAutoRefresh: !opts.autoRefresh})
	if err != nil {
		return fmt.Errorf("failed to initialize app model: %w", err)
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("bwb failed: %w", err)
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
	repo        string // "beads" (default), "memory", or "caching"
	repoFile    string // path to JSONL file; cache file for caching mode, required for memory, informational for beads
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
		_, _ = fmt.Fprintf(stdout, "bwb %s (commit %s, built %s)\n", bwbversion.Version, bwbversion.Commit, bwbversion.Date)
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
			BuildVersion: bwbversion.Version,
		})
		if logManager != nil {
			defer func() {
				_ = logManager.Close()
			}()
		}
	}

	resolvedConfigPath := resolveAgainstStartCWD(startCWD, opts.configPath)
	startupLogger := logManagerComponent(logManager, "startup")

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
			_, _ = fmt.Fprintf(stderr, "bwb config warning: %s\n", warning)
		}
	}

	autoRefresh := !opts.noAuto

	// Resolve --repo-file: derive defaults and resolve relative paths.
	// - caching mode: per-session cache file at ~/.cache/bwb/<hash>-<sessionID>/repo.jsonl
	// - beads mode: informational path at ~/.cache/bwb/<hash>/repo.jsonl (not read/written)
	// - memory mode: path was validated non-empty in parseCLI; resolve relative paths against startCWD
	sessionID := ""
	if logManager != nil {
		sessionID = logManager.SessionID()
	}
	resolvedRepoFile := opts.repoFile
	switch opts.repo {
	case "caching":
		if resolvedRepoFile == "" {
			resolvedRepoFile = defaultRepoFilePath(resolvedCWD, sessionID)
		}
	case "beads":
		if resolvedRepoFile == "" {
			resolvedRepoFile = defaultRepoFilePath(resolvedCWD, "")
		}
	case "memory":
		resolvedRepoFile = resolveAgainstStartCWD(startCWD, resolvedRepoFile)
	}

	if startupLogger != nil {
		startupLogger.Info("resolved config path", "path", configResult.Path)
		startupLogger.Info("resolved cwd", "cwd", resolvedCWD)
		startupLogger.Info("auto-refresh", "enabled", autoRefresh)
		startupLogger.Info("repo backend", "repo", opts.repo, "repo_file", resolvedRepoFile)
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

func logManagerComponent(manager *logging.Manager, component string) *slog.Logger {
	if manager == nil {
		return nil
	}
	return manager.Component(component)
}

func parseCLI(args []string, stderr io.Writer) (cliOptions, int, bool) {
	var opts cliOptions

	fs := flag.NewFlagSet("bwb", flag.ContinueOnError)
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
	fs.StringVar(&opts.repo, "repo", "beads", "repository backend: beads (default), memory, or caching")
	fs.StringVar(&opts.repoFile, "repo-file", "", "path to JSONL repository file (required for --repo=memory; cache file for --repo=caching)")

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
	case "beads", "memory", "caching":
		// valid
	default:
		_, _ = fmt.Fprintf(stderr, "--repo must be beads, memory, or caching, got %q\n", opts.repo)
		fs.Usage()
		return cliOptions{}, 2, false
	}

	// --repo=memory requires --repo-file.
	if opts.repo == "memory" && strings.TrimSpace(opts.repoFile) == "" {
		_, _ = fmt.Fprintln(stderr, "--repo=memory requires --repo-file <path>")
		fs.Usage()
		return cliOptions{}, 2, false
	}

	return opts, 0, true
}

// defaultRepoFilePath derives the default JSONL cache path for a given project
// root and optional session ID.
//
// The project hash is sha256(absPath)[:12] — a deterministic 12-hex-character
// identifier derived from the absolute project root. When sessionID is non-empty,
// the directory component is "<project-hash>-<sessionID>", giving each session
// its own cache file. When sessionID is empty (e.g. for beads mode where the
// path is informational), the legacy "<project-hash>" directory is used.
//
// Cache directory layout:
//
//	~/.cache/bwb/<project-hash>[-<sessionID>]/repo.jsonl
//
// os.UserCacheDir() is used in preference to $HOME/.cache because it is
// platform-aware (returns %AppData%\Local on Windows, ~/Library/Caches on
// macOS, $XDG_CACHE_HOME or ~/.cache on Linux).
func defaultRepoFilePath(projectRoot, sessionID string) string {
	sum := sha256.Sum256([]byte(projectRoot))
	hash := fmt.Sprintf("%x", sum[:6]) // 6 bytes = 12 hex chars
	dirComponent := hash
	if strings.TrimSpace(sessionID) != "" {
		dirComponent = hash + "-" + strings.TrimSpace(sessionID)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback: use a fixed relative path so callers always get a non-empty string.
		cacheDir = filepath.Join(os.TempDir(), ".cache")
	}
	return filepath.Join(cacheDir, "bwb", dirComponent, "repo.jsonl")
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
	_, _ = fmt.Fprintln(w, "Usage: bwb [options]")
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
	_, _ = fmt.Fprintln(w, "      --repo <backend>       Repository backend: beads|memory|caching (default: beads)")
	_, _ = fmt.Fprintln(w, "      --repo-file <path>     JSONL repository file")
	_, _ = fmt.Fprintln(w, "                             Default (beads):   ~/.cache/bwb/<project-hash>/repo.jsonl (informational)")
	_, _ = fmt.Fprintln(w, "                             Default (caching): ~/.cache/bwb/<project-hash>-<session-id>/repo.jsonl")
	_, _ = fmt.Fprintln(w, "                             Required when --repo=memory (the file is the source of truth)")
}
