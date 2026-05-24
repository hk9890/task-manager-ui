package beads

import (
	"errors"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// assertGatewayErrorCode asserts that err is a domain.GatewayError with the
// given error code. It is the package-internal analogue of the helper by the
// same name in internal/gateway/beads/runner_test.go.
func assertGatewayErrorCode(t *testing.T, err error, expected domain.ErrorCode) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var gatewayErr domain.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected domain.GatewayError, got %T (%v)", err, err)
	}

	if gatewayErr.Code != expected {
		t.Fatalf("unexpected error code: got %q want %q", gatewayErr.Code, expected)
	}
}

// assertContains asserts that got contains wantSubstring.
func assertContains(t *testing.T, got string, wantSubstring string) {
	t.Helper()

	if !strings.Contains(got, wantSubstring) {
		t.Fatalf("expected %q to contain %q", got, wantSubstring)
	}
}
