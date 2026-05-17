//go:build integration

package contract_test

import (
	"os/exec"
	"testing"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contract"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// TestRealGatewayWriteContract wires RunWriteContract against the real bd CLI
// gateway backed by a fresh, empty per-test database created by
// datasets.WritableTempFixture (mktemp -d + bd init). Each sub-test that calls
// t.Parallel() gets the SAME gateway (the factory is called once per
// RunWriteContract invocation), so sub-tests must create their own issues and
// not rely on pre-existing state.
//
// The real-bd integration tests are the ground-truth tier: if they fail, it
// indicates either a contract documentation error in interface.go or a genuine
// gateway implementation bug. File a bug per the 9x70 epic discipline in either
// case.
func TestRealGatewayWriteContract(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping real gateway write contract test")
	}

	contract.RunWriteContract(t, func(t *testing.T) beads.BeadsGateway {
		t.Helper()

		ds := datasets.WritableTempFixture(t)

		runner := beads.NewCommandRunner(beads.RunnerConfig{
			WorkDir: ds.Path,
		})

		return beads.NewCLIGateway(runner)
	})
}
