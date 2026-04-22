package config

import "os"

const (
	defaultEditorCommand = "vi"
)

// Model contains runtime configuration consumed by the app shell.
type Model struct {
	Editor      Editor      `yaml:"editor"`
	Launcher    Launcher    `yaml:"launcher"`
	KeyBindings KeyBindings `yaml:"keybindings"`
	UI          UI          `yaml:"ui"`
}

// Editor contains editor-launch configuration.
type Editor struct {
	Command string `yaml:"command"`
}

// Launcher contains launcher action definitions used by the shell.
type Launcher struct {
	Definitions []LauncherDefinition `yaml:"definitions"`
}

// LauncherDefinition describes one launcher action and its command argv.
type LauncherDefinition struct {
	Action  string   `yaml:"action"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
	Env     []string `yaml:"env,omitempty"`
	WorkDir string   `yaml:"workdir,omitempty"`
}

// UI contains shell-level presentation preferences.
type UI struct {
	ShowModeSwitcherHelp bool `yaml:"show_mode_switcher_help"`
}

func resolvedDefaultEditorCommand() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditorCommand
	}
	return editor
}

func defaultLauncherDefinitions(editor string) []LauncherDefinition {
	return []LauncherDefinition{
		{
			Action:  "editor",
			Command: editor,
			Args:    nil,
		},
		{
			Action:  "nvim",
			Command: "nvim",
			Args: []string{
				"+normal! gg",
				"+setlocal nomodifiable",
				"+file [Issue {{issue.id}}]",
				"+call append(0, [\"Issue: {{issue.id}}\", \"Title: {{issue.title}}\", \"Assignee: {{issue.assignee}}\", \"Labels: {{issue.labels}}\"])",
			},
		},
		{
			Action:  "opencode",
			Command: "opencode",
			Args: []string{
				"run",
				"--issue",
				"{{issue.id}}",
				"--title",
				"{{issue.title}}",
				"--assignee",
				"{{issue.assignee}}",
				"--labels",
				"{{issue.labels}}",
			},
			Env: []string{
				"BWB_ISSUE_ID={{issue.id}}",
				"BWB_ISSUE_TITLE={{issue.title}}",
				"BWB_ISSUE_ASSIGNEE={{issue.assignee}}",
				"BWB_ISSUE_LABELS={{issue.labels}}",
				"BWB_PROJECT_ROOT={{project.root}}",
			},
			WorkDir: "{{project.root}}",
		},
		{
			Action:  "shell-command",
			Command: "sh",
			Args: []string{
				"-lc",
				"printf 'issue=%s\\ntitle=%s\\nassignee=%s\\nlabels=%s\\n' \"{{issue.id}}\" \"{{issue.title}}\" \"{{issue.assignee}}\" \"{{issue.labels}}\"",
			},
			WorkDir: "{{project.root}}",
		},
	}
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

// Default returns the baseline runtime configuration.
func Default() Model {
	editor := resolvedDefaultEditorCommand()

	return Model{
		Editor:      Editor{Command: editor},
		Launcher:    Launcher{Definitions: defaultLauncherDefinitions(editor)},
		KeyBindings: DefaultKeyBindings(),
		UI:          UI{ShowModeSwitcherHelp: true},
	}
}
