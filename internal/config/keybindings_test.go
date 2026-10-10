package config

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDefaultKeyBindingsResolveAndMatch(t *testing.T) {
	t.Parallel()

	resolved, err := ResolveKeyBindings(DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	if !resolved.Match(ShellContext, ShellActionQuit, tea.KeyMsg{Type: tea.KeyCtrlC}) {
		t.Fatal("expected shell quit to match ctrl+c")
	}
	if !resolved.Match(ShellContext, ShellActionHelp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true}) {
		t.Fatal("expected shell help to match alt+h")
	}
	if resolved.Match(ShellContext, ShellActionHelp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}) {
		t.Fatal("expected shell help not to match a bare h")
	}
	if !resolved.Match(ShellContext, ShellActionOpenSearch, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}) {
		t.Fatal("expected shell open-search to match alt+f")
	}
	if !resolved.Match(ShellContext, ShellActionOpenConfig, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}) {
		t.Fatal("expected shell open-config to match alt+c")
	}
	if !resolved.Match(ShellContext, ShellActionCloseIssue, tea.KeyMsg{Type: tea.KeyDelete}) {
		t.Fatal("expected shell close-issue to match delete")
	}
	if !resolved.Match(BoardContext, BoardActionMoveLeft, tea.KeyMsg{Type: tea.KeyLeft}) {
		t.Fatal("expected board move-left to match left arrow")
	}
	if resolved.Primary(ShellContext, ShellActionModeCycleNext) != "tab" {
		t.Fatalf("expected mode cycle next key to be tab, got %q", resolved.Primary(ShellContext, ShellActionModeCycleNext))
	}
	if resolved.Primary(ShellContext, ShellActionModeCyclePrev) != "shift+tab" {
		t.Fatalf("expected mode cycle prev key to be shift+tab, got %q", resolved.Primary(ShellContext, ShellActionModeCyclePrev))
	}
	// tab/shift+tab belong to the shell tab strip: no other context outside a
	// modal may bind them, or a browse surface would claim the key first.
	for _, context := range []string{BoardContext, DetailContext} {
		for action, keys := range bindingsOf(DefaultKeyBindings(), context) {
			for _, key := range keys {
				if key == "tab" || key == "shift+tab" {
					t.Fatalf("%s action %q binds %q, which belongs to the tab strip", context, action, key)
				}
			}
		}
	}
}

func bindingsOf(k KeyBindings, context string) map[string][]string {
	switch context {
	case ShellContext:
		return k.Shell
	case BoardContext:
		return k.Board
	case DetailContext:
		return k.Detail
	}
	return k.Modal
}

// TestDefaultKeyBindingsOutsideModalAreNotPrintable pins the rule the filter
// depends on: a printable key types into the query, so no default binding
// outside a modal may be one.
func TestDefaultKeyBindingsOutsideModalAreNotPrintable(t *testing.T) {
	t.Parallel()

	for _, context := range []string{ShellContext, BoardContext, DetailContext} {
		for action, keys := range bindingsOf(DefaultKeyBindings(), context) {
			for _, key := range keys {
				if isPrintableKey(canonicalKeyName(key)) {
					t.Errorf("%s action %q is bound to the printable key %q", context, action, key)
				}
			}
		}
	}
}

func TestResolveKeyBindingsRejectsPrintableKeysOutsideModal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		bind    func(k *KeyBindings)
		wantErr string
	}{
		{
			name:    "shell letter",
			bind:    func(k *KeyBindings) { k.Shell[ShellActionHelp] = []string{"?"} },
			wantErr: `key "?" for action "toggle_help" in shell context is a printable key, which types into the filter; bind it with alt+ or ctrl+`,
		},
		{
			name:    "board letter beside a valid key",
			bind:    func(k *KeyBindings) { k.Board[BoardActionMoveDown] = []string{"down", "j"} },
			wantErr: `key "j" for action "move_down" in board context is a printable key, which types into the filter; bind it with alt+ or ctrl+`,
		},
		{
			name:    "detail space",
			bind:    func(k *KeyBindings) { k.Detail[DetailActionPageDown] = []string{" "} },
			wantErr: `key "space" for action "page_down" in detail context is a printable key, which types into the filter; bind it with alt+ or ctrl+`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bindings := DefaultKeyBindings()
			tc.bind(&bindings)
			_, err := ResolveKeyBindings(bindings)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}

	// A modal takes no filter text, so its keys stay free.
	modal := DefaultKeyBindings()
	modal.Modal[ModalActionEscape] = []string{"q"}
	modal.Modal[ModalActionEnter] = []string{"space"}
	if _, err := ResolveKeyBindings(modal); err != nil {
		t.Fatalf("printable modal keys rejected: %v", err)
	}
}

// TestDefaultKeyBindingsOutsideModalAreNotReserved: a browse surface takes a
// reserved key before any binding, so no default binding outside a modal may
// be one.
func TestDefaultKeyBindingsOutsideModalAreNotReserved(t *testing.T) {
	t.Parallel()

	for _, context := range []string{ShellContext, BoardContext, DetailContext} {
		for action, keys := range bindingsOf(DefaultKeyBindings(), context) {
			for _, key := range keys {
				if _, reserved := reservedKeys[canonicalKeyName(key)]; reserved {
					t.Errorf("%s action %q is bound to the reserved key %q", context, action, key)
				}
			}
		}
	}
}

