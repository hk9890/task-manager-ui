package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/logging"
	appversion "github.com/hk9890/task-manager-ui/internal/version"
)

func TestArchitectureGuardrails(t *testing.T) {
	t.Parallel()

	modulePath := currentModulePath(t)
	pkgs := listCmdPackages(t)
	firstPartyDeps := filterFirstPartyImportPaths(pkgs, modulePath)

	t.Run("no direct SQL in active product path", func(t *testing.T) {
		t.Parallel()
		assertNoForbiddenDirectImportsInFirstPartyDeps(t, pkgs, modulePath, func(dep string) bool {
			return dep == "database/sql" || dep == "database/sql/driver"
		})
	})

	t.Run("no internal/bql dependency in standalone app", func(t *testing.T) {
		t.Parallel()
		assertNoForbiddenDeps(t, firstPartyDeps, func(dep string) bool {
			return strings.Contains(dep, "/internal/bql")
		})
	})

	t.Run("no orchestration/control-plane subsystem in active product path", func(t *testing.T) {
		t.Parallel()
		orchestrationPattern := regexp.MustCompile(`(^|/)orchestration($|/)|(^|/)control[-_]?plane($|/)`)
		assertNoForbiddenDeps(t, firstPartyDeps, func(dep string) bool {
			return orchestrationPattern.MatchString(dep)
		})
	})
}

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func currentModulePath(t *testing.T) string {
	t.Helper()

	moduleRoot := moduleRootDir(t)

	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedName,
		Dir:  moduleRoot,
	}
	pkgs, err := packages.Load(cfg, "./cmd/taskmgr-ui")
	if err != nil {
		t.Fatalf("resolving module path failed: %v", err)
	}
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		t.Fatal("packages.Load returned no module information")
	}

	modulePath := pkgs[0].Module.Path
	if modulePath == "" {
		t.Fatal("packages.Load returned empty module path")
	}

	return modulePath
}

func listCmdPackages(t *testing.T) []listedPackage {
	t.Helper()

	moduleRoot := moduleRootDir(t)

	cfg := &packages.Config{
		Mode: packages.NeedDeps | packages.NeedImports | packages.NeedName,
		Dir:  moduleRoot,
	}
	roots, err := packages.Load(cfg, "./cmd/taskmgr-ui")
	if err != nil {
		t.Fatalf("listing package metadata for ./cmd/taskmgr-ui failed: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("packages.Load returned no packages for ./cmd/taskmgr-ui")
	}

	// Walk the full transitive dependency graph.
	seen := make(map[string]bool)
	result := make([]listedPackage, 0)
	var walk func(pkg *packages.Package)
	walk = func(pkg *packages.Package) {
		if seen[pkg.PkgPath] {
			return
		}
		seen[pkg.PkgPath] = true

		imports := make([]string, 0, len(pkg.Imports))
		for importPath := range pkg.Imports {
			imports = append(imports, importPath)
		}
		result = append(result, listedPackage{
			ImportPath: pkg.PkgPath,
			Imports:    imports,
		})

		for _, dep := range pkg.Imports {
			walk(dep)
		}
	}
	for _, root := range roots {
		walk(root)
	}

	if len(result) == 0 {
		t.Fatal("packages.Load returned no package metadata for ./cmd/taskmgr-ui")
	}

	return result
}

func moduleRootDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func filterFirstPartyImportPaths(pkgs []listedPackage, modulePath string) []string {
	deps := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if strings.HasPrefix(pkg.ImportPath, modulePath+"/") {
			deps = append(deps, pkg.ImportPath)
		}
	}

	return deps
}

func assertNoForbiddenDeps(t *testing.T, deps []string, forbidden func(string) bool) {
	t.Helper()

	violations := make([]string, 0)
	for _, dep := range deps {
		if forbidden(dep) {
			violations = append(violations, dep)
		}
	}

	if len(violations) == 0 {
		return
	}

	slices.Sort(violations)
	violations = slices.Compact(violations)
	t.Fatalf("forbidden dependencies detected: %s", strings.Join(violations, ", "))
}

