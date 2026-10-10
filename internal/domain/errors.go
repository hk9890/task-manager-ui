package domain

import "fmt"

// ErrorCode identifies normalized repository error categories for UI handling.
type ErrorCode string

const (
	ErrorCodeNoDatabaseFound  ErrorCode = "no_database_found"
	ErrorCodeCommandFailed    ErrorCode = "command_failed"
	ErrorCodeDecodeFailed     ErrorCode = "decode_failed"
	ErrorCodeValidationFailed ErrorCode = "validation_failed"
	ErrorCodeNotFound         ErrorCode = "not_found"
	ErrorCodeUnauthorized     ErrorCode = "unauthorized"
	ErrorCodeTimeout          ErrorCode = "timeout"
	ErrorCodeConflict         ErrorCode = "conflict"
	ErrorCodeHookDenied       ErrorCode = "hook_denied"
	ErrorCodeUnknown          ErrorCode = "unknown"
)

// RepositoryError is a normalized source operation error for TUI presentation.
type RepositoryError struct {
	Code      ErrorCode
	Operation string
	Message   string
	Cause     error
}

// Error names the failure one time. Message is what the operator reads, so it
// stands alone; the cause's text is appended only when there is no message to
// say why. Unwrap returns the cause either way.
func (e RepositoryError) Error() string {
	if e.Message != "" {
		if e.Operation == "" {
			return e.Message
		}
		return fmt.Sprintf("%s: %s", e.Operation, e.Message)
	}

	base := string(e.Code)
	if e.Operation != "" {
		base = fmt.Sprintf("%s: %s", e.Operation, e.Code)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s", base, e.Cause.Error())
	}
	return base
}

func (e RepositoryError) Unwrap() error {
	return e.Cause
}