func TestResolveKeyBindingsRejectsReservedKeysInShellAndBoard(t *testing.T) {
	t.Parallel()

	contexts := []struct {
		context string
		action  string
		bind    func(k *KeyBindings, keys []string)
	}{
		{ShellContext, ShellActionHelp, func(k *KeyBindings, keys []string) { k.Shell[ShellActionHelp] = keys }},
		{BoardContext, BoardActionReload, func(k *KeyBindings, keys []string) { k.Board[BoardActionReload] = keys }},
	}
	uses := map[string]string{
		"backspace": "edits the filter",
		"ctrl+w":    "edits the filter",
		"ctrl+u":    "edits the filter",
		"ctrl+t":    "toggles the scope of the store search",
	}
	if got := ReservedKeys(); len(got) != len(uses) {
		t.Fatalf("ReservedKeys() = %v, want the %d keys of this test", got, len(uses))
	}

	for _, tc := range contexts {
		for _, key := range ReservedKeys() {
			t.Run(tc.context+" "+key, func(t *testing.T) {
				t.Parallel()

				bindings := DefaultKeyBindings()
				// The raw name is upper case: the check reads the canonical one.
				tc.bind(&bindings, []string{"alt+z", strings.ToUpper(key)})
				_, err := ResolveKeyBindings(bindings)
				want := `key "` + key + `" for action "` + tc.action + `" in ` + tc.context + ` context ` + uses[key] + `, which takes it before any binding; bind another key`
				if err == nil || err.Error() != want {
					t.Fatalf("error = %v, want %q", err, want)
				}
			})
		}
	}

	// The detail has no query and a modal is above every browse surface, so
	// their keys stay free.
	modal := DefaultKeyBindings()
	modal.Detail[DetailActionPageUp] = []string{"ctrl+u"}
	modal.Modal[ModalActionPrev] = []string{"backspace", "ctrl+w"}
	modal.Modal[ModalActionLeft] = []string{"ctrl+u"}
	modal.Modal[ModalActionRight] = []string{"ctrl+t"}
	if _, err := ResolveKeyBindings(modal); err != nil {
		t.Fatalf("reserved modal keys rejected: %v", err)
	}
}

func TestMergeKeyBindingsOverridesPerAction(t *testing.T) {
	t.Parallel()

	merged := MergeKeyBindings(DefaultKeyBindings(), &KeyBindingOverride{
		Shell: map[string][]string{ShellActionQuit: {"ctrl+q"}},
		Board: map[string][]string{BoardActionMoveLeft: {"alt+a"}},
	})

	if got := strings.Join(merged.Shell[ShellActionQuit], ","); got != "ctrl+q" {
		t.Fatalf("expected shell quit override, got %q", got)
	}
	if got := strings.Join(merged.Board[BoardActionMoveLeft], ","); got != "alt+a" {
		t.Fatalf("expected board move-left override, got %q", got)
	}
	if got := strings.Join(merged.Board[BoardActionMoveRight], ","); got != "right" {
		t.Fatalf("expected unspecified bindings to remain, got %q", got)
	}
}

func TestResolveKeyBindingsRejectsConflictsAndUnknownActions(t *testing.T) {
	t.Parallel()

	conflicting := DefaultKeyBindings()
	conflicting.Board[BoardActionMoveLeft] = []string{"alt+h"}
	conflicting.Board[BoardActionMoveRight] = []string{"alt+h"}
	if _, err := ResolveKeyBindings(conflicting); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}

	unknown := DefaultKeyBindings()
	unknown.Shell["mystery"] = []string{"alt+z"}
	if _, err := ResolveKeyBindings(unknown); err == nil || !strings.Contains(err.Error(), "unknown keybinding action") {
		t.Fatalf("expected unknown action error, got %v", err)
	}
}

