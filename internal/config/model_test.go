package config

import (
	"strings"
	"testing"
)

func TestDefault_UsesEditorEnvForEditorAndLauncher(t *testing.T) {
	t.Setenv("EDITOR", "nvim")

	cfg := Default()

	if cfg.Editor.Command != "nvim" {
		t.Fatalf("expected editor command from EDITOR, got %q", cfg.Editor.Command)
	}

	if len(cfg.Launcher.Definitions) != 4 {
		t.Fatalf("expected four default launcher definitions, got %d", len(cfg.Launcher.Definitions))
	}

	launcher := cfg.Launcher.Definitions[0]
	if launcher.Action != "editor" {
		t.Fatalf("expected launcher action editor, got %q", launcher.Action)
	}
	if launcher.Command != "nvim" {
		t.Fatalf("expected launcher command from EDITOR, got %q", launcher.Command)
	}
	if launcher.Args != nil {
		t.Fatalf("expected nil default launcher args, got %#v", launcher.Args)
	}

	if cfg.Launcher.Definitions[1].Action != "nvim" {
		t.Fatalf("expected second launcher action nvim, got %q", cfg.Launcher.Definitions[1].Action)
	}
	if cfg.Launcher.Definitions[2].Action != "opencode" {
		t.Fatalf("expected third launcher action opencode, got %q", cfg.Launcher.Definitions[2].Action)
	}
	if cfg.Launcher.Definitions[3].Action != "shell-command" {
		t.Fatalf("expected fourth launcher action shell-command, got %q", cfg.Launcher.Definitions[3].Action)
	}
}

func TestDefault_FallsBackToViWhenEditorEnvMissing(t *testing.T) {
	t.Setenv("EDITOR", "")

	cfg := Default()

	if cfg.Editor.Command != "vi" {
		t.Fatalf("expected vi fallback editor, got %q", cfg.Editor.Command)
	}
	if cfg.Launcher.Definitions[0].Command != "vi" {
		t.Fatalf("expected launcher to use vi fallback, got %q", cfg.Launcher.Definitions[0].Command)
	}
}

func TestDefault_UIPreferences(t *testing.T) {
	cfg := Default()

	if !cfg.UI.ShowModeSwitcherHelp {
		t.Fatal("expected mode switcher help to be enabled by default")
	}
}

func TestCloneStringSlice(t *testing.T) {
	values := []string{"a", "b"}
	cloned := cloneStringSlice(values)
	if len(cloned) != 2 || cloned[0] != "a" || cloned[1] != "b" {
		t.Fatalf("unexpected cloned values: %#v", cloned)
	}
	cloned[0] = "changed"
	if values[0] != "a" {
		t.Fatalf("expected original slice to remain unchanged, got %#v", values)
	}

	if cloneStringSlice(nil) != nil {
		t.Fatal("expected nil clone for nil input")
	}
}

// TestDefault_NvimLauncherPassesIssueFieldsThroughEnv pins the shipped nvim
// definition against the shell-launcher security rule. nvim re-parses a "+cmd"
// argument as an Ex command line, so an issue placeholder in one is executable
// text; the fields must travel through Env and be read back as $VAR.
func TestDefault_NvimLauncherPassesIssueFieldsThroughEnv(t *testing.T) {
	cfg := Default()

	var nvim LauncherDefinition
	for _, def := range cfg.Launcher.Definitions {
		if def.Action == "nvim" {
			nvim = def
			break
		}
	}
	if nvim.Action == "" {
		t.Fatal("expected a built-in nvim launcher definition")
	}

	for _, arg := range nvim.Args {
		for _, placeholder := range []string{"{{issue.id}}", "{{issue.title}}", "{{issue.assignee}}", "{{issue.labels}}"} {
			if strings.Contains(arg, placeholder) {
				t.Errorf("nvim arg %q must not interpolate %s — nvim re-parses it as an Ex command line", arg, placeholder)
			}
		}
	}

	wantEnv := []string{
		"TASKMGR_UI_ISSUE_ID={{issue.id}}",
		"TASKMGR_UI_ISSUE_TITLE={{issue.title}}",
		"TASKMGR_UI_ISSUE_ASSIGNEE={{issue.assignee}}",
		"TASKMGR_UI_ISSUE_LABELS={{issue.labels}}",
	}
	for _, want := range wantEnv {
		found := false
		for _, got := range nvim.Env {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("nvim definition must carry env entry %q, got %#v", want, nvim.Env)
		}
	}
}
