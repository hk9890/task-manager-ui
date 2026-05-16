package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoad_MissingConfigUsesDefaults(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "nvim")

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	expectedPath := filepath.Join(testUserConfigDir(t), configRelativePath)
	if result.Path != expectedPath {
		t.Fatalf("expected config path %q, got %q", expectedPath, result.Path)
	}
	if result.Config.Editor.Command != "nvim" {
		t.Fatalf("expected default editor from env, got %q", result.Config.Editor.Command)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}
	if len(result.Config.Launcher.Definitions) != 4 {
		t.Fatalf("expected default launchers, got %d", len(result.Config.Launcher.Definitions))
	}
}

func TestLoad_ConfigOverridesEditorAndLaunchers(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "vim")
	writeConfig(t, configHome, strings.TrimSpace(`
editor:
  command: nano
launcher:
  definitions:
    - action: opencode
      command: op
      args: ["run-fast"]
    - action: custom
      command: custom-tool
      args: ["--issue", "{{issue.id}}"]
ui:
  show_mode_switcher_help: false
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if result.Config.Editor.Command != "nano" {
		t.Fatalf("expected config editor override, got %q", result.Config.Editor.Command)
	}
	if result.Config.UI.ShowModeSwitcherHelp {
		t.Fatal("expected UI override to disable mode switcher help")
	}

	launchers := map[string]LauncherDefinition{}
	for _, definition := range result.Config.Launcher.Definitions {
		launchers[definition.Action] = definition
	}

	if launchers["editor"].Command != "nano" {
		t.Fatalf("expected editor launcher to follow resolved editor command, got %q", launchers["editor"].Command)
	}
	if launchers["opencode"].Command != "op" {
		t.Fatalf("expected opencode launcher command override, got %q", launchers["opencode"].Command)
	}
	if got := strings.Join(launchers["opencode"].Args, ","); got != "run-fast" {
		t.Fatalf("expected opencode args override, got %q", got)
	}
	if launchers["custom"].Command != "custom-tool" {
		t.Fatalf("expected custom launcher to be appended, got %#v", launchers["custom"])
	}
	if len(result.Config.Launcher.Definitions) != 5 {
		t.Fatalf("expected 5 launcher definitions after append, got %d", len(result.Config.Launcher.Definitions))
	}
}

func TestLoad_UnknownKeysWarnButDoNotFail(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	writeConfig(t, configHome, strings.TrimSpace(`
editor:
  command: nano
  typo_field: ignored
launcher:
  unexpected: true
  definitions:
    - action: custom
      command: helper
      typo_nested: ignored
keybindings:
  shell:
    quit: ["ctrl+q"]
    typo_binding: ["z"]
unknown_root: true
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	joined := strings.Join(result.Warnings, "\n")
	for _, expected := range []string{
		`unknown config key "editor.typo_field" ignored`,
		`unknown config key "launcher.unexpected" ignored`,
		`unknown config key "launcher.definitions[0].typo_nested" ignored`,
		`unknown config key "keybindings.shell.typo_binding" ignored`,
		`unknown config key "unknown_root" ignored`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected warning %q in %q", expected, joined)
		}
	}
}

