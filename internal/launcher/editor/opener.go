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

func splitEditorCommand(raw string) (string, []string, error) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("editor command is empty")
	}

	return parts[0], parts[1:], nil
}
