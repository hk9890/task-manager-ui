package config

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	ShellContext  = "shell"
	BoardContext  = "board"
	SearchContext = "search"
	DetailContext = "detail"
	ModalContext  = "modal"
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

	SearchActionMoveUp         = "move_up"
	SearchActionMoveDown       = "move_down"
	SearchActionFocusLeft      = "focus_left"
	SearchActionFocusRight     = "focus_right"
	SearchActionFocusQuery     = "focus_query"
	SearchActionReload         = "reload"
	SearchActionOpenDetail     = "open_detail"
	SearchActionCycleFocusNext = "cycle_focus_next"
	SearchActionCycleFocusPrev = "cycle_focus_prev"

	DetailActionScrollUp   = "scroll_up"
	DetailActionScrollDown = "scroll_down"
	DetailActionPageUp     = "page_up"
	DetailActionPageDown   = "page_down"
	DetailActionHome       = "home"
	DetailActionEnd        = "end"

	ModalActionNext   = "next"
	ModalActionPrev   = "prev"
	ModalActionLeft   = "left"
	ModalActionRight  = "right"
	ModalActionEnter  = "enter"
	ModalActionEscape = "escape"
)

type KeyBindings struct {
	Shell  map[string][]string
	Board  map[string][]string
	Search map[string][]string
	Detail map[string][]string
	Modal  map[string][]string
}

type KeyBindingOverride struct {
	Shell  map[string][]string `yaml:"shell"`
	Board  map[string][]string `yaml:"board"`
	Search map[string][]string `yaml:"search"`
	Detail map[string][]string `yaml:"detail"`
	Modal  map[string][]string `yaml:"modal"`
}

type ResolvedKeyBindings struct {
	contexts map[string]ContextBindings
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
			ShellActionQuit:           {"ctrl+q"},
			ShellActionHelp:           {"?"},
			ShellActionModeBoard:      {"f13"},
			ShellActionModeSearch:     {"f14"},
			ShellActionToggleSearch:   {"ctrl+@"},
			ShellActionModeDetail:     {"3"},
			ShellActionModeCycleNext:  {"f15"},
			ShellActionModeCyclePrev:  {"f16"},
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
		Search: map[string][]string{
			SearchActionMoveUp:         {"k", "up"},
			SearchActionMoveDown:       {"j", "down"},
			SearchActionFocusLeft:      {"h", "left"},
			SearchActionFocusRight:     {"l", "right"},
			SearchActionFocusQuery:     {"/"},
			SearchActionReload:         {"r"},
			SearchActionOpenDetail:     {"enter"},
			SearchActionCycleFocusNext: {"tab", "ctrl+j"},
			SearchActionCycleFocusPrev: {"shift+tab", "ctrl+k"},
		},
		Detail: map[string][]string{
			DetailActionScrollUp:   {"k", "up"},
			DetailActionScrollDown: {"j", "down"},
			DetailActionPageUp:     {"pgup"},
			DetailActionPageDown:   {"pgdown"},
			DetailActionHome:       {"home"},
			DetailActionEnd:        {"end"},
		},
		Modal: map[string][]string{
			ModalActionNext:   {"tab", "down"},
			ModalActionPrev:   {"shift+tab", "up"},
			ModalActionLeft:   {"left"},
			ModalActionRight:  {"right"},
			ModalActionEnter:  {"enter"},
			ModalActionEscape: {"esc"},
		},
	}
}

func (k KeyBindings) Clone() KeyBindings {
	return KeyBindings{
		Shell:  cloneBindingMap(k.Shell),
		Board:  cloneBindingMap(k.Board),
		Search: cloneBindingMap(k.Search),
		Detail: cloneBindingMap(k.Detail),
		Modal:  cloneBindingMap(k.Modal),
	}
}

func MergeKeyBindings(base KeyBindings, override *KeyBindingOverride) KeyBindings {
	merged := base.Clone()
	if override == nil {
		return merged
	}
	merged.Shell = mergeContextBindingsInPlace(merged.Shell, override.Shell)
	merged.Board = mergeContextBindingsInPlace(merged.Board, override.Board)
	merged.Search = mergeContextBindingsInPlace(merged.Search, override.Search)
	merged.Detail = mergeContextBindingsInPlace(merged.Detail, override.Detail)
	merged.Modal = mergeContextBindingsInPlace(merged.Modal, override.Modal)
	return merged
}

// mergeContextBindingsInPlace merges override entries into base in place and returns base.
// Callers must ensure base is a freshly cloned map (not a shared default), as this
// function mutates its first argument.
func mergeContextBindingsInPlace(base, override map[string][]string) map[string][]string {
	if override == nil {
		return base
	}

	for action, keys := range override {
		base[action] = cloneStringSlice(keys)
	}

	return base
}

func ResolveKeyBindings(k KeyBindings) (ResolvedKeyBindings, error) {
	contexts := map[string]map[string][]string{
		ShellContext:  k.Shell,
		BoardContext:  k.Board,
		SearchContext: k.Search,
		DetailContext: k.Detail,
		ModalContext:  k.Modal,
	}

	resolved := ResolvedKeyBindings{
		contexts: make(map[string]ContextBindings, len(contexts)),
	}

	for _, context := range []string{ShellContext, BoardContext, SearchContext, DetailContext, ModalContext} {
		bindings, err := buildContextBindings(context, contexts[context], allowedActionsForContext(context))
		if err != nil {
			return ResolvedKeyBindings{}, err
		}
		resolved.contexts[context] = bindings
	}

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

func (r ResolvedKeyBindings) IsZero() bool {
	return len(r.contexts) == 0
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
	case SearchContext:
		for _, action := range []string{
			SearchActionMoveUp,
			SearchActionMoveDown,
			SearchActionFocusLeft,
			SearchActionFocusRight,
			SearchActionFocusQuery,
			SearchActionReload,
			SearchActionOpenDetail,
			SearchActionCycleFocusNext,
			SearchActionCycleFocusPrev,
		} {
			allowed[action] = struct{}{}
		}
	case DetailContext:
		for _, action := range []string{
			DetailActionScrollUp,
			DetailActionScrollDown,
			DetailActionPageUp,
			DetailActionPageDown,
			DetailActionHome,
			DetailActionEnd,
		} {
			allowed[action] = struct{}{}
		}
	case ModalContext:
		for _, action := range []string{
			ModalActionNext,
			ModalActionPrev,
			ModalActionLeft,
			ModalActionRight,
			ModalActionEnter,
			ModalActionEscape,
		} {
			allowed[action] = struct{}{}
		}
	}
	return allowed
}
