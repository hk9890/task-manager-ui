// Package agent is taskmgr-ui's view of the coding agents that work on a
// project: the Host that runs them, and the Agent and Spec it takes and gives.
// A Host is one implementation, such as revier (internal/agent/revier); the
// core reads no implementation's details. Standard library only.
package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

// StatusWord is what the operator reads for status: "needs you" for
// NeedsYou, and the status itself for every other one.
func StatusWord(status Status) string {
	if status == NeedsYou {
		return "needs you"
	}
	return string(status)
}

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
// host does not know it. Label is where the operator finds it, such as its
// tab, and Name what its harness calls it, empty when the harness names none;
// both are ready to draw once cleaned. Activity is a short title of what it
// does, empty when the host does not say.
type Agent struct {
	Ref      string
	Session  string
	Label    string
	Name     string
	Status   Status
	Activity string
}

// Title is what the operator calls a: its Name, or its Label when the
// harness names none.
func (a Agent) Title() string {
	if name := strings.TrimSpace(a.Name); name != "" {
		return name
	}
	return a.Label
}

// Key tells a from the other agents of a host: its Session, or its Ref when
// the host does not know the session.
func (a Agent) Key() string {
	if a.Session != "" {
		return a.Session
	}
	return a.Ref
}

// Spec is the agent Start opens: in the project at Root, without the focus,
// and prompted with Prompt when it is not empty.
type Spec struct {
	Root   string
	Prompt string
}

// ErrNoProject is the error a Host wraps when the root it is asked about is
// no project of its own. It is not a fault: most projects are not.
var ErrNoProject = errors.New("not a project of the agent host")

// Host runs the agents of a project. Every call is bounded: none outlives its
// context, and a Host sets its own timeout on top.
type Host interface {
	// Start opens an agent on spec and returns once its session is known.
	// When a later step, such as the prompt, fails, it returns the agent it
	// opened with the error.
	Start(ctx context.Context, spec Spec) (Agent, error)
	// List returns the live agents of the project at root on this machine.
	List(ctx context.Context, root string) ([]Agent, error)
	// Focus brings the agent to the front.
	Focus(ctx context.Context, a Agent) error
	// Resume opens an agent in the project at root on the conversation
	// session, without the focus, and returns once it can be focused.
	Resume(ctx context.Context, root, session string) (Agent, error)
	// Wait returns once the agent's current turn has ended, with the status
	// it ended on: Idle, or NeedsYou when the agent waits for the operator.
	Wait(ctx context.Context, a Agent) (Status, error)
	// Read returns the last thing the agent said.
	Read(ctx context.Context, a Agent) (string, error)
	// Close ends the agent. Without force a host may refuse an agent that is
	// running or waits for the operator.
	Close(ctx context.Context, a Agent, force bool) error
}

// LinkLabelPrefix starts the issue label that links an agent's session to the
// issue it was started on.
const LinkLabelPrefix = "agent-session:"

// linkSessionPattern and maxLabelLength are what the store takes of a label
// after LinkLabelPrefix: the rest of its grammar, ^[a-z0-9][a-z0-9:._/-]*$,
// and its length.
var linkSessionPattern = regexp.MustCompile(`^[a-z0-9:._/-]+$`)

const maxLabelLength = 64

// LinkLabel is the issue label that links the agent on session to the issue.
// It fails for a session the store would refuse in a label.
func LinkLabel(session string) (string, error) {
	label := LinkLabelPrefix + session
	if !linkSessionPattern.MatchString(session) || len(label) > maxLabelLength {
		return "", fmt.Errorf("the agent session %q makes no valid issue label "+
			"(at most %d characters of a-z, 0-9 and :._/-)", session, maxLabelLength-len(LinkLabelPrefix))
	}
	return label, nil
}

// IsLinkLabel reports whether label links an agent's session to its issue.
// A link label is kept in the store and hidden from the operator.
func IsLinkLabel(label string) bool {
	return strings.HasPrefix(label, LinkLabelPrefix)
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
		if session, ok := strings.CutPrefix(label, LinkLabelPrefix); ok && session != "" {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// goneLabelLength is how much of its session the label of a gone agent shows.
const goneLabelLength = 8

// GoneAgent is the agent of a session that a link label names and no live
// agent holds.
func GoneAgent(session string) Agent {
	return Agent{Session: session, Label: "Session " + session[:min(len(session), goneLabelLength)], Status: Gone}
}

// ByNeed orders a before b when it needs the operator more: by Rank, then by
// Title.
func ByNeed(a, b Agent) int {
	if rank := Rank(a.Status) - Rank(b.Status); rank != 0 {
		return rank
	}
	return strings.Compare(a.Title(), b.Title())
}

// IssueLinks is what links one issue to agents: its labels, among them the
// link labels, the sessions of the agents that commented on it, and whether
// it is closed. A caller gives the comment sessions of an In Progress issue
// only.
type IssueLinks struct {
	Labels          []string
	CommentSessions []string
	Closed          bool
}

// LinkedAgent is an agent linked to an issue: Issue is the index of the
// issue in the list Link was given.
type LinkedAgent struct {
	Agent Agent
	Issue int
}

// Link is the agents linked to issues, merged by session with the live
// agents of the project. live is nil when the live agents are not known: no
// poll answered yet, or the last one failed.
//
//   - A session a link label names is linked to the issue of the label. A
//     label wins over a comment, and the first issue over a later one.
//   - A session a label names that no live agent holds is Gone, but only on
//     an issue that is not closed and only when live is known: a closed
//     issue's agent finished and the store refuses the unlink of its label,
//     and an unknown list must not report a live agent gone.
//   - A comment session no live agent holds is left out: every past session
//     that commented would show.
//
// The agent that needs the operator most is first (ByNeed).
func Link(live map[string]Agent, issues []IssueLinks) []LinkedAgent {
	known := live != nil
	var linked []LinkedAgent
	seen := make(map[string]bool)
	add := func(session string, issue int, mayBeGone bool) {
		if session == "" || seen[session] {
			return
		}
		a, ok := live[session]
		if !ok && !mayBeGone {
			return
		}
		if !ok {
			a = GoneAgent(session)
		}
		seen[session] = true
		linked = append(linked, LinkedAgent{Agent: a, Issue: issue})
	}
	for i, issue := range issues {
		for _, session := range LinkSessions(issue.Labels) {
			add(session, i, known && !issue.Closed)
		}
	}
	for i, issue := range issues {
		for _, session := range issue.CommentSessions {
			add(session, i, false)
		}
	}
	slices.SortStableFunc(linked, func(a, b LinkedAgent) int { return ByNeed(a.Agent, b.Agent) })
	return linked
}

// Linked is the agents Link links to the one issue.
func Linked(live map[string]Agent, issue IssueLinks) []Agent {
	var agents []Agent
	for _, linked := range Link(live, []IssueLinks{issue}) {
		agents = append(agents, linked.Agent)
	}
	return agents
}
