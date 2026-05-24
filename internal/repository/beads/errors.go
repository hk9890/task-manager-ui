package beads

import "github.com/hk9890/beads-workbench/internal/domain"

// newGatewayError constructs a domain.GatewayError. This helper mirrors the
// one in internal/gateway/beads/runner.go and is needed by the gateway methods
// (read_gateway.go, writes.go, read_payloads.go) that have moved into this
// package while runner.go remains in internal/gateway/beads.
func newGatewayError(code domain.ErrorCode, operation, message string, cause error) error {
	return domain.GatewayError{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}
