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

// Run starts an external process and returns immediately (fire-and-forget).
//
// Design decisions:
//
//  1. exec.Command is used instead of exec.CommandContext so that the parent
//     context being cancelled (e.g. at BWB exit) does NOT send SIGKILL to the
//     launched subprocess. Launched processes must outlive BWB — that is the
//     fire-and-forget contract.
//
//  2. setSysProcAttr(cmd) is called to detach the subprocess from BWB's process
//     group so that signals sent to BWB's process group (SIGHUP, SIGINT) do not
//     propagate to the launched tool. Platform-specific: Linux/macOS use
//     syscall.SysProcAttr{Setsid: true}; Windows does not support Setsid and
//     receives no-op behaviour (see process_runner_windows.go).
//
//  3. A reaper goroutine calls cmd.Wait() after Start succeeds. This claims the
//     exit status from the kernel, preventing the child from becoming a zombie in
//     BWB's process table for the duration of the session.
func (execProcessRunner) Run(_ context.Context, command string, args []string, dir string, env []string) error {
	cmd := exec.Command(command, args...) //nolint:gosec // command comes from operator-controlled config, not user input
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}

	// Reap the child so it does not remain a zombie in the process table.
	go func() { _ = cmd.Wait() }()

	return nil
}
