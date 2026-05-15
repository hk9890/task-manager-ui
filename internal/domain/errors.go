package domain

import "fmt"

// ErrorCode identifies normalized gateway error categories for UI handling.
type ErrorCode string

const (
	ErrorCodeCommandUnavailable ErrorCode = "command_unavailable"
	ErrorCodeNoDatabaseFound    ErrorCode = "no_database_found"
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
	var base string

	if e.Message == "" {
		if e.Operation == "" {
			base = string(e.Code)
		} else {
			base = fmt.Sprintf("%s: %s", e.Operation, e.Code)
		}
	} else if e.Operation == "" {
		base = e.Message
	} else {
		base = fmt.Sprintf("%s: %s", e.Operation, e.Message)
	}

	if e.Cause != nil {
		return fmt.Sprintf("%s: %s", base, e.Cause.Error())
	}

	return base
}

func (e GatewayError) Unwrap() error {
	return e.Cause
}
