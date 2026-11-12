package config

// Model contains runtime configuration consumed by the app shell.
type Model struct {
	Launcher    Launcher    `yaml:"launcher"`
	Agent       Agent       `yaml:"agent"`
	KeyBindings KeyBindings `yaml:"keybindings"`
	UI          UI          `yaml:"ui"`
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

// Agent configures the agents the shell starts on an issue.
type Agent struct {
	// Intents are the entries of the intent menu, in menu order.
	Intents []Intent `yaml:"intents"`
}

// Intent is one thing an agent can be started to do on an issue. Prompt is
// the one line the agent gets first, with the launcher placeholders filled.
type Intent struct {
	Name   string `yaml:"name"`
	Prompt string `yaml:"prompt"`
}

// UI contains shell-level presentation preferences.
type UI struct {
	ShowModeSwitcherHelp bool `yaml:"show_mode_switcher_help"`
	// Theme and Glyphs name the palette and the glyph set every surface draws
	// with. internal/ui/styles owns the valid names.
	Theme  string `yaml:"theme"`
	Glyphs string `yaml:"glyphs"`
}

func defaultLauncherDefinitions() []LauncherDefinition {
	return []LauncherDefinition{
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

func defaultIntents() []Intent {
	return []Intent{
		{Name: "implement", Prompt: "Implement taskmgr issue {{issue.id}}. Read it with taskmgr first."},
		{Name: "review", Prompt: "Review the work done on taskmgr issue {{issue.id}} and report what is missing."},
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
	return Model{
		Launcher:    Launcher{Definitions: defaultLauncherDefinitions()},
		Agent:       Agent{Intents: defaultIntents()},
		KeyBindings: DefaultKeyBindings(),
		UI:          UI{ShowModeSwitcherHelp: true, Theme: "catppuccin-mocha", Glyphs: "unicode"},
	}
}
