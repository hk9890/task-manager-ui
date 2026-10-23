// Package revier reads the live agents of a revier project and brings one to
// the front, through the revier command line. It is a read and focus seam: it
// starts no agent and holds no state between calls.
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
	"time"
)

// callTimeout bounds each revier call: a poll must not pile up behind a
// revier that hangs.
const callTimeout = 5 * time.Second

// Agent is one live agent of a revier project. Status is revier's idle,
// running or attention; another value passes through unchanged. Session is the
// conversation the agent holds, empty when its harness does not say.
type Agent struct {
	Panel    string
	Status   string
	Session  string
	Activity string
}

// The statuses revier reports for an agent.
const (
	StatusIdle      = "idle"
	StatusRunning   = "running"
	StatusAttention = "attention"
)

// Rank orders the statuses by how much the agent needs the operator: the
// lowest rank first. A status revier does not document ranks last.
func Rank(status string) int {
	switch status {
	case StatusAttention:
		return 0
	case StatusRunning:
		return 1
	case StatusIdle:
		return 2
	default:
		return 3
	}
}

// Client is what taskmgr-ui asks revier.
type Client interface {
	// Project returns the name of the revier project whose path is root.
	Project(ctx context.Context, root string) (string, error)
	// Agents returns the live agents of the project on this machine.
	Agents(ctx context.Context, project string) ([]Agent, error)
	// Focus brings the agent at addr to the front.
	Focus(ctx context.Context, addr string) error
}

// Address returns the address revier names the agent in a panel of a project by.
func Address(project, panel string) string {
	return project + ":" + panel
}

type execClient struct{}

// NewExecClient returns the Client that runs the revier binary on PATH.
func NewExecClient() Client {
	return execClient{}
}

func (execClient) Project(ctx context.Context, root string) (string, error) {
	out, err := run(ctx, root, "status")
	if err != nil {
		return "", err
	}
	return projectFromStatus(out, root)
}

func (execClient) Agents(ctx context.Context, project string) ([]Agent, error) {
	out, err := run(ctx, "", "list", "--json", "--conversations", project)
	if err != nil {
		return nil, err
	}
	return parseAgents(out)
}

func (execClient) Focus(ctx context.Context, addr string) error {
	_, err := run(ctx, "", "agent", "focus", addr)
	return err
}

// run runs revier with args in dir and returns its standard output. A failed
// call's error carries revier's standard error.
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
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
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("revier is not installed: %w", err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return nil, fmt.Errorf("revier %s: %w: %s", args[0], err, stderr)
		}
	}
	return nil, fmt.Errorf("revier %s: %w", args[0], err)
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

// listedProject is the part of one element of revier list --json that Agents
// reads.
type listedProject struct {
	Agents []struct {
		Panel string `json:"panel"`
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
func parseAgents(out []byte) ([]Agent, error) {
	var projects []listedProject
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, fmt.Errorf("read revier list: %w", err)
	}
	var agents []Agent
	for _, p := range projects {
		for _, a := range p.Agents {
			if a.Ref == nil {
				continue
			}
			agents = append(agents, Agent{
				Panel:    a.Panel,
				Status:   a.State.Status,
				Session:  a.State.Session,
				Activity: a.State.Activity,
			})
		}
	}
	return agents, nil
}
