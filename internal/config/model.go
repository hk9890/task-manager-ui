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
			// The issue fields travel through Env, never through an argument.
			// nvim re-parses a +cmd argument as an Ex command line, so an
			// interpolated field there is executable text (CODING.md's
			// shell-launcher security rule). Inside the Ex commands the fields
			// are read back as $VAR references, which evaluate to data and are
			// never re-parsed; the buffer name goes through nvim_buf_set_name
			// rather than :execute for the same reason.
			Action:  "nvim",
			Command: "nvim",
			Args: []string{
				`+call append(0, ["Issue: " . $TASKMGR_UI_ISSUE_ID, "Title: " . $TASKMGR_UI_ISSUE_TITLE, "Assignee: " . $TASKMGR_UI_ISSUE_ASSIGNEE, "Labels: " . $TASKMGR_UI_ISSUE_LABELS])`,
				`+call nvim_buf_set_name(0, "[Issue " . $TASKMGR_UI_ISSUE_ID . "]")`,
				"+setlocal nomodifiable",
				"+normal! gg",
			},
			Env: []string{
				"TASKMGR_UI_ISSUE_ID={{issue.id}}",
				"TASKMGR_UI_ISSUE_TITLE={{issue.title}}",
				"TASKMGR_UI_ISSUE_ASSIGNEE={{issue.assignee}}",
				"TASKMGR_UI_ISSUE_LABELS={{issue.labels}}",
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
				"TASKMGR_UI_ISSUE_ID={{issue.id}}",
				"TASKMGR_UI_ISSUE_TITLE={{issue.title}}",
				"TASKMGR_UI_ISSUE_ASSIGNEE={{issue.assignee}}",
				"TASKMGR_UI_ISSUE_LABELS={{issue.labels}}",
				"TASKMGR_UI_PROJECT_ROOT={{project.root}}",
			},
			WorkDir: "{{project.root}}",
		},
		{
			Action:  "shell-command",
			Command: "sh",
			Args: []string{
				"-lc",
				"printf 'issue=%s\\ntitle=%s\\nassignee=%s\\nlabels=%s\\n' \"$0\" \"$1\" \"$2\" \"$3\"",
				"{{issue.id}}",
				"{{issue.title}}",
				"{{issue.assignee}}",
				"{{issue.labels}}",
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
