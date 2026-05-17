//go:build !windows

package embeddedfixture

import (
	"os"
	"syscall"
)

func acquireFileLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func releaseFileLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
