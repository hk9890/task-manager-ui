package agent

import (
	"slices"
	"testing"
)

func TestRankOrdersTheStatusesByNeed(t *testing.T) {
	statuses := []Status{Gone, "starting", Idle, Running, NeedsYou}
	slices.SortFunc(statuses, func(a, b Status) int { return Rank(a) - Rank(b) })
	if want := []Status{NeedsYou, Running, Idle, "starting", Gone}; !slices.Equal(statuses, want) {
		t.Fatalf("statuses by rank = %v, want %v", statuses, want)
	}
}