func assertNoForbiddenDirectImportsInFirstPartyDeps(t *testing.T, pkgs []listedPackage, modulePath string, forbidden func(string) bool) {
	t.Helper()

	violations := make([]string, 0)
	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.ImportPath, modulePath+"/") {
			continue
		}

		for _, importedPkg := range pkg.Imports {
			if forbidden(importedPkg) {
				violations = append(violations, pkg.ImportPath+" -> "+importedPkg)
			}
		}
	}

	if len(violations) == 0 {
		return
	}

	slices.Sort(violations)
	violations = slices.Compact(violations)
	t.Fatalf("forbidden direct imports detected: %s", strings.Join(violations, ", "))
}

func TestRun_NonInteractiveFlagsDoNotStartBubbleTea(t *testing.T) {
	t.Parallel()

	resolved := config.Default()
	tests := []struct {
		name         string
		args         []string
		expectLoad   bool
		expectLogger bool
		expectStderr bool
	}{
		{name: "help", args: []string{"--help"}, expectLoad: false},
		{name: "version", args: []string{"--version"}, expectLoad: false},
		{name: "print config", args: []string{"--print-config"}, expectLoad: true, expectLogger: true},
		{name: "check config", args: []string{"--check-config"}, expectLoad: true, expectLogger: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			loadCalls := 0
			loggerCalls := 0
			started := false

			code := runWithLogger(tc.args, &stdout, &stderr,
				func(opts config.LoadOptions) (config.Result, error) {
					loadCalls++
					return config.Result{Config: resolved, Path: "/tmp/config.yaml"}, nil
				},
				func(cfg config.Model, opts startupOptions) error {
					started = true
					return nil
				},
				func(opts logging.Options) *logging.Manager {
					loggerCalls++
					return nil
				},
			)

			if code != 0 {
				t.Fatalf("expected exit code 0, got %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
			}
			if started {
				t.Fatal("expected Bubble Tea startup to be skipped")
			}
			if tc.expectLoad && loadCalls != 1 {
				t.Fatalf("expected config load once, got %d", loadCalls)
			}
			if !tc.expectLoad && loadCalls != 0 {
				t.Fatalf("expected config load to be skipped, got %d calls", loadCalls)
			}
			expectedLoggerCalls := 0
			if tc.expectLogger {
				expectedLoggerCalls = 1
			}
			if loggerCalls != expectedLoggerCalls {
				t.Fatalf("expected logger construction calls %d, got %d", expectedLoggerCalls, loggerCalls)
			}
			if !tc.expectStderr && stderr.Len() != 0 {
				t.Fatalf("expected empty stderr, got %q", stderr.String())
			}
		})
	}
}

