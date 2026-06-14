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
