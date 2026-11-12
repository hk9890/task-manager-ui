// Package revier is the agent.Host over the revier command line: revier opens
// each agent in a tab of the project's open workspace. Every call runs the
// revier binary on PATH with an argument list, never through a shell, and
// under a timeout. Standard library only.
package revier

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hk9890/task-manager-ui/internal/agent"
)

const (
	// callTimeout bounds a call that answers at once: a poll must not pile
	// up behind a revier that hangs.
	callTimeout = 5 * time.Second
	// promptTimeout bounds a prompt, which returns once the agent started on
	// the text.
	promptTimeout = 30 * time.Second
	// harnessWait is the seconds one wait for a new agent's harness to come
	// up may take, and harnessAttempts how many such waits Start makes: a
	// wait fails until the harness in the new tab has started.
	harnessWait     = 60
	harnessAttempts = 30
	// sessionAttempts is how often Start lists the agents for the session of
	// a new agent whose harness does not name it yet.
	sessionAttempts = 5
	// startTimeout bounds all of Start: the tab, its harness, its session,
	// the prompt and the focus.
	startTimeout = 3 * time.Minute
	// turnWait is the seconds Wait waits for a turn to end.
	turnWait = 30 * 60
	// waitTimedOut is the exit status of a revier agent wait that ran out
	// of time.
	waitTimedOut = 2
)

// Host is the agent.Host that runs revier. It remembers the revier project
// of each root it resolved, so a poll runs one revier call, not two.
type Host struct {
	// pause is the time between two attempts of a retried call.
	pause time.Duration

	mu       sync.Mutex
	projects map[string]string
}

var _ agent.Host = (*Host)(nil)

// NewHost returns the Host that runs the revier binary on PATH.
func NewHost() *Host {
	return &Host{pause: 2 * time.Second, projects: map[string]string{}}
}

// Start opens an agent tab without the focus, waits for its harness, reads
// its session from the list, then prompts and focuses it as spec asks.
// revier has no command that prints the session of a new agent, hence the
// list.
func (h *Host) Start(ctx context.Context, spec agent.Spec) (agent.Agent, error) {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	project, err := h.project(ctx, spec.Root)
	if err != nil {
		return agent.Agent{}, err
	}
	addr, err := newAgent(ctx, project)
	if err != nil {
		return agent.Agent{}, err
	}
	if err := h.awaitHarness(ctx, addr); err != nil {
		return agent.Agent{}, err
	}
	started, err := h.awaitSession(ctx, project, addr)
	if err != nil {
		return agent.Agent{}, err
	}
	if spec.Prompt != "" {
		if _, err := run(ctx, promptTimeout, "", "agent", "prompt", addr, "--", spec.Prompt); err != nil {
			return started, err
		}
		started.Status = agent.Running
	}
	if spec.Focus {
		if err := h.Focus(ctx, started); err != nil {
			return started, err
		}
	}
	return started, nil
}

// List returns the agents of the revier project at root that run on this
// machine.
func (h *Host) List(ctx context.Context, root string) ([]agent.Agent, error) {
	project, err := h.project(ctx, root)
	if err != nil {
		return nil, err
	}
	return listAgents(ctx, project)
}

// Focus makes the agent's tab current and raises its window.
func (h *Host) Focus(ctx context.Context, a agent.Agent) error {
	_, err := run(ctx, callTimeout, "", "agent", "focus", a.Ref)
	return err
}

// Resume opens an agent tab on the conversation session, without the focus.
// It returns at once: the harness in the tab may not have started yet.
func (h *Host) Resume(ctx context.Context, root, session string) (agent.Agent, error) {
	project, err := h.project(ctx, root)
	if err != nil {
		return agent.Agent{}, err
	}
	addr, err := newAgent(ctx, project, "--resume", session)
	if err != nil {
		return agent.Agent{}, err
	}
	return agent.Agent{Ref: addr, Session: session, Label: label(panelOf(addr)), Status: agent.Idle}, nil
}

// Wait returns once the agent is idle or waits for the operator, or with an
// error after turnWait seconds.
func (h *Host) Wait(ctx context.Context, a agent.Agent) error {
	_, err := wait(ctx, a.Ref, "stopped", turnWait)
	return err
}

// Read returns the last thing the agent said.
func (h *Host) Read(ctx context.Context, a agent.Agent) (string, error) {
	out, err := run(ctx, callTimeout, "", "agent", "read", a.Ref)
	return string(out), err
}

// Close closes the agent's panel. Without force revier refuses an agent that
// is running or waits for an answer.
func (h *Host) Close(ctx context.Context, a agent.Agent, force bool) error {
	args := []string{"agent", "close", a.Ref}
	if force {
		args = append(args, "--force")
	}
	_, err := run(ctx, callTimeout, "", args...)
	return err
}

// project returns the revier project whose path is root, from the cache or
// from revier status. Every failure wraps agent.ErrNoProject and is not
// cached, so a root that becomes a project is found on a later call.
func (h *Host) project(ctx context.Context, root string) (string, error) {
	h.mu.Lock()
	project, ok := h.projects[root]
	h.mu.Unlock()
	if ok {
		return project, nil
	}
	out, err := run(ctx, callTimeout, root, "status")
	if err == nil {
		project, err = projectFromStatus(out, root)
	}
	if err != nil {
		return "", fmt.Errorf("%w (%w)", err, agent.ErrNoProject)
	}
	h.mu.Lock()
	h.projects[root] = project
	h.mu.Unlock()
	return project, nil
}