func TestRun_PrintConfigWritesResolvedYAMLAndSource(t *testing.T) {
	t.Parallel()

	resolved := config.Default()
	configPath := filepath.Join(t.TempDir(), "custom.yaml")

	var stdout, stderr bytes.Buffer
	var seenOpts config.LoadOptions

	code := runWithLogger([]string{"--print-config", "--config", configPath}, &stdout, &stderr,
		func(opts config.LoadOptions) (config.Result, error) {
			seenOpts = opts
			return config.Result{Config: resolved, Path: configPath, Warnings: []string{"unused"}}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			return errors.New("should not start")
		},
		func(opts logging.Options) *logging.Manager {
			opts.StateDir = t.TempDir()
			return logging.New(opts)
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stderr.String(), "warn: config warning") || !strings.Contains(stderr.String(), "warning=unused") {
		t.Fatalf("expected warning mirrored to stderr, got %q", stderr.String())
	}
	if !seenOpts.RequireExplicit || seenOpts.Path != configPath {
		t.Fatalf("expected explicit path load options, got %#v", seenOpts)
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "# source: "+configPath+"\n") {
		t.Fatalf("expected source comment prefix, got %q", out)
	}

	yamlBody := strings.TrimPrefix(out, "# source: "+configPath+"\n")
	var decoded config.Model
	if err := yaml.Unmarshal([]byte(yamlBody), &decoded); err != nil {
		t.Fatalf("expected valid YAML body, got error: %v", err)
	}
	if decoded.Editor.Command != resolved.Editor.Command {
		t.Fatalf("expected resolved config in yaml body, got editor=%q", decoded.Editor.Command)
	}
}

func TestRun_CheckConfigPrintsWarningsAndSuccess(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runWithLogger([]string{"--check-config", "--debug"}, &stdout, &stderr,
		func(opts config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Default(), Path: "/tmp/config.yaml", Warnings: []string{"unknown key"}}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			return errors.New("should not start")
		},
		func(opts logging.Options) *logging.Manager {
			opts.StateDir = t.TempDir()
			return logging.New(opts)
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "config OK") {
		t.Fatalf("expected success message, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] session_id=") {
		t.Fatalf("expected debug session line, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] resolved config path") || !strings.Contains(stderr.String(), "path=/tmp/config.yaml") {
		t.Fatalf("expected config path debug output, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] resolved cwd") {
		t.Fatalf("expected cwd debug output, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "warn: config warning") || !strings.Contains(stderr.String(), "warning=unknown key") {
		t.Fatalf("expected warning output, got %q", stderr.String())
	}
}

func TestRun_NonInteractiveDebugCreatesPersistentStartupLogs(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := runWithLogger([]string{"--print-config", "--debug"}, &stdout, &stderr,
		func(opts config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Default(), Path: "/tmp/config.yaml"}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			return errors.New("should not start")
		},
		func(opts logging.Options) *logging.Manager {
			opts.StateDir = stateDir
			return logging.New(opts)
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "# source: /tmp/config.yaml\n") {
		t.Fatalf("expected print-config output, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] session_id=") || !strings.Contains(stderr.String(), "[taskmgr-ui-debug] auto-refresh") || !strings.Contains(stderr.String(), "enabled=true") {
		t.Fatalf("expected startup debug diagnostics, got %q", stderr.String())
	}

	// Log file is now per-process: taskmgr-ui-<session_id>.log. Use a glob to find it.
	matches, err := filepath.Glob(filepath.Join(stateDir, "taskmgr-ui", "taskmgr-ui-*.log"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one taskmgr-ui-*.log file in %s", filepath.Join(stateDir, "taskmgr-ui"))
	}
	var logText string
	for _, logPath := range matches {
		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ReadFile %q returned error: %v", logPath, err)
		}
		logText += string(content)
	}
	for _, want := range []string{"\"message\":\"resolved config path\"", "\"message\":\"resolved cwd\"", "\"message\":\"auto-refresh\""} {
		if !strings.Contains(logText, want) {
			t.Fatalf("expected persistent log to contain %q, got %q", want, logText)
		}
	}
}

func TestRun_VersionUsesFallback(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr,
		func(opts config.LoadOptions) (config.Result, error) {
			return config.Result{}, errors.New("should not load")
		},
		func(cfg config.Model, opts startupOptions) error { return nil },
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	got := stdout.String()
	if !strings.Contains(got, appversion.Version) {
		t.Fatalf("expected version output to contain %q, got %q", appversion.Version, got)
	}
	if !strings.HasPrefix(got, "taskmgr-ui ") {
		t.Fatalf("expected version output to start with %q, got %q", "taskmgr-ui ", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRun_UsageErrorsExitCode2(t *testing.T) {
	t.Parallel()

	tests := [][]string{
		{"--does-not-exist"},
		{"--config"},
	}

	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr,
			func(opts config.LoadOptions) (config.Result, error) {
				return config.Result{}, errors.New("should not load")
			},
			func(cfg config.Model, opts startupOptions) error { return nil },
		)

		if code != 2 {
			t.Fatalf("args %v: expected exit code 2, got %d", args, code)
		}
		if stderr.Len() == 0 {
			t.Fatalf("args %v: expected usage error on stderr", args)
		}
	}
}

func TestRun_CWDAndConfigResolutionAndStartOptions(t *testing.T) {
	startDir := t.TempDir()
	projectDir := filepath.Join(startDir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	// On macOS, t.TempDir() returns a path under /var/folders which is a symlink
	// to /private/var/folders.  os.Getwd() (called inside run()) returns the
	// canonical form, so we canonicalize here so all subsequent comparisons use
	// the same representation (beads-workbench-2rfx).
	if canonical, err := filepath.EvalSymlinks(startDir); err == nil {
		startDir = canonical
	}
	if canonical, err := filepath.EvalSymlinks(projectDir); err == nil {
		projectDir = canonical
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	if err := os.Chdir(startDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	var stdout, stderr bytes.Buffer
	var seenLoad config.LoadOptions
	var seenStart startupOptions
	var seenLoggerOpts logging.Options
	loggerCalls := 0
	started := false

	code := runWithLogger([]string{"--cwd", "project", "--config", "cfg.yaml", "--debug", "--no-auto-refresh"}, &stdout, &stderr,
		func(opts config.LoadOptions) (config.Result, error) {
			seenLoad = opts
			return config.Result{Config: config.Default(), Path: opts.Path}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			started = true
			seenStart = opts
			return nil
		},
		func(opts logging.Options) *logging.Manager {
			loggerCalls++
			seenLoggerOpts = opts
			opts.StateDir = t.TempDir()
			return logging.New(opts)
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, stderr.String())
	}
	if !started {
		t.Fatal("expected interactive startup")
	}
	if seenLoad.Path != filepath.Join(startDir, "cfg.yaml") {
		t.Fatalf("expected config path resolved against start cwd, got %q", seenLoad.Path)
	}
	if seenStart.projectRoot != projectDir {
		t.Fatalf("expected project root from --cwd, got %q", seenStart.projectRoot)
	}
	if !seenStart.debug {
		t.Fatal("expected debug option enabled")
	}
	if seenStart.autoRefresh {
		t.Fatal("expected --no-auto-refresh to disable auto refresh")
	}
	if seenStart.logManager == nil {
		t.Fatal("expected log manager to be passed into startup options")
	}
	if loggerCalls != 1 {
		t.Fatalf("expected logger to be constructed once, got %d", loggerCalls)
	}
	if seenLoggerOpts.ProjectRoot != projectDir {
		t.Fatalf("expected logger project root %q, got %q", projectDir, seenLoggerOpts.ProjectRoot)
	}
	if seenLoggerOpts.BuildVersion != appversion.Version {
		t.Fatalf("expected logger build version %q, got %q", appversion.Version, seenLoggerOpts.BuildVersion)
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] session_id=") {
		t.Fatalf("expected debug session line, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] resolved config path") || !strings.Contains(stderr.String(), "path="+filepath.Join(startDir, "cfg.yaml")) {
		t.Fatalf("expected debug config path line, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] resolved cwd") || !strings.Contains(stderr.String(), "cwd="+projectDir) {
		t.Fatalf("expected debug cwd line, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "project_root="+projectDir) || !strings.Contains(stderr.String(), "build_version="+appversion.Version) {
		t.Fatalf("expected provenance in debug output, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[taskmgr-ui-debug] auto-refresh") || !strings.Contains(stderr.String(), "enabled=false") {
		t.Fatalf("expected debug auto-refresh line, got %q", stderr.String())
	}
}

func TestRun_CWDMustExistAndBeDirectory(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		started := false
		code := run([]string{"--cwd", filepath.Join(t.TempDir(), "missing")}, &stdout, &stderr,
			func(opts config.LoadOptions) (config.Result, error) {
				return config.Result{Config: config.Default(), Path: "unused"}, nil
			},
			func(cfg config.Model, opts startupOptions) error {
				started = true
				return nil
			},
		)
		if code == 0 {
			t.Fatalf("expected non-zero exit for missing cwd")
		}
		if started {
			t.Fatal("expected startup skipped for invalid cwd")
		}
	})

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
		var stdout, stderr bytes.Buffer
		started := false
		code := run([]string{"--cwd", filePath}, &stdout, &stderr,
			func(opts config.LoadOptions) (config.Result, error) {
				return config.Result{Config: config.Default(), Path: "unused"}, nil
			},
			func(cfg config.Model, opts startupOptions) error {
				started = true
				return nil
			},
		)
		if code == 0 {
			t.Fatalf("expected non-zero exit for non-directory cwd")
		}
		if started {
			t.Fatal("expected startup skipped for invalid cwd")
		}
	})
}
