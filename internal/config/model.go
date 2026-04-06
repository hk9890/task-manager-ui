package config

import "os"

const (
	defaultEditorCommand = "vi"
)

// Model contains runtime configuration consumed by the app shell.
type Model struct {
	Editor   Editor
	Launcher Launcher
	UI       UI
}

// Editor contains editor-launch configuration.
type Editor struct {
	Command string
}

// Launcher contains launcher action definitions used by the shell.
//
// v1 keeps this intentionally small: one built-in editor action that can be
// expanded by later launcher tasks.
type Launcher struct {
	Definitions []LauncherDefinition
}

// LauncherDefinition describes one launcher action and its command argv.
type LauncherDefinition struct {
	Action  string
	Command string
	Args    []string
	Env     []string
	WorkDir string
}

// UI contains shell-level presentation preferences.
type UI struct {
	ShowModeSwitcherHelp bool
}

// Default returns the baseline runtime configuration.
func Default() Model {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditorCommand
	}

	return Model{
		Editor: Editor{Command: editor},
		Launcher: Launcher{Definitions: []LauncherDefinition{
			{
				Action:  "editor",
				Command: editor,
				Args:    nil,
			},
		}},
		UI: UI{ShowModeSwitcherHelp: true},
	}
}
