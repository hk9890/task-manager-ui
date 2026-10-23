package config

// Model contains runtime configuration consumed by the app shell.
type Model struct {
	Launcher    Launcher    `yaml:"launcher"`
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
		{
			// Adds an agent tab to the project's open revier workspace without
			// taking the focus, waits until its harness is idle, and prompts it
			// to comment on the issue, which links the agent's session to it.
			// Outside a project revier status resolves to the focused window's
			// project instead of failing, so the resolved path is checked
			// against the project root. revier agent wait fails until the
			// harness in the new tab has started, hence the retry. Its exit
			// status 2 is a timeout: the agent is there and not idle, which
			// no retry changes.
			Action:  "agent",
			Command: "sh",
			Args: []string{
				"-c",
				`status=$(revier status) || exit 1
[ "$(printf '%s\n' "$status" | sed -n 's/^path  *//p')" = "$PWD" ] || exit 1
project=$(printf '%s\n' "$status" | sed -n 's/^project  *//p')
a=$(revier agent new --no-focus -p "$project") || exit 1
i=0
until revier agent wait "$a" --until idle --timeout 60; do
	[ $? -eq 2 ] && exit 1
	i=$((i + 1))
	[ "$i" -ge 30 ] && exit 1
	sleep 2
done
exec revier agent prompt "$a" -- "Run this command, then stop: taskmgr comment add $0 hello"`,
				"{{issue.id}}",
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
	return Model{
		Launcher:    Launcher{Definitions: defaultLauncherDefinitions()},
		KeyBindings: DefaultKeyBindings(),
		UI:          UI{ShowModeSwitcherHelp: true, Theme: "catppuccin-mocha", Glyphs: "unicode"},
	}
}
