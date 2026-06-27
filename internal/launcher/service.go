package launcher

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// envEntryRe validates that an interpolated Env entry has the form
// NAME=value where NAME follows POSIX env-variable naming conventions.
var envEntryRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)

// shellBaseNames are the POSIX-shell executables that interpret a -c/-lc script
// body. A launcher whose command is one of these can re-parse a body argument as
// code, which is the shell-injection surface the security rule guards.
var shellBaseNames = map[string]struct{}{
	"sh": {}, "bash": {}, "dash": {}, "zsh": {}, "ksh": {}, "ash": {}, "busybox": {},
}

// shellCommandFlagRe matches a single-dash shell flag bundle that contains the
// "command" option (c), e.g. -c, -lc, -ic, -lic. The argument immediately
// following such a flag is the script body.
var shellCommandFlagRe = regexp.MustCompile(`^-[a-z]*c[a-z]*$`)

// issueFieldPlaceholders are the operator-untrusted interpolation placeholders.
// They carry issue content (id, title, labels, assignee) that anyone able to
// file or edit an issue controls. They must never be interpolated into a shell
// body; only project.root is operator-trusted. See docs/CODING.md
// "Shell-launcher security rule".
var issueFieldPlaceholders = []string{
	"{{issue.id}}",
	"{{issue.title}}",
	"{{issue.labels}}",
	"{{issue.assignee}}",
}

// validateShellBodySafety enforces the shell-launcher security invariant: when a
// launcher invokes a POSIX shell with a -c/-lc body, that body must not contain
// any issue-field placeholder. Issue fields are operator-untrusted input;
// interpolating them into a re-parsed shell body allows command injection / RCE
// via issue content. Operators must pass issue fields as positional arguments
// after the body and reference them via $1, $2, … instead. Enforced at
// definition-build time so a dangerous config fails fast at startup rather than
// silently shelling out attacker-controlled issue content at launch.
//
// The whole argv (command + args) is scanned, not just the leading command, so
// an exec wrapper that fronts the shell — e.g. `env sh -c …`, `/usr/bin/env bash
// -lc …`, `timeout 10 sh -c …`, `nice -n5 sh -c …` — cannot smuggle the same
// injection past a command-only check. Anywhere a shell token is followed by a
// -c-style flag, the body argument after that flag is checked.
//
// Note: issue-field placeholders inside Env entries are intentionally NOT
// rejected — passing issue data through environment variables is a documented
// safe pattern (see the built-in "opencode" launcher) because env values are not
// re-parsed as shell code. An operator who deliberately writes `eval "$VAR"` in a
// shell body re-introduces the risk; that is out of scope for this static check.
func validateShellBodySafety(def Definition) error {
	// Combined token stream: the command plus its args. A wrapper prefix (env,
	// timeout, nice, …) simply appears before the shell token and is skipped over.
	tokens := make([]string, 0, len(def.Args)+1)
	tokens = append(tokens, def.Command)
	tokens = append(tokens, def.Args...)

	for i, tok := range tokens {
		if !isShellCommandName(tok) {
			continue
		}
		// Find this shell's -c/-lc flag. POSIX shells place the script body in the
		// argument immediately after the command flag; tolerate intervening shell
		// options (e.g. `sh -l -c BODY`) but stop at the first non-flag token.
		for j := i + 1; j < len(tokens); j++ {
			t := strings.TrimSpace(tokens[j])
			if shellCommandFlagRe.MatchString(t) {
				if j+1 < len(tokens) {
					if ph := issuePlaceholderIn(tokens[j+1]); ph != "" {
						return fmt.Errorf(
							"launcher action %q: issue-field placeholder %s must not be interpolated into a shell %q body (command-injection risk); pass it as a positional argument after the body and reference it via $1/$2/… instead",
							strings.TrimSpace(def.Action), ph, t,
						)
					}
				}
				break // found this shell's command flag; done with it
			}
			if strings.HasPrefix(t, "-") {
				continue // another shell option (e.g. -l, -i); keep looking for -c
			}
			break // a non-flag token before any -c: not a -c invocation
		}
	}
	return nil
}

// issuePlaceholderIn returns the first issue-field placeholder found in s, or ""
// when none is present.
func issuePlaceholderIn(s string) string {
	for _, ph := range issueFieldPlaceholders {
		if strings.Contains(s, ph) {
			return ph
		}
	}
	return ""
}

// isShellCommandName reports whether command names a POSIX shell (by basename),
// ignoring any directory path. The command template is matched as-is (before
// interpolation); shell commands in practice are literal (e.g. "sh", "/bin/sh").
func isShellCommandName(command string) bool {
	name := strings.TrimSpace(command)
	if name == "" {
		return false
	}
	if idx := strings.LastIndexAny(name, "/\\"); idx >= 0 {
		name = name[idx+1:]
	}
	_, ok := shellBaseNames[name]
	return ok
}

