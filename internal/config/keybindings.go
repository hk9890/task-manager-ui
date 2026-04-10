package config

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	ShellContext = "shell"
	BoardContext = "board"
)

const (
	ShellActionQuit           = "quit"
	ShellActionHelp           = "toggle_help"
	ShellActionModeBoard      = "mode_board"
	ShellActionModeSearch     = "mode_search"
	ShellActionToggleSearch   = "toggle_search"
	ShellActionModeDetail     = "mode_detail"
	ShellActionModeCycleNext  = "mode_cycle_next"
	ShellActionModeCyclePrev  = "mode_cycle_prev"
	ShellActionEscape         = "escape"
	ShellActionReloadDetail   = "reload_detail"
	ShellActionEditIssue      = "edit_issue"
	ShellActionCreateIssue    = "create_issue"
	ShellActionUpdateIssue    = "update_issue"
	ShellActionCloseIssue     = "close_issue"
	ShellActionCommentIssue   = "comment_issue"
	ShellActionLaunchNvim     = "launch_nvim"
	ShellActionLaunchOpencode = "launch_opencode"
	ShellActionLaunchShell    = "launch_shell_command"

	BoardActionMoveLeft   = "move_left"
	BoardActionMoveRight  = "move_right"
	BoardActionMoveUp     = "move_up"
	BoardActionMoveDown   = "move_down"
	BoardActionOpenDetail = "open_detail"
	BoardActionReload     = "reload"
)

type KeyBindings struct {
	Shell map[string][]string
	Board map[string][]string
}

type KeyBindingOverride struct {
	Shell map[string][]string `yaml:"shell"`
	Board map[string][]string `yaml:"board"`
}

type ResolvedKeyBindings struct {
	contexts map[string]ContextBindings
	keysByID map[string]string
}

type ContextBindings struct {
	actions map[string]ActionBinding
	index   map[string]string
}

type ActionBinding struct {
	Action string
	Keys   []string
	Set    map[string]struct{}
}

func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
		Shell: map[string][]string{
			ShellActionQuit:           {"q", "ctrl+c"},
			ShellActionHelp:           {"?"},
			ShellActionModeBoard:      {"1", "b"},
			ShellActionModeSearch:     {"2", "s"},
			ShellActionToggleSearch:   {"ctrl+@"},
			ShellActionModeDetail:     {"3"},
			ShellActionModeCycleNext:  {"tab"},
			ShellActionModeCyclePrev:  {"shift+tab"},
			ShellActionEscape:         {"esc"},
			ShellActionReloadDetail:   {"r"},
			ShellActionEditIssue:      {"e"},
			ShellActionCreateIssue:    {"c"},
			ShellActionUpdateIssue:    {"u"},
			ShellActionCloseIssue:     {"x"},
			ShellActionCommentIssue:   {"a"},
			ShellActionLaunchNvim:     {"n"},
			ShellActionLaunchOpencode: {"p"},
			ShellActionLaunchShell:    {"l"},
		},
		Board: map[string][]string{
			BoardActionMoveLeft:   {"h", "left"},
			BoardActionMoveRight:  {"l", "right", "tab"},
			BoardActionMoveUp:     {"k", "up"},
			BoardActionMoveDown:   {"j", "down"},
			BoardActionOpenDetail: {"enter", "o"},
			BoardActionReload:     {"r"},
		},
	}
}

func (k KeyBindings) Clone() KeyBindings {
	return KeyBindings{
		Shell: cloneBindingMap(k.Shell),
		Board: cloneBindingMap(k.Board),
	}
}

func MergeKeyBindings(base KeyBindings, override *KeyBindingOverride) KeyBindings {
	merged := base.Clone()
	if override == nil {
		return merged
	}
	if override.Shell != nil {
		for action, keys := range override.Shell {
			merged.Shell[action] = cloneStringSlice(keys)
		}
	}
	if override.Board != nil {
		for action, keys := range override.Board {
			merged.Board[action] = cloneStringSlice(keys)
		}
	}
	return merged
}

func ResolveKeyBindings(k KeyBindings) (ResolvedKeyBindings, error) {
	contexts := map[string]map[string][]string{
		ShellContext: k.Shell,
		BoardContext: k.Board,
	}

	resolved := ResolvedKeyBindings{
		contexts: make(map[string]ContextBindings, len(contexts)),
		keysByID: make(map[string]string),
	}

	for _, context := range []string{ShellContext, BoardContext} {
		bindings, err := buildContextBindings(context, contexts[context], allowedActionsForContext(context))
		if err != nil {
			return ResolvedKeyBindings{}, err
		}
		resolved.contexts[context] = bindings
	}

	resolved.keysByID[string(ShellContext)+":"+ShellActionModeBoard] = firstBinding(k.Shell[ShellActionModeBoard])
	resolved.keysByID[string(ShellContext)+":"+ShellActionModeSearch] = firstBinding(k.Shell[ShellActionModeSearch])
	resolved.keysByID[string(ShellContext)+":"+ShellActionModeDetail] = firstBinding(k.Shell[ShellActionModeDetail])
	return resolved, nil
}

