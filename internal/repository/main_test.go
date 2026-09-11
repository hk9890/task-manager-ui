package repository_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the SDK's per-user config at a scratch home for the whole
// package, for the reason given on the same hook in
// internal/repository/taskmgr: the conformance suite opens real stores under
// t.TempDir(), and the SDK applies the developer's own ~/.taskmgr config to
// each one.
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
