package beads

import "github.com/hk9890/beads-workbench/internal/domain"

// newGatewayError constructs a domain.GatewayError. This helper mirrors the
// one in internal/gateway/beads/runner.go and is used by Repository methods
// in this package to wrap bd subprocess errors with a consistent shape.
func newGatewayError(code domain.ErrorCode, operation, message string, cause error) error {
	return domain.GatewayError{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}
