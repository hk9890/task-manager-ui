// Package agent is taskmgr-ui's view of the coding agents that work on a
// project: the Host that runs them, and the Agent and Spec it takes and gives.
// A Host is one implementation, such as revier (internal/agent/revier); the
// core reads no implementation's details. Standard library only.
package agent

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// Status is how much an agent needs the operator. A host may pass a status it
// does not map through unchanged; it ranks last among the live ones.
type Status string

// The statuses of an agent.
const (
	// Running is an agent at work on a turn.
	Running Status = "running"
	// Idle is an agent that waits for a prompt.
	Idle Status = "idle"
	// NeedsYou is an agent that waits for the operator to answer it.
	NeedsYou Status = "needs-you"
	// Gone is an agent linked to an issue that the host no longer runs.
	Gone Status = "gone"
)

// Rank orders the statuses by how much the agent needs the operator: the
// lowest rank first. An unmapped status ranks after the live ones, and Gone
// last.
func Rank(status Status) int {
	switch status {
	case NeedsYou:
		return 0
	case Running:
		return 1
	case Idle:
		return 2
	case Gone:
		return 4
	default:
		return 3
	}
}

// Agent is one agent of a host. Ref is the host's own handle for it and
// opaque to the core. Session is the conversation it holds, empty when the
// host does not know it. Label is what the operator calls it, ready to draw
// once cleaned. Activity is a short title of what it does, empty when the host
// does not say.
type Agent struct {
	Ref      string
	Session  string
	Label    string
	Status   Status
	Activity string
}

// Spec is the agent Start opens: in the project at Root, prompted with Prompt
// when it is not empty, and brought to the front when Focus is true.
type Spec struct {
	Root   string
	Prompt string
	Focus  bool
}

// ErrNoProject is the error a Host wraps when the root it is asked about is
// no project of its own. It is not a fault: most projects are not.
var ErrNoProject = errors.New("not a project of the agent host")

// Host runs the agents of a project. Every call is bounded: none outlives its
// context, and a Host sets its own timeout on top.
type Host interface {
	// Start opens an agent on spec and returns once its session is known.
	Start(ctx context.Context, spec Spec) (Agent, error)
	// List returns the live agents of the project at root on this machine.
	List(ctx context.Context, root string) ([]Agent, error)
	// Focus brings the agent to the front.
	Focus(ctx context.Context, a Agent) error
	// Resume opens an agent in the project at root on the conversation
	// session, without the focus.
	Resume(ctx context.Context, root, session string) (Agent, error)
	// Wait returns once the agent's current turn has ended.
	Wait(ctx context.Context, a Agent) error
	// Read returns the last thing the agent said.
	Read(ctx context.Context, a Agent) (string, error)
	// Close ends the agent. Without force a host may refuse an agent that is
	// running or waits for the operator.
	Close(ctx context.Context, a Agent, force bool) error
}

// linkLabelPrefix starts the issue label that links an agent's session to the
// issue it was started on.
const linkLabelPrefix = "agent:"

// LinkLabel is the issue label that links the agent on session to the issue.
func LinkLabel(session string) string {
	return linkLabelPrefix + session
}

// IsLinkLabel reports whether label links an agent's session to its issue.
// A link label is kept in the store and hidden from the operator.
func IsLinkLabel(label string) bool {
	return strings.HasPrefix(label, linkLabelPrefix)
}

// VisibleLabels is labels without the link labels: the ones the operator
// reads and edits.
func VisibleLabels(labels []string) []string {
	return slices.DeleteFunc(slices.Clone(labels), IsLinkLabel)
}

// LinkSessions is the sessions the link labels among labels name.
func LinkSessions(labels []string) []string {
	var sessions []string
	for _, label := range labels {
		if session, ok := strings.CutPrefix(label, linkLabelPrefix); ok && session != "" {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// FirstLive is the first agent of agents that is not Gone.
func FirstLive(agents []Agent) (Agent, bool) {
	for _, a := range agents {
		if a.Status != Gone {
			return a, true
		}
	}
	return Agent{}, false
}

// goneLabelLength is how much of its session the label of a gone agent shows.
const goneLabelLength = 8

// Linked is the agents linked to an issue: those whose session a link label
// among labels names, and those whose session wrote one of its comments,
// merged by session with the live agents of the project. A session a link
// label names that no live agent holds is Gone. A comment session no live
// agent holds is left out: every past session that commented would show. The
// one that needs the operator most is first: by Rank, then by Label.
func Linked(live map[string]Agent, labels, commentSessions []string) []Agent {
	var linked []Agent
	seen := make(map[string]bool)
	add := func(session string, keepGone bool) {
		if session == "" || seen[session] {
			return
		}
		a, ok := live[session]
		if !ok && !keepGone {
			return
		}
		if !ok {
			a = Agent{Session: session, Label: "Session " + session[:min(len(session), goneLabelLength)], Status: Gone}
		}
		seen[session] = true
		linked = append(linked, a)
	}
	for _, session := range LinkSessions(labels) {
		add(session, true)
	}
	for _, session := range commentSessions {
		add(session, false)
	}
	slices.SortFunc(linked, func(a, b Agent) int {
		if rank := Rank(a.Status) - Rank(b.Status); rank != 0 {
			return rank
		}
		return strings.Compare(a.Label, b.Label)
	})
	return linked
}
