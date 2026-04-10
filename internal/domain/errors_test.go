package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestGatewayErrorErrorWithoutCausePreservesBaseFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  GatewayError
		want string
	}{
		{
			name: "empty message and operation uses code",
			err:  GatewayError{Code: ErrorCodeTimeout},
			want: string(ErrorCodeTimeout),
		},
		{
			name: "empty message with operation uses operation and code",
			err:  GatewayError{Code: ErrorCodeTimeout, Operation: "list issues"},
			want: "list issues: timeout",
		},
		{
			name: "message without operation uses message",
			err:  GatewayError{Code: ErrorCodeTimeout, Message: "command timed out"},
			want: "command timed out",
		},
		{
			name: "message with operation uses operation and message",
			err:  GatewayError{Code: ErrorCodeTimeout, Operation: "list issues", Message: "command timed out"},
			want: "list issues: command timed out",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("unexpected error string: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestGatewayErrorErrorWithCauseAppendsCauseForAllBaseFormats(t *testing.T) {
	t.Parallel()

	cause := errors.New("transport reset")

	tests := []struct {
		name string
		err  GatewayError
	}{
		{
			name: "code only",
			err:  GatewayError{Code: ErrorCodeCommandFailed, Cause: cause},
		},
		{
			name: "operation plus code",
			err:  GatewayError{Code: ErrorCodeCommandFailed, Operation: "list issues", Cause: cause},
		},
		{
			name: "message only",
			err:  GatewayError{Code: ErrorCodeCommandFailed, Message: "failed to execute command", Cause: cause},
		},
		{
			name: "operation plus message",
			err:  GatewayError{Code: ErrorCodeCommandFailed, Operation: "list issues", Message: "failed to execute command", Cause: cause},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nilCause := tc.err
			nilCause.Cause = nil
			want := fmt.Sprintf("%s: %s", nilCause.Error(), cause.Error())

			if got := tc.err.Error(); got != want {
				t.Fatalf("unexpected error string: got %q want %q", got, want)
			}
		})
	}
}
