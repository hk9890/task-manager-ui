package fakes_test

import (
	"context"
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// gateSettleTimeout bounds the wait for a gated call to reach the gate. It is a
// synchronisation budget, not a latency assertion: the call arrives in
// microseconds, and a machine slow enough to miss this is one where the test
// could not be trusted anyway.
const gateSettleTimeout = 2 * time.Second

// callDelayingSearch starts a gated Search in the background and returns a
// channel carrying its error. memory.Repository is the inner backend because it
// is safe to call from another goroutine, which the -race gate requires.
func callDelayingSearch(ctx context.Context, d *fakes.DelayingRepository) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := d.Search(ctx, domain.SearchIssuesQuery{})
		done <- err
	}()

	return done
}

func TestDelayingRepositoryBlocksOnlyTheGatedMethod(t *testing.T) {
	t.Parallel()

	d := fakes.NewDelayingSearchRepository(memory.New())

	// The ungated methods must not touch the gate at all. The bounded context
	// is what turns a gate that wrongly blocks them into a failure here rather
	// than a hang that only the go test timeout ends.
	ungated, cancelUngated := context.WithTimeout(context.Background(), gateSettleTimeout)
	defer cancelUngated()
	if _, err := d.Dashboard(ungated, repository.DashboardOptions{}); err != nil {
		t.Fatalf("ungated Dashboard entered the gate or failed: %v", err)
	}
	if d.InFlight() != 0 {
		t.Fatalf("ungated call entered the gate: InFlight = %d, want 0", d.InFlight())
	}

	done := callDelayingSearch(context.Background(), d)
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 1 })

	select {
	case err := <-done:
		t.Fatalf("gated Search returned before Release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	d.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("gated Search returned error after Release: %v", err)
		}
	case <-time.After(gateSettleTimeout):
		t.Fatal("gated Search did not return after Release")
	}
	if d.InFlight() != 0 {
		t.Fatalf("InFlight not restored after Release: got %d, want 0", d.InFlight())
	}
}

func TestDelayingRepositoryReleaseUnblocksExactlyOneCall(t *testing.T) {
	t.Parallel()

	d := fakes.NewDelayingSearchRepository(memory.New())

	first := callDelayingSearch(context.Background(), d)
	second := callDelayingSearch(context.Background(), d)
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 2 })

	d.Release()
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 1 })

	select {
	case <-first:
	case <-second:
	case <-time.After(gateSettleTimeout):
		t.Fatal("neither gated call returned after a single Release")
	}
	if d.InFlight() != 1 {
		t.Fatalf("Release freed more than one call: InFlight = %d, want 1", d.InFlight())
	}

	d.ReleaseAll()
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 0 })
}

func TestDelayingRepositoryReleaseAllPassesFutureCallsThrough(t *testing.T) {
	t.Parallel()

	d := fakes.NewDelayingSearchRepository(memory.New())
	d.ReleaseAll()

	// A gated call after ReleaseAll must not block, so no goroutine is needed.
	ctx, cancel := context.WithTimeout(context.Background(), gateSettleTimeout)
	defer cancel()
	if _, err := d.Search(ctx, domain.SearchIssuesQuery{}); err != nil {
		t.Fatalf("gated Search after ReleaseAll returned error: %v", err)
	}
	if d.InFlight() != 0 {
		t.Fatalf("InFlight = %d after ReleaseAll, want 0", d.InFlight())
	}

	// ReleaseAll closes the gate channel; a second call must be a no-op rather
	// than a close of a closed channel.
	d.ReleaseAll()
	d.Release()
}

func TestDelayingRepositoryGatedCallReportsContextCancellation(t *testing.T) {
	t.Parallel()

	d := fakes.NewDelayingSearchRepository(memory.New())
	ctx, cancel := context.WithCancel(context.Background())

	done := callDelayingSearch(ctx, d)
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 1 })

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled gated Search returned nil error")
		}
	case <-time.After(gateSettleTimeout):
		t.Fatal("cancelled gated Search did not return")
	}
	if d.InFlight() != 0 {
		t.Fatalf("InFlight not restored after cancellation: got %d, want 0", d.InFlight())
	}
}

func TestDelayingDashboardRepositoryGatesDashboardInstead(t *testing.T) {
	t.Parallel()

	d := fakes.NewDelayingDashboardRepository(memory.New())

	ungated, cancelUngated := context.WithTimeout(context.Background(), gateSettleTimeout)
	defer cancelUngated()
	if _, err := d.Search(ungated, domain.SearchIssuesQuery{}); err != nil {
		t.Fatalf("ungated Search entered the gate or failed: %v", err)
	}
	if d.InFlight() != 0 {
		t.Fatalf("ungated Search entered the gate: InFlight = %d, want 0", d.InFlight())
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.Dashboard(context.Background(), repository.DashboardOptions{})
		done <- err
	}()
	testui.WaitForConditionWithTimeout(t, gateSettleTimeout, func() bool { return d.InFlight() == 1 })

	d.ReleaseAll()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("gated Dashboard returned error after ReleaseAll: %v", err)
		}
	case <-time.After(gateSettleTimeout):
		t.Fatal("gated Dashboard did not return after ReleaseAll")
	}
}