// awaitHarness waits until the agent at addr is idle. A wait fails at once
// until the harness in the new tab has started, so a failed wait is tried
// again, harnessAttempts times at most; a wait that ran out of time is not.
func (h *Host) awaitHarness(ctx context.Context, addr string) error {
	var err error
	for range harnessAttempts {
		if _, err = wait(ctx, addr, "idle", harnessWait); err == nil {
			return nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == waitTimedOut {
			return fmt.Errorf("the agent at %s is not idle after %d seconds: %w", addr, harnessWait, err)
		}
		if err := h.sleep(ctx); err != nil {
			return err
		}
	}
	return fmt.Errorf("the harness of the agent at %s did not start: %w", addr, err)
}

// awaitSession lists the agents of project until the one at addr names its
// session, sessionAttempts times at most.
func (h *Host) awaitSession(ctx context.Context, project, addr string) (agent.Agent, error) {
	panel := panelOf(addr)
	for range sessionAttempts {
		agents, err := listAgents(ctx, project)
		if err != nil {
			return agent.Agent{}, err
		}
		for _, a := range agents {
			if panelOf(a.Ref) == panel && a.Session != "" {
				a.Ref = addr
				return a, nil
			}
		}
		if err := h.sleep(ctx); err != nil {
			return agent.Agent{}, err
		}
	}
	return agent.Agent{}, fmt.Errorf("revier lists no session for the agent at %s", addr)
}

func (h *Host) sleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(h.pause):
		return nil
	}
}

// newAgent opens an agent tab of project without the focus and returns its
// address, which revier prints before the harness in the tab has started.
func newAgent(ctx context.Context, project string, extra ...string) (string, error) {
	args := append([]string{"agent", "new", "--no-focus", "-p", project}, extra...)
	out, err := run(ctx, callTimeout, "", args...)
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(out))
	if addr == "" {
		return "", errors.New("revier agent new printed no address")
	}
	return addr, nil
}

// wait runs revier agent wait on addr until status, for seconds at most, and
// returns the status it ended on.
func wait(ctx context.Context, addr, status string, seconds int) (string, error) {
	timeout := time.Duration(seconds)*time.Second + callTimeout
	out, err := run(ctx, timeout, "", "agent", "wait", addr, "--until", status, "--timeout", fmt.Sprint(seconds))
	return strings.TrimSpace(string(out)), err
}

func listAgents(ctx context.Context, project string) ([]agent.Agent, error) {
	out, err := run(ctx, callTimeout, "", "list", "--json", "--conversations", project)
	if err != nil {
		return nil, err
	}
	return parseAgents(out)
}

// run runs revier with args in dir, for timeout at most, and returns its
// standard output. A failed call's error carries revier's standard error and
// wraps the *exec.ExitError.
func run(ctx context.Context, timeout time.Duration, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "revier", args...)
	cmd.Dir = dir
	// A child of revier that keeps the output pipe open must not hold the
	// call past its timeout.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	name := strings.Join(args[:min(2, len(args))], " ")
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("revier is not installed: %w", err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return nil, fmt.Errorf("revier %s: %w: %s", name, err, stderr)
		}
	}
	return nil, fmt.Errorf("revier %s: %w", name, err)
}

// projectFromStatus returns the project of revier status output whose path is
// root. Outside a project revier status reports the project of the focused
// window instead of failing, so a project at another path is an error.
func projectFromStatus(out []byte, root string) (string, error) {
	var project, path string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), " ")
		switch key {
		case "project":
			project = strings.TrimSpace(value)
		case "path":
			path = strings.TrimSpace(value)
		}
	}
	if project == "" || path == "" {
		return "", errors.New("revier status names no project and path")
	}
	if filepath.Clean(path) != filepath.Clean(root) {
		return "", fmt.Errorf("%s is not a revier project: revier status resolves it to %s", root, path)
	}
	return project, nil
}

// listedProject is the part of one element of revier list --json that List
// reads.
type listedProject struct {
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	Agents []struct {
		Panel  string `json:"panel"`
		Target string `json:"target"`
		// Ref is the instance holding the panel. An agent without one is
		// on a link's remote host.
		Ref   *struct{} `json:"ref"`
		State struct {
			Status   string `json:"status"`
			Session  string `json:"session"`
			Activity string `json:"activity"`
		} `json:"state"`
	} `json:"agents"`
}

// parseAgents returns the agents on this machine in revier list --json output.
func parseAgents(out []byte) ([]agent.Agent, error) {
	var projects []listedProject
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, fmt.Errorf("read revier list: %w", err)
	}
	var agents []agent.Agent
	for _, p := range projects {
		for _, a := range p.Agents {
			if a.Ref == nil {
				continue
			}
			agents = append(agents, agent.Agent{
				Ref:      address(p.Project.Name, a.Target, a.Panel),
				Session:  a.State.Session,
				Label:    label(a.Panel),
				Status:   status(a.State.Status),
				Activity: a.State.Activity,
			})
		}
	}
	return agents, nil
}

// address returns the address revier names an agent by:
// <project>:<target>:<panel>, or <project>:<panel> without a target.
func address(project, target, panel string) string {
	if target == "" {
		return project + ":" + panel
	}
	return project + ":" + target + ":" + panel
}

// panelOf returns the panel of an agent address, its last segment.
func panelOf(addr string) string {
	return addr[strings.LastIndex(addr, ":")+1:]
}

// label is what the operator calls the agent in a panel: the tab it is in.
func label(panel string) string {
	return "Tab " + panel
}

// status maps a revier status onto agent.Status. One revier does not document
// passes through unchanged.
func status(revier string) agent.Status {
	switch revier {
	case "idle":
		return agent.Idle
	case "running":
		return agent.Running
	case "attention":
		return agent.NeedsYou
	default:
		return agent.Status(revier)
	}
}
