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
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	assertLiteralArg(t, runner.Calls()[0].Args, payload, sentinel)
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	assertLiteralArg(t, runner.Calls()[0].Args, payload, "")
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	assertLiteralArg(t, runner.Calls()[0].Args, payload, sentinel)
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	// Labels are comma-joined; the injected value should appear verbatim in args.
	assertLiteralArg(t, runner.Calls()[0].Args, label, sentinel)
}

// TestNewlineInAssigneeIsStripped asserts that \n in a field value is stripped
// before reaching argv. Option (a): all C0 control chars
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	// \n must be stripped; the sanitised value must NOT contain a newline.
	stripped := fmt.Sprintf("hanstouch %s", sentinel)
	assertLiteralArg(t, runner.Calls()[0].Args, stripped, sentinel)
}

// TestNewlineInTitleIsStrippedFromArgv asserts that a title containing \n has
// the newline character removed before reaching argv.
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	// \n is a C0 char and must be stripped; argv receives the joined form.
	assertLiteralArg(t, runner.Calls()[0].Args, "line1line2", "")
}

// TestANSIEscapeInTitleIsStrippedFromArgv asserts that \x1b (ESC) in a field
// value is removed before the value reaches argv.
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

	if len(runner.Calls()) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.Calls()))
	}
	// \x1b (ESC, 0x1b) is a C0 control char and must be stripped.
	assertLiteralArg(t, runner.Calls()[0].Args, "danger", "")
}

// TestC0StrippingBoundary pins the exact cut-off of the control-character
// filter. The other sanitisation tests sample points well inside the range
// (\n at 0x0a, ESC at 0x1b) and well outside it, so the boundary itself is the
// one value they never exercise — and it is the value an off-by-one moves.
//
// U+001F is the last C0 control character and must be stripped; U+0020 is the
// space and must survive, or every multi-word issue title reaches the child
// process mangled.
func TestC0StrippingBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"last control character is stripped", "before\x1fafter", "beforeafter"},
		{"space is kept", "before after", "before after"},
		{"first control character is stripped", "before\x00after", "beforeafter"},
		{"printable above the boundary is kept", "before!after", "before!after"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakes.FakeProcessRunner{}
			svc, err := launcher.NewService([]launcher.Definition{shellCommandDefinition()}, "/tmp", runner)
			if err != nil {
				t.Fatalf("NewService error: %v", err)
			}

			issue := domain.IssueDetail{Summary: domain.IssueSummary{ID: "sec-08", Title: tc.title}}
			if err := svc.Launch(context.Background(), "shell-command", issue); err != nil {
				t.Fatalf("Launch error: %v", err)
			}
			if len(runner.Calls()) != 1 {
				t.Fatalf("expected one call, got %d", len(runner.Calls()))
			}
			assertLiteralArg(t, runner.Calls()[0].Args, tc.want, "")
		})
	}
}

// TestEnvEntryMissingEqualsIsRejected asserts that an Env template that
// produces no "=" after interpolation causes Launch to return an error.
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
	if len(runner.Calls()) != 0 {
		t.Fatalf("runner must not be called when env validation fails, got %d calls", len(runner.Calls()))
	}
}

// --- Ex-command editors -----------------------------------------------------
//
// nvim re-parses a "+cmd" argument as an Ex command line. Ex chains on "|" and
// reaches a login shell through ":!", so an interpolated issue field there is
// executable text, exactly as a shell body is. These tests pin the validator
// against that shape; the shipped default is pinned in the config package.

func TestValidateRejectsIssueFieldInNvimExCommandArgument(t *testing.T) {
	def := launcher.Definition{
		Action:  "nvim",
		Command: "nvim",
		Args:    []string{`+call append(0, ["Title: {{issue.title}}"])`},
	}

	err := launcher.ValidateDefinitions([]launcher.Definition{def})
	if err == nil {
		t.Fatal("expected an error for an issue field in an nvim +cmd argument, got nil")
	}
	if !strings.Contains(err.Error(), "{{issue.title}}") {
		t.Errorf("error should name the offending placeholder, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Ex-command") {
		t.Errorf("error should say the argument is an Ex command line, got: %v", err)
	}
}

func TestValidateRejectsIssueFieldAfterEditorCommandFlag(t *testing.T) {
	for _, flag := range []string{"-c", "--cmd"} {
		t.Run(flag, func(t *testing.T) {
			def := launcher.Definition{
				Action:  "nvim",
				Command: "nvim",
				Args:    []string{flag, `call append(0, ["{{issue.id}}"])`},
			}
			if err := launcher.ValidateDefinitions([]launcher.Definition{def}); err == nil {
				t.Fatalf("expected an error for an issue field after %s, got nil", flag)
			}
		})
	}
}

func TestValidateAllowsIssueFieldInEditorFileArgument(t *testing.T) {
	// A file argument is data, not an Ex command line. Rejecting it would
	// forbid opening a per-issue file, which the rule does not intend.
	def := launcher.Definition{
		Action:  "nvim",
		Command: "nvim",
		Args:    []string{"/tmp/{{issue.id}}.md"},
	}
	if err := launcher.ValidateDefinitions([]launcher.Definition{def}); err != nil {
		t.Errorf("a file argument must be allowed, got: %v", err)
	}
}

func TestValidateChecksEveryExCommandEditorAlias(t *testing.T) {
	for _, name := range []string{"vi", "vim", "nvim", "view", "vimdiff", "ex", "/usr/bin/nvim"} {
		t.Run(name, func(t *testing.T) {
			def := launcher.Definition{
				Action:  "editor-like",
				Command: name,
				Args:    []string{`+call append(0, ["{{issue.title}}"])`},
			}
			if err := launcher.ValidateDefinitions([]launcher.Definition{def}); err == nil {
				t.Fatalf("expected %s to be checked for Ex-command injection", name)
			}
		})
	}
}
