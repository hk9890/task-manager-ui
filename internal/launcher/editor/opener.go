package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultEditorCommand = "vi"

// ProcessOpener launches the configured editor command for a file path.
type ProcessOpener struct {
	EditorCommand string
}

var _ Opener = (*ProcessOpener)(nil)

// Open executes the editor command with the provided file path.
func (o ProcessOpener) Open(ctx context.Context, path string) error {
	command, args, err := splitEditorCommand(o.editorCommand())
	if err != nil {
		return err
	}

	args = append(args, path)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open editor: %w", err)
	}

	return nil
}

func (o ProcessOpener) editorCommand() string {
	cmd := strings.TrimSpace(o.EditorCommand)
	if cmd != "" {
		return cmd
	}

	env := strings.TrimSpace(os.Getenv("EDITOR"))
	if env != "" {
		return env
	}

	return defaultEditorCommand
}

// splitEditorCommand parses a shell-like command string into an executable and
// argument list. It supports single-quoted and double-quoted tokens (including
// tokens with embedded spaces) but does not expand escape sequences or variable
// references — it is intentionally minimal for editor command configuration.
//
// Quoting rules:
//   - Single quotes preserve literal content; no escape processing inside them.
//   - Double quotes preserve literal content; no escape processing inside them.
//   - Unquoted whitespace separates tokens.
//
// An unclosed quote returns an error.
func splitEditorCommand(raw string) (string, []string, error) {
	parts, err := shellSplit(strings.TrimSpace(raw))
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("editor command is empty")
	}

	return parts[0], parts[1:], nil
}

// shellSplit splits s into tokens using POSIX-style quoting (single and double
// quotes only; no backslash escaping). It is intentionally small and covers
// the editor.command configuration surface.
func shellSplit(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inToken := false

	i := 0
	for i < len(s) {
		ch := s[i]

		switch ch {
		case ' ', '\t', '\n', '\r':
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
			i++

		case '\'':
			// Single-quoted: consume until closing single quote; no escapes.
			inToken = true
			i++ // skip opening quote
			for i < len(s) && s[i] != '\'' {
				current.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("editor command has unclosed single quote")
			}
			i++ // skip closing quote

		case '"':
			// Double-quoted: consume until closing double quote; no escapes.
			inToken = true
			i++ // skip opening quote
			for i < len(s) && s[i] != '"' {
				current.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("editor command has unclosed double quote")
			}
			i++ // skip closing quote

		default:
			inToken = true
			current.WriteByte(ch)
			i++
		}
	}

	if inToken {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}
