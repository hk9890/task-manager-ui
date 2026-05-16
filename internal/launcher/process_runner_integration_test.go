//go:build integration

package launcher

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestExecProcessRunnerReapsChildren verifies that after N sequential launches
// of a fast-exiting helper command ("true"), the reaper goroutine calls Wait()
// so that no child remains a zombie. We verify by checking that the process
// state is populated (i.e. Wait returned) within a bounded deadline.
//
// This exercises the go func() { _ = cmd.Wait() }() reaper path.
func TestExecProcessRunnerReapsChildren(t *testing.T) {
	t.Parallel()

	const n = 5

	// Install the test hook so the reaper signals completion without sleeping.
	// Use a buffered channel of size n so each goroutine never blocks.
	hook := make(chan struct{}, n)
	reaperHook = hook
	t.Cleanup(func() { reaperHook = nil })

	runner := NewExecProcessRunner()
	ctx := context.Background()

	for i := range n {
		if err := runner.Run(ctx, "true", nil, "", nil); err != nil {
			t.Fatalf("launch %d: Run returned error: %v", i, err)
		}
	}

	// Wait for all n reaper goroutines to complete, with a 5s safety-net timeout.
	timeout := time.After(5 * time.Second)
	for range n {
		select {
		case <-hook:
			// one reaper done
		case <-timeout:
			t.Fatal("timed out waiting for reaper goroutines to call Wait() — possible zombie leak")
		}
	}

	// If we reach here, all reaper goroutines called Wait() and the children
	// are fully reaped. No process-table zombies remain.
}

// TestExecProcessRunnerChildSurvivesParentCtxCancel verifies the fire-and-forget
// contract: cancelling the parent context does NOT kill launched subprocesses.
//
// Strategy: launch a subprocess that writes a sentinel file after a short delay,
// then cancel the context before the delay elapses, then wait and confirm the
// file was written (proving the child was not killed).
func TestExecProcessRunnerChildSurvivesParentCtxCancel(t *testing.T) {
	t.Parallel()

	runner := NewExecProcessRunner()

	ctx, cancel := context.WithCancel(context.Background())

	sentinel := t.TempDir() + "/alive"

	// Launch a subprocess that sleeps briefly then writes the sentinel.
	// Using sh -c with a fixed positional path — no interpolation of
	// operator-supplied issue fields into the shell body (safe per security rule).
	err := runner.Run(ctx, "sh", []string{
		"-c",
		// body: sleep then touch the positional-arg path
		"sleep 0.3 && touch \"$0\"",
		sentinel,
	}, "", nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Cancel the context almost immediately — before the child writes the file.
	cancel()

	// Wait long enough for the child to have completed if still alive.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sentinel); err == nil {
			return // child survived context cancel: contract holds
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("sentinel file never appeared: child was likely killed when context was cancelled, violating the fire-and-forget contract")
}
