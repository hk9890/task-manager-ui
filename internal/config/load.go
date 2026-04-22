package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configRelativePath = "bwb/config.yaml"

type LoadOptions struct {
	Path            string
	RequireExplicit bool
}

// Result contains resolved runtime configuration and any non-fatal warnings.
type Result struct {
	Config   Model
	Path     string
	Warnings []string
}

// Load resolves the config path via os.UserConfigDir, reads an optional
// bwb/config.yaml file, merges file-backed values over defaults, and returns
// any non-fatal parse warnings.
func Load() (Result, error) {
	return LoadWithOptions(LoadOptions{})
}

// LoadWithOptions loads runtime config using optional caller-provided path
// overrides and explicit-path requirements.
func LoadWithOptions(opts LoadOptions) (Result, error) {
	path, err := resolveConfigPath(opts.Path)
	if err != nil {
		return Result{}, err
	}

	result := Result{Config: Default(), Path: path}

	data, warnings, err := readConfigFile(path, opts.RequireExplicit)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	if data == nil {
		return result, nil
	}

	override, warnings, err := decodeOverride(data)
	if err != nil {
		return Result{}, fmt.Errorf("load config %q: %w", path, err)
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Config = merge(result.Config, override)
	if err := validateResolved(result.Config); err != nil {
		return Result{}, fmt.Errorf("load config %q: %w", path, err)
	}
	return result, nil
}

func resolveConfigPath(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(configDir, configRelativePath), nil
}

type overrideModel struct {
	Editor      *overrideEditor     `yaml:"editor"`
	Launcher    *overrideLauncher   `yaml:"launcher"`
	KeyBindings *KeyBindingOverride `yaml:"keybindings"`
	UI          *overrideUI         `yaml:"ui"`
}

type overrideEditor struct {
	Command *string `yaml:"command"`
}

type overrideLauncher struct {
	Definitions []overrideLauncherDefinition `yaml:"definitions"`
}

type overrideLauncherDefinition struct {
	Action  string   `yaml:"action"`
	Command *string  `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
	WorkDir *string  `yaml:"workdir"`
}

type overrideUI struct {
	ShowModeSwitcherHelp *bool `yaml:"show_mode_switcher_help"`
}

func readConfigFile(path string, requireExists bool) ([]byte, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if requireExists {
				return nil, nil, fmt.Errorf("config path %q does not exist", path)
			}
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("stat config %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("config path %q is a directory, expected a file", path)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("config path %q is not a regular file", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config %q: %w", path, err)
	}
	return data, nil, nil
}

func decodeOverride(data []byte) (overrideModel, []string, error) {
	var node yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&node); err != nil {
		return overrideModel{}, nil, fmt.Errorf("decode yaml: %w", err)
	}
	if len(node.Content) == 0 {
		return overrideModel{}, nil, nil
	}
	if len(node.Content) > 1 {
		return overrideModel{}, nil, fmt.Errorf("expected a single YAML document")
	}

	warnings := collectUnknownWarnings(node.Content[0], nil)

	var override overrideModel
	if err := node.Content[0].Decode(&override); err != nil {
		return overrideModel{}, warnings, fmt.Errorf("decode config values: %w", err)
	}
	override = sanitizeOverride(override)
	if err := validateOverride(override); err != nil {
		return overrideModel{}, warnings, err
	}
	return override, warnings, nil
}

func sanitizeOverride(override overrideModel) overrideModel {
	if override.KeyBindings == nil {
		return override
	}
	if override.KeyBindings.Shell != nil {
		override.KeyBindings.Shell = filterBindingOverrideMap(override.KeyBindings.Shell, allowedActionsForContext(ShellContext))
	}
	if override.KeyBindings.Board != nil {
		override.KeyBindings.Board = filterBindingOverrideMap(override.KeyBindings.Board, allowedActionsForContext(BoardContext))
	}
	if override.KeyBindings.Search != nil {
		override.KeyBindings.Search = filterBindingOverrideMap(override.KeyBindings.Search, allowedActionsForContext(SearchContext))
	}
	if override.KeyBindings.Detail != nil {
		override.KeyBindings.Detail = filterBindingOverrideMap(override.KeyBindings.Detail, allowedActionsForContext(DetailContext))
	}
	if override.KeyBindings.Modal != nil {
		override.KeyBindings.Modal = filterBindingOverrideMap(override.KeyBindings.Modal, allowedActionsForContext(ModalContext))
	}
	return override
}

func filterBindingOverrideMap(input map[string][]string, allowed map[string]struct{}) map[string][]string {
	filtered := make(map[string][]string, len(input))
	for action, keys := range input {
		if _, ok := allowed[action]; !ok {
			continue
		}
		filtered[action] = cloneStringSlice(keys)
	}
	return filtered
}

func validateOverride(override overrideModel) error {
	if override.Launcher == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(override.Launcher.Definitions))
	for i, definition := range override.Launcher.Definitions {
		action := strings.TrimSpace(definition.Action)
		if action == "" {
			return fmt.Errorf("launcher.definitions[%d].action is required", i)
		}
		if _, exists := seen[action]; exists {
			return fmt.Errorf("launcher.definitions contains duplicate action %q", action)
		}
		seen[action] = struct{}{}
	}

	return nil
}

func validateResolved(cfg Model) error {
	if strings.TrimSpace(cfg.Editor.Command) == "" {
		return fmt.Errorf("editor.command must not be empty")
	}

	seen := make(map[string]struct{}, len(cfg.Launcher.Definitions))
	for i, definition := range cfg.Launcher.Definitions {
		action := strings.TrimSpace(definition.Action)
		if action == "" {
			return fmt.Errorf("launcher.definitions[%d].action is required", i)
		}
		if strings.TrimSpace(definition.Command) == "" {
			return fmt.Errorf("launcher.definitions[%d].command is required for action %q", i, action)
		}
		if _, exists := seen[action]; exists {
			return fmt.Errorf("launcher.definitions contains duplicate action %q", action)
		}
		seen[action] = struct{}{}
	}

	if _, err := ResolveKeyBindings(cfg.KeyBindings); err != nil {
		return err
	}

	return nil
}

func merge(base Model, override overrideModel) Model {
	merged := base

	if override.Editor != nil && override.Editor.Command != nil {
		merged.Editor.Command = *override.Editor.Command
	}
	merged.Launcher.Definitions = syncEditorLauncher(merged.Launcher.Definitions, merged.Editor.Command)

	if override.Launcher != nil && len(override.Launcher.Definitions) > 0 {
		merged.Launcher.Definitions = mergeLauncherDefinitions(merged.Launcher.Definitions, override.Launcher.Definitions)
	}
	merged.KeyBindings = MergeKeyBindings(merged.KeyBindings, override.KeyBindings)

	if override.UI != nil && override.UI.ShowModeSwitcherHelp != nil {
		merged.UI.ShowModeSwitcherHelp = *override.UI.ShowModeSwitcherHelp
	}

	return merged
}

func mergeLauncherDefinitions(base []LauncherDefinition, overrides []overrideLauncherDefinition) []LauncherDefinition {
	merged := make([]LauncherDefinition, 0, len(base)+len(overrides))
	indexByAction := make(map[string]int, len(base)+len(overrides))

	for _, definition := range base {
		copied := LauncherDefinition{
			Action:  definition.Action,
			Command: definition.Command,
			Args:    cloneStringSlice(definition.Args),
			Env:     cloneStringSlice(definition.Env),
			WorkDir: definition.WorkDir,
		}
		indexByAction[copied.Action] = len(merged)
		merged = append(merged, copied)
	}

	for _, override := range overrides {
		action := strings.TrimSpace(override.Action)
		definition := LauncherDefinition{Action: action}
		if idx, exists := indexByAction[action]; exists {
			definition = merged[idx]
		}

		if override.Command != nil {
			definition.Command = *override.Command
		}
		if override.Args != nil {
			definition.Args = cloneStringSlice(override.Args)
		}
		if override.Env != nil {
			definition.Env = cloneStringSlice(override.Env)
		}
		if override.WorkDir != nil {
			definition.WorkDir = *override.WorkDir
		}

		if idx, exists := indexByAction[action]; exists {
			merged[idx] = definition
			continue
		}

		indexByAction[action] = len(merged)
		merged = append(merged, definition)
	}

	return merged
}

func syncEditorLauncher(definitions []LauncherDefinition, editorCommand string) []LauncherDefinition {
	merged := make([]LauncherDefinition, 0, len(definitions))
	for _, definition := range definitions {
		copied := LauncherDefinition{
			Action:  definition.Action,
			Command: definition.Command,
			Args:    cloneStringSlice(definition.Args),
			Env:     cloneStringSlice(definition.Env),
			WorkDir: definition.WorkDir,
		}
		if copied.Action == "editor" {
			copied.Command = editorCommand
		}
		merged = append(merged, copied)
	}
	return merged
}

var allowedMappingKeys = map[string]map[string]struct{}{
	"": {
		"editor":      {},
		"launcher":    {},
		"keybindings": {},
		"ui":          {},
	},
	"editor": {
		"command": {},
	},
	"launcher": {
		"definitions": {},
	},
	"ui": {
		"show_mode_switcher_help": {},
	},
	"keybindings": {
		"shell":  {},
		"board":  {},
		"search": {},
		"detail": {},
		"modal":  {},
	},
	"keybindings.shell":  allowedActionsForContext(ShellContext),
	"keybindings.board":  allowedActionsForContext(BoardContext),
	"keybindings.search": allowedActionsForContext(SearchContext),
	"keybindings.detail": allowedActionsForContext(DetailContext),
	"keybindings.modal":  allowedActionsForContext(ModalContext),
}

var allowedSequenceMappingKeys = map[string]map[string]struct{}{
	"launcher.definitions": {
		"action":  {},
		"command": {},
		"args":    {},
		"env":     {},
		"workdir": {},
	},
}

func collectUnknownWarnings(node *yaml.Node, path []string) []string {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.MappingNode:
		return collectMappingWarnings(node, path)
	case yaml.SequenceNode:
		return collectSequenceWarnings(node, path)
	default:
		return nil
	}
}

func collectMappingWarnings(node *yaml.Node, path []string) []string {
	allowed := allowedMappingKeys[strings.Join(path, ".")]
	var warnings []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		key := keyNode.Value
		if _, ok := allowed[key]; !ok {
			warnings = append(warnings, fmt.Sprintf("unknown config key %q ignored", joinPath(append(path, key))))
			continue
		}
		warnings = append(warnings, collectUnknownWarnings(valueNode, append(path, key))...)
	}
	return warnings
}

func collectSequenceWarnings(node *yaml.Node, path []string) []string {
	joined := strings.Join(path, ".")
	if allowed, ok := allowedSequenceMappingKeys[joined]; ok {
		var warnings []string
		for idx, item := range node.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i+1 < len(item.Content); i += 2 {
				key := item.Content[i].Value
				if _, exists := allowed[key]; !exists {
					warnings = append(warnings, fmt.Sprintf("unknown config key %q ignored", joinPath(append(path, fmt.Sprintf("[%d]", idx), key))))
				}
			}
		}
		return warnings
	}

	var warnings []string
	for _, item := range node.Content {
		warnings = append(warnings, collectUnknownWarnings(item, path)...)
	}
	return warnings
}

func joinPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	joined := path[0]
	for _, part := range path[1:] {
		if strings.HasPrefix(part, "[") {
			joined += part
			continue
		}
		joined += "." + part
	}
	return joined
}
