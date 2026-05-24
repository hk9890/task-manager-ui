package domain

import (
	"errors"
	"fmt"
	"testing"
)

// sentinel is a package-level test sentinel error used to verify errors.Is
// unwrap propagation through RepositoryError.Unwrap.
var errSentinel = errors.New("sentinel error")

func TestRepositoryErrorErrorWithoutCausePreservesBaseFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  RepositoryError
		want string
	}{
		{
			name: "empty message and operation uses code",
			err:  RepositoryError{Code: ErrorCodeTimeout},
			want: string(ErrorCodeTimeout),
		},
		{
			name: "empty message with operation uses operation and code",
			err:  RepositoryError{Code: ErrorCodeTimeout, Operation: "list issues"},
			want: "list issues: timeout",
		},
		{
			name: "message without operation uses message",
			err:  RepositoryError{Code: ErrorCodeTimeout, Message: "command timed out"},
			want: "command timed out",
		},
		{
			name: "message with operation uses operation and message",
			err:  RepositoryError{Code: ErrorCodeTimeout, Operation: "list issues", Message: "command timed out"},
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

// TestRepositoryErrorUnwrapPropagatesSentinel verifies that errors.Is correctly
// traverses the RepositoryError.Unwrap chain to find a wrapped sentinel. This
// covers the contract that repository callers relying on errors.Is(err, sentinel)
// will find the root cause even after it has been wrapped in a RepositoryError.
func TestRepositoryErrorUnwrapPropagatesSentinel(t *testing.T) {
	t.Parallel()

	wrapped := RepositoryError{
		Code:      ErrorCodeCommandFailed,
		Operation: "test operation",
		Message:   "something failed",
		Cause:     errSentinel,
	}

	if !errors.Is(wrapped, errSentinel) {
		t.Fatalf("errors.Is(wrapped, errSentinel) = false; want true — RepositoryError.Unwrap must propagate the cause sentinel")
	}

	// Also verify a non-matching sentinel is not found.
	otherSentinel := errors.New("other sentinel")
	if errors.Is(wrapped, otherSentinel) {
		t.Fatal("errors.Is(wrapped, otherSentinel) = true; want false — only the wrapped cause must match")
	}
}

// TestRepositoryErrorUnwrapPropagatesThroughMultipleLayers verifies that
// errors.Is traverses a chain of two RepositoryError wrappers.
func TestRepositoryErrorUnwrapPropagatesThroughMultipleLayers(t *testing.T) {
	t.Parallel()

	inner := RepositoryError{
		Code:    ErrorCodeTimeout,
		Message: "inner timeout",
		Cause:   errSentinel,
	}
	outer := RepositoryError{
		Code:    ErrorCodeCommandFailed,
		Message: "outer failure",
		Cause:   inner,
	}

	if !errors.Is(outer, errSentinel) {
		t.Fatalf("errors.Is(outer, errSentinel) = false; want true — errors.Is must traverse two RepositoryError layers")
	}
}

func TestRepositoryErrorErrorWithCauseAppendsCauseForAllBaseFormats(t *testing.T) {
	t.Parallel()

	cause := errors.New("transport reset")

	tests := []struct {
		name string
		err  RepositoryError
	}{
		{
			name: "code only",
			err:  RepositoryError{Code: ErrorCodeCommandFailed, Cause: cause},
		},
		{
			name: "operation plus code",
			err:  RepositoryError{Code: ErrorCodeCommandFailed, Operation: "list issues", Cause: cause},
		},
		{
			name: "message only",
			err:  RepositoryError{Code: ErrorCodeCommandFailed, Message: "failed to execute command", Cause: cause},
		},
		{
			name: "operation plus message",
			err:  RepositoryError{Code: ErrorCodeCommandFailed, Operation: "list issues", Message: "failed to execute command", Cause: cause},
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
