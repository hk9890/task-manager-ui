package editor

import (
	"strings"
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

// TestSplitEditorCommandQuoting covers the quote-aware split cases required by
// the editor.command configuration surface.
func TestSplitEditorCommandQuoting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantCmd     string
		wantArgs    []string
		wantErrFrag string // non-empty means an error is expected containing this fragment
	}{
		{
			name:     "simple no quotes",
			input:    "code --wait",
			wantCmd:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "double-quoted arg with space",
			input:    `code --wait "with space"`,
			wantCmd:  "code",
			wantArgs: []string{"--wait", "with space"},
		},
		{
			name:     "single-quoted arg with space",
			input:    "code --wait 'single quoted'",
			wantCmd:  "code",
			wantArgs: []string{"--wait", "single quoted"},
		},
		{
			name:     "double-quoted arg embedded in token",
			input:    `code --flag="val ue"`,
			wantCmd:  "code",
			wantArgs: []string{"--flag=val ue"},
		},
		{
			name:     "single-quoted arg embedded in token",
			input:    "code --flag='val ue'",
			wantCmd:  "code",
			wantArgs: []string{"--flag=val ue"},
		},
		{
			name:     "multiple spaces between tokens",
			input:    "code   --wait",
			wantCmd:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  code --wait  ",
			wantCmd:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "command only no args",
			input:    "vi",
			wantCmd:  "vi",
			wantArgs: nil,
		},
		{
			name:        "unclosed double quote",
			input:       `code --wait "unclosed`,
			wantErrFrag: "unclosed double quote",
		},
		{
			name:        "unclosed single quote",
			input:       "code --wait 'unclosed",
			wantErrFrag: "unclosed single quote",
		},
		{
			name:     "empty double-quoted arg",
			input:    `code ""`,
			wantCmd:  "code",
			wantArgs: []string{""},
		},
		{
			name:     "empty single-quoted arg",
			input:    "code ''",
			wantCmd:  "code",
			wantArgs: []string{""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd, args, err := splitEditorCommand(tc.input)

			if tc.wantErrFrag != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrFrag)
				}
				if !strings.Contains(err.Error(), tc.wantErrFrag) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErrFrag, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != tc.wantCmd {
				t.Fatalf("command: want %q, got %q", tc.wantCmd, cmd)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args length: want %d, got %d (%#v)", len(tc.wantArgs), len(args), args)
			}
			for i, want := range tc.wantArgs {
				if args[i] != want {
					t.Fatalf("args[%d]: want %q, got %q", i, want, args[i])
				}
			}
		})
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
