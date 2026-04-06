package launcher

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

type recordingRunner struct {
	err   error
	calls []runCall
}

type runCall struct {
	command string
	args    []string
	dir     string
	env     []string
}

func (r *recordingRunner) Run(_ context.Context, command string, args []string, dir string, env []string) error {
	r.calls = append(r.calls, runCall{
		command: command,
		args:    append([]string(nil), args...),
		dir:     dir,
		env:     append([]string(nil), env...),
	})
	return r.err
}

func TestServiceLaunchInterpolatesIssueContextAndDelegatesRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	service, err := NewService([]Definition{{
		Action:  "opencode",
		Command: "tool-{{issue.id}}",
		Args: []string{
			"--title", "{{issue.title}}",
			"--labels", "{{issue.labels}}",
			"--assignee", "{{issue.assignee}}",
			"--root", "{{project.root}}",
		},
		Env: []string{
			"BWB_ISSUE_ID={{issue.id}}",
			"BWB_ISSUE_TITLE={{issue.title}}",
			"BWB_ISSUE_LABELS={{issue.labels}}",
			"BWB_ISSUE_ASSIGNEE={{issue.assignee}}",
			"BWB_PROJECT_ROOT={{project.root}}",
		},
		WorkDir: "{{project.root}}",
	}}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:       "bw-77",
		Title:    "Implement launcher framework",
		Assignee: "hans",
		Labels:   []string{"infra", "launcher"},
	}}

	if err := service.Launch(context.Background(), "opencode", issue); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected exactly one process run, got %d", len(runner.calls))
	}

	call := runner.calls[0]
	if call.command != "tool-bw-77" {
		t.Fatalf("expected interpolated command, got %q", call.command)
	}
	if call.dir != "/repo/root" {
		t.Fatalf("expected interpolated workdir /repo/root, got %q", call.dir)
	}
	if len(call.args) != 8 {
		t.Fatalf("expected interpolated args, got %#v", call.args)
	}
	if call.args[1] != "Implement launcher framework" || call.args[3] != "infra,launcher" || call.args[5] != "hans" || call.args[7] != "/repo/root" {
		t.Fatalf("unexpected interpolated args: %#v", call.args)
	}
	if len(call.env) != 5 {
		t.Fatalf("expected interpolated env entries, got %#v", call.env)
	}
	if call.env[0] != "BWB_ISSUE_ID=bw-77" || call.env[4] != "BWB_PROJECT_ROOT=/repo/root" {
		t.Fatalf("unexpected interpolated env: %#v", call.env)
	}
}

func TestServiceLaunchDefaultsWorkDirToProjectRoot(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	service, err := NewService([]Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if err := service.Launch(context.Background(), "editor", domain.IssueDetail{}); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if len(runner.calls) != 1 || runner.calls[0].dir != "/repo/root" {
		t.Fatalf("expected default workdir /repo/root, got %#v", runner.calls)
	}
}

func TestServiceLaunchPropagatesRunnerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("spawn failed")
	runner := &recordingRunner{err: wantErr}
	service, err := NewService([]Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	err = service.Launch(context.Background(), "editor", domain.IssueDetail{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestServiceLaunchReturnsErrorForUnknownAction(t *testing.T) {
	t.Parallel()

	service, err := NewService([]Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", &recordingRunner{})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	err = service.Launch(context.Background(), "missing", domain.IssueDetail{})
	if err == nil {
		t.Fatal("expected error for undefined action")
	}
}

func TestServiceBuiltInDefinitionsForV1Actions(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	service, err := NewService([]Definition{
		{Action: "nvim", Command: "nvim", Args: []string{"[{{issue.id}}]", "{{issue.title}}"}},
		{Action: "opencode", Command: "opencode", Args: []string{"run", "--issue", "{{issue.id}}", "--title", "{{issue.title}}"}},
		{Action: "shell-command", Command: "sh", Args: []string{"-lc", "echo {{issue.id}}"}},
	}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-42", Title: "Launcher task"}}

	if err := service.Launch(context.Background(), "nvim", issue); err != nil {
		t.Fatalf("Launch nvim returned error: %v", err)
	}
	if err := service.Launch(context.Background(), "opencode", issue); err != nil {
		t.Fatalf("Launch opencode returned error: %v", err)
	}
	if err := service.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch shell-command returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected three launch calls, got %d", len(runner.calls))
	}

	if runner.calls[0].command != "nvim" || runner.calls[1].command != "opencode" || runner.calls[2].command != "sh" {
		t.Fatalf("unexpected built-in launcher commands: %#v", runner.calls)
	}
}

func TestNewServiceValidatesInputs(t *testing.T) {
	t.Parallel()

	_, err := NewService([]Definition{{Action: "", Command: "nvim"}}, "/repo/root", &recordingRunner{})
	if err == nil {
		t.Fatal("expected error for missing action")
	}

	_, err = NewService([]Definition{{Action: "editor", Command: ""}}, "/repo/root", &recordingRunner{})
	if err == nil {
		t.Fatal("expected error for missing command")
	}

	_, err = NewService([]Definition{{Action: "editor", Command: "nvim"}, {Action: "editor", Command: "vi"}}, "/repo/root", &recordingRunner{})
	if err == nil {
		t.Fatal("expected error for duplicate actions")
	}

	_, err = NewService([]Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", nil)
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}
