package bd

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestRunnerIgnoresRequestWorkDir verifies that a CommandRequest.WorkDir value
// pointing at an arbitrary directory is silently ignored. The executor must
// always receive the repository's bound defaultWorkDir (CODING.md rule #3).
func TestRunnerIgnoresRequestWorkDir(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  "/repository/bound/dir",
		Executor: execStub,
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
		WorkDir:   "/tmp/x",
	})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if execStub.workDir != "/repository/bound/dir" {
		t.Fatalf("executor received workDir %q; want bound /repository/bound/dir (request WorkDir /tmp/x must be ignored)", execStub.workDir)
	}
}

// TestRunnerStripsDisallowedEnvVarsFromParentEnv verifies that env vars not in
// the allowlist (e.g. BD_DB_PATH) are stripped before the executor runs.
func TestRunnerStripsDisallowedEnvVarsFromParentEnv(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}

	// Simulate a "parent env" containing a dangerous BD_DB_PATH override by
	// providing it explicitly as RunnerConfig.Env (the nil-fallback path also
	// filters, but we test the explicit path here for determinism).
	runner := NewCommandRunner(RunnerConfig{
		Env:      []string{"BD_DB_PATH=/etc/passwd", "PATH=/usr/bin"},
		Executor: execStub,
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
	})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	for _, entry := range execStub.env {
		if strings.HasPrefix(entry, "BD_DB_PATH=") {
			t.Fatalf("executor received BD_DB_PATH in env — must be stripped by allowlist; env=%v", execStub.env)
		}
	}

	// PATH must survive because it is in the allowlist.
	foundPATH := false
	for _, entry := range execStub.env {
		if entry == "PATH=/usr/bin" {
			foundPATH = true
			break
		}
	}
	if !foundPATH {
		t.Fatalf("executor did not receive PATH=/usr/bin; env=%v (PATH must pass the allowlist)", execStub.env)
	}
}

// TestRunnerAllowsAllowlistedEnvVars verifies that PATH and HOME survive the
// allowlist filter so bd can locate binaries and the user home directory.
func TestRunnerAllowsAllowlistedEnvVars(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	runner := NewCommandRunner(RunnerConfig{
		Env:      []string{"PATH=/usr/local/bin:/usr/bin", "HOME=/home/user", "BD_DB_PATH=/should/be/stripped"},
		Executor: execStub,
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
	})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	want := map[string]bool{
		"PATH=/usr/local/bin:/usr/bin": false,
		"HOME=/home/user":              false,
	}
	for _, entry := range execStub.env {
		if _, ok := want[entry]; ok {
			want[entry] = true
		}
		if strings.HasPrefix(entry, "BD_DB_PATH=") {
			t.Fatalf("BD_DB_PATH must be stripped; executor env=%v", execStub.env)
		}
	}
	for entry, found := range want {
		if !found {
			t.Fatalf("expected allowlisted env entry %q not received; executor env=%v", entry, execStub.env)
		}
	}
}

// TestRunnerForcesBDNonInteractive verifies that BD_NON_INTERACTIVE=1 is
// always injected into the child env, even when the caller did not provide it
// and even if the caller tried to set it to a different value. Without this,
// repository calls to bd would hang waiting for tty input.
func TestRunnerForcesBDNonInteractive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		env  []string
	}{
		{"no env supplied", nil},
		{"caller did not include BD_NON_INTERACTIVE", []string{"PATH=/usr/bin"}},
		{"caller tries to set BD_NON_INTERACTIVE=0 (stripped by allowlist, then re-injected as 1)", []string{"PATH=/usr/bin", "BD_NON_INTERACTIVE=0"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
			runner := NewCommandRunner(RunnerConfig{Env: tc.env, Executor: execStub})

			_, err := runner.Run(context.Background(), CommandRequest{
				Operation: "list issues",
				Args:      []string{"list", "--json"},
			})
			if err != nil {
				t.Fatalf("Run returned unexpected error: %v", err)
			}

			lastBNIIdx := -1
			for i, entry := range execStub.env {
				if entry == "BD_NON_INTERACTIVE=1" {
					lastBNIIdx = i
				}
				if entry == "BD_NON_INTERACTIVE=0" {
					t.Fatalf("BD_NON_INTERACTIVE=0 must not survive; executor env=%v", execStub.env)
				}
			}
			if lastBNIIdx == -1 {
				t.Fatalf("BD_NON_INTERACTIVE=1 must be injected; executor env=%v", execStub.env)
			}
			if lastBNIIdx != len(execStub.env)-1 {
				t.Fatalf("BD_NON_INTERACTIVE=1 must be the LAST env entry (so it wins on duplicate keys); executor env=%v", execStub.env)
			}
		})
	}
}

// TestRunnerReadOnlyPrependsFlag verifies that when RunnerConfig.ReadOnly is
// true, "--readonly" is prepended to every argv before the executor sees it.
// This is the unit-level enforcement of the read-only arg hook.
func TestRunnerReadOnlyPrependsFlag(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("[]")}}
	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  "/some/dir",
		Env:      []string{"PATH=/usr/bin"},
		Executor: execStub,
		ReadOnly: true,
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Executor must see --readonly prepended to the args.
	want := []string{"--readonly", "list", "--json"}
	if !reflect.DeepEqual(execStub.args, want) {
		t.Fatalf("unexpected args: got %v, want %v", execStub.args, want)
	}
}

// TestRunnerNonReadOnlyDoesNotPrependFlag verifies that when ReadOnly is false
// (the default), "--readonly" is NOT added to the argv.
func TestRunnerNonReadOnlyDoesNotPrependFlag(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("[]")}}
	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  "/some/dir",
		Env:      []string{"PATH=/usr/bin"},
		Executor: execStub,
		ReadOnly: false,
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := []string{"list", "--json"}
	if !reflect.DeepEqual(execStub.args, want) {
		t.Fatalf("unexpected args: got %v, want %v (--readonly must not be present)", execStub.args, want)
	}
}
