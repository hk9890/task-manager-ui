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
	labels := []string{"backend", "agent-session:s-label", "agent-session:0123456789abcdef"}
	comments := []string{"s-comment", "s-label", "s-old", ""}

	got := Linked(live, IssueLinks{Labels: labels, CommentSessions: comments})
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
	labels := []string{"agent-session:s-1"}
	if got := Linked(nil, IssueLinks{Labels: labels}); len(got) != 0 {
		t.Fatalf("Linked(nil) = %+v, want no agent", got)
	}
	got := Linked(map[string]Agent{}, IssueLinks{Labels: labels})
	if len(got) != 1 || got[0].Status != Gone {
		t.Fatalf("Linked(empty) = %+v, want s-1 gone", got)
	}
}

// A closed issue has no gone agent: its agent finished, and the store
// refuses the unlink of its label. A live one stays linked.
func TestLinkedReportsNoGoneAgentOfAClosedIssue(t *testing.T) {
	live := map[string]Agent{"s-live": {Session: "s-live", Label: "Tab 1", Status: Idle}}
	labels := []string{"agent-session:s-live", "agent-session:s-old"}
	got := Linked(live, IssueLinks{Labels: labels, Closed: true})
	if want := []Agent{live["s-live"]}; !slices.Equal(got, want) {
		t.Fatalf("Linked(closed) = %+v, want %+v", got, want)
	}
}

// Agents of one rank are ordered by Title, the name the operator reads, not
// by Label.
func TestLinkOrdersAgentsOfARankByTitle(t *testing.T) {
	live := map[string]Agent{
		"s-1": {Session: "s-1", Label: "Tab 1", Name: "zeta", Status: Idle},
		"s-2": {Session: "s-2", Label: "Tab 2", Name: "alpha", Status: Idle},
	}
	got := Linked(live, IssueLinks{Labels: []string{"agent-session:s-1", "agent-session:s-2"}})
	if want := []Agent{live["s-2"], live["s-1"]}; !slices.Equal(got, want) {
		t.Fatalf("Linked = %+v, want alpha first", got)
	}
}

// A session two issues link is linked to the first: a label on any issue
// before a comment, and among the comments the first issue.
func TestLinkLinksASessionToTheFirstIssue(t *testing.T) {
	live := map[string]Agent{
		"s-1": {Session: "s-1", Label: "Tab 1", Status: Idle},
		"s-2": {Session: "s-2", Label: "Tab 2", Status: Idle},
	}
	issues := []IssueLinks{
		{CommentSessions: []string{"s-1", "s-2"}},
		{CommentSessions: []string{"s-2"}},
		{Labels: []string{"agent-session:s-1"}},
	}
	got := Link(live, issues)
	want := []LinkedAgent{{Agent: live["s-1"], Issue: 2}, {Agent: live["s-2"], Issue: 0}}
	if !slices.Equal(got, want) {
		t.Fatalf("Link = %+v, want %+v", got, want)
	}
}

// A session the store would refuse in a label makes no link label.
func TestLinkLabelRefusesASessionTheStoreRefuses(t *testing.T) {
	if got, err := LinkLabel("0f3c-9a.b_c/d:e"); err != nil || got != "agent-session:0f3c-9a.b_c/d:e" {
		t.Fatalf("LinkLabel = %q, %v; want agent-session:0f3c-9a.b_c/d:e", got, err)
	}
	for _, session := range []string{"", "Upper", "with space", strings.Repeat("a", 51)} {
		if got, err := LinkLabel(session); err == nil {
			t.Errorf("LinkLabel(%q) = %q, want an error", session, got)
		}
	}
	if _, err := LinkLabel(strings.Repeat("a", 50)); err != nil {
		t.Errorf("LinkLabel of a 64-character label = %v, want nil", err)
	}
}

func TestVisibleLabelsHidesTheLinkLabels(t *testing.T) {
	labels := []string{"agent-session:s-1", "backend", "agentic", "agent-session:s-2"}
	if got, want := VisibleLabels(labels), []string{"backend", "agentic"}; !slices.Equal(got, want) {
		t.Fatalf("VisibleLabels = %v, want %v", got, want)
	}
	if want := []string{"agent-session:s-1", "backend", "agentic", "agent-session:s-2"}; !slices.Equal(labels, want) {
		t.Fatalf("VisibleLabels changed its input to %v", labels)
	}
	if got, want := LinkSessions(labels), []string{"s-1", "s-2"}; !slices.Equal(got, want) {
		t.Fatalf("LinkSessions = %v, want %v", got, want)
	}
}

// A label with the old prefix agent: links nothing: it is a label as any
// other, and the operator sees it.
func TestTheOldPrefixIsNoLinkLabel(t *testing.T) {
	labels := []string{"agent:s-1", "agent-session:s-2"}
	if IsLinkLabel("agent:s-1") {
		t.Error("IsLinkLabel(agent:s-1) = true, want false")
	}
	if got, want := VisibleLabels(labels), []string{"agent:s-1"}; !slices.Equal(got, want) {
		t.Errorf("VisibleLabels = %v, want %v", got, want)
	}
	if got, want := LinkSessions(labels), []string{"s-2"}; !slices.Equal(got, want) {
		t.Errorf("LinkSessions = %v, want %v", got, want)
	}
}

// An agent's title is its name, its label when the harness names none; its
// key is its session, its ref when the host does not know the session.
func TestTitleAndKeyFallBack(t *testing.T) {
	named := Agent{Ref: "demo:home:3", Session: "s-3", Label: "Tab 3", Name: "fix-board"}
	unnamed := Agent{Ref: "demo:11", Label: "Tab 11", Name: "  "}
	if named.Title() != "fix-board" || unnamed.Title() != "Tab 11" {
		t.Errorf("titles = %q, %q; want fix-board, Tab 11", named.Title(), unnamed.Title())
	}
	if named.Key() != "s-3" || unnamed.Key() != "demo:11" {
		t.Errorf("keys = %q, %q; want s-3, demo:11", named.Key(), unnamed.Key())
	}
	if StatusWord(NeedsYou) != "needs you" || StatusWord(Running) != "running" {
		t.Errorf("StatusWord = %q, %q", StatusWord(NeedsYou), StatusWord(Running))
	}
}
