package launcher

import (
	"context"
	"os"
	"os/exec"
)

type execProcessRunner struct{}

// NewExecProcessRunner returns the default subprocess launcher.
func NewExecProcessRunner() ProcessRunner {
	return execProcessRunner{}
}

// Run starts an external process and returns immediately.
func (execProcessRunner) Run(ctx context.Context, command string, args []string, dir string, env []string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	return cmd.Start()
}
