//go:build integration

package embeddedfixture

import (
	"os/exec"
	"testing"
)

func TestSeedSkipsWhenToolsUnavailable(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}

	repoPath := TempRepoPath(t)
	Seed(t, repoPath)
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
