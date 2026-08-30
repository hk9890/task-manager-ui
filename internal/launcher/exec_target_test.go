package launcher_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// TestValidateDefinitionsRejectsAnIssuePlaceholderChoosingTheProgram pins the
// half of the shell-launcher security rule that covers the exec targets
// themselves. validateShellBodySafety only ever asked whether a token was a
// shell that re-parses its argument, so a placeholder in `command` or `workdir`
// — both interpolated by Launch and handed to exec.Command — passed startup and
// --check-config untouched.
//
// The rule is that the operator writes the literal that fixes the program and
// the directory; an issue field may only extend it.
func TestValidateDefinitionsRejectsAnIssuePlaceholderChoosingTheProgram(t *testing.T) {
	t.Parallel()

	rejected := []struct {
		name string
		def  launcher.Definition
	}{
		{"command is a bare issue field", launcher.Definition{Action: "tool", Command: "{{issue.assignee}}"}},
		{"command is a bare issue title", launcher.Definition{Action: "tool", Command: "{{issue.title}} --flag"}},
		{"workdir is a bare issue field", launcher.Definition{Action: "tool", Command: "tool", WorkDir: "{{issue.id}}"}},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := launcher.ValidateDefinitions([]launcher.Definition{tc.def})
			if err == nil {
				t.Fatalf("ValidateDefinitions(%+v) = nil, want a rejection", tc.def)
			}
			if !strings.Contains(err.Error(), "arbitrary-execution risk") {
				t.Errorf("error does not explain the rule: %v", err)
			}
		})
	}

	accepted := []struct {
		name string
		def  launcher.Definition
	}{
		{"literal prefix on the command", launcher.Definition{Action: "tool", Command: "/opt/tools/run-{{issue.id}}"}},
		{"project root prefix on the workdir", launcher.Definition{Action: "tool", Command: "tool", WorkDir: "{{project.root}}/{{issue.id}}"}},
		{"no placeholders at all", launcher.Definition{Action: "tool", Command: "tool", Args: []string{"{{issue.title}}"}}},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := launcher.ValidateDefinitions([]launcher.Definition{tc.def}); err != nil {
				t.Errorf("ValidateDefinitions(%+v) = %v, want it accepted", tc.def, err)
			}
		})
	}
}

// TestLaunchRefusesAnIssueFieldThatWalksOutOfTheCommandPath pins the launch-time
// half: the operator's literal prefix is what confines the target, and an issue
// field must not introduce a path separator that walks out of it.
func TestLaunchRefusesAnIssueFieldThatWalksOutOfTheCommandPath(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	service, err := launcher.NewService([]launcher.Definition{
		{Action: "tool", Command: "/opt/tools/run-{{issue.assignee}}"},
		{Action: "in-dir", Command: "tool", WorkDir: "{{project.root}}/{{issue.assignee}}"},
	}, "/repo/root", runner)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	escaping := domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Assignee: "../../bin/sh"}}

	for _, action := range []string{"tool", "in-dir"} {
		if err := service.Launch(context.Background(), action, escaping); err == nil {
			t.Errorf("Launch(%q) with an escaping issue field = nil, want a refusal", action)
		}
	}
	if len(runner.Calls()) != 0 {
		t.Errorf("expected no process started, got %d", len(runner.Calls()))
	}

	// An ordinary value still launches, and still interpolates.
	ordinary := domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Assignee: "hans"}}
	if err := service.Launch(context.Background(), "tool", ordinary); err != nil {
		t.Fatalf("Launch with an ordinary assignee: %v", err)
	}
	calls := runner.Calls()
	if len(calls) != 1 || calls[0].Command != "/opt/tools/run-hans" {
		t.Errorf("expected the interpolated command /opt/tools/run-hans, got %+v", calls)
	}
}
