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

	expectedPath := filepath.Join(configHome, configRelativePath)
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
	path := filepath.Join(configHome, configRelativePath)
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

func writeConfig(t *testing.T, configHome, body string) string {
	t.Helper()

	path := filepath.Join(configHome, configRelativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}
