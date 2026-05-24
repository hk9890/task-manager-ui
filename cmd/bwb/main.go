package main

import (
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
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/logging"
	repositorybeads "github.com/hk9890/beads-workbench/internal/repository/beads"
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
}

var startInteractive = func(cfg config.Model, opts startupOptions) error {
	runnerCfg := beads.RunnerConfig{WorkDir: opts.projectRoot}
	if opts.logManager != nil {
		runnerCfg.Logger = opts.logManager.Component("gateway")
	}
	gateway := beads.NewCLIGateway(beads.NewCommandRunner(runnerCfg))
	repo := repositorybeads.New(gateway)

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
	if startupLogger != nil {
		startupLogger.Info("resolved config path", "path", configResult.Path)
		startupLogger.Info("resolved cwd", "cwd", resolvedCWD)
		startupLogger.Info("auto-refresh", "enabled", autoRefresh)
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
	// gateway encounters it with a confusing error.
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
	_, _ = fmt.Fprintln(w, "  -h, --help            Show help")
	_, _ = fmt.Fprintln(w, "  -v, --version         Show version")
	_, _ = fmt.Fprintln(w, "  -c, --config <path>   Use explicit config file")
	_, _ = fmt.Fprintln(w, "      --cwd <path>      Target project directory")
	_, _ = fmt.Fprintln(w, "  -d, --debug           Enable debug diagnostics")
	_, _ = fmt.Fprintln(w, "      --no-auto-refresh Disable automatic refresh triggers")
	_, _ = fmt.Fprintln(w, "      --print-config    Print resolved config YAML")
	_, _ = fmt.Fprintln(w, "      --check-config    Validate config and exit")
}
