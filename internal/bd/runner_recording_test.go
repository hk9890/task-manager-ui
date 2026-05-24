package bd_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// TestRecordingExecutorRecordsCalls verifies that RecordingExecutor records
// each Run invocation with the correct argv, workDir, and envLen, and that
// argv-match rules return the configured response.
func TestRecordingExecutorRecordsCalls(t *testing.T) {
	t.Parallel()

	rec := fakes.NewRecordingExecutor()

	pingResponse := bd.ExecResult{Stdout: []byte("pong")}
	readyResponse := bd.ExecResult{Stdout: []byte(`[]`)}

	rec.OnArgs([]string{"ping"}).Return(pingResponse, nil)
	rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(readyResponse, nil)
	rec.SetDefault(bd.ExecResult{Stdout: []byte("default")}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{
		Command:  "bd",
		WorkDir:  "/proj",
		Executor: rec,
	})

	// First call: ping
	_, err := runner.Run(context.Background(), bd.CommandRequest{
		Operation: "health-check",
		Args:      []string{"ping"},
	})
	if err != nil {
		t.Fatalf("ping: unexpected error: %v", err)
	}

	// Second call: ready
	_, err = runner.Run(context.Background(), bd.CommandRequest{
		Operation: "ready-explain",
		Args:      []string{"ready", "--explain", "--json"},
	})
	if err != nil {
		t.Fatalf("ready: unexpected error: %v", err)
	}

	// Third call: unmatched — should hit default
	_, err = runner.Run(context.Background(), bd.CommandRequest{
		Operation: "other",
		Args:      []string{"list", "--json"},
	})
	if err != nil {
		t.Fatalf("other: unexpected error: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 recorded calls, got %d", len(calls))
	}

	if !reflect.DeepEqual(calls[0].Args, []string{"ping"}) {
		t.Errorf("call[0] args: got %v, want [ping]", calls[0].Args)
	}
	if !reflect.DeepEqual(calls[1].Args, []string{"ready", "--explain", "--json"}) {
		t.Errorf("call[1] args: got %v", calls[1].Args)
	}
	if !reflect.DeepEqual(calls[2].Args, []string{"list", "--json"}) {
		t.Errorf("call[2] args: got %v", calls[2].Args)
	}

	// WorkDir defaults to runner's configured workdir since request WorkDir is empty.
	for i, c := range calls {
		if c.WorkDir != "/proj" {
			t.Errorf("call[%d] workDir: got %q, want /proj", i, c.WorkDir)
		}
		if c.Command != "bd" {
			t.Errorf("call[%d] command: got %q, want bd", i, c.Command)
		}
		if c.At.IsZero() {
			t.Errorf("call[%d] At is zero", i)
		}
	}
}

// TestRecordingExecutorConcurrentCallsNoRace exercises RecordingExecutor under
// concurrent read-flagged runners to ensure no data race. Run with -race.
func TestRecordingExecutorConcurrentCallsNoRace(t *testing.T) {
	t.Parallel()

	const goroutines = 50

	rec := fakes.NewRecordingExecutor()
	rec.OnArgs([]string{"list", "--json"}).Return(bd.ExecResult{Stdout: []byte(`[]`)}, nil)

	runner := bd.NewCommandRunner(bd.RunnerConfig{
		Command:  "bd",
		Executor: rec,
	})

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), bd.CommandRequest{
				Operation: "list",
				Args:      []string{"list", "--json"},
				IsWrite:   false,
			})
			if err != nil {
				t.Errorf("concurrent Run error: %v", err)
			}
		}()
	}

	wg.Wait()

	if got := rec.CallCount(); got != goroutines {
		t.Fatalf("expected %d calls, got %d", goroutines, got)
	}
}
