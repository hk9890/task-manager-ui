package config

import (
	"strings"
	"testing"
)

func TestDefault_LauncherDefinitions(t *testing.T) {
	cfg := Default()

	want := []string{"nvim", "opencode", "shell-command"}
	if len(cfg.Launcher.Definitions) != len(want) {
		t.Fatalf("expected %d default launcher definitions, got %d", len(want), len(cfg.Launcher.Definitions))
	}
	for idx, action := range want {
		if got := cfg.Launcher.Definitions[idx].Action; got != action {
			t.Fatalf("launcher definition %d is %q, want %q", idx, got, action)
		}
	}
}

func TestDefault_UIPreferences(t *testing.T) {
	cfg := Default()

	if !cfg.UI.ShowModeSwitcherHelp {
		t.Fatal("expected mode switcher help to be enabled by default")
	}
	if cfg.UI.Theme != "catppuccin-mocha" || cfg.UI.Glyphs != "unicode" {
		t.Fatalf("expected the default theme and glyph set, got theme=%q glyphs=%q", cfg.UI.Theme, cfg.UI.Glyphs)
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