func (r ResolvedKeyBindings) Match(context, action string, msg tea.KeyMsg) bool {
	ctx, ok := r.contexts[context]
	if !ok {
		return false
	}
	binding, ok := ctx.actions[action]
	if !ok {
		return false
	}
	_, exists := binding.Set[canonicalKeyName(msg.String())]
	return exists
}

func (r ResolvedKeyBindings) Keys(context, action string) []string {
	ctx, ok := r.contexts[context]
	if !ok {
		return nil
	}
	binding, ok := ctx.actions[action]
	if !ok {
		return nil
	}
	return cloneStringSlice(binding.Keys)
}

func (r ResolvedKeyBindings) Primary(context, action string) string {
	keys := r.Keys(context, action)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func (r ResolvedKeyBindings) DisplayPrimary(context, action string) string {
	return DisplayKeyName(r.Primary(context, action))
}

func (r ResolvedKeyBindings) DisplayLabel(context, action string) string {
	keys := r.Keys(context, action)
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, DisplayKeyName(key))
	}
	return strings.Join(parts, "/")
}

func (r ResolvedKeyBindings) Label(context, action string) string {
	keys := r.Keys(context, action)
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, "/")
}

func (r ResolvedKeyBindings) ModeBoardKey() string {
	return r.keysByID[ShellContext+":"+ShellActionModeBoard]
}

func (r ResolvedKeyBindings) ModeSearchKey() string {
	return r.keysByID[ShellContext+":"+ShellActionModeSearch]
}

func (r ResolvedKeyBindings) ModeDetailKey() string {
	return r.keysByID[ShellContext+":"+ShellActionModeDetail]
}

func cloneBindingMap(input map[string][]string) map[string][]string {
	if input == nil {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, values := range input {
		out[key] = cloneStringSlice(values)
	}
	return out
}

func buildContextBindings(context string, input map[string][]string, allowed map[string]struct{}) (ContextBindings, error) {
	ctx := ContextBindings{actions: make(map[string]ActionBinding, len(input)), index: make(map[string]string)}
	actions := make([]string, 0, len(input))
	for action := range input {
		actions = append(actions, action)
	}
	sort.Strings(actions)

	for _, action := range actions {
		if _, ok := allowed[action]; !ok {
			return ContextBindings{}, fmt.Errorf("unknown keybinding action %q in %s context", action, context)
		}
		keys := input[action]
		if len(keys) == 0 {
			return ContextBindings{}, fmt.Errorf("keybinding action %q in %s context must define at least one key", action, context)
		}
		binding := ActionBinding{Action: action, Keys: make([]string, 0, len(keys)), Set: make(map[string]struct{}, len(keys))}
		for _, raw := range keys {
			key := canonicalKeyName(raw)
			if !isValidKeyName(key) {
				return ContextBindings{}, fmt.Errorf("invalid key %q for action %q in %s context", raw, action, context)
			}
			if existing, exists := ctx.index[key]; exists {
				return ContextBindings{}, fmt.Errorf("key %q conflicts between actions %q and %q in %s context", key, existing, action, context)
			}
			ctx.index[key] = action
			binding.Keys = append(binding.Keys, key)
			binding.Set[key] = struct{}{}
		}
		ctx.actions[action] = binding
	}

	for action := range allowed {
		if _, ok := ctx.actions[action]; !ok {
			return ContextBindings{}, fmt.Errorf("missing keybinding action %q in %s context", action, context)
		}
	}

	return ctx, nil
}

func isValidKeyName(key string) bool {
	if key == "" {
		return false
	}
	if strings.Contains(key, " ") {
		return key == "space"
	}
	return true
}

func canonicalKeyName(key string) string {
	trimmed := strings.TrimSpace(key)
	if key == " " {
		return "space"
	}
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "space":
		return "space"
	case "ctrl+space", "ctrl+@":
		return "ctrl+@"
	}
	if len([]rune(trimmed)) == 1 {
		return trimmed
	}
	return lower
}

func DisplayKeyName(key string) string {
	switch canonicalKeyName(key) {
	case "ctrl+@":
		return "ctrl+space"
	default:
		return canonicalKeyName(key)
	}
}

func allowedActionsForContext(context string) map[string]struct{} {
	allowed := make(map[string]struct{})
	switch context {
	case ShellContext:
		for _, action := range []string{
			ShellActionQuit,
			ShellActionHelp,
			ShellActionModeBoard,
			ShellActionModeSearch,
			ShellActionToggleSearch,
			ShellActionModeDetail,
			ShellActionModeCycleNext,
			ShellActionModeCyclePrev,
			ShellActionEscape,
			ShellActionReloadDetail,
			ShellActionEditIssue,
			ShellActionCreateIssue,
			ShellActionUpdateIssue,
			ShellActionCloseIssue,
			ShellActionCommentIssue,
			ShellActionLaunchNvim,
			ShellActionLaunchOpencode,
			ShellActionLaunchShell,
		} {
			allowed[action] = struct{}{}
		}
	case BoardContext:
		for _, action := range []string{
			BoardActionMoveLeft,
			BoardActionMoveRight,
			BoardActionMoveUp,
			BoardActionMoveDown,
			BoardActionOpenDetail,
			BoardActionReload,
		} {
			allowed[action] = struct{}{}
		}
	}
	return allowed
}

func firstBinding(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}