// stripC0 removes all C0 control characters (U+0000–U+001F) from s.
// This prevents ANSI-escape injection, newline injection into env entries, and
// NUL-byte issues in argv before values reach the child process.
func stripC0(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Service launches external tools using an issue context.
type Service interface {
	Launch(ctx context.Context, action string, issue domain.IssueDetail) error
}

// ProcessRunner is the subprocess boundary used by launchers.
//
// Implementations should start a process and return immediately. The launcher
// service intentionally does not wait, poll, or coordinate launched processes.
type ProcessRunner interface {
	Run(ctx context.Context, command string, args []string, dir string, env []string) error
}

// Definition describes one launcher action template.
type Definition struct {
	Action  string
	Command string
	Args    []string
	Env     []string
	WorkDir string
}

// InterpolationContext provides structured values available to launcher
// templates.
type InterpolationContext struct {
	IssueID       string
	IssueTitle    string
	IssueLabels   []string
	IssueAssignee string
	ProjectRoot   string
}

// Placeholders returns the supported interpolation placeholders.
func (c InterpolationContext) Placeholders() map[string]string {
	return map[string]string{
		"{{issue.id}}":       c.IssueID,
		"{{issue.title}}":    c.IssueTitle,
		"{{issue.labels}}":   strings.Join(c.IssueLabels, ","),
		"{{issue.assignee}}": c.IssueAssignee,
		"{{project.root}}":   c.ProjectRoot,
	}
}

// definitionResolver resolves launcher definitions from action names.
type definitionResolver struct {
	definitions map[string]Definition
}

// newDefinitionResolver indexes launcher definitions by action.
func newDefinitionResolver(definitions []Definition) (definitionResolver, error) {
	indexed := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		action := strings.TrimSpace(definition.Action)
		if action == "" {
			return definitionResolver{}, errors.New("launcher action is required")
		}
		if strings.TrimSpace(definition.Command) == "" {
			return definitionResolver{}, fmt.Errorf("launcher command is required for action %q", action)
		}
		if err := validateShellBodySafety(definition); err != nil {
			return definitionResolver{}, err
		}
		if _, exists := indexed[action]; exists {
			return definitionResolver{}, fmt.Errorf("duplicate launcher action %q", action)
		}

		indexed[action] = definition
	}

	return definitionResolver{definitions: indexed}, nil
}

// Resolve returns a definition for the requested action.
func (r definitionResolver) Resolve(action string) (Definition, bool) {
	definition, ok := r.definitions[action]
	return definition, ok
}

// templateInterpolator interpolates supported launcher placeholders.
type templateInterpolator struct{}

// Interpolate substitutes placeholders in input using the provided context.
// C0 control characters (\x00–\x1f) are stripped from each substituted value
// before insertion to prevent ANSI/newline injection in argv and env entries.
func (templateInterpolator) Interpolate(input string, ctx InterpolationContext) string {
	placeholders := ctx.Placeholders()
	keys := make([]string, 0, len(placeholders))
	for key := range placeholders {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	value := input
	for _, key := range keys {
		value = strings.ReplaceAll(value, key, stripC0(placeholders[key]))
	}

	return value
}

type launcherService struct {
	resolver     definitionResolver
	runner       ProcessRunner
	interpolator templateInterpolator
	projectRoot  string
}

// NewService builds a launcher service that resolves action definitions and
// starts external tools using the configured process runner.
func NewService(definitions []Definition, projectRoot string, runner ProcessRunner) (Service, error) {
	if runner == nil {
		return nil, errors.New("process runner is required")
	}

	resolver, err := newDefinitionResolver(definitions)
	if err != nil {
		return nil, err
	}

	return launcherService{
		resolver:     resolver,
		runner:       runner,
		interpolator: templateInterpolator{},
		projectRoot:  projectRoot,
	}, nil
}

// Launch resolves the action and starts a subprocess without waiting.
func (s launcherService) Launch(ctx context.Context, action string, issue domain.IssueDetail) error {
	definition, ok := s.resolver.Resolve(action)
	if !ok {
		return fmt.Errorf("launcher action %q is not defined", action)
	}

	interpolationContext := InterpolationContext{
		IssueID:       issue.Summary.ID,
		IssueTitle:    issue.Summary.Title,
		IssueLabels:   append([]string(nil), issue.Summary.Labels...),
		IssueAssignee: issue.Summary.Assignee,
		ProjectRoot:   s.projectRoot,
	}

	command := s.interpolator.Interpolate(definition.Command, interpolationContext)
	args := make([]string, 0, len(definition.Args))
	for _, arg := range definition.Args {
		args = append(args, s.interpolator.Interpolate(arg, interpolationContext))
	}

	env := make([]string, 0, len(definition.Env))
	for _, entry := range definition.Env {
		interpolated := s.interpolator.Interpolate(entry, interpolationContext)
		if !envEntryRe.MatchString(interpolated) {
			return fmt.Errorf("launcher action %q: invalid env entry %q: must match NAME=value", action, interpolated)
		}
		env = append(env, interpolated)
	}

	dir := strings.TrimSpace(definition.WorkDir)
	if dir == "" {
		dir = s.projectRoot
	} else {
		dir = s.interpolator.Interpolate(dir, interpolationContext)
	}

	return s.runner.Run(ctx, command, args, dir, env)
}
