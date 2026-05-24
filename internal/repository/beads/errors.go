package beads

import "github.com/hk9890/beads-workbench/internal/domain"

// newRepositoryError constructs a domain.RepositoryError. This helper mirrors the
// one in internal/repository/beads/runner.go and is used by Repository methods
// in this package to wrap bd subprocess errors with a consistent shape.
func newRepositoryError(code domain.ErrorCode, operation, message string, cause error) error {
	return domain.RepositoryError{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}
