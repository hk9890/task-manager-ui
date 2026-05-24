package bd

import (
	"context"
	"os/exec"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// TestVCStatusHashSuccess verifies that a well-formed `bd vc status --json`
// response returns the commit hash without error.
func TestVCStatusHashSuccess(t *testing.T) {
	t.Parallel()

	stub := &stubExecutor{
		result: ExecResult{Stdout: []byte(`{"commit":"grgt545i8h0r5ulqn1pqhjl1je6m0udn","branch":"main","schema_version":1}`)},
	}
	runner := NewCommandRunner(RunnerConfig{Executor: stub})

	got, err := VCStatusHash(context.Background(), runner)
	if err != nil {
		t.Fatalf("VCStatusHash returned unexpected error: %v", err)
	}
	if got != "grgt545i8h0r5ulqn1pqhjl1je6m0udn" {
		t.Fatalf("unexpected hash: got %q, want %q", got, "grgt545i8h0r5ulqn1pqhjl1je6m0udn")
	}

	// Verify the correct argv was sent to the executor.
	if len(stub.args) < 2 || stub.args[0] != "vc" || stub.args[1] != "status" {
		t.Fatalf("unexpected args sent to executor: %v", stub.args)
	}
}

// TestVCStatusHashExecError verifies that an execution-level failure
// (e.g. bd not on PATH) returns a wrapped RepositoryError and does not panic.
func TestVCStatusHashExecError(t *testing.T) {
	t.Parallel()

	stub := &stubExecutor{err: exec.ErrNotFound}
	runner := NewCommandRunner(RunnerConfig{Executor: stub})

	_, err := VCStatusHash(context.Background(), runner)
	assertRepositoryErrorCode(t, err, domain.ErrorCodeCommandUnavailable)
}

// TestVCStatusHashEmptyCommit verifies that a response with an empty commit
// field returns a parse error and does not panic. This covers both the
// {"commit":""} case and the {"branch":"main"} (missing commit field) case
// since Go decodes the missing field as the zero value "".
func TestVCStatusHashEmptyCommit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		stdout string
	}{
		{
			name:   "empty commit field",
			stdout: `{"commit":"","branch":"main"}`,
		},
		{
			name:   "missing commit field",
			stdout: `{"branch":"main","schema_version":1}`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubExecutor{
				result: ExecResult{Stdout: []byte(tc.stdout)},
			}
			runner := NewCommandRunner(RunnerConfig{Executor: stub})

			_, err := VCStatusHash(context.Background(), runner)
			assertRepositoryErrorCode(t, err, domain.ErrorCodeDecodeFailed)
		})
	}
}
