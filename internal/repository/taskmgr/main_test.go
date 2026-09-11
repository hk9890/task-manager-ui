package taskmgr

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the SDK's per-user config at a scratch home for the whole
// package.
//
// The SDK resolves $TASKMGR_HOME, else <user-home>/.taskmgr, and applies that
// config to every store it opens — including the temp store a test just made.
// A developer whose own config enables a package with a pre-create hook
// therefore failed this package on changes that touched nothing related, while
// GitHub CI stayed green because the runner has no per-user config.
//
// This is TestMain rather than the t.Setenv helper the storecatalog and cmd
// suites use: the env has to be set before any test runs, and t.Setenv cannot
// be combined with the t.Parallel calls in this package. The SDK's own suite
// pins it the same way.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "taskmgr-home")
	if err != nil {
		panic("create scratch TASKMGR_HOME: " + err.Error())
	}
	os.Setenv("TASKMGR_HOME", filepath.Join(home, "taskmgr-home"))
	os.Setenv("TASKMGR_DIR", "")

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