// TestResolveKeyBindingsRejectsAContextMissingARequiredAction pins the
// fail-fast rule docs/CONFIGURATION.md states: a context that does not bind
// every action allowed in it fails startup. Without this, an action added to
// allowedActionsForContext but not to DefaultKeyBindings ships as a key the
// operator presses to no effect, with no startup error and no failing test.
func TestResolveKeyBindingsRejectsAContextMissingARequiredAction(t *testing.T) {
	t.Parallel()

	// One case per context, so a context left out of the completeness check is
	// itself a failure rather than a silent gap.
	cases := []struct {
		context string
		remove  func(k *KeyBindings) string
	}{
		{ShellContext, func(k *KeyBindings) string { delete(k.Shell, ShellActionQuit); return ShellActionQuit }},
		{BoardContext, func(k *KeyBindings) string { delete(k.Board, BoardActionMoveLeft); return BoardActionMoveLeft }},
		{DetailContext, func(k *KeyBindings) string { delete(k.Detail, DetailActionScrollUp); return DetailActionScrollUp }},
		{ModalContext, func(k *KeyBindings) string { delete(k.Modal, ModalActionEnter); return ModalActionEnter }},
	}

	for _, tc := range cases {
		t.Run(tc.context, func(t *testing.T) {
			t.Parallel()

			incomplete := DefaultKeyBindings()
			action := tc.remove(&incomplete)

			_, err := ResolveKeyBindings(incomplete)
			if err == nil {
				t.Fatalf("resolving %s without %q succeeded, want a startup failure", tc.context, action)
			}
			want := "missing keybinding action \"" + action + "\" in " + tc.context + " context"
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestMergeKeyBindingsDoesNotMutateBase(t *testing.T) {
	t.Parallel()

	// Capture the original default before any merge.
	original := DefaultKeyBindings()
	originalQuitKeys := append([]string(nil), original.Shell[ShellActionQuit]...)

	// Apply an override that changes shell quit.
	_ = MergeKeyBindings(DefaultKeyBindings(), &KeyBindingOverride{
		Shell: map[string][]string{ShellActionQuit: {"ctrl+q"}},
	})

	// The original base must be unchanged.
	after := DefaultKeyBindings()
	afterQuitKeys := after.Shell[ShellActionQuit]
	if len(afterQuitKeys) != len(originalQuitKeys) {
		t.Fatalf("DefaultKeyBindings shell quit mutated: before=%v after=%v", originalQuitKeys, afterQuitKeys)
	}
	for i := range originalQuitKeys {
		if originalQuitKeys[i] != afterQuitKeys[i] {
			t.Fatalf("DefaultKeyBindings shell quit mutated at index %d: before=%v after=%v", i, originalQuitKeys, afterQuitKeys)
		}
	}
}

func TestResolveKeyBindingsRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	invalid := DefaultKeyBindings()
	invalid.Shell[ShellActionHelp] = []string{"bad key"}
	if _, err := ResolveKeyBindings(invalid); err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func resolvedDefault(t *testing.T) ResolvedKeyBindings {
	t.Helper()
	resolved, err := ResolveKeyBindings(DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return resolved
}

func TestDisplayKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"ctrl+@", "ctrl+space"},
		{"ctrl+space", "ctrl+space"},
		{"q", "q"},
		{"ctrl+q", "ctrl+q"},
		{"left", "left"},
		{"f13", "f13"},
		{"", ""},
		{"Space", "space"},
		{" ", "space"},
	}

	for _, tc := range tests {
		got := DisplayKeyName(tc.input)
		if got != tc.want {
			t.Errorf("DisplayKeyName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolvedKeyBindingsDisplayPrimary(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	// ctrl+@ is the canonical name of ctrl+space; DisplayPrimary must return "ctrl+space".
	bindings := DefaultKeyBindings()
	bindings.Shell[ShellActionHelp] = []string{"ctrl+@"}
	rebound, err := ResolveKeyBindings(bindings)
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	if got := rebound.DisplayPrimary(ShellContext, ShellActionHelp); got != "ctrl+space" {
		t.Errorf("DisplayPrimary(shell, toggle_help) = %q, want %q", got, "ctrl+space")
	}

	if got := r.DisplayPrimary(ShellContext, ShellActionQuit); got != "ctrl+c" {
		t.Errorf("DisplayPrimary(shell, quit) = %q, want %q", got, "ctrl+c")
	}
}

func TestResolvedKeyBindingsDisplayPrimaryMissingContextOrAction(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	if got := r.DisplayPrimary("nosuchcontext", ShellActionQuit); got != "" {
		t.Errorf("DisplayPrimary(missing context) = %q, want empty", got)
	}
	if got := r.DisplayPrimary(ShellContext, "nosuchaction"); got != "" {
		t.Errorf("DisplayPrimary(missing action) = %q, want empty", got)
	}
}

func TestResolvedKeyBindingsDisplayLabel(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	// shell mode_cycle_next has keys "tab", "ctrl+pgdown" → label is "tab/ctrl+pgdown".
	got := r.DisplayLabel(ShellContext, ShellActionModeCycleNext)
	if got != "tab/ctrl+pgdown" {
		t.Errorf("DisplayLabel(shell, mode_cycle_next) = %q, want %q", got, "tab/ctrl+pgdown")
	}

	// Missing context / action must return "".
	if got := r.DisplayLabel("nosuch", BoardActionMoveRight); got != "" {
		t.Errorf("DisplayLabel(missing context) = %q, want empty", got)
	}
	if got := r.DisplayLabel(BoardContext, "nosuchaction"); got != "" {
		t.Errorf("DisplayLabel(missing action) = %q, want empty", got)
	}
}

func TestResolvedKeyBindingsIsZero(t *testing.T) {
	t.Parallel()

	// Zero value must report IsZero == true.
	var zero ResolvedKeyBindings
	if !zero.IsZero() {
		t.Fatal("expected zero-value ResolvedKeyBindings to report IsZero() == true")
	}

	// Resolved from defaults must report IsZero == false.
	r := resolvedDefault(t)
	if r.IsZero() {
		t.Fatal("expected populated ResolvedKeyBindings to report IsZero() == false")
	}
}
