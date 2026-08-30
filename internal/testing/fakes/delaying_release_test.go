package fakes_test

import (
	"testing"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// TestReleaseAfterReleaseAllDoesNotPanic pins Release against the gate being
// closed underneath it.
//
// Release checked d.released, unlocked, and then sent on the channel. A
// ReleaseAll landing in that window closed the channel before the send, and the
// send panicked with "send on closed channel" — inside whichever test happened
// to be running, not the one that got the ordering wrong.
func TestReleaseAfterReleaseAllDoesNotPanic(t *testing.T) {
	t.Parallel()

	gate := fakes.NewDelayingSearchRepository(memoryrepo.New())

	gate.ReleaseAll()
	gate.Release()
	gate.Release()
	gate.ReleaseAll()
}
