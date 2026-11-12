package agent

import (
	"slices"
	"strings"
	"testing"
)

func TestRankOrdersTheStatusesByNeed(t *testing.T) {
	statuses := []Status{Gone, "starting", Idle, Running, NeedsYou}
	slices.SortFunc(statuses, func(a, b Status) int { return Rank(a) - Rank(b) })
	if want := []Status{NeedsYou, Running, Idle, "starting", Gone}; !slices.Equal(statuses, want) {
		t.Fatalf("statuses by rank = %v, want %v", statuses, want)
	}
}

func TestLinkedMergesTheLinksWithTheLiveAgents(t *testing.T) {
	live := map[string]Agent{
		"s-label":   {Ref: "p:t:1", Session: "s-label", Label: "Tab 1", Status: Idle, Activity: "fixing tests"},
		"s-comment": {Ref: "p:t:2", Session: "s-comment", Label: "Tab 2", Status: Running},
		"s-other":   {Ref: "p:t:3", Session: "s-other", Label: "Tab 3", Status: NeedsYou},
	}
	labels := []string{"backend", "agent:s-label", "agent:0123456789abcdef"}
	comments := []string{"s-comment", "s-label", "s-old", ""}

	got := Linked(live, labels, comments)
	want := []Agent{
		live["s-comment"],
		live["s-label"],
		{Session: "0123456789abcdef", Label: "Session 01234567", Status: Gone},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Linked = %+v, want %+v", got, want)
	}
}

// A nil live list is not known, so a link label no live agent holds is not
// Gone; a known list with no agent makes it Gone.
func TestLinkedReportsNoGoneAgentWhileTheLiveListIsNotKnown(t *testing.T) {
	labels := []string{"agent:s-1"}
	if got := Linked(nil, labels, nil); len(got) != 0 {
		t.Fatalf("Linked(nil) = %+v, want no agent", got)
	}
	got := Linked(map[string]Agent{}, labels, nil)
	if len(got) != 1 || got[0].Status != Gone {
		t.Fatalf("Linked(empty) = %+v, want s-1 gone", got)
	}
}

// A session the store would refuse in a label makes no link label.
func TestLinkLabelRefusesASessionTheStoreRefuses(t *testing.T) {
	if got, err := LinkLabel("0f3c-9a.b_c/d:e"); err != nil || got != "agent:0f3c-9a.b_c/d:e" {
		t.Fatalf("LinkLabel = %q, %v; want agent:0f3c-9a.b_c/d:e", got, err)
	}
	for _, session := range []string{"", "Upper", "with space", strings.Repeat("a", 59)} {
		if got, err := LinkLabel(session); err == nil {
			t.Errorf("LinkLabel(%q) = %q, want an error", session, got)
		}
	}
	if _, err := LinkLabel(strings.Repeat("a", 58)); err != nil {
		t.Errorf("LinkLabel of a 64-character label = %v, want nil", err)
	}
}

func TestVisibleLabelsHidesTheLinkLabels(t *testing.T) {
	labels := []string{"agent:s-1", "backend", "agentic", "agent:s-2"}
	if got, want := VisibleLabels(labels), []string{"backend", "agentic"}; !slices.Equal(got, want) {
		t.Fatalf("VisibleLabels = %v, want %v", got, want)
	}
	if want := []string{"agent:s-1", "backend", "agentic", "agent:s-2"}; !slices.Equal(labels, want) {
		t.Fatalf("VisibleLabels changed its input to %v", labels)
	}
	if got, want := LinkSessions(labels), []string{"s-1", "s-2"}; !slices.Equal(got, want) {
		t.Fatalf("LinkSessions = %v, want %v", got, want)
	}
}
