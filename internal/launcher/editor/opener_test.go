package editor

import (
	"testing"
)

func TestSplitEditorCommand(t *testing.T) {
	t.Parallel()

	command, args, err := splitEditorCommand("nvim -f")
	if err != nil {
		t.Fatalf("splitEditorCommand returned error: %v", err)
	}

	if command != "nvim" {
		t.Fatalf("expected command nvim, got %q", command)
	}

	if len(args) != 1 || args[0] != "-f" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestSplitEditorCommandRejectsEmpty(t *testing.T) {
	t.Parallel()

	_, _, err := splitEditorCommand("  ")
	if err == nil {
		t.Fatalf("expected error for empty command")
	}
}

func TestEditorCommandUsesExplicitCommandFirst(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	opener := ProcessOpener{EditorCommand: "nvim -f"}

	if got := opener.editorCommand(); got != "nvim -f" {
		t.Fatalf("expected explicit editor command, got %q", got)
	}
}

func TestEditorCommandUsesEnvWhenExplicitEmpty(t *testing.T) {
	t.Setenv("EDITOR", "emacs")
	opener := ProcessOpener{EditorCommand: "   "}

	if got := opener.editorCommand(); got != "emacs" {
		t.Fatalf("expected $EDITOR fallback, got %q", got)
	}
}

func TestEditorCommandFallsBackToViWhenUnset(t *testing.T) {
	t.Setenv("EDITOR", "")
	opener := ProcessOpener{}

	if got := opener.editorCommand(); got != defaultEditorCommand {
		t.Fatalf("expected default editor %q, got %q", defaultEditorCommand, got)
	}
}
