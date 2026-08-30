package launcher

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// envEntryRe validates that an interpolated Env entry has the form
// NAME=value where NAME follows POSIX env-variable naming conventions.
var envEntryRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)

// shellBaseNames are the executables that interpret the argument after a
// -c/-lc-style flag as a command line. A launcher whose argv contains one of
// these can re-parse a body argument as code, which is the shell-injection
// surface the security rule guards.
//
// The list is not only POSIX shells: `su -c` and `python -c` take an executed
// string in exactly the same position. Interpreters whose execute flag is not
// -c (perl -e, ruby -e, node -e) are out of scope of this static check, as is an
// operator who writes `eval "$VAR"` inside a body.
var shellBaseNames = map[string]struct{}{
	"sh": {}, "bash": {}, "dash": {}, "zsh": {}, "ksh": {}, "ash": {}, "busybox": {},
	"csh": {}, "tcsh": {}, "fish": {}, "su": {}, "python": {}, "python3": {},
}

// shellDispatchBaseNames are the executables that hand a plain argument to a
// shell with no -c flag to mark it: tmux joins the trailing arguments of
// new-window / new-session / split-window / run-shell and parses them with
// /bin/sh, ssh runs its command string through the remote login shell, and watch
// runs its argument through sh. There is no fixed body position to check, so
// every argument of such a command is treated as a shell body.
var shellDispatchBaseNames = map[string]struct{}{
	"ssh": {}, "tmux": {}, "watch": {},
}

// exCommandEditorBaseNames are the editors that re-parse a +cmd, -c or --cmd
// argument as an Ex command line rather than as data. Ex chains on "|" and
// reaches a login shell through ":!", so an interpolated issue field in one of
// those arguments is executable text, exactly as a shell body is. A file
// argument to the same editor is data and is not checked.
//
// Editors whose execute flag is neither -c nor +cmd (emacs --eval, for example)
// are out of scope of this static check, as are the interpreters named in the
// shellBaseNames comment.
var exCommandEditorBaseNames = map[string]struct{}{
	"vi": {}, "vim": {}, "nvim": {}, "view": {}, "vimdiff": {}, "ex": {},
}

// exCommandFlagRe matches the flags whose following argument nvim and vim
// execute as an Ex command line.
var exCommandFlagRe = regexp.MustCompile(`^(-c|--cmd)$`)

// shellCommandFlagRe matches a single-dash shell flag bundle that contains the
// "command" option (c), e.g. -c, -lc, -ic, -lic. The argument immediately
// following such a flag is the script body.
var shellCommandFlagRe = regexp.MustCompile(`^-[a-z]*c[a-z]*$`)

// projectRootPlaceholder is the one operator-trusted placeholder: it resolves to
// the store's project path, never to issue content.
const projectRootPlaceholder = "{{project.root}}"

