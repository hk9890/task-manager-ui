package config

import "testing"

func TestDefault_UsesEditorEnvForEditorAndLauncher(t *testing.T) {
	t.Setenv("EDITOR", "nvim")

	cfg := Default()

	if cfg.Editor.Command != "nvim" {
		t.Fatalf("expected editor command from EDITOR, got %q", cfg.Editor.Command)
	}

	if len(cfg.Launcher.Definitions) != 1 {
		t.Fatalf("expected one default launcher definition, got %d", len(cfg.Launcher.Definitions))
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
