//go:build !windows

package launcher

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr configures the subprocess to start in a new session (setsid),
// detaching it from BWB's process group. This prevents SIGHUP/SIGINT delivered
// to BWB's process group from propagating to launched tools.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
