package contract_test

import (
	"testing"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contract"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// TestFakeGatewayWriteContract wires RunWriteContract against the bare
// FakeBeadsGateway. All write-contract invariants pass because the fake now
// maintains an in-memory write-state store: CreateIssue generates a unique ID
// and stores the issue; UpdateIssue/CloseIssue/AddComment mutate the stored
// entry; ShowIssue reads from the store; CountIssues counts live from the store.
func TestFakeGatewayWriteContract(t *testing.T) {
	contract.RunWriteContract(t, func(t *testing.T) beads.BeadsGateway {
		t.Helper()
		return fakes.NewFakeBeadsGateway()
	})
}
