package domain

import "fmt"

// ErrorCode identifies normalized gateway error categories for UI handling.
type ErrorCode string

const (
	ErrorCodeCommandUnavailable ErrorCode = "command_unavailable"
	ErrorCodeCommandFailed      ErrorCode = "command_failed"
	ErrorCodeDecodeFailed       ErrorCode = "decode_failed"
	ErrorCodeValidationFailed   ErrorCode = "validation_failed"
	ErrorCodeNotFound           ErrorCode = "not_found"
	ErrorCodeUnauthorized       ErrorCode = "unauthorized"
	ErrorCodeTimeout            ErrorCode = "timeout"
	ErrorCodeConflict           ErrorCode = "conflict"
	ErrorCodeUnknown            ErrorCode = "unknown"
)

// GatewayError is a normalized source operation error for TUI presentation.
type GatewayError struct {
	Code      ErrorCode
	Operation string
	Message   string
	Cause     error
}

func (e GatewayError) Error() string {
	if e.Message == "" {
		if e.Operation == "" {
			return string(e.Code)
		}

		return fmt.Sprintf("%s: %s", e.Operation, e.Code)
	}

	if e.Operation == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Operation, e.Message)
}

func (e GatewayError) Unwrap() error {
	return e.Cause
}
