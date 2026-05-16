package launcher_test

// service_security_test.go verifies that shell-injection-style payloads in
// issue fields are passed as literal data to the child process, never
// re-parsed as shell code by sh -lc.
//
// The shell-command launcher template uses positional args ($0..$N) so that
// sh receives issue field values as arguments, not as part of the -lc body.
// These tests confirm that the FakeProcessRunner receives the raw payload
// strings unchanged and that no side-effecting file is created.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/launcher"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// shellCommandDefinition returns the positional-arg shell-command definition
// that mirrors the safe built-in default: issue fields are args, not body.
func shellCommandDefinition() launcher.Definition {
	return launcher.Definition{
		Action:  "shell-command",
		Command: "sh",
		Args: []string{
			"-lc",
			"printf 'issue=%s\\ntitle=%s\\nassignee=%s\\nlabels=%s\\n' \"$0\" \"$1\" \"$2\" \"$3\"",
			"{{issue.id}}",
			"{{issue.title}}",
			"{{issue.assignee}}",
			"{{issue.labels}}",
		},
		WorkDir: "/tmp",
	}
}

// assertLiteralArg checks that the expected string is present as a literal
// element in args and that no side-effecting file was created at sideEffectPath.
func assertLiteralArg(t *testing.T, args []string, expected, sideEffectPath string) {
	t.Helper()

	found := false
	for _, a := range args {
		if a == expected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected literal payload %q in argv %#v", expected, args)
	}

	if sideEffectPath != "" {
		if _, err := os.Stat(sideEffectPath); err == nil {
			t.Errorf("side-effect file %q was created — injection occurred", sideEffectPath)
			os.Remove(sideEffectPath) // clean up so repeated runs don't false-negative
		}
	}
}

func TestShellCommandLauncherDoesNotExecuteInjectedTitle(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	sentinel := filepath.Join(t.TempDir(), "pwned_title")
	payload := fmt.Sprintf("$(touch %s)", sentinel)
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:    "sec-01",
		Title: payload,
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	assertLiteralArg(t, runner.Calls[0].Args, payload, sentinel)
}

func TestShellCommandLauncherDoesNotExecuteInjectedTitleQuotedSemicolon(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	payload := `";rm -rf /;"`
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:    "sec-02",
		Title: payload,
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	assertLiteralArg(t, runner.Calls[0].Args, payload, "")
}

func TestShellCommandLauncherDoesNotExecuteBackticksInTitle(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	sentinel := filepath.Join(t.TempDir(), "pwned_backtick")
	payload := fmt.Sprintf("`touch %s`", sentinel)
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:    "sec-03",
		Title: payload,
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	assertLiteralArg(t, runner.Calls[0].Args, payload, sentinel)
}

func TestShellCommandLauncherDoesNotExecuteAndAndOrInLabels(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	// && and || are dangerous only when interpolated into the shell body.
	// As positional args they must arrive literally.
	sentinel := filepath.Join(t.TempDir(), "pwned_and")
	label := fmt.Sprintf("area:security && touch %s || true", sentinel)
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:     "sec-04",
		Title:  "safe",
		Labels: []string{label},
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	// Labels are comma-joined; the injected value should appear verbatim in args.
	assertLiteralArg(t, runner.Calls[0].Args, label, sentinel)
}

// TestNewlineInAssigneeIsStripped asserts that \n in a field value is stripped
// before reaching argv. Option (a) from ticket db0z.8: all C0 control chars
// including \x0a (newline) are removed; the sanitised value arrives without the
// newline so log/ANSI injection via env or argv is not possible.
func TestNewlineInAssigneeIsStripped(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	sentinel := filepath.Join(t.TempDir(), "pwned_newline")
	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:       "sec-05",
		Title:    "safe",
		Assignee: fmt.Sprintf("hans\ntouch %s", sentinel),
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	// \n must be stripped; the sanitised value must NOT contain a newline.
	stripped := fmt.Sprintf("hanstouch %s", sentinel)
	assertLiteralArg(t, runner.Calls[0].Args, stripped, sentinel)
}

// TestNewlineInTitleIsStrippedFromArgv asserts that a title containing \n has
// the newline character removed before reaching argv (db0z.8 AC step 3).
func TestNewlineInTitleIsStrippedFromArgv(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:    "sec-06",
		Title: "line1\nline2",
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	// \n is a C0 char and must be stripped; argv receives the joined form.
	assertLiteralArg(t, runner.Calls[0].Args, "line1line2", "")
}

// TestANSIEscapeInTitleIsStrippedFromArgv asserts that \x1b (ESC) in a field
// value is removed before the value reaches argv (db0z.8 AC step 3).
func TestANSIEscapeInTitleIsStrippedFromArgv(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	issue := domain.IssueDetail{Summary: domain.IssueSummary{
		ID:    "sec-07",
		Title: "\x1bdanger",
	}}

	if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
		t.Fatalf("Launch error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls))
	}
	// \x1b (ESC, 0x1b) is a C0 control char and must be stripped.
	assertLiteralArg(t, runner.Calls[0].Args, "danger", "")
}

// TestEnvEntryMissingEqualsIsRejected asserts that an Env template that
// produces no "=" after interpolation causes Launch to return an error
// (db0z.8 AC step 1).
func TestEnvEntryMissingEqualsIsRejected(t *testing.T) {
	t.Parallel()

	runner := &fakes.FakeProcessRunner{}
	svc, err := launcher.NewService([]launcher.Definition{{
		Action:  "bad-env",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Env:     []string{"NO_EQ"},
	}}, "/tmp", runner)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "sec-08", Title: "t"}}
	err = svc.Launch(context.Background(), "bad-env", issue)
	if err == nil {
		t.Fatal("expected error for env entry without '=', got nil")
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("runner must not be called when env validation fails, got %d calls", len(runner.Calls))
	}
}
