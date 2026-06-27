//go:build windows

package launcher

import "os/exec"

// setSysProcAttr is a no-op on Windows: syscall.SysProcAttr does not support
// Setsid on this platform. Launched processes on Windows are not explicitly
// detached from taskmgr-ui's process group; they still outlive taskmgr-ui because exec.Command
// (not exec.CommandContext) is used.
func setSysProcAttr(_ *exec.Cmd) {}