// issueFieldPlaceholders returns the operator-untrusted interpolation
// placeholders — everything a person able to file or edit an issue controls.
// They must never be interpolated into a body that is re-parsed as code. See
// docs/CODING.md "Shell-launcher security rule".
//
// Derived from InterpolationContext.Placeholders rather than listed a second
// time, so a newly supported {{issue.*}} placeholder is guarded the moment it
// exists.
func issueFieldPlaceholders() []string {
	all := InterpolationContext{}.Placeholders()
	out := make([]string, 0, len(all))
	for key := range all {
		if key == projectRootPlaceholder {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// validateShellBodySafety enforces the shell-launcher security invariant: an
// argument a launcher hands to something that re-parses it as a command line
// must not contain any issue-field placeholder. That covers two shapes — a
// -c/-lc body (shellBaseNames) and a plain argument dispatched to a shell
// (shellDispatchBaseNames, e.g. `tmux new-window`, `ssh host`, `watch`).
// Issue fields are operator-untrusted input;
// interpolating them into a re-parsed shell body allows command injection / RCE
// via issue content. Operators must pass issue fields as positional arguments
// after the body and reference them via $1, $2, … instead. Enforced at
// definition-build time so a dangerous config fails fast at startup rather than
// silently shelling out attacker-controlled issue content at launch.
//
// The whole argv (command + args) is scanned for a shell token, not just the
// leading command, so an exec wrapper that fronts the shell — e.g. `env sh -c …`,
// `/usr/bin/env bash -lc …`, `timeout 10 sh -c …`, `nice -n5 sh -c …` — cannot
// smuggle the same injection past a command-only check. Anywhere a shell token
// is followed by a -c-style flag, the body argument after that flag is checked.
//
// What the executable itself is named is a separate question, and
// validateExecTargetSafety below answers it.
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
		if isExCommandEditorName(tok) {
			// Only the Ex-command arguments are code: a "+cmd" argument carries
			// its body inline, and -c/--cmd take it as the next argument.
			for j := i + 1; j < len(tokens); j++ {
				t := strings.TrimSpace(tokens[j])
				body := ""
				switch {
				case strings.HasPrefix(t, "+"):
					body = t
				case exCommandFlagRe.MatchString(t) && j+1 < len(tokens):
					body = tokens[j+1]
					j++
				default:
					continue
				}
				if ph := issuePlaceholderIn(body); ph != "" {
					return fmt.Errorf(
						"launcher action %q: issue-field placeholder %s must not be interpolated into an Ex-command argument of %q, which re-parses it as an editor command line (command-injection risk); pass the issue field through Env and read it back as $VAR instead",
						strings.TrimSpace(def.Action), ph, strings.TrimSpace(tok),
					)
				}
			}
			continue
		}
		if isShellDispatchCommandName(tok) {
			// No flag marks the body, so every following argument is one.
			for _, arg := range tokens[i+1:] {
				if ph := issuePlaceholderIn(arg); ph != "" {
					return fmt.Errorf(
						"launcher action %q: issue-field placeholder %s must not be interpolated into an argument of %q, which re-parses it as a shell command (command-injection risk); pass the issue field through Env instead",
						strings.TrimSpace(def.Action), ph, strings.TrimSpace(tok),
					)
				}
			}
			continue
		}
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

// validateExecTargetSafety enforces the other half of the shell-launcher
// security rule: an issue field may help NAME the program and its working
// directory — `command` and `workdir` are part of the documented interpolation
// surface (docs/CONFIGURATION.md) — but it must not be what CHOOSES them.
//
// Neither value is a body another program re-parses, so validateShellBodySafety
// never looked at either, and Launch interpolates both before exec.Command. The
// rule here is that the operator writes the literal that fixes the directory and
// the start of the name: a template that *begins* with an issue placeholder
// leaves the whole target to issue content — `command: "{{issue.assignee}}"`
// execs whatever that resolves to on PATH. Its other half is enforced at launch
// by interpolateExecTarget, which refuses a value that would introduce a path
// separator and walk out of the operator's directory.
func validateExecTargetSafety(def Definition) error {
	action := strings.TrimSpace(def.Action)

	if ph := leadingIssuePlaceholder(def.Command); ph != "" {
		return fmt.Errorf(
			"launcher action %q: command %q starts with the issue-field placeholder %s, which leaves the program itself to issue content (arbitrary-execution risk); write the literal program or directory first, as in \"/opt/tools/prefix-%s\"",
			action, strings.TrimSpace(def.Command), ph, ph,
		)
	}

	if ph := leadingIssuePlaceholder(def.WorkDir); ph != "" {
		return fmt.Errorf(
			"launcher action %q: workdir %q starts with the issue-field placeholder %s, which leaves the working directory to issue content (arbitrary-execution risk); start it with %s or a literal path",
			action, strings.TrimSpace(def.WorkDir), ph, projectRootPlaceholder,
		)
	}

	return nil
}

// leadingIssuePlaceholder returns the issue-field placeholder s starts with, or
// "" when it starts with anything else.
func leadingIssuePlaceholder(s string) string {
	trimmed := strings.TrimSpace(s)
	for _, ph := range issueFieldPlaceholders() {
		if strings.HasPrefix(trimmed, ph) {
			return ph
		}
	}
	return ""
}

// interpolateExecTarget interpolates a command or workdir template and refuses
// the launch when an issue field would introduce a path separator or a parent
// segment into it.
//
// The operator's literal prefix is what confines the target; without this an
// issue titled "../../bin/sh" walks straight out of it. Only the two exec
// targets are checked: an argument is data the program reads, not a path the
// kernel resolves.
func interpolateExecTarget(interpolator templateInterpolator, field, template string, ctx InterpolationContext) (string, error) {
	for placeholder, value := range ctx.Placeholders() {
		if placeholder == projectRootPlaceholder || !strings.Contains(template, placeholder) {
			continue
		}
		if escape := pathEscapeIn(value); escape != "" {
			return "", fmt.Errorf(
				"%s %q: issue field %s is %q, which contains %q — an issue must not redirect the program outside the path the launcher template names",
				field, template, placeholder, value, escape,
			)
		}
	}
	return interpolator.Interpolate(template, ctx), nil
}

// pathEscapeIn returns the first path-escaping element in value, or "".
func pathEscapeIn(value string) string {
	switch {
	case strings.ContainsRune(value, '/'):
		return "/"
	case strings.ContainsRune(value, '\\'):
		return "\\"
	case slices.Contains(strings.Split(value, string(filepath.Separator)), ".."):
		return ".."
	}
	return ""
}

// ValidateDefinitions reports whether a launcher definition set is usable and
// safe, without constructing a service or a process runner. --check-config uses
// it so the documented validation command rejects exactly what an interactive
// start rejects.
func ValidateDefinitions(definitions []Definition) error {
	_, err := newDefinitionResolver(definitions)
	return err
}

// issuePlaceholderIn returns the first issue-field placeholder found in s, or ""
// when none is present.
func issuePlaceholderIn(s string) string {
	for _, ph := range issueFieldPlaceholders() {
		if strings.Contains(s, ph) {
			return ph
		}
	}
	return ""
}

// isShellCommandName reports whether command names an executable that executes
// the argument after a -c-style flag (by basename), ignoring any directory path.
// The command template is matched as-is (before interpolation); these commands
// in practice are literal (e.g. "sh", "/bin/sh").
func isShellCommandName(command string) bool {
	return baseNameIn(command, shellBaseNames)
}

// isShellDispatchCommandName reports whether command names an executable that
// re-parses a plain argument as a shell command line.
func isShellDispatchCommandName(command string) bool {
	return baseNameIn(command, shellDispatchBaseNames)
}

// isExCommandEditorName reports whether command names an editor that executes
// its +cmd, -c and --cmd arguments as an Ex command line.
func isExCommandEditorName(command string) bool {
	return baseNameIn(command, exCommandEditorBaseNames)
}

func baseNameIn(command string, names map[string]struct{}) bool {
	name := strings.TrimSpace(command)
	if name == "" {
		return false
	}
	if idx := strings.LastIndexAny(name, "/\\"); idx >= 0 {
		name = name[idx+1:]
	}
	_, ok := names[name]
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
		"{{issue.id}}":         c.IssueID,
		"{{issue.title}}":      c.IssueTitle,
		"{{issue.labels}}":     strings.Join(c.IssueLabels, ","),
		"{{issue.assignee}}":   c.IssueAssignee,
		projectRootPlaceholder: c.ProjectRoot,
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
		if err := validateExecTargetSafety(definition); err != nil {
			return definitionResolver{}, err
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

	command, err := interpolateExecTarget(s.interpolator, "command", definition.Command, interpolationContext)
	if err != nil {
		return fmt.Errorf("launcher action %q: %w", action, err)
	}
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
		dir, err = interpolateExecTarget(s.interpolator, "workdir", dir, interpolationContext)
		if err != nil {
			return fmt.Errorf("launcher action %q: %w", action, err)
		}
	}

	return s.runner.Run(ctx, command, args, dir, env)
}
