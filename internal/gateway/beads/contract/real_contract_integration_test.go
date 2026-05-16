//go:build integration

package contract_test

import (
	"os/exec"
	"testing"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contract"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
)

// TestRealGatewayReadContract wires RunReadContract against the real bd CLI
// gateway backed by the shared embedded fixture snapshot.
func TestRealGatewayReadContract(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping real gateway contract test")
	}

	contract.RunReadContract(t, func(t *testing.T) beads.BeadsGateway {
		t.Helper()

		repoPath := embeddedfixture.SharedFixtureRepoPath(t)

		runner := beads.NewCommandRunner(beads.RunnerConfig{
			WorkDir: repoPath,
		})

		return beads.NewCLIGateway(runner)
	})
}