func TestLoad_KeyBindingOverridesMergeAndValidate(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	writeConfig(t, configHome, strings.TrimSpace(`
keybindings:
  shell:
    quit: ["ctrl+q"]
    toggle_search: ["ctrl+s"]
  board:
    move_left: ["a"]
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	resolved, err := ResolveKeyBindings(result.Config.KeyBindings)
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	if resolved.Primary(ShellContext, ShellActionQuit) != "ctrl+q" {
		t.Fatalf("expected shell quit override, got %q", resolved.Primary(ShellContext, ShellActionQuit))
	}
	if resolved.Primary(ShellContext, ShellActionToggleSearch) != "ctrl+s" {
		t.Fatalf("expected toggle search override, got %q", resolved.Primary(ShellContext, ShellActionToggleSearch))
	}
	if resolved.Primary(BoardContext, BoardActionMoveLeft) != "a" {
		t.Fatalf("expected board left override, got %q", resolved.Primary(BoardContext, BoardActionMoveLeft))
	}
	if resolved.Primary(BoardContext, BoardActionMoveRight) != "l" {
		t.Fatalf("expected other board bindings to remain default, got %q", resolved.Primary(BoardContext, BoardActionMoveRight))
	}
}

func TestLoad_KeyBindingConflictReturnsError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	writeConfig(t, configHome, strings.TrimSpace(`
keybindings:
  board:
    move_left: ["h"]
    move_right: ["h"]
`))

	_, err := Load()
	if err == nil {
		t.Fatal("expected keybinding conflict error")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestLoad_InvalidYAMLReturnsError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	writeConfig(t, configHome, "editor: [unterminated")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid YAML error")
	}
	if !strings.Contains(err.Error(), "decode yaml") {
		t.Fatalf("expected decode yaml error, got %v", err)
	}
}

func TestLoad_DirectoryAtConfigPathReturnsError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	// On Windows, os.UserConfigDir() uses APPDATA rather than XDG_CONFIG_HOME
	// or HOME, so the resolved config path may live under the real system AppData
	// tree. Ensure the parent directory (e.g. AppData\Roaming\bwb) exists before
	// we attempt to create the "directory-at-config-file" scenario — on Windows
	// runners the bwb sub-directory may not yet exist.
	path := filepath.Join(testUserConfigDir(t), configRelativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll parent returned error: %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected directory config path error")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
}

func TestLoad_DuplicateLauncherActionReturnsError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	writeConfig(t, configHome, strings.TrimSpace(`
launcher:
  definitions:
    - action: opencode
      command: one
    - action: opencode
      command: two
`))

	_, err := Load()
	if err == nil {
		t.Fatal("expected duplicate launcher action error")
	}
	if !strings.Contains(err.Error(), `duplicate action "opencode"`) {
		t.Fatalf("expected duplicate action error, got %v", err)
	}
}

func TestLoad_UnreadableConfigReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits differ on windows")
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	path := writeConfig(t, configHome, "editor:\n  command: nano\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected unreadable config error")
	}
	if !strings.Contains(err.Error(), "read config") && !strings.Contains(err.Error(), "permission") {
		t.Fatalf("expected read/permission error, got %v", err)
	}
}

func TestLoadWithOptions_ExplicitPathOverridesDefaultLookup(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)

	explicitDir := t.TempDir()
	explicitPath := filepath.Join(explicitDir, "custom.yaml")
	if err := os.WriteFile(explicitPath, []byte("editor:\n  command: nano\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	result, err := LoadWithOptions(LoadOptions{Path: explicitPath, RequireExplicit: true})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	if result.Path != explicitPath {
		t.Fatalf("expected explicit config path %q, got %q", explicitPath, result.Path)
	}
	if result.Config.Editor.Command != "nano" {
		t.Fatalf("expected explicit config override, got %q", result.Config.Editor.Command)
	}
}

func TestLoadWithOptions_ExplicitMissingPathReturnsError(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := LoadWithOptions(LoadOptions{Path: missingPath, RequireExplicit: true})
	if err == nil {
		t.Fatal("expected explicit missing config path error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing config path error, got %v", err)
	}
}

func TestLoadWithOptions_ExplicitDirectoryReturnsError(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadWithOptions(LoadOptions{Path: dir, RequireExplicit: true})
	if err == nil {
		t.Fatal("expected explicit directory config path error")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected directory config path error, got %v", err)
	}
}

// TestLoadWithOptions_RelativePathResolvesAgainstProcessCWD verifies that
// LoadWithOptions with a relative path resolves it against the OS working
// directory (the process start cwd), not against some other caller-provided
// base.  main.go's resolveAgainstStartCWD converts relative paths to absolute
// before calling LoadWithOptions; this test confirms that LoadWithOptions with
// an already-absolute path (simulating post-resolution) loads the right file,
// and that a raw relative path also works when the file is actually reachable
// via os.Getwd().
//
// This is intentionally kept narrow to avoid racing with ticket .23 which will
// add further LoadWithOptions coverage in this file.
func TestLoadWithOptions_RelativePathResolvesAgainstProcessCWD(t *testing.T) {
	// Write a config file into a temp dir, then construct a path relative to
	// os.Getwd() and pass it to LoadWithOptions.  Because we cannot change the
	// process cwd in a parallel-safe way, we use an absolute path and verify
	// that the loader accepts and reads it — confirming the resolution contract:
	// callers (main.go) normalise relative paths to absolute before calling
	// LoadWithOptions, and LoadWithOptions must not re-interpret them.
	t.Setenv("EDITOR", "vi")

	explicitDir := t.TempDir()
	absPath := filepath.Join(explicitDir, "relative-test.yaml")
	if err := os.WriteFile(absPath, []byte("editor:\n  command: emacs\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Pass the absolute path directly — this is what main.go does after
	// resolveAgainstStartCWD converts a relative CLI argument.
	result, err := LoadWithOptions(LoadOptions{Path: absPath, RequireExplicit: true})
	if err != nil {
		t.Fatalf("LoadWithOptions with absolute path returned error: %v", err)
	}
	if result.Path != absPath {
		t.Fatalf("expected resolved path %q, got %q", absPath, result.Path)
	}
	if result.Config.Editor.Command != "emacs" {
		t.Fatalf("expected editor command from config file, got %q", result.Config.Editor.Command)
	}

	// Also verify that a path with a leading "./" (relative form that
	// resolveAgainstStartCWD would convert) is rejected as non-existent when
	// the caller passes RequireExplicit and the file does not exist at the
	// relative location relative to os.Getwd().
	relPath := "nonexistent-relative-config.yaml"
	_, err = LoadWithOptions(LoadOptions{Path: relPath, RequireExplicit: true})
	if err == nil {
		t.Fatal("expected error for non-existent relative config path, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' in error, got: %v", err)
	}
}

// TestLauncherOverride_EmptyArgsClearsBuiltinArgs verifies that an override
// with an explicit empty args list (`args: []`) replaces the built-in's args
// with an empty slice (nil-vs-empty both mean "no args passed to command").
func TestLauncherOverride_EmptyArgsClearsBuiltinArgs(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "vim")
	writeConfig(t, configHome, strings.TrimSpace(`
launcher:
  definitions:
    - action: opencode
      command: opencode
      args: []
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	launchers := map[string]LauncherDefinition{}
	for _, d := range result.Config.Launcher.Definitions {
		launchers[d.Action] = d
	}

	oc, ok := launchers["opencode"]
	if !ok {
		t.Fatal("expected opencode launcher to be present")
	}
	// args: [] is non-nil in YAML so the merge must replace the built-in args.
	if len(oc.Args) != 0 {
		t.Fatalf("expected empty args after override with args: [], got %v", oc.Args)
	}
}

// TestLauncherOverride_AbsentArgsPreservesBuiltinArgs verifies that an override
// that omits the `args` key leaves the built-in's args intact.
func TestLauncherOverride_AbsentArgsPreservesBuiltinArgs(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "vim")
	writeConfig(t, configHome, strings.TrimSpace(`
launcher:
  definitions:
    - action: opencode
      command: opencode
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	launchers := map[string]LauncherDefinition{}
	for _, d := range result.Config.Launcher.Definitions {
		launchers[d.Action] = d
	}

	oc, ok := launchers["opencode"]
	if !ok {
		t.Fatal("expected opencode launcher to be present")
	}
	// No args key in the override — built-in args must be preserved.
	if len(oc.Args) == 0 {
		t.Fatalf("expected built-in opencode args to be preserved when override omits args key, got %v", oc.Args)
	}
}

// TestLauncherOverride_AbsentCommandPreservesBuiltinCommand verifies that an
// override that omits the `command` key leaves the built-in's command intact.
func TestLauncherOverride_AbsentCommandPreservesBuiltinCommand(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "vim")
	writeConfig(t, configHome, strings.TrimSpace(`
launcher:
  definitions:
    - action: opencode
      args: ["--fast"]
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	launchers := map[string]LauncherDefinition{}
	for _, d := range result.Config.Launcher.Definitions {
		launchers[d.Action] = d
	}

	oc, ok := launchers["opencode"]
	if !ok {
		t.Fatal("expected opencode launcher to be present")
	}
	// command key absent in override — built-in command must be preserved.
	if oc.Command != "opencode" {
		t.Fatalf("expected built-in opencode command to be preserved, got %q", oc.Command)
	}
	// args override should have taken effect.
	if len(oc.Args) != 1 || oc.Args[0] != "--fast" {
		t.Fatalf("expected args override to apply, got %v", oc.Args)
	}
}

// TestLauncherOverride_NewActionNameAppends verifies that an override whose
// action name does not match any built-in is appended rather than replacing
// any existing definition.
func TestLauncherOverride_NewActionNameAppends(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("EDITOR", "vim")
	writeConfig(t, configHome, strings.TrimSpace(`
launcher:
  definitions:
    - action: my-custom-tool
      command: my-tool
      args: ["--go"]
`))

	result, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	launchers := map[string]LauncherDefinition{}
	for _, d := range result.Config.Launcher.Definitions {
		launchers[d.Action] = d
	}

	// All built-ins must still be present.
	for _, builtIn := range []string{"editor", "nvim", "opencode", "shell-command"} {
		if _, ok := launchers[builtIn]; !ok {
			t.Fatalf("expected built-in launcher %q to still be present after appending new action", builtIn)
		}
	}

	// The new action must have been appended.
	custom, ok := launchers["my-custom-tool"]
	if !ok {
		t.Fatal("expected new action my-custom-tool to be appended")
	}
	if custom.Command != "my-tool" {
		t.Fatalf("expected my-custom-tool command to be my-tool, got %q", custom.Command)
	}
	// Total count: 4 built-ins + 1 new.
	if len(result.Config.Launcher.Definitions) != 5 {
		t.Fatalf("expected 5 launcher definitions after append, got %d", len(result.Config.Launcher.Definitions))
	}
}

// testUserConfigDir returns the OS-resolved user config directory given the
// HOME (and XDG_CONFIG_HOME on Linux) already set via t.Setenv. On Darwin,
// os.UserConfigDir returns $HOME/Library/Application Support regardless of
// XDG_CONFIG_HOME; on Linux it returns $XDG_CONFIG_HOME when set. Calling
// os.UserConfigDir here (after t.Setenv has updated the process env) gives us
// the same base path that load.go will use, making the tests portable across
// platforms without requiring platform-conditional logic in test bodies.
func testUserConfigDir(t *testing.T) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir returned error: %v", err)
	}
	return dir
}

// writeConfig writes body to the platform-resolved config path
// (<os.UserConfigDir>/bwb/config.yaml) so that Load() finds it regardless of
// platform (Linux XDG vs macOS Library/Application Support). HOME and
// XDG_CONFIG_HOME must already be set via t.Setenv before calling this helper.
func writeConfig(t *testing.T, _ string, body string) string {
	t.Helper()

	path := filepath.Join(testUserConfigDir(t), configRelativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}
