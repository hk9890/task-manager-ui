package launcher

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

type execProcessRunner struct{}

// NewExecProcessRunner returns the default subprocess launcher.
func NewExecProcessRunner() ProcessRunner {
	return execProcessRunner{}
}

// reaperHook is a test-only hook. When non-nil, the reaper goroutine sends an
// empty struct to this channel after each cmd.Wait() completes. Production code
// never sets or reads this variable; it is nil at all times outside of tests.
// Access is mutex-guarded so parallel tests can swap it without racing the
// reaper goroutines spawned by concurrent Run() calls.
var (
	reaperHookMu sync.Mutex
	reaperHook   chan<- struct{}
)

func getReaperHook() chan<- struct{} {
	reaperHookMu.Lock()
	defer reaperHookMu.Unlock()
	return reaperHook
}

func setReaperHook(h chan<- struct{}) {
	reaperHookMu.Lock()
	defer reaperHookMu.Unlock()
	reaperHook = h
}

// Run starts an external process and returns immediately (fire-and-forget).
//
// Design decisions:
//
//  1. exec.Command is used instead of exec.CommandContext so that the parent
//     context being cancelled (e.g. at taskmgr-ui exit) does NOT send SIGKILL to the
//     launched subprocess. Launched processes must outlive taskmgr-ui — that is the
//     fire-and-forget contract.
//
//  2. setSysProcAttr(cmd) is called to detach the subprocess from taskmgr-ui's process
//     group so that signals sent to taskmgr-ui's process group (SIGHUP, SIGINT) do not
//     propagate to the launched tool. Platform-specific: Linux/macOS use
//     syscall.SysProcAttr{Setsid: true}; Windows does not support Setsid and
//     receives no-op behaviour (see process_runner_windows.go).
//
//  3. A reaper goroutine calls cmd.Wait() after Start succeeds. This claims the
//     exit status from the kernel, preventing the child from becoming a zombie in
//     taskmgr-ui's process table for the duration of the session.
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
	// If reaperHook is set (tests only), signal completion after Wait returns.
	go func() {
		_ = cmd.Wait()
		if h := getReaperHook(); h != nil {
			h <- struct{}{}
		}
	}()

	return nil
}
