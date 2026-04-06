package editor

import "testing"

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
